package messaging

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pmc/internal/model"
)

func TestNotifierSendsBrevoRequestAndNeverPanicsOnFailure(t *testing.T) {
	var gotKey string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("api-key")
		_ = json.NewDecoder(r.Body).Decode(&body)
	}))
	defer srv.Close()
	notes := "no water"
	ev := model.ReportSubmittedEvent{LocalityID: 7, LocalityName: "Wagholi", ProposedStatus: "TANKER_DEPENDENT", Notes: &notes, SubmittedAt: time.Now()}
	n := Notifier{APIKey: "k", From: "a@x.com", To: "b@x.com", URL: srv.URL}
	n.Handle(context.Background(), ev)
	if gotKey != "k" || body["subject"] != "New water status report: Wagholi" || !strings.Contains(body["textContent"].(string), "POST /api/localities/reports/7/approve") {
		t.Fatalf("bad request: key=%q body=%v", gotKey, body)
	}
	n.URL = "http://127.0.0.1:1" // unreachable: must just log
	n.Handle(context.Background(), ev)
	(Notifier{}).Handle(context.Background(), ev) // unconfigured: must just log
}

func TestSafePublisherSkipsWhenDisabledAndSwallowsErrors(t *testing.T) {
	Safe{Enabled: false, Inner: failing{}}.PublishReportSubmitted(context.Background(), model.ReportSubmittedEvent{})
	Safe{Enabled: true, Inner: failing{}}.PublishReportSubmitted(context.Background(), model.ReportSubmittedEvent{})
}

type failing struct{}

func (failing) Publish(context.Context, model.ReportSubmittedEvent) error { return context.Canceled }
