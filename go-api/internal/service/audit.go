// Package service holds the business logic (the Spring @Service classes).
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"pmc/internal/auth"
	"pmc/internal/model"
	"pmc/internal/store"
)

// Auditor is the @Audited aspect: who, what, with what result, success or failure.
type Auditor struct{ Store store.Store }

func (a Auditor) Record(ctx context.Context, action, detail string, err error) {
	entry := model.AuditLog{Username: auth.Username(ctx), Action: action, Success: err == nil, Timestamp: time.Now().UTC()}
	if detail != "" {
		entry.Detail = &detail
	}
	if err != nil {
		m := err.Error()
		entry.ErrorMessage = &m
	}
	if serr := a.Store.AddAudit(ctx, entry); serr != nil {
		slog.ErrorContext(ctx, "failed to write audit entry", "action", action, "error", serr)
	}
}

func DescribeLocality(l model.Locality) string {
	id := "null"
	if l.ID != nil {
		id = fmt.Sprint(*l.ID)
	}
	return fmt.Sprintf("Locality(id=%s, name=%s, status=%s)", id, l.Name, l.Status)
}
