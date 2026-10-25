package store

import (
	"context"
	"sort"
	"strings"
	"sync"

	"pmc/internal/model"
)

// Memory is the dev/test store (the Spring `dev` profile's in-memory H2 equivalent).
type Memory struct {
	mu       sync.RWMutex
	locs     map[int64]model.Locality
	users    map[int64]model.User
	audits   []model.AuditLog
	nextLoc  int64
	nextUser int64
	nextAud  int64
}

func NewMemory() *Memory {
	return &Memory{locs: map[int64]model.Locality{}, users: map[int64]model.User{}, nextLoc: 1, nextUser: 1, nextAud: 1}
}

func (m *Memory) ListLocalities(context.Context) ([]model.Locality, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]model.Locality, 0, len(m.locs))
	for _, l := range m.locs {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return *out[i].ID < *out[j].ID })
	return out, nil
}

func (m *Memory) GetLocality(_ context.Context, id int64) (model.Locality, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	l, ok := m.locs[id]
	if !ok {
		return model.Locality{}, ErrNotFound
	}
	return l, nil
}

func (m *Memory) SaveLocality(_ context.Context, l model.Locality) (model.Locality, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l.ID == nil {
		id := m.nextLoc
		m.nextLoc++
		l.ID = &id
	} else if _, ok := m.locs[*l.ID]; !ok {
		return model.Locality{}, ErrNotFound
	}
	m.locs[*l.ID] = l
	return l, nil
}

func (m *Memory) DeleteLocality(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.locs, id) // Spring's deleteById is a no-op for a missing id
	return nil
}

func (m *Memory) ExistsLocalityByName(_ context.Context, name string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, l := range m.locs {
		if l.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func (m *Memory) CountUsers(context.Context) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.users), nil
}

func (m *Memory) ListUsers(context.Context) ([]model.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]model.User, 0, len(m.users))
	for _, u := range m.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *Memory) FindUserByUsername(_ context.Context, username string) (model.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if u.Username == username {
			return u, nil
		}
	}
	return model.User{}, ErrNotFound
}

func (m *Memory) CreateUser(_ context.Context, u model.User) (model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.users {
		if e.Username == u.Username {
			return model.User{}, ErrDuplicate
		}
	}
	u.ID = m.nextUser
	m.nextUser++
	m.users[u.ID] = u
	return u, nil
}

func (m *Memory) DeleteUser(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[id]; !ok {
		return ErrNotFound
	}
	delete(m.users, id)
	return nil
}

func (m *Memory) AddAudit(_ context.Context, a model.AuditLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a.ID = m.nextAud
	m.nextAud++
	m.audits = append(m.audits, a)
	return nil
}

func (m *Memory) ListAudit(context.Context) ([]model.AuditLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := append([]model.AuditLog(nil), m.audits...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp.After(out[j].Timestamp) })
	return out, nil
}

// SeedIfMissing inserts any seed row whose name isn't present - safe on every start, and picks up
// newly added seed rows, exactly like LocalityDataInitializer.
func SeedIfMissing(ctx context.Context, s Store, seeds []model.Seed) (int, error) {
	n := 0
	for _, seed := range seeds {
		exists, err := s.ExistsLocalityByName(ctx, seed.Name)
		if err != nil {
			return n, err
		}
		if exists {
			continue
		}
		if _, err := s.SaveLocality(ctx, seed.ToLocality()); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ContainsFold is shared by the in-memory search paths.
func ContainsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
