package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"pmc/internal/cache"
	"pmc/internal/geo"
	"pmc/internal/messaging"
	"pmc/internal/model"
	"pmc/internal/store"
)

type NotFoundError struct{ Msg string }

func (e NotFoundError) Error() string { return e.Msg }

type Analyzer interface {
	Analyze(ctx context.Context, description, candidateDescription string) *Analysis
}

type Locality struct {
	Store    store.Store
	Cache    *cache.Cache
	Geo      geo.Service
	Analyzer Analyzer
	Events   messaging.Safe
	Audit    Auditor
}

func (s *Locality) all(ctx context.Context) ([]model.Locality, error) {
	return s.Store.ListLocalities(ctx)
}

// FindVisible: verified entries only, optionally by status (wins) or name search. Cached.
func (s *Locality) FindVisible(ctx context.Context, status *model.WaterStatus, search string) ([]model.Locality, error) {
	st := "null"
	if status != nil {
		st = string(*status)
	}
	key := fmt.Sprintf("list:status=%s,search=%s", st, search)
	return cache.Load(ctx, s.Cache, key, func() ([]model.Locality, error) {
		rows, err := s.all(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]model.Locality, 0)
		for _, l := range rows {
			if !l.Verified {
				continue
			}
			switch {
			case status != nil:
				if l.Status != *status {
					continue
				}
			case strings.TrimSpace(search) != "":
				if !store.ContainsFold(l.Name, search) {
					continue
				}
			}
			out = append(out, l)
		}
		return out, nil
	})
}

func (s *Locality) FindByID(ctx context.Context, id int64) (model.Locality, error) {
	return cache.Load(ctx, s.Cache, fmt.Sprintf("id:%d", id), func() (model.Locality, error) { return s.get(ctx, id) })
}

func (s *Locality) get(ctx context.Context, id int64) (model.Locality, error) {
	l, err := s.Store.GetLocality(ctx, id)
	if err == store.ErrNotFound {
		return l, NotFoundError{fmt.Sprintf("No locality with id %d", id)}
	}
	return l, err
}

func (s *Locality) FindNearby(ctx context.Context, lat, lng float64, radiusKm *float64) ([]model.NearbyLocality, error) {
	rows, err := s.all(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[int64]model.Locality{}
	var pts []geo.Point
	for _, l := range rows {
		if c, ok := l.Lat(); ok && l.Verified {
			byID[*l.ID] = l
			pts = append(pts, geo.Point{ID: *l.ID, Lat: c, Lng: *l.Longitude})
		}
	}
	out := make([]model.NearbyLocality, 0, len(pts))
	for _, r := range s.Geo.Nearby(ctx, lat, lng, radiusKm, pts, 0) {
		out = append(out, model.NearbyLocality{Locality: byID[r.ID], DistanceKm: r.DistanceKm})
	}
	return out, nil
}

// FindNearestReliable: closest other MUNICIPAL verified locality, nil if none / no coordinates.
func (s *Locality) FindNearestReliable(ctx context.Context, id int64) (*model.NearbyLocality, error) {
	origin, err := s.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	lat, ok := origin.Lat()
	if !ok {
		return nil, nil
	}
	rows, err := s.all(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[int64]model.Locality{}
	var pts []geo.Point
	for _, l := range rows {
		c, has := l.Lat()
		if has && l.Verified && l.Status == model.Municipal && *l.ID != id {
			byID[*l.ID] = l
			pts = append(pts, geo.Point{ID: *l.ID, Lat: c, Lng: *l.Longitude})
		}
	}
	ranked := s.Geo.Nearby(ctx, lat, *origin.Longitude, nil, pts, 1)
	if len(ranked) == 0 {
		return nil, nil
	}
	return &model.NearbyLocality{Locality: byID[ranked[0].ID], DistanceKm: ranked[0].DistanceKm}, nil
}

func (s *Locality) CreateVerified(ctx context.Context, l model.Locality) (out model.Locality, err error) {
	defer func() { s.audit(ctx, "CREATE_LOCALITY", out, err, l) }()
	l.ID, l.Verified = nil, true
	if l.LastVerified == nil {
		d := model.Today()
		l.LastVerified = &d
	}
	if out, err = s.Store.SaveLocality(ctx, l); err != nil {
		return out, err
	}
	s.Cache.EvictAll(ctx)
	slog.InfoContext(ctx, "created verified locality", "name", out.Name, "id", *out.ID, "status", out.Status)
	return out, nil
}

// CreateUnverifiedReport: citizen report, held for review. Optional Ollama triage; publishes an
// event. Does not evict the cache - unverified rows never appear in FindVisible.
func (s *Locality) CreateUnverifiedReport(ctx context.Context, l model.Locality) (model.Locality, error) {
	l.ID, l.Verified = nil, false
	today := model.Today()
	l.LastVerified = &today

	if l.CitizenDescription != nil && strings.TrimSpace(*l.CitizenDescription) != "" {
		candidate, err := s.closestCandidate(ctx, l)
		if err != nil {
			return model.Locality{}, err
		}
		var candDesc string
		if candidate != nil && candidate.CitizenDescription != nil {
			candDesc = *candidate.CitizenDescription
		}
		if a := s.Analyzer.Analyze(ctx, *l.CitizenDescription, candDesc); a != nil {
			if l.ReportedUrgency == nil {
				u, score, sig := a.Urgency, a.UrgencyConfidence, a.UrgencyReasoning
				l.ReportedUrgency, l.UrgencyScore, l.UrgencySignals = &u, &score, &sig
			}
			if candidate != nil && a.IsDuplicate {
				l.PossibleDuplicateOfID = candidate.ID
				l.DuplicateConfidence = a.DuplicateConfidence
			}
		}
	}

	saved, err := s.Store.SaveLocality(ctx, l)
	if err != nil {
		return model.Locality{}, err
	}
	slog.InfoContext(ctx, "citizen report submitted", "name", saved.Name, "id", *saved.ID, "proposedStatus", saved.Status)
	if saved.PossibleDuplicateOfID != nil {
		slog.InfoContext(ctx, "report flagged as possible duplicate", "id", *saved.ID, "of", *saved.PossibleDuplicateOfID)
	}
	s.Events.PublishReportSubmitted(ctx, model.ReportSubmittedEvent{
		LocalityID: *saved.ID, LocalityName: saved.Name, ProposedStatus: string(saved.Status),
		Notes: saved.Notes, SubmittedAt: time.Now().UTC()})
	return saved, nil
}

// closestCandidate = NearbyReportFinder: nearest earlier unverified report with a description,
// within 1 km and 14 days.
func (s *Locality) closestCandidate(ctx context.Context, c model.Locality) (*model.Locality, error) {
	lat, ok := c.Lat()
	if !ok {
		return nil, nil
	}
	rows, err := s.all(ctx)
	if err != nil {
		return nil, err
	}
	today := model.Today()
	var best *model.Locality
	bestD := 0.0
	for i, e := range rows {
		ec, has := e.Lat()
		if e.Verified || e.ID == nil || !has || e.CitizenDescription == nil || strings.TrimSpace(*e.CitizenDescription) == "" ||
			e.LastVerified == nil || today.Sub(e.LastVerified.Time) > 14*24*time.Hour {
			continue
		}
		d := geo.DistanceKm(lat, *c.Longitude, ec, *e.Longitude)
		if d <= 1.0 && (best == nil || d < bestD) {
			best, bestD = &rows[i], d
		}
	}
	return best, nil
}

// PendingReports: unverified, most urgent (highest score) first.
func (s *Locality) PendingReports(ctx context.Context) ([]model.Locality, error) {
	rows, err := s.all(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Locality, 0)
	for _, l := range rows {
		if !l.Verified {
			out = append(out, l)
		}
	}
	score := func(l model.Locality) int {
		if l.UrgencyScore != nil {
			return *l.UrgencyScore
		}
		return 0
	}
	sort.SliceStable(out, func(i, j int) bool { return score(out[i]) > score(out[j]) })
	return out, nil
}

func (s *Locality) ApproveReport(ctx context.Context, id int64) (out model.Locality, err error) {
	defer func() { s.audit(ctx, "APPROVE_REPORT", out, err, id) }()
	l, err := s.get(ctx, id)
	if err != nil {
		return l, err
	}
	l.Verified = true
	if out, err = s.Store.SaveLocality(ctx, l); err != nil {
		return out, err
	}
	s.Cache.EvictAll(ctx)
	slog.InfoContext(ctx, "approved report - now publicly visible", "name", out.Name, "id", id)
	return out, nil
}

func (s *Locality) Update(ctx context.Context, id int64, patch model.Locality) (out model.Locality, err error) {
	defer func() { s.audit(ctx, "UPDATE_LOCALITY", out, err, id, patch) }()
	existing, err := s.get(ctx, id)
	if err != nil {
		return existing, err
	}
	existing.Name, existing.Ward, existing.Status = patch.Name, patch.Ward, patch.Status
	existing.Latitude, existing.Longitude, existing.Notes = patch.Latitude, patch.Longitude, patch.Notes
	existing.SourceName, existing.SourceURL = patch.SourceName, patch.SourceURL
	if patch.LastVerified != nil {
		existing.LastVerified = patch.LastVerified
	} else {
		d := model.Today()
		existing.LastVerified = &d
	}
	if out, err = s.Store.SaveLocality(ctx, existing); err != nil {
		return out, err
	}
	s.Cache.EvictAll(ctx)
	return out, nil
}

func (s *Locality) Delete(ctx context.Context, id int64) (err error) {
	defer func() { s.audit(ctx, "DELETE_LOCALITY", model.Locality{}, err, id) }()
	slog.WarnContext(ctx, "deleting locality", "id", id)
	if err = s.Store.DeleteLocality(ctx, id); err != nil {
		return err
	}
	s.Cache.EvictAll(ctx)
	return nil
}

// RecomputeWard recomputes one locality's ward from its coordinates (admin/editor, audited).
func (s *Locality) RecomputeWard(ctx context.Context, id int64) (out model.Locality, err error) {
	defer func() { s.audit(ctx, "RECOMPUTE_WARD", out, err, id) }()
	l, err := s.FindByID(ctx, id)
	if err != nil {
		return l, err
	}
	if lat, ok := l.Lat(); ok {
		if w := s.Geo.Ward(ctx, lat, *l.Longitude); w.Found {
			ward := w.Ward
			l.Ward = &ward
		}
	}
	return s.Update(ctx, id, l)
}

func (s *Locality) audit(ctx context.Context, action string, result model.Locality, err error, args ...any) {
	detail := fmt.Sprint(args...)
	if err == nil && result.Name != "" {
		detail = DescribeLocality(result)
	}
	s.Audit.Record(ctx, action, detail, err)
}
