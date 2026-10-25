// Package events is the cluster's short-term memory: scheduling decisions,
// restarts, nodes ghosting us, that kind of thing.
//
// Events live in RAM only and vanish on restart. That's on purpose: etcd is
// for state, not gossip. See docs/DECISIONS.md.
package events

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Event types.
const (
	TypeNormal  = "Normal"
	TypeWarning = "Warning"
)

// ObjectRef says which object an event is about.
type ObjectRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// Event is one thing that happened.
type Event struct {
	ID      int64     `json:"id"`
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`
	Reason  string    `json:"reason"`
	Object  ObjectRef `json:"object"`
	Message string    `json:"message"`
	Source  string    `json:"source"`
}

// Filter narrows down List. Zero values mean "whatever".
type Filter struct {
	Kind  string
	Name  string
	Type  string
	Limit int
}

// Recorder is a fixed-size ring buffer of events, safe for concurrent use. Oldest events get evicted first.
type Recorder struct {
	mu       sync.Mutex
	buf      []Event
	capacity int
	nextID   int64
	now      func() time.Time
	log      *slog.Logger
}

// NewRecorder keeps at most capacity events.
func NewRecorder(capacity int, log *slog.Logger) *Recorder {
	if capacity < 1 {
		capacity = 1
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Recorder{capacity: capacity, now: func() time.Time { return time.Now().UTC() }, log: log}
}

// Capacity returns the maximum number of retained events.
func (r *Recorder) Capacity() int { return r.capacity }

// Record saves an event and also logs it, so even after a restart you can
// still dig it out of the logs.
func (r *Recorder) Record(source, eventType, kind, name, reason, message string) {
	r.mu.Lock()
	r.nextID++
	e := Event{
		ID:      r.nextID,
		Time:    r.now(),
		Type:    eventType,
		Reason:  reason,
		Object:  ObjectRef{Kind: kind, Name: name},
		Message: message,
		Source:  source,
	}
	if len(r.buf) < r.capacity {
		r.buf = append(r.buf, e)
	} else {
		copy(r.buf, r.buf[1:])
		r.buf[len(r.buf)-1] = e
	}
	r.mu.Unlock()

	level := slog.LevelInfo
	if eventType == TypeWarning {
		level = slog.LevelWarn
	}
	r.log.Log(context.Background(), level, "event",
		"source", source, "kind", kind, "name", name, "reason", reason, "message", message)
}

// List returns matching events, newest first.
func (r *Recorder) List(f Filter) []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Event, 0)
	for i := len(r.buf) - 1; i >= 0; i-- {
		e := r.buf[i]
		if f.Kind != "" && e.Object.Kind != f.Kind {
			continue
		}
		if f.Name != "" && e.Object.Name != f.Name {
			continue
		}
		if f.Type != "" && e.Type != f.Type {
			continue
		}
		out = append(out, e)
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	return out
}
