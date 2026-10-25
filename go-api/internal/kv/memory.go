package kv

import (
	"context"
	"strconv"
	"sync"
	"time"
)

type entry struct {
	val string
	exp time.Time // zero = no expiry
}

// Memory is for single-process dev and tests. With >1 replica use Redis.
type Memory struct {
	mu   sync.Mutex
	data map[string]entry
	subs map[string][]chan string
	now  func() time.Time
}

func NewMemory() *Memory {
	return &Memory{data: map[string]entry{}, subs: map[string][]chan string{}, now: time.Now}
}

func (m *Memory) live(key string) (entry, bool) {
	e, ok := m.data[key]
	if !ok {
		return entry{}, false
	}
	if !e.exp.IsZero() && !m.now().Before(e.exp) {
		delete(m.data, key)
		return entry{}, false
	}
	return e, true
}

func exp(m *Memory, ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}
	return m.now().Add(ttl)
}

func (m *Memory) Get(_ context.Context, k string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.live(k)
	return e.val, ok, nil
}

func (m *Memory) Set(_ context.Context, k, v string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[k] = entry{v, exp(m, ttl)}
	return nil
}

func (m *Memory) Del(_ context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		delete(m.data, k)
	}
	return nil
}

func (m *Memory) Incr(_ context.Context, k string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, _ := m.live(k)
	n, _ := strconv.ParseInt(e.val, 10, 64)
	n++
	m.data[k] = entry{strconv.FormatInt(n, 10), e.exp}
	return n, nil
}

func (m *Memory) Expire(_ context.Context, k string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.live(k); ok {
		e.exp = exp(m, ttl)
		m.data[k] = e
	}
	return nil
}

func (m *Memory) SetNX(_ context.Context, k, v string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.live(k); ok {
		return false, nil
	}
	m.data[k] = entry{v, exp(m, ttl)}
	return true, nil
}

func (m *Memory) Publish(_ context.Context, ch, msg string) error {
	m.mu.Lock()
	subs := append([]chan string(nil), m.subs[ch]...)
	m.mu.Unlock()
	for _, s := range subs {
		select {
		case s <- msg:
		default: // slow subscriber: drop, like Redis pub/sub
		}
	}
	return nil
}

func (m *Memory) Subscribe(ctx context.Context, ch string) (<-chan string, error) {
	c := make(chan string, 64)
	m.mu.Lock()
	m.subs[ch] = append(m.subs[ch], c)
	m.mu.Unlock()
	go func() {
		<-ctx.Done()
		m.mu.Lock()
		list := m.subs[ch]
		for i, s := range list {
			if s == c {
				m.subs[ch] = append(list[:i], list[i+1:]...)
				break
			}
		}
		m.mu.Unlock()
		close(c)
	}()
	return c, nil
}
