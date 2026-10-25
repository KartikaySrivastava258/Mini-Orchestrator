package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"mini-orchestrator/internal/domain"
)

// DefaultPrefix is the versioned key namespace. Keys look like this:
//
//	{prefix}/pods/{name}
//	{prefix}/nodes/{name}
//	{prefix}/replicasets/{name}
const DefaultPrefix = "/mini-orchestrator/v1"

// updateRetries is how many times a CAS write retries before giving up.
const updateRetries = 5

// Store is the typed layer on top of KV. You talk pods and nodes, it talks bytes.
type Store struct {
	kv     KV
	prefix string
	log    *slog.Logger
}

// New returns a Store. prefix must not end with '/'.
func New(kv KV, prefix string, log *slog.Logger) *Store {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Store{kv: kv, prefix: strings.TrimRight(prefix, "/"), log: log}
}

// Ping checks that the backing store is reachable.
func (s *Store) Ping(ctx context.Context) error { return s.kv.Ping(ctx) }

// Close releases the backing client.
func (s *Store) Close() error { return s.kv.Close() }

func (s *Store) key(kind, name string) string  { return s.prefix + "/" + kind + "/" + name }
func (s *Store) kindPrefix(kind string) string { return s.prefix + "/" + kind + "/" }

const (
	kindPods        = "pods"
	kindNodes       = "nodes"
	kindReplicaSets = "replicasets"
)

// --- generic helpers -------------------------------------------------------

func getObj[T any](ctx context.Context, kv KV, key string) (T, int64, error) {
	var zero T
	it, err := kv.Get(ctx, key)
	if err != nil {
		return zero, 0, err
	}
	var obj T
	if err := json.Unmarshal(it.Value, &obj); err != nil {
		return zero, 0, fmt.Errorf("%w: %s: %v", ErrCorrupt, key, err)
	}
	return obj, it.Revision, nil
}

// listObj decodes everything under prefix. Corrupt records get skipped and
// logged, so one bad record can't take a whole list endpoint down with it.
func listObj[T any](ctx context.Context, kv KV, prefix string, log *slog.Logger) ([]T, error) {
	items, err := kv.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(items))
	for _, it := range items {
		var obj T
		if err := json.Unmarshal(it.Value, &obj); err != nil {
			log.Warn("skipping malformed stored object", "key", it.Key, "error", err)
			continue
		}
		out = append(out, obj)
	}
	return out, nil
}

func createObj[T any](ctx context.Context, kv KV, key string, obj T) error {
	b, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("encode %s: %w", key, err)
	}
	_, err = kv.Create(ctx, key, b)
	return err
}

// updateObj is read, tweak, write, but the write only lands if nobody touched
// the key in between. Lost the race? Re-read and go again.
//
// If mutate returns ErrNoChange the write is skipped. Don't modify the object
// before returning that, or future you will be very confused.
func updateObj[T any](ctx context.Context, kv KV, key string, mutate func(*T) error) (T, error) {
	var zero T
	for attempt := 0; attempt < updateRetries; attempt++ {
		obj, rev, err := getObj[T](ctx, kv, key)
		if err != nil {
			return zero, err
		}
		if err := mutate(&obj); err != nil {
			if errors.Is(err, ErrNoChange) {
				return obj, nil
			}
			return zero, err
		}
		b, err := json.Marshal(obj)
		if err != nil {
			return zero, fmt.Errorf("encode %s: %w", key, err)
		}
		if _, err := kv.Update(ctx, key, b, rev); err != nil {
			if errors.Is(err, ErrConflict) {
				continue
			}
			return zero, err
		}
		return obj, nil
	}
	return zero, ErrConflict
}

// deleteObj deletes key if check is happy with the current value, and only if
// that value hasn't changed since check looked at it.
func deleteObj[T any](ctx context.Context, kv KV, key string, check func(T) error) error {
	for attempt := 0; attempt < updateRetries; attempt++ {
		obj, rev, err := getObj[T](ctx, kv, key)
		if err != nil {
			return err
		}
		if check != nil {
			if err := check(obj); err != nil {
				return err
			}
		}
		if err := kv.Delete(ctx, key, rev); err != nil {
			if errors.Is(err, ErrConflict) {
				continue
			}
			return err
		}
		return nil
	}
	return ErrConflict
}

// --- pods ------------------------------------------------------------------

func (s *Store) CreatePod(ctx context.Context, p domain.Pod) error {
	return createObj(ctx, s.kv, s.key(kindPods, p.Name), p)
}

func (s *Store) GetPod(ctx context.Context, name string) (domain.Pod, error) {
	p, _, err := getObj[domain.Pod](ctx, s.kv, s.key(kindPods, name))
	return p, err
}

// ListPods returns all pods sorted by name.
func (s *Store) ListPods(ctx context.Context) ([]domain.Pod, error) {
	return listObj[domain.Pod](ctx, s.kv, s.kindPrefix(kindPods), s.log)
}

func (s *Store) UpdatePod(ctx context.Context, name string, mutate func(*domain.Pod) error) (domain.Pod, error) {
	return updateObj(ctx, s.kv, s.key(kindPods, name), mutate)
}

func (s *Store) DeletePod(ctx context.Context, name string, check func(domain.Pod) error) error {
	return deleteObj(ctx, s.kv, s.key(kindPods, name), check)
}

// --- nodes -----------------------------------------------------------------

func (s *Store) CreateNode(ctx context.Context, n domain.Node) error {
	return createObj(ctx, s.kv, s.key(kindNodes, n.Name), n)
}

func (s *Store) GetNode(ctx context.Context, name string) (domain.Node, error) {
	n, _, err := getObj[domain.Node](ctx, s.kv, s.key(kindNodes, name))
	return n, err
}

// ListNodes returns all nodes sorted by name.
func (s *Store) ListNodes(ctx context.Context) ([]domain.Node, error) {
	return listObj[domain.Node](ctx, s.kv, s.kindPrefix(kindNodes), s.log)
}

func (s *Store) UpdateNode(ctx context.Context, name string, mutate func(*domain.Node) error) (domain.Node, error) {
	return updateObj(ctx, s.kv, s.key(kindNodes, name), mutate)
}

func (s *Store) DeleteNode(ctx context.Context, name string, check func(domain.Node) error) error {
	return deleteObj(ctx, s.kv, s.key(kindNodes, name), check)
}

// --- replica sets ----------------------------------------------------------

func (s *Store) CreateReplicaSet(ctx context.Context, rs domain.ReplicaSet) error {
	return createObj(ctx, s.kv, s.key(kindReplicaSets, rs.Name), rs)
}

func (s *Store) GetReplicaSet(ctx context.Context, name string) (domain.ReplicaSet, error) {
	rs, _, err := getObj[domain.ReplicaSet](ctx, s.kv, s.key(kindReplicaSets, name))
	return rs, err
}

// ListReplicaSets returns all ReplicaSets sorted by name.
func (s *Store) ListReplicaSets(ctx context.Context) ([]domain.ReplicaSet, error) {
	return listObj[domain.ReplicaSet](ctx, s.kv, s.kindPrefix(kindReplicaSets), s.log)
}

func (s *Store) UpdateReplicaSet(ctx context.Context, name string, mutate func(*domain.ReplicaSet) error) (domain.ReplicaSet, error) {
	return updateObj(ctx, s.kv, s.key(kindReplicaSets, name), mutate)
}

func (s *Store) DeleteReplicaSet(ctx context.Context, name string, check func(domain.ReplicaSet) error) error {
	return deleteObj(ctx, s.kv, s.key(kindReplicaSets, name), check)
}
