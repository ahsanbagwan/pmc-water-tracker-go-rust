package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"pmc/internal/geo"
	"pmc/internal/lock"
	"pmc/internal/logx"
	"pmc/internal/store"
)

// LinkChecker verifies every locality's source URL (the actual HTTP fan-out runs in the Rust
// service) and records the result. Weekly via the scheduler, or on demand by an admin.
type LinkChecker struct {
	Store  store.Store
	Geo    geo.Service
	Locker *lock.Locker
	Audit  Auditor
}

func (c *LinkChecker) CheckAll(ctx context.Context) {
	rows, err := c.Store.ListLocalities(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "source-link check: could not list localities", "error", err)
		return
	}
	var links []geo.LinkIn
	names := map[int64]string{}
	urls := map[int64]string{}
	for _, l := range rows {
		if l.SourceURL != nil && strings.TrimSpace(*l.SourceURL) != "" {
			links = append(links, geo.LinkIn{ID: *l.ID, URL: *l.SourceURL})
			names[*l.ID], urls[*l.ID] = l.Name, *l.SourceURL
		}
	}
	broken := 0
	for _, r := range c.Geo.CheckLinks(ctx, links) {
		l, err := c.Store.GetLocality(ctx, r.ID) // re-read: don't clobber concurrent edits
		if err != nil {
			continue
		}
		healthy, now := r.Healthy, time.Now().UTC()
		l.SourceURLHealthy, l.SourceURLLastChecked = &healthy, &now
		if _, err := c.Store.SaveLocality(ctx, l); err != nil {
			slog.ErrorContext(ctx, "could not save link result", "id", r.ID, "error", err)
		}
		if !r.Healthy {
			broken++
			slog.WarnContext(ctx, "broken source link", "locality", names[r.ID], "url", urls[r.ID])
		}
	}
	slog.InfoContext(ctx, "checked source links", "checked", len(links), "broken", broken)
}

// TriggerManual returns immediately; the check runs in the background with the request's log
// fields (correlation id) but not its cancellation.
func (c *LinkChecker) TriggerManual(ctx context.Context) {
	c.Audit.Record(ctx, "CHECK_SOURCE_LINKS", "", nil)
	bg := logx.Detach(ctx)
	go c.CheckAll(bg)
}

// Scheduled is guarded by the distributed lock so N replicas run it once, not N times.
func (c *LinkChecker) Scheduled(ctx context.Context) {
	c.Locker.TryRun(ctx, "sourceLinkHealthCheck", 30*time.Minute, 5*time.Minute, func() { c.CheckAll(ctx) })
}

// NextMonday3AM is the default schedule ("0 0 3 * * MON").
func NextMonday3AM(now time.Time) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	for t.Weekday() != time.Monday || !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

func (c *LinkChecker) RunWeekly(ctx context.Context) {
	for {
		next := NextMonday3AM(time.Now())
		select {
		case <-time.After(time.Until(next)):
			c.Scheduled(logx.With(ctx, "job", "sourceLinkHealthCheck"))
		case <-ctx.Done():
			return
		}
	}
}
