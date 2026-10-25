// Package messaging: the report-submitted event (RabbitMQ in production) and the Brevo email
// notifier. Publishing is fire-and-forget and NEVER fails the citizen's request.
package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"pmc/internal/model"
)

const (
	Exchange         = "pmc-water-tracker-exchange"
	ReportQueue      = "report-submitted-queue"
	ReportRoutingKey = "report.submitted"
)

type Publisher interface {
	Publish(ctx context.Context, ev model.ReportSubmittedEvent) error
}

type Noop struct{}

func (Noop) Publish(context.Context, model.ReportSubmittedEvent) error { return nil }

// Safe wraps a Publisher: skips when disabled and swallows (but logs) broker errors - the report
// is already saved by the time this runs, only the notification is at risk.
type Safe struct {
	Enabled bool
	Inner   Publisher
}

func (s Safe) PublishReportSubmitted(ctx context.Context, ev model.ReportSubmittedEvent) {
	if !s.Enabled || s.Inner == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.Inner.Publish(ctx, ev); err != nil {
		slog.WarnContext(ctx, "failed to publish report-submitted event; the report itself was already saved successfully",
			"locality", ev.LocalityName, "error", err)
	}
}

// Notifier emails the admin through Brevo's HTTPS API (Render's free tier blocks SMTP).
type Notifier struct {
	APIKey, From, To string
	URL              string // defaults to Brevo
	HC               *http.Client
}

const brevoURL = "https://api.brevo.com/v3/smtp/email"

// Handle never returns an error: a notification problem must not look like a processing failure
// (which would trigger broker redelivery).
func (n Notifier) Handle(ctx context.Context, ev model.ReportSubmittedEvent) {
	if n.APIKey == "" || n.From == "" || n.To == "" {
		slog.WarnContext(ctx, "skipping email notification (BREVO_API_KEY, MAIL_FROM_ADDRESS or ADMIN_NOTIFICATION_EMAIL not set); the report was still saved",
			"locality", ev.LocalityName)
		return
	}
	notes := "(none)"
	if ev.Notes != nil && strings.TrimSpace(*ev.Notes) != "" {
		notes = *ev.Notes
	}
	text := fmt.Sprintf("A citizen submitted a report for %s.\n\nProposed status: %s\nNotes: %s\nSubmitted at: %s\nLocality id: %d\n\nReview and approve it with:\nPOST /api/localities/reports/%d/approve",
		ev.LocalityName, ev.ProposedStatus, notes, ev.SubmittedAt.Format(time.RFC3339), ev.LocalityID, ev.LocalityID)
	body, _ := json.Marshal(map[string]any{
		"sender":      map[string]string{"email": n.From},
		"to":          []map[string]string{{"email": n.To}},
		"subject":     "New water status report: " + ev.LocalityName,
		"textContent": text,
	})
	url := n.URL
	if url == "" {
		url = brevoURL
	}
	hc := n.HC
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("api-key", n.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			err = fmt.Errorf("brevo returned %d", resp.StatusCode)
		}
	}
	if err != nil {
		slog.WarnContext(ctx, "failed to send email; the report itself was still saved successfully", "locality", ev.LocalityName, "error", err)
		return
	}
	slog.InfoContext(ctx, "sent report notification email", "locality", ev.LocalityName)
}
