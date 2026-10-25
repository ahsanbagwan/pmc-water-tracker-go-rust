// Package store is the persistence boundary (the Spring repositories).
package store

import (
	"context"
	"errors"

	"pmc/internal/model"
)

var ErrNotFound = errors.New("not found")
var ErrDuplicate = errors.New("duplicate")

type Store interface {
	// Localities
	ListLocalities(ctx context.Context) ([]model.Locality, error) // all rows, id ascending
	GetLocality(ctx context.Context, id int64) (model.Locality, error)
	SaveLocality(ctx context.Context, l model.Locality) (model.Locality, error) // insert if ID nil, else update
	DeleteLocality(ctx context.Context, id int64) error
	ExistsLocalityByName(ctx context.Context, name string) (bool, error)

	// Users
	CountUsers(ctx context.Context) (int, error)
	ListUsers(ctx context.Context) ([]model.User, error)
	FindUserByUsername(ctx context.Context, username string) (model.User, error)
	CreateUser(ctx context.Context, u model.User) (model.User, error) // ErrDuplicate on clash
	DeleteUser(ctx context.Context, id int64) error                   // ErrNotFound if absent

	// Audit
	AddAudit(ctx context.Context, a model.AuditLog) error
	ListAudit(ctx context.Context) ([]model.AuditLog, error) // newest first
}
