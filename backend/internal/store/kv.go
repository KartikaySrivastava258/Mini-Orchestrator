// Package store is where cluster state lives. KV is the thin waist between
// us and etcd, and MemoryKV copies its behaviour (revisions and all) so the
// tests run without an etcd anywhere in sight.
package store

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
)

// The errors everything in this package speaks.
var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrConflict      = errors.New("revision conflict")
	ErrUnavailable   = errors.New("state store unavailable")
	ErrCorrupt       = errors.New("stored object is malformed")
	// ErrNoChange: return it from an update callback to say "nah, nothing to write".
	ErrNoChange = errors.New("no change")
)

// Item is a raw key/value plus the revision it was last modified at.
type Item struct {
	Key      string
	Value    []byte
	Revision int64
}

// KV is a tiny transactional key-value store.
//
// Revisions are etcd ModRevisions. Update and conditional Delete only go
// through if your expected revision still matches, which gives us optimistic
// locking without any actual locks.
type KV interface {
	Get(ctx context.Context, key string) (Item, error)
	// List returns all items whose key starts with prefix, sorted by key.
	List(ctx context.Context, prefix string) ([]Item, error)
	// Create stores value only if key does not exist.
	Create(ctx context.Context, key string, value []byte) (int64, error)
	// Update replaces value only if the stored revision equals expectedRevision.
	Update(ctx context.Context, key string, value []byte, expectedRevision int64) (int64, error)
	// Delete removes key. expectedRevision 0 deletes unconditionally.
	Delete(ctx context.Context, key string, expectedRevision int64) error
	Ping(ctx context.Context) error
	Close() error
}

// MemoryKV is etcd at home: same revision rules, zero persistence. Tests only.
type MemoryKV struct {
	mu      sync.Mutex
	rev     int64
	data    map[string]Item
	failErr error
}

// NewMemoryKV returns an empty store.
func NewMemoryKV() *MemoryKV { return &MemoryKV{data: map[string]Item{}} }

// SetFailure makes every call fail with err until you pass nil. Perfect for
// "what if etcd just dies" tests.
func (m *MemoryKV) SetFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failErr = err
}

// PutRaw writes bytes with zero checks, so tests can see how we cope with garbage.
func (m *MemoryKV) PutRaw(key string, value []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rev++
	m.data[key] = Item{Key: key, Value: clone(value), Revision: m.rev}
}

func clone(b []byte) []byte { return append([]byte(nil), b...) }

func (m *MemoryKV) check(ctx context.Context) error {
	if m.failErr != nil {
		return m.failErr
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrUnavailable, err)
	}
	return nil
}

func (m *MemoryKV) Get(ctx context.Context, key string) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.check(ctx); err != nil {
		return Item{}, err
	}
	it, ok := m.data[key]
	if !ok {
		return Item{}, ErrNotFound
	}
	return Item{Key: it.Key, Value: clone(it.Value), Revision: it.Revision}, nil
}

func (m *MemoryKV) List(ctx context.Context, prefix string) ([]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.check(ctx); err != nil {
		return nil, err
	}
	out := make([]Item, 0)
	for k, it := range m.data {
		if strings.HasPrefix(k, prefix) {
			out = append(out, Item{Key: it.Key, Value: clone(it.Value), Revision: it.Revision})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (m *MemoryKV) Create(ctx context.Context, key string, value []byte) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.check(ctx); err != nil {
		return 0, err
	}
	if _, ok := m.data[key]; ok {
		return 0, ErrAlreadyExists
	}
	m.rev++
	m.data[key] = Item{Key: key, Value: clone(value), Revision: m.rev}
	return m.rev, nil
}

func (m *MemoryKV) Update(ctx context.Context, key string, value []byte, expectedRevision int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.check(ctx); err != nil {
		return 0, err
	}
	it, ok := m.data[key]
	if !ok {
		return 0, ErrNotFound
	}
	if it.Revision != expectedRevision {
		return 0, ErrConflict
	}
	m.rev++
	m.data[key] = Item{Key: key, Value: clone(value), Revision: m.rev}
	return m.rev, nil
}

func (m *MemoryKV) Delete(ctx context.Context, key string, expectedRevision int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.check(ctx); err != nil {
		return err
	}
	it, ok := m.data[key]
	if !ok {
		return ErrNotFound
	}
	if expectedRevision != 0 && it.Revision != expectedRevision {
		return ErrConflict
	}
	delete(m.data, key)
	return nil
}

func (m *MemoryKV) Ping(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.check(ctx)
}

func (m *MemoryKV) Close() error { return nil }
