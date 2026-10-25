package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"pmc/internal/kv"
	"pmc/internal/model"
)

const OllamaToggleKey = "feature-toggle:ollama-enabled"

type Analysis struct {
	Urgency             model.ReportUrgency
	UrgencyConfidence   int
	UrgencyReasoning    string
	IsDuplicate         bool
	DuplicateConfidence *float64
	DuplicateReasoning  string
}

// Ollama triages citizen reports with a local LLM. The on/off flag lives in the shared KV so
// every replica agrees; if the KV is down it falls back to the startup default. Best-effort:
// nil on any failure - classification must never block a report.
type Ollama struct {
	KV             kv.KV
	StartupDefault bool
	Host, Model    string
	HC             *http.Client
}

func (o *Ollama) client() *http.Client {
	if o.HC != nil {
		return o.HC
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (o *Ollama) Enabled(ctx context.Context) bool {
	v, ok, err := o.KV.Get(ctx, OllamaToggleKey)
	if err != nil {
		slog.WarnContext(ctx, "KV unreachable checking Ollama toggle; using OLLAMA_ENABLED default", "error", err)
		return o.StartupDefault
	}
	if !ok {
		return o.StartupDefault // never toggled: absent is NOT "false"
	}
	b, _ := strconv.ParseBool(v)
	return b
}

func (o *Ollama) SetEnabled(ctx context.Context, on bool) (bool, error) {
	return on, o.KV.Set(ctx, OllamaToggleKey, strconv.FormatBool(on), 0)
}

func (o *Ollama) Analyze(ctx context.Context, description, candidate string) *Analysis {
	if strings.TrimSpace(description) == "" || !o.Enabled(ctx) {
		return nil
	}
	a, err := o.call(ctx, description, candidate)
	if err != nil {
		slog.WarnContext(ctx, "Ollama call failed (is `ollama serve` running?); leaving this report unclassified", "error", err)
		return nil
	}
	return a
}

func buildPrompt(description, candidate string) string {
	has := strings.TrimSpace(candidate) != ""
	var p strings.Builder
	p.WriteString("You are triaging a citizen-submitted water-infrastructure complaint for a municipal water tracker.\n\n")
	fmt.Fprintf(&p, "Report description: \"%s\"\n\n", description)
	p.WriteString("Classify its urgency as one of LOW, MEDIUM, HIGH, CRITICAL, considering duration, ")
	p.WriteString("safety/health risk (contamination, sewage, illness), scope (how many people affected), ")
	p.WriteString("and infrastructure damage (bursts, flooding).\n\n")
	if has {
		fmt.Fprintf(&p, "A separate report was submitted nearby around the same time, with this description: \"%s\"\n", candidate)
		p.WriteString("Judge whether these two reports likely describe the SAME underlying incident.\n\n")
	}
	p.WriteString("Respond with ONLY valid JSON, no other text, in exactly this shape:\n")
	p.WriteString(`{"urgency": "LOW|MEDIUM|HIGH|CRITICAL", "urgencyConfidence": <integer 0-100>, "urgencyReasoning": "<one short sentence>"`)
	if has {
		p.WriteString(`, "isDuplicate": <true|false>, "duplicateConfidence": <number 0.0-1.0>, "duplicateReasoning": "<one short sentence>"`)
	}
	p.WriteString("}")
	return p.String()
}

func (o *Ollama) call(ctx context.Context, description, candidate string) (*Analysis, error) {
	body, _ := json.Marshal(map[string]any{"model": o.Model, "prompt": buildPrompt(description, candidate), "stream": false, "format": "json"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(o.Host, "/")+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama returned %d", resp.StatusCode)
	}
	var env struct{ Response string }
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("could not parse Ollama response envelope: %w", err)
	}
	return parseAnalysis(env.Response, strings.TrimSpace(candidate) != "")
}

var fenceStart = regexp.MustCompile("^```(json)?\\s*")
var fenceEnd = regexp.MustCompile("```\\s*$")

func parseAnalysis(text string, expectDup bool) (*Analysis, error) {
	cleaned := strings.TrimSpace(fenceEnd.ReplaceAllString(fenceStart.ReplaceAllString(strings.TrimSpace(text), ""), ""))
	var j struct {
		Urgency             *string  `json:"urgency"`
		UrgencyConfidence   *int     `json:"urgencyConfidence"`
		UrgencyReasoning    *string  `json:"urgencyReasoning"`
		IsDuplicate         *bool    `json:"isDuplicate"`
		DuplicateConfidence *float64 `json:"duplicateConfidence"`
		DuplicateReasoning  *string  `json:"duplicateReasoning"`
	}
	if err := json.Unmarshal([]byte(cleaned), &j); err != nil {
		return nil, err
	}
	urg := "MEDIUM"
	if j.Urgency != nil {
		urg = strings.ToUpper(*j.Urgency)
	}
	u, ok := model.ParseUrgency(urg)
	if !ok {
		return nil, fmt.Errorf("unknown urgency %q", urg)
	}
	a := &Analysis{Urgency: u, UrgencyConfidence: 50}
	if j.UrgencyConfidence != nil {
		a.UrgencyConfidence = *j.UrgencyConfidence
	}
	if j.UrgencyReasoning != nil {
		a.UrgencyReasoning = *j.UrgencyReasoning
	}
	if expectDup {
		a.IsDuplicate = j.IsDuplicate != nil && *j.IsDuplicate
		a.DuplicateConfidence = j.DuplicateConfidence
		if j.DuplicateReasoning != nil {
			a.DuplicateReasoning = *j.DuplicateReasoning
		}
	}
	return a, nil
}
