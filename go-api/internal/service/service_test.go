package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"pmc/internal/cache"
	"pmc/internal/geo"
	"pmc/internal/kv"
	"pmc/internal/messaging"
	"pmc/internal/model"
	"pmc/internal/store"
)

type localGeo struct{}

func (localGeo) Nearby(_ context.Context, lat, lng float64, r *float64, p []geo.Point, limit int) []geo.Ranked {
	return geo.RankLocal(lat, lng, r, p, limit)
}
func (localGeo) Ward(context.Context, float64, float64) geo.WardResult { return geo.WardResult{} }
func (localGeo) CheckLinks(_ context.Context, l []geo.LinkIn) []geo.LinkOut {
	out := make([]geo.LinkOut, len(l))
	for i, x := range l {
		out[i] = geo.LinkOut{ID: x.ID, Healthy: x.URL != "http://bad"}
	}
	return out
}
func (localGeo) Boundaries(context.Context) ([]byte, bool) { return nil, false }

type fakeAnalyzer struct {
	got []string
	res *Analysis
}

func (f *fakeAnalyzer) Analyze(_ context.Context, d, c string) *Analysis {
	f.got = append(f.got, d, c)
	return f.res
}

type capturePub struct {
	mu   sync.Mutex
	evs  []model.ReportSubmittedEvent
	fail bool
}

func (c *capturePub) Publish(_ context.Context, ev model.ReportSubmittedEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail {
		return context.DeadlineExceeded
	}
	c.evs = append(c.evs, ev)
	return nil
}

func newSvc(t *testing.T) (*Locality, *store.Memory, *fakeAnalyzer, *capturePub) {
	st := store.NewMemory()
	if _, err := store.SeedIfMissing(context.Background(), st, store.SeedLocalities); err != nil {
		t.Fatal(err)
	}
	an, pub := &fakeAnalyzer{}, &capturePub{}
	return &Locality{Store: st, Cache: cache.New(kv.NewMemory(), "l", time.Minute), Geo: localGeo{}, Analyzer: an,
		Events: messaging.Safe{Enabled: true, Inner: pub}, Audit: Auditor{Store: st}}, st, an, pub
}

func f64(v float64) *float64 { return &v }
func str(s string) *string   { return &s }

func TestSeedMatchesSpringAndIsIdempotent(t *testing.T) {
	st := store.NewMemory()
	n, _ := store.SeedIfMissing(context.Background(), st, store.SeedLocalities)
	again, _ := store.SeedIfMissing(context.Background(), st, store.SeedLocalities)
	if n != 39 || again != 0 {
		t.Fatalf("seeded %d then %d, want 39 then 0", n, again)
	}
}

func TestFindVisibleFiltersAndOnlyVerified(t *testing.T) {
	s, _, _, _ := newSvc(t)
	ctx := context.Background()
	all, _ := s.FindVisible(ctx, nil, "")
	if len(all) != 39 {
		t.Fatalf("got %d", len(all))
	}
	mixed := model.Mixed
	m, _ := s.FindVisible(ctx, &mixed, "")
	for _, l := range m {
		if l.Status != model.Mixed {
			t.Fatal("status filter leaked")
		}
	}
	if k, _ := s.FindVisible(ctx, nil, "KOTHR"); len(k) != 1 || k[0].Name != "Kothrud" {
		t.Fatalf("case-insensitive search: %v", k)
	}
	// unverified report is invisible
	_, _ = s.CreateUnverifiedReport(ctx, model.Locality{Name: "Hidden", Status: model.Mixed, Latitude: f64(18.5), Longitude: f64(73.8)})
	if after, _ := s.FindVisible(ctx, nil, ""); len(after) != 39 {
		t.Fatal("unverified report leaked into public list")
	}
}

func TestWritesEvictCacheForReaders(t *testing.T) {
	s, _, _, _ := newSvc(t)
	ctx := context.Background()
	_, _ = s.FindVisible(ctx, nil, "") // warm
	created, err := s.CreateVerified(ctx, model.Locality{Name: "NewVille", Status: model.Municipal, Latitude: f64(18.5), Longitude: f64(73.8)})
	if err != nil || created.LastVerified == nil || !created.Verified {
		t.Fatalf("%v %+v", err, created)
	}
	if all, _ := s.FindVisible(ctx, nil, ""); len(all) != 40 {
		t.Fatalf("stale cache after create: %d", len(all))
	}
	if _, err := s.Update(ctx, *created.ID, model.Locality{Name: "Renamed", Status: model.Mixed, Latitude: f64(18.5), Longitude: f64(73.8)}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.FindByID(ctx, *created.ID); got.Name != "Renamed" {
		t.Fatalf("stale by-id cache: %s", got.Name)
	}
	_ = s.Delete(ctx, *created.ID)
	if _, err := s.FindByID(ctx, *created.ID); err == nil {
		t.Fatal("deleted locality still served")
	} else if _, ok := err.(NotFoundError); !ok {
		t.Fatalf("want NotFoundError, got %T", err)
	}
}

func TestNearbyAndNearestReliable(t *testing.T) {
	s, _, _, _ := newSvc(t)
	ctx := context.Background()
	near, _ := s.FindNearby(ctx, 18.5204, 73.8567, f64(3))
	if len(near) == 0 {
		t.Fatal("expected localities within 3km of Pune centre")
	}
	for i := 1; i < len(near); i++ {
		if near[i].DistanceKm < near[i-1].DistanceKm || near[i].DistanceKm > 3 {
			t.Fatalf("not sorted/limited: %v", near)
		}
	}
	all, _ := s.FindVisible(ctx, nil, "Wagholi")
	r, err := s.FindNearestReliable(ctx, *all[0].ID)
	if err != nil || r == nil || r.Locality.Status != model.Municipal || *r.Locality.ID == *all[0].ID {
		t.Fatalf("nearest reliable wrong: %+v %v", r, err)
	}
}

func TestReportTriageDuplicateAndEvent(t *testing.T) {
	s, _, an, pub := newSvc(t)
	ctx := context.Background()
	first, _ := s.CreateUnverifiedReport(ctx, model.Locality{Name: "A", Status: model.TankerDependent, Latitude: f64(18.55), Longitude: f64(73.85), CitizenDescription: str("no water since monday")})
	if first.ReportedUrgency != nil {
		t.Fatal("analyzer returned nil but urgency was set")
	}
	an.res = &Analysis{Urgency: model.UrgencyHigh, UrgencyConfidence: 80, UrgencyReasoning: "3 days", IsDuplicate: true, DuplicateConfidence: f64(0.9)}
	second, _ := s.CreateUnverifiedReport(ctx, model.Locality{Name: "B", Status: model.TankerDependent, Latitude: f64(18.551), Longitude: f64(73.851), CitizenDescription: str("still no water")})
	if second.ReportedUrgency == nil || *second.ReportedUrgency != model.UrgencyHigh || *second.UrgencyScore != 80 {
		t.Fatalf("urgency not applied: %+v", second)
	}
	if second.PossibleDuplicateOfID == nil || *second.PossibleDuplicateOfID != *first.ID {
		t.Fatalf("duplicate not linked to nearby earlier report: %+v", second.PossibleDuplicateOfID)
	}
	if an.got[3] != "no water since monday" {
		t.Fatalf("candidate description not passed to model: %q", an.got[3])
	}
	// far away report gets no candidate
	far, _ := s.CreateUnverifiedReport(ctx, model.Locality{Name: "C", Status: model.Mixed, Latitude: f64(18.40), Longitude: f64(73.70), CitizenDescription: str("leak")})
	if far.PossibleDuplicateOfID != nil {
		t.Fatal("far report flagged as duplicate")
	}
	// explicit urgency is left alone
	u := model.UrgencyLow
	keep, _ := s.CreateUnverifiedReport(ctx, model.Locality{Name: "D", Status: model.Mixed, Latitude: f64(18.3), Longitude: f64(73.6), CitizenDescription: str("x"), ReportedUrgency: &u})
	if *keep.ReportedUrgency != model.UrgencyLow {
		t.Fatal("explicit urgency overwritten")
	}
	if len(pub.evs) != 4 || pub.evs[0].LocalityName != "A" {
		t.Fatalf("events: %+v", pub.evs)
	}
	pending, _ := s.PendingReports(ctx)
	if len(pending) != 4 || *pending[0].UrgencyScore != 80 {
		t.Fatal("pending not sorted by urgency score desc")
	}
}

func TestBrokerFailureNeverFailsTheReport(t *testing.T) {
	s, _, _, pub := newSvc(t)
	pub.fail = true
	if _, err := s.CreateUnverifiedReport(context.Background(), model.Locality{Name: "Z", Status: model.Mixed, Latitude: f64(1), Longitude: f64(1)}); err != nil {
		t.Fatalf("broker outage failed the citizen's request: %v", err)
	}
}

func TestApproveAndAudit(t *testing.T) {
	s, st, _, _ := newSvc(t)
	ctx := context.Background()
	rep, _ := s.CreateUnverifiedReport(ctx, model.Locality{Name: "P", Status: model.Mixed, Latitude: f64(1), Longitude: f64(1)})
	ok, err := s.ApproveReport(ctx, *rep.ID)
	if err != nil || !ok.Verified {
		t.Fatal(err)
	}
	_, err = s.ApproveReport(ctx, 9999)
	audits, _ := st.ListAudit(ctx)
	if err == nil || len(audits) != 2 || audits[0].Success && audits[1].Success {
		t.Fatalf("audit should record 1 success + 1 failure: %+v", audits)
	}
}

func TestOllamaToggleParsingAndFailureModes(t *testing.T) {
	var prompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		prompt, _ = req["prompt"].(string)
		resp := "```json\n{\"urgency\":\"critical\",\"urgencyConfidence\":95,\"urgencyReasoning\":\"sewage\",\"isDuplicate\":true,\"duplicateConfidence\":0.8}\n```"
		_ = json.NewEncoder(w).Encode(map[string]string{"response": resp})
	}))
	defer srv.Close()
	k := kv.NewMemory()
	o := &Ollama{KV: k, StartupDefault: false, Host: srv.URL, Model: "m"}
	ctx := context.Background()

	if a := o.Analyze(ctx, "sewage in tap water", ""); a != nil {
		t.Fatal("analysed while disabled")
	}
	_, _ = o.SetEnabled(ctx, true)
	a := o.Analyze(ctx, "sewage in tap water", "")
	if a == nil || a.Urgency != model.UrgencyCritical || a.UrgencyConfidence != 95 || a.IsDuplicate {
		t.Fatalf("no candidate => duplicate fields ignored: %+v", a)
	}
	a = o.Analyze(ctx, "sewage in tap water", "other report")
	if !a.IsDuplicate || *a.DuplicateConfidence != 0.8 {
		t.Fatalf("%+v", a)
	}
	if want := "SAME underlying incident"; !contains(prompt, want) {
		t.Fatal("candidate prompt not built")
	}
	srv.Close()
	if o.Analyze(ctx, "x", "") != nil {
		t.Fatal("unreachable Ollama must yield nil, not an error")
	}
	if _, err := parseAnalysis(`{"urgency":"BANANA"}`, false); err == nil {
		t.Fatal("invalid urgency accepted")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool { return indexOf(s, sub) >= 0 })()
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestToggleIsSharedAcrossReplicasAndFallsBackToDefault(t *testing.T) {
	shared := kv.NewMemory()
	podA := &Ollama{KV: shared, StartupDefault: false}
	podB := &Ollama{KV: shared, StartupDefault: false}
	_, _ = podA.SetEnabled(context.Background(), true)
	if !podB.Enabled(context.Background()) {
		t.Fatal("pod B did not see pod A's toggle")
	}
	down := &Ollama{KV: kv.NewRedis("127.0.0.1", "1", "", false), StartupDefault: true}
	if !down.Enabled(context.Background()) {
		t.Fatal("KV outage should fall back to the startup default")
	}
}

func TestLinkCheckerRecordsResults(t *testing.T) {
	s, st, _, _ := newSvc(t)
	ctx := context.Background()
	bad, _ := s.CreateVerified(ctx, model.Locality{Name: "Bad", Status: model.Mixed, Latitude: f64(1), Longitude: f64(1), SourceURL: str("http://bad")})
	lc := &LinkChecker{Store: st, Geo: localGeo{}, Audit: s.Audit}
	lc.CheckAll(ctx)
	g, _ := st.GetLocality(ctx, *bad.ID)
	if g.SourceURLHealthy == nil || *g.SourceURLHealthy || g.SourceURLLastChecked == nil {
		t.Fatalf("bad link not recorded: %+v", g)
	}
	k, _ := st.ListLocalities(ctx)
	checked := 0
	for _, l := range k {
		if l.SourceURLHealthy != nil {
			checked++
		}
	}
	if checked < 10 {
		t.Fatalf("only %d links checked", checked)
	}
}

func TestNextMonday3AM(t *testing.T) {
	loc := time.UTC
	got := NextMonday3AM(time.Date(2026, 10, 24, 10, 0, 0, 0, loc)) // Saturday
	if want := time.Date(2026, 10, 26, 3, 0, 0, 0, loc); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	got = NextMonday3AM(time.Date(2026, 10, 26, 3, 0, 0, 0, loc)) // exactly Monday 03:00 -> next week
	if want := time.Date(2026, 11, 2, 3, 0, 0, 0, loc); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
