package store

import (
	"context"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
)

// EtcdKV is the real KV, backed by etcd. Every call has a deadline and every
// write is a transaction, so two writers can't quietly overwrite each other.
type EtcdKV struct {
	cli       *clientv3.Client
	opTimeout time.Duration
}

// NewEtcdKV builds the client. The etcd client connects lazily, so this works
// even when etcd is down; Ping tells you the truth.
//
// Two tweaks on top of the defaults. gRPC's reconnect backoff is capped at 5s,
// because the default can leave us waiting up to two minutes after etcd comes
// back (learned that one the hard way). And the client's own logger is muted,
// since the store already logs every failure with more useful context.
func NewEtcdKV(endpoints []string, dialTimeout, opTimeout time.Duration) (*EtcdKV, error) {
	connect := grpc.ConnectParams{
		Backoff: backoff.Config{
			BaseDelay:  500 * time.Millisecond,
			Multiplier: 1.6,
			Jitter:     0.2,
			MaxDelay:   5 * time.Second,
		},
		MinConnectTimeout: dialTimeout,
	}
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: dialTimeout,
		DialOptions: []grpc.DialOption{grpc.WithConnectParams(connect)},
		Logger:      zap.NewNop(),
	})
	if err != nil {
		return nil, fmt.Errorf("create etcd client: %w", err)
	}
	return &EtcdKV{cli: cli, opTimeout: opTimeout}, nil
}

func (e *EtcdKV) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, e.opTimeout)
}

// unavailable wraps a client error. The details stay in our logs; the API
// never shows them to clients.
func unavailable(op string, err error) error {
	return fmt.Errorf("%w: etcd %s: %v", ErrUnavailable, op, err)
}

func (e *EtcdKV) Get(ctx context.Context, key string) (Item, error) {
	ctx, cancel := e.withTimeout(ctx)
	defer cancel()
	resp, err := e.cli.Get(ctx, key)
	if err != nil {
		return Item{}, unavailable("get", err)
	}
	if len(resp.Kvs) == 0 {
		return Item{}, ErrNotFound
	}
	kv := resp.Kvs[0]
	return Item{Key: string(kv.Key), Value: kv.Value, Revision: kv.ModRevision}, nil
}

func (e *EtcdKV) List(ctx context.Context, prefix string) ([]Item, error) {
	ctx, cancel := e.withTimeout(ctx)
	defer cancel()
	resp, err := e.cli.Get(ctx, prefix,
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
	)
	if err != nil {
		return nil, unavailable("list", err)
	}
	out := make([]Item, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		out = append(out, Item{Key: string(kv.Key), Value: kv.Value, Revision: kv.ModRevision})
	}
	return out, nil
}

func (e *EtcdKV) Create(ctx context.Context, key string, value []byte) (int64, error) {
	ctx, cancel := e.withTimeout(ctx)
	defer cancel()
	resp, err := e.cli.Txn(ctx).
		If(clientv3.Compare(clientv3.CreateRevision(key), "=", 0)).
		Then(clientv3.OpPut(key, string(value))).
		Commit()
	if err != nil {
		return 0, unavailable("create", err)
	}
	if !resp.Succeeded {
		return 0, ErrAlreadyExists
	}
	return resp.Header.Revision, nil
}

func (e *EtcdKV) Update(ctx context.Context, key string, value []byte, expectedRevision int64) (int64, error) {
	ctx, cancel := e.withTimeout(ctx)
	defer cancel()
	resp, err := e.cli.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", expectedRevision)).
		Then(clientv3.OpPut(key, string(value))).
		Else(clientv3.OpGet(key)).
		Commit()
	if err != nil {
		return 0, unavailable("update", err)
	}
	if !resp.Succeeded {
		return 0, failedCompare(resp)
	}
	return resp.Header.Revision, nil
}

func (e *EtcdKV) Delete(ctx context.Context, key string, expectedRevision int64) error {
	ctx, cancel := e.withTimeout(ctx)
	defer cancel()
	if expectedRevision == 0 {
		resp, err := e.cli.Delete(ctx, key)
		if err != nil {
			return unavailable("delete", err)
		}
		if resp.Deleted == 0 {
			return ErrNotFound
		}
		return nil
	}
	resp, err := e.cli.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", expectedRevision)).
		Then(clientv3.OpDelete(key)).
		Else(clientv3.OpGet(key)).
		Commit()
	if err != nil {
		return unavailable("delete", err)
	}
	if !resp.Succeeded {
		return failedCompare(resp)
	}
	return nil
}

// failedCompare works out why a transaction failed: the key vanished, or someone changed it first.
func failedCompare(resp *clientv3.TxnResponse) error {
	if len(resp.Responses) > 0 {
		if rr := resp.Responses[0].GetResponseRange(); rr != nil && len(rr.Kvs) == 0 {
			return ErrNotFound
		}
	}
	return ErrConflict
}

func (e *EtcdKV) Ping(ctx context.Context) error {
	ctx, cancel := e.withTimeout(ctx)
	defer cancel()
	if _, err := e.cli.Get(ctx, "mini-orchestrator-readiness-probe", clientv3.WithCountOnly()); err != nil {
		return unavailable("ping", err)
	}
	return nil
}

func (e *EtcdKV) Close() error { return e.cli.Close() }
