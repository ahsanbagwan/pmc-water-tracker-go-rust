package web

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pmc/internal/auth"
	"pmc/internal/cache"
	"pmc/internal/geo"
	"pmc/internal/kv"
	"pmc/internal/lock"
	"pmc/internal/messaging"
	"pmc/internal/model"
	"pmc/internal/service"
	"pmc/internal/store"
)

type noGeo struct{}

func (noGeo) Nearby(_ context.Context, lat, lng float64, r *float64, p []geo.Point, l int) []geo.Ranked {
	return geo.RankLocal(lat, lng, r, p, l)
}
func (noGeo) Ward(context.Context, float64, float64) geo.WardResult  { return geo.WardResult{} }
func (noGeo) CheckLinks(context.Context, []geo.LinkIn) []geo.LinkOut { return nil }
func (noGeo) Boundaries(context.Context) ([]byte, bool)              { return nil, false }

type nopAnalyzer struct{}

func (nopAnalyzer) Analyze(context.Context, string, string) *service.Analysis { return nil }

type env struct {
	h     http.Handler
	st    *store.Memory
	kv    kv.KV
	hub   *Hub
	jwt   *auth.JWT
	admin string
	edit  string
}

// newEnv builds one "replica". Pass a shared kv/store/jwt to simulate several replicas.
func newEnv(t *testing.T, shared kv.KV, st *store.Memory, jwt *auth.JWT, chaos *Chaos) *env {
	t.Helper()
	ctx := context.Background()
	hasher := auth.PBKDF2{Iterations: 1000}
	if st == nil {
		st = store.NewMemory()
		_, _ = store.SeedIfMissing(ctx, st, store.SeedLocalities)
		h, _ := hasher.Hash("Admin12345")
		_, _ = st.CreateUser(ctx, model.User{Username: "admin", PasswordHash: h, Role: model.RoleAdmin})
		e, _ := hasher.Hash("Editor12345")
		_, _ = st.CreateUser(ctx, model.User{Username: "ed", PasswordHash: e, Role: model.RoleEditor})
	}
	if shared == nil {
		shared = kv.NewMemory()
	}
	if jwt == nil {
		jwt, _ = auth.NewJWT("", "", time.Hour, false)
	}
	audit := service.Auditor{Store: st}
	ollama := &service.Ollama{KV: shared}
	loc := &service.Locality{Store: st, Cache: cache.New(shared, "localities", time.Minute), Geo: noGeo{}, Analyzer: nopAnalyzer{},
		Events: messaging.Safe{}, Audit: audit}
	hub := NewHub(100)
	e := &env{st: st, kv: shared, hub: hub, jwt: jwt}
	e.h = New(Deps{Store: st, KV: shared, Localities: loc, Links: &service.LinkChecker{Store: st, Geo: noGeo{}, Locker: lock.New(shared, "t"), Audit: audit},
		Ollama: ollama, Audit: audit, JWT: jwt, Hasher: hasher, Geo: noGeo{}, Hub: hub, Chaos: chaos,
		MDCHeaders: []string{"X-Tenant-Id"}, CORSOrigins: []string{"http://localhost:*", "https://pmc-water-tracker.vercel.app"}, SiteURL: "https://site"})
	e.admin, _ = jwt.Generate("admin", "ADMIN")
	e.edit, _ = jwt.Generate("ed", "EDITOR")
	return e
}

func (e *env) do(method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "10.0.0.1:5555"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

const validLoc = `{"name":"T","status":"MIXED","latitude":18.5,"longitude":73.8}`

func TestSecurityMatrixMatchesSpringRules(t *testing.T) {
	e := newEnv(t, nil, nil, nil, nil)
	cases := []struct {
		name, method, path, token, body string
		want                            int
	}{
		{"public list", "GET", "/api/localities", "", "", 200},
		{"public get", "GET", "/api/localities/1", "", "", 200},
		{"public nearby", "GET", "/api/localities/nearby?lat=18.5&lng=73.8", "", "", 200},
		{"public rss", "GET", "/rss.xml", "", "", 200},
		{"public ward lookup", "GET", "/api/wards/lookup?lat=1&lng=1", "", "", 200},
		{"citizen report", "POST", "/api/localities/reports", "", validLoc, 201},
		{"create anon", "POST", "/api/localities", "", validLoc, 403},
		{"create editor", "POST", "/api/localities", e.edit, validLoc, 201},
		{"update anon", "PUT", "/api/localities/1", "", validLoc, 403},
		{"update editor", "PUT", "/api/localities/1", e.edit, validLoc, 200},
		{"delete editor forbidden", "DELETE", "/api/localities/1", e.edit, "", 403},
		{"delete admin", "DELETE", "/api/localities/2", e.admin, "", 204},
		{"pending anon", "GET", "/api/localities/reports/pending", "", "", 403},
		{"pending editor", "GET", "/api/localities/reports/pending", e.edit, "", 200},
		{"users editor forbidden", "GET", "/api/users", e.edit, "", 403},
		{"users admin", "GET", "/api/users", e.admin, "", 200},
		{"audit editor forbidden", "GET", "/api/audit-log", e.edit, "", 403},
		{"features editor forbidden", "GET", "/api/admin/features/ollama", e.edit, "", 403},
		{"features admin", "GET", "/api/admin/features/ollama", e.admin, "", 200},
		{"recompute anon", "PUT", "/api/wards/1/recompute", "", "", 403},
		{"recompute editor", "PUT", "/api/wards/1/recompute", e.edit, "", 200},
		{"garbage token", "GET", "/api/users", "not.a.jwt", "", 403},
		{"missing id", "GET", "/api/localities/99999", "", "", 404},
		{"bad id", "GET", "/api/localities/abc", "", "", 400},
		{"bad status filter", "GET", "/api/localities?status=NOPE", "", "", 400},
		{"check links editor", "POST", "/api/localities/check-source-links", e.edit, "", 202},
		{"check links anon", "POST", "/api/localities/check-source-links", "", "", 403},
	}
	for _, c := range cases {
		if got := e.do(c.method, c.path, c.token, c.body); got.Code != c.want {
			t.Errorf("%s: %s %s => %d, want %d (%s)", c.name, c.method, c.path, got.Code, c.want, got.Body.String())
		}
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	short, _ := auth.NewJWT("", "", -time.Minute, false)
	e := newEnv(t, nil, nil, short, nil)
	if r := e.do("GET", "/api/users", e.admin, ""); r.Code != 403 {
		t.Fatalf("expired token accepted: %d", r.Code)
	}
}

func TestJSONShapeMatchesJackson(t *testing.T) {
	e := newEnv(t, nil, nil, nil, nil)
	var list []map[string]any
	_ = json.Unmarshal(e.do("GET", "/api/localities?search=Kothrud", "", "").Body.Bytes(), &list)
	l := list[0]
	for _, k := range []string{"id", "name", "ward", "status", "latitude", "longitude", "notes", "sourceName", "sourceUrl", "lastVerified",
		"verified", "sourceUrlHealthy", "sourceUrlLastChecked", "citizenDescription", "reportedUrgency", "urgencyScore", "urgencySignals",
		"possibleDuplicateOfId", "duplicateConfidence"} {
		if _, ok := l[k]; !ok {
			t.Errorf("missing field %q (Jackson emits nulls)", k)
		}
	}
	if l["lastVerified"] != "2026-07-01" || l["verified"] != true || l["sourceUrlHealthy"] != nil {
		t.Errorf("value shapes wrong: %v", l)
	}
	var nearby []map[string]any
	_ = json.Unmarshal(e.do("GET", "/api/localities/nearby?lat=18.52&lng=73.85&radiusKm=2", "", "").Body.Bytes(), &nearby)
	if _, ok := nearby[0]["locality"].(map[string]any); !ok || nearby[0]["distanceKm"] == nil {
		t.Errorf("nearby shape: %v", nearby[0])
	}
	if r := e.do("GET", "/api/localities?status=TANKER_DEPENDENT", "", ""); !strings.HasPrefix(r.Body.String(), "[") {
		t.Error("list must be a JSON array")
	}
	if r := e.do("GET", "/api/localities?search=zzzzzz", "", ""); strings.TrimSpace(r.Body.String()) != "[]" {
		t.Errorf("empty result must be [] not null: %q", r.Body.String())
	}
}

func TestValidationErrorsAndNoContent(t *testing.T) {
	e := newEnv(t, nil, nil, nil, nil)
	r := e.do("POST", "/api/localities/reports", "", `{"name":"","status":"MIXED"}`)
	if r.Code != 400 || !strings.Contains(r.Body.String(), "name: must not be blank") || !strings.Contains(r.Body.String(), "latitude: must not be null") {
		t.Fatalf("%d %s", r.Code, r.Body.String())
	}
	if r := e.do("POST", "/api/localities/reports", "", `{not json`); r.Code != 400 {
		t.Fatalf("malformed json => %d", r.Code)
	}
	// Wagholi-like origin with no municipal... any id with no reliable neighbour -> 204 is covered by service; here ensure 200 shape
	if r := e.do("GET", "/api/localities/1/nearest-reliable", "", ""); r.Code != 200 && r.Code != 204 {
		t.Fatal(r.Code)
	}
	u := e.do("POST", "/api/users", e.admin, `{"username":"x","password":"short","role":"EDITOR"}`)
	if u.Code != 400 || !strings.Contains(u.Body.String(), "at least 8 characters") {
		t.Fatalf("weak password accepted: %d %s", u.Code, u.Body.String())
	}
}

func TestUserLifecycleLoginAndDuplicate(t *testing.T) {
	e := newEnv(t, nil, nil, nil, nil)
	r := e.do("POST", "/api/users", e.admin, `{"username":"newbie","password":"Passw0rd99","role":"EDITOR"}`)
	if r.Code != 201 || strings.Contains(r.Body.String(), "assword") {
		t.Fatalf("create user: %d %s (must not leak hash)", r.Code, r.Body.String())
	}
	if dup := e.do("POST", "/api/users", e.admin, `{"username":"newbie","password":"Passw0rd99","role":"EDITOR"}`); dup.Code != 409 {
		t.Fatalf("duplicate => %d", dup.Code)
	}
	login := e.do("POST", "/api/auth/login", "", `{"username":"newbie","password":"Passw0rd99"}`)
	var lr map[string]string
	_ = json.Unmarshal(login.Body.Bytes(), &lr)
	if login.Code != 200 || lr["role"] != "EDITOR" || lr["username"] != "newbie" || lr["token"] == "" {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	if r := e.do("POST", "/api/localities", lr["token"], validLoc); r.Code != 201 {
		t.Fatalf("token from login should work: %d", r.Code)
	}
	if r := e.do("DELETE", "/api/users/999", e.admin, ""); r.Code != 404 {
		t.Fatalf("delete missing user => %d", r.Code)
	}
	logs := e.do("GET", "/api/audit-log", e.admin, "").Body.String()
	if !strings.Contains(logs, "CREATE_USER") || !strings.Contains(logs, "Username already taken") && !strings.Contains(logs, "duplicate") {
		t.Fatalf("audit log missing entries: %s", logs)
	}
}

func TestLoginRateLimitIsSharedAcrossReplicas(t *testing.T) {
	shared := kv.NewMemory()
	st := store.NewMemory()
	a := newEnv(t, shared, st, nil, nil)
	b := newEnv(t, shared, st, a.jwt, nil)
	bad := `{"username":"x","password":"y"}`
	codes := []int{}
	for i := 0; i < 8; i++ {
		target := a
		if i%2 == 1 {
			target = b // attacker alternates replicas
		}
		codes = append(codes, target.do("POST", "/api/auth/login", "", bad).Code)
	}
	want := []int{401, 401, 401, 401, 401, 429, 429, 429}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("alternating replicas must share ONE budget: %v", codes)
		}
	}
}

func TestRateLimitFailsOpenWhenKVDown(t *testing.T) {
	e := newEnv(t, kv.NewRedis("127.0.0.1", "1", "", false), nil, nil, nil)
	for i := 0; i < 8; i++ {
		if c := e.do("POST", "/api/auth/login", "", `{"username":"x","password":"y"}`).Code; c != 401 {
			t.Fatalf("request %d: %d (must fail open to 401, not block)", i, c)
		}
	}
}

func TestCorrelationIdAndCORS(t *testing.T) {
	e := newEnv(t, nil, nil, nil, nil)
	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("X-Correlation-Id", "abc-1")
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Header().Get("X-Correlation-Id") != "abc-1" || rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("headers: %v", rec.Header())
	}
	if id := e.do("GET", "/healthz", "", "").Header().Get("X-Correlation-Id"); len(id) != 36 {
		t.Fatalf("generated id %q", id)
	}
	pre := httptest.NewRequest("OPTIONS", "/api/localities", nil)
	pre.Header.Set("Origin", "https://pmc-water-tracker.vercel.app")
	pre.Header.Set("Access-Control-Request-Method", "POST")
	rec = httptest.NewRecorder()
	e.h.ServeHTTP(rec, pre)
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "DELETE") {
		t.Fatalf("preflight: %d %v", rec.Code, rec.Header())
	}
	evil := httptest.NewRequest("GET", "/healthz", nil)
	evil.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	e.h.ServeHTTP(rec, evil)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("foreign origin allowed")
	}
}

func TestRSSIsValidEscapedAndLimited(t *testing.T) {
	e := newEnv(t, nil, nil, nil, nil)
	_ = e.do("POST", "/api/localities", e.admin, `{"name":"A & B <Colony>","status":"MIXED","latitude":1,"longitude":1,"notes":"x < y & \"z\""}`)
	r := e.do("GET", "/rss.xml", "", "")
	body := r.Body.String()
	if !strings.HasPrefix(r.Header().Get("Content-Type"), "application/rss+xml") || !strings.Contains(body, "A &amp; B &lt;Colony&gt;: MIXED") ||
		strings.Contains(body, "<Colony>") || strings.Count(body, "<item>") != 30 {
		t.Fatalf("rss wrong (items=%d): %.400s", strings.Count(body, "<item>"), body)
	}
}

func TestPanicBecomes500JSONNotACrash(t *testing.T) {
	c := NewChaos(true)
	c.cfg.Level, c.cfg.LatencyActive, c.cfg.ExceptionsActive = 1, false, true
	e := newEnv(t, nil, nil, nil, c)
	r := e.do("GET", "/api/localities", "", "")
	if r.Code != 500 || !strings.Contains(r.Body.String(), "An unexpected error occurred.") {
		t.Fatalf("%d %s", r.Code, r.Body.String())
	}
	if e.do("GET", "/healthz", "", "").Code != 200 {
		t.Fatal("health must be exempt from chaos")
	}
	if e.do("POST", "/actuator/chaosmonkey/disable", e.admin, "").Code != 200 || e.do("GET", "/api/localities", "", "").Code != 200 {
		t.Fatal("admin could not turn chaos off")
	}
	if e.do("POST", "/actuator/chaosmonkey/enable", "", "").Code != 403 {
		t.Fatal("chaos control must be admin only")
	}
}

func TestOllamaToggleEndpointsAreAuditedAndShared(t *testing.T) {
	shared := kv.NewMemory()
	st := store.NewMemory()
	a := newEnv(t, shared, st, nil, nil)
	b := newEnv(t, shared, st, a.jwt, nil)
	h, _ := auth.PBKDF2{Iterations: 1000}.Hash("x")
	_, _ = st.CreateUser(context.Background(), model.User{Username: "admin", PasswordHash: h, Role: model.RoleAdmin})
	if r := a.do("POST", "/api/admin/features/ollama/enable", a.admin, ""); r.Body.String() != "{\"enabled\":true}\n" {
		t.Fatalf("%q", r.Body.String())
	}
	if r := b.do("GET", "/api/admin/features/ollama", b.admin, ""); !strings.Contains(r.Body.String(), "true") {
		t.Fatal("replica B did not see replica A's toggle")
	}
	if !strings.Contains(b.do("GET", "/api/audit-log", b.admin, "").Body.String(), "TOGGLE_OLLAMA_FEATURE") {
		t.Fatal("toggle not audited")
	}
}

// The scalability claim: a write handled by replica A reaches a browser connected to replica B.
func TestSSEUpdateFansOutAcrossReplicas(t *testing.T) {
	shared := kv.NewMemory()
	st := store.NewMemory()
	a := newEnv(t, shared, st, nil, nil)
	b := newEnv(t, shared, st, a.jwt, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.hub.Run(ctx, shared)
	go b.hub.Run(ctx, shared)

	srvB := httptest.NewServer(b.h)
	defer srvB.Close()
	resp, err := http.Get(srvB.URL + "/api/stream/localities") // browser connected to replica B
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type %q", resp.Header.Get("Content-Type"))
	}
	lines := make(chan string, 32)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	waitFor(t, lines, "data: connected")
	time.Sleep(50 * time.Millisecond) // let the hub subscription settle

	r := a.do("POST", "/api/localities", a.admin, validLoc) // write handled by replica A
	if r.Code != 201 {
		t.Fatal(r.Code)
	}
	got := waitFor(t, lines, "event: locality-updated")
	_ = got
	data := waitFor(t, lines, "data: ")
	if !strings.Contains(data, `"action":"created"`) || !strings.Contains(data, `"status":"MIXED"`) {
		t.Fatalf("payload: %s", data)
	}
	if b.hub.Clients() != 1 || a.hub.Clients() != 0 {
		t.Fatalf("clients a=%d b=%d", a.hub.Clients(), b.hub.Clients())
	}
}

func TestSlowSSEClientIsDroppedNotBlocking(t *testing.T) {
	h := NewHub(10)
	c, _ := h.add()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			h.Broadcast("x") // nobody reads c
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Broadcast blocked on a slow client")
	}
	if h.Clients() != 0 {
		t.Fatal("slow client should have been dropped")
	}
	_ = c
	full := NewHub(1)
	full.add()
	if _, ok := full.add(); ok {
		t.Fatal("max clients not enforced")
	}
}

func waitFor(t *testing.T, lines <-chan string, prefix string) string {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatalf("stream closed waiting for %q", prefix)
			}
			if strings.HasPrefix(l, prefix) {
				return l
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %q", prefix)
		}
	}
}

var _ = io.EOF
