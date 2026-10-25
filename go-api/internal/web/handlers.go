package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	"pmc/internal/auth"
	"pmc/internal/model"
	"pmc/internal/store"
)

type handlers struct{ Deps }

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = (b[6]&0x0f)|0x40, (b[8]&0x3f)|0x80 // UUIDv4
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, validationError{"id: must be a number"}
	}
	return id, nil
}

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "UP"})
}

// ---- auth -------------------------------------------------------------------------------

func (h *handlers) login(w http.ResponseWriter, r *http.Request) {
	var req struct{ Username, Password string }
	if err := decode(r, &req); err != nil {
		fail(w, r, err)
		return
	}
	u, err := h.Store.FindUserByUsername(r.Context(), req.Username)
	// Same message for unknown user and wrong password - never reveal which was wrong.
	if err != nil || !h.Hasher.Verify(req.Password, u.PasswordHash) {
		logWarn(r, "failed login attempt", "username", req.Username)
		writeErr(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}
	token, err := h.JWT.Generate(u.Username, string(u.Role))
	if err != nil {
		fail(w, r, err)
		return
	}
	logInfo(r, "successful login", "username", u.Username, "role", u.Role)
	writeJSON(w, 200, map[string]string{"token": token, "username": u.Username, "role": string(u.Role)})
}

// ---- localities -------------------------------------------------------------------------

func (h *handlers) listLocalities(w http.ResponseWriter, r *http.Request) {
	var status *model.WaterStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st, ok := model.ParseStatus(s)
		if !ok {
			fail(w, r, validationError{"status: must be one of MUNICIPAL, TANKER_DEPENDENT, MIXED, PIPELINE_IN_PROGRESS"})
			return
		}
		status = &st
	}
	out, err := h.Localities.FindVisible(r.Context(), status, r.URL.Query().Get("search"))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, 200, out)
}

func (h *handlers) getLocality(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, r, err)
		return
	}
	l, err := h.Localities.FindByID(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, 200, l)
}

func (h *handlers) nearby(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat, e1 := strconv.ParseFloat(q.Get("lat"), 64)
	lng, e2 := strconv.ParseFloat(q.Get("lng"), 64)
	if e1 != nil || e2 != nil {
		fail(w, r, validationError{"lat and lng are required numbers"})
		return
	}
	var radius *float64
	if s := q.Get("radiusKm"); s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			fail(w, r, validationError{"radiusKm: must be a number"})
			return
		}
		radius = &v
	}
	out, err := h.Localities.FindNearby(r.Context(), lat, lng, radius)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, 200, out)
}

func (h *handlers) nearestReliable(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, r, err)
		return
	}
	res, err := h.Localities.FindNearestReliable(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	if res == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, 200, res)
}

func (h *handlers) bindLocality(w http.ResponseWriter, r *http.Request) (model.Locality, bool) {
	var l model.Locality
	if err := decode(r, &l); err != nil {
		fail(w, r, err)
		return l, false
	}
	if err := l.Validate(); err != nil {
		fail(w, r, validationError{err.Error()})
		return l, false
	}
	return l, true
}

func (h *handlers) createLocality(w http.ResponseWriter, r *http.Request) {
	l, ok := h.bindLocality(w, r)
	if !ok {
		return
	}
	out, err := h.Localities.CreateVerified(r.Context(), l)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.Hub.Announce(r.Context(), h.KV, *out.ID, "created", string(out.Status))
	writeJSON(w, http.StatusCreated, out)
}

func (h *handlers) submitReport(w http.ResponseWriter, r *http.Request) {
	l, ok := h.bindLocality(w, r)
	if !ok {
		return
	}
	out, err := h.Localities.CreateUnverifiedReport(r.Context(), l)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *handlers) pendingReports(w http.ResponseWriter, r *http.Request) {
	out, err := h.Localities.PendingReports(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, 200, out)
}

func (h *handlers) approve(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, r, err)
		return
	}
	out, err := h.Localities.ApproveReport(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.Hub.Announce(r.Context(), h.KV, id, "approved", string(out.Status))
	writeJSON(w, 200, out)
}

func (h *handlers) updateLocality(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, r, err)
		return
	}
	l, ok := h.bindLocality(w, r)
	if !ok {
		return
	}
	out, err := h.Localities.Update(r.Context(), id, l)
	if err != nil {
		fail(w, r, err)
		return
	}
	h.Hub.Announce(r.Context(), h.KV, id, "updated", string(out.Status))
	writeJSON(w, 200, out)
}

func (h *handlers) deleteLocality(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err := h.Localities.Delete(r.Context(), id); err != nil {
		fail(w, r, err)
		return
	}
	h.Hub.Announce(r.Context(), h.KV, id, "deleted", "")
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) checkSourceLinks(w http.ResponseWriter, r *http.Request) {
	h.Links.TriggerManual(r.Context())
	w.WriteHeader(http.StatusAccepted)
}

// ---- wards ------------------------------------------------------------------------------

func (h *handlers) wardLookup(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat, e1 := strconv.ParseFloat(q.Get("lat"), 64)
	lng, e2 := strconv.ParseFloat(q.Get("lng"), 64)
	if e1 != nil || e2 != nil {
		fail(w, r, validationError{"lat and lng are required numbers"})
		return
	}
	writeJSON(w, 200, h.Geo.Ward(r.Context(), lat, lng))
}

func (h *handlers) recomputeWard(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, r, err)
		return
	}
	out, err := h.Localities.RecomputeWard(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, 200, out)
}

func (h *handlers) boundaries(w http.ResponseWriter, r *http.Request) {
	b, ok := h.Geo.Boundaries(r.Context())
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/geo+json")
	_, _ = w.Write(b)
}

// ---- users / audit / features -----------------------------------------------------------

func (h *handlers) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.Store.ListUsers(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	out := make([]model.UserView, 0, len(users))
	for _, u := range users {
		out = append(out, u.View())
	}
	writeJSON(w, 200, out)
}

func (h *handlers) createUser(w http.ResponseWriter, r *http.Request) {
	var req model.CreateUserRequest
	if err := decode(r, &req); err != nil {
		fail(w, r, err)
		return
	}
	if err := req.Validate(); err != nil {
		fail(w, r, validationError{err.Error()})
		return
	}
	hash, err := h.Hasher.Hash(req.Password)
	if err != nil {
		fail(w, r, err)
		return
	}
	saved, err := h.Store.CreateUser(r.Context(), model.User{Username: req.Username, PasswordHash: hash, Role: req.Role})
	if err == store.ErrDuplicate {
		h.Audit.Record(r.Context(), "CREATE_USER", req.Username, err)
		writeErr(w, http.StatusConflict, "Username already taken")
		return
	}
	if err != nil {
		h.Audit.Record(r.Context(), "CREATE_USER", req.Username, err)
		fail(w, r, err)
		return
	}
	h.Audit.Record(r.Context(), "CREATE_USER", "UserView[id="+strconv.FormatInt(saved.ID, 10)+", username="+saved.Username+", role="+string(saved.Role)+"]", nil)
	writeJSON(w, http.StatusCreated, saved.View())
}

func (h *handlers) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, r, err)
		return
	}
	err = h.Store.DeleteUser(r.Context(), id)
	h.Audit.Record(r.Context(), "DELETE_USER", strconv.FormatInt(id, 10), err)
	if err == store.ErrNotFound {
		writeErr(w, http.StatusNotFound, "No user with id "+strconv.FormatInt(id, 10))
		return
	}
	if err != nil {
		fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) auditLog(w http.ResponseWriter, r *http.Request) {
	out, err := h.Store.ListAudit(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	if out == nil {
		out = []model.AuditLog{}
	}
	writeJSON(w, 200, out)
}

func (h *handlers) ollamaStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]bool{"enabled": h.Ollama.Enabled(r.Context())})
}

func (h *handlers) ollamaSet(on bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := h.Ollama.SetEnabled(r.Context(), on)
		h.Audit.Record(r.Context(), "TOGGLE_OLLAMA_FEATURE", strconv.FormatBool(on), err)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"enabled": state})
	}
}

var _ = auth.Username
var _ = strings.TrimSpace
