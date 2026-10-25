// Package api is the control plane's HTTP/JSON API. The resource names are
// Kubernetes-inspired, but it's its own small REST API; kubectl won't talk to
// it.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/events"
	"mini-orchestrator/internal/httpjson"
	"mini-orchestrator/internal/lifecycle"
	"mini-orchestrator/internal/store"
)

// Options configures the server.
type Options struct {
	Version             string
	AllowedOrigins      []string
	MaxBodyBytes        int64
	AgentRequestTimeout time.Duration
	HeartbeatTimeout    time.Duration
	EvictionTimeout     time.Duration
	SchedulerPolicy     string
	// OnPodsChanged and OnReplicaSetsChanged wake the scheduler and the
	// ReplicaSet controller early. They must not block. Either may be nil.
	OnPodsChanged        func()
	OnReplicaSetsChanged func()
}

// Server holds everything the handlers need.
type Server struct {
	store   *store.Store
	events  *events.Recorder
	log     *slog.Logger
	opts    Options
	origins map[string]bool
	client  *http.Client
	now     func() time.Time
}

// New returns a Server.
func New(st *store.Store, rec *events.Recorder, log *slog.Logger, opts Options) *Server {
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = 1 << 20
	}
	if opts.AgentRequestTimeout <= 0 {
		opts.AgentRequestTimeout = 5 * time.Second
	}
	if opts.OnPodsChanged == nil {
		opts.OnPodsChanged = func() {}
	}
	if opts.OnReplicaSetsChanged == nil {
		opts.OnReplicaSetsChanged = func() {}
	}
	origins := make(map[string]bool, len(opts.AllowedOrigins))
	for _, o := range opts.AllowedOrigins {
		origins[strings.TrimRight(o, "/")] = true
	}
	return &Server{
		store: st, events: rec, log: log, opts: opts, origins: origins,
		client: &http.Client{
			Timeout: opts.AgentRequestTimeout,
			// agents don't get to redirect us anywhere
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		now: func() time.Time { return time.Now().UTC() },
	}
}

// SetClock swaps the time source for tests.
func (s *Server) SetClock(now func() time.Time) { s.now = now }

// Handler returns the router wrapped in middleware: request ID, access log,
// panic recovery, CORS, and then the actual routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /api/v1/info", s.info)

	mux.HandleFunc("GET /api/v1/pods", s.listPods)
	mux.HandleFunc("POST /api/v1/pods", s.createPod)
	mux.HandleFunc("GET /api/v1/pods/{name}", s.getPod)
	mux.HandleFunc("DELETE /api/v1/pods/{name}", s.deletePod)
	mux.HandleFunc("GET /api/v1/pods/{name}/logs", s.podLogs)
	mux.HandleFunc("PUT /api/v1/pods/{name}/status", s.updatePodStatus)
	mux.HandleFunc("POST /api/v1/pods/{name}/finalize", s.finalizePod)

	mux.HandleFunc("GET /api/v1/replicasets", s.listReplicaSets)
	mux.HandleFunc("POST /api/v1/replicasets", s.createReplicaSet)
	mux.HandleFunc("GET /api/v1/replicasets/{name}", s.getReplicaSet)
	mux.HandleFunc("PATCH /api/v1/replicasets/{name}", s.patchReplicaSet)
	mux.HandleFunc("DELETE /api/v1/replicasets/{name}", s.deleteReplicaSet)

	mux.HandleFunc("GET /api/v1/nodes", s.listNodes)
	mux.HandleFunc("POST /api/v1/nodes", s.registerNode)
	mux.HandleFunc("GET /api/v1/nodes/{name}", s.getNode)
	mux.HandleFunc("PATCH /api/v1/nodes/{name}", s.patchNode)
	mux.HandleFunc("DELETE /api/v1/nodes/{name}", s.deleteNode)
	mux.HandleFunc("POST /api/v1/nodes/{name}/heartbeat", s.heartbeat)

	mux.HandleFunc("GET /api/v1/events", s.listEvents)

	// anything else (wrong path or wrong method) gets a JSON 404, not a sad plain-text one
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.writeError(w, r, http.StatusNotFound, httpjson.CodeNotFound, fmt.Sprintf("no route for %s %s", r.Method, r.URL.Path), "")
	})

	var h http.Handler = mux
	h = s.cors(h)
	h = s.recoverPanics(h)
	h = s.accessLog(h)
	h = s.requestID(h)
	return h
}

// --- middleware ------------------------------------------------------------

type ctxKey int

const requestIDKey ctxKey = 1

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !requestIDPattern.MatchString(id) {
			var b [8]byte
			_, _ = rand.Read(b[:])
			id = hex.EncodeToString(b[:])
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

func requestIDFrom(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey).(string)
	return id
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		level := slog.LevelInfo
		switch {
		case rec.status >= 500:
			level = slog.LevelError
		case r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || strings.HasSuffix(r.URL.Path, "/heartbeat") ||
			(r.Method == http.MethodGet && rec.status < 400):
			// polling and heartbeats go to debug, or the info logs turn into a wall of GETs
			level = slog.LevelDebug
		}
		s.log.Log(r.Context(), level, "http request",
			"request_id", requestIDFrom(r), "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.log.Error("panic in handler", "request_id", requestIDFrom(r), "panic", fmt.Sprint(v))
				s.writeError(w, r, http.StatusInternalServerError, httpjson.CodeInternal, "internal server error", "")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// cors only lets in origins from the allow-list. No credentials, no wildcard,
// no exceptions.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := origin != "" && s.origins[origin]
		if allowed {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")
			h.Set("Access-Control-Expose-Headers", "X-Request-ID")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			if !allowed {
				s.writeError(w, r, http.StatusForbidden, httpjson.CodeForbiddenOrigin,
					"origin is not in ORCH_CORS_ALLOWED_ORIGINS", "")
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- responses and errors --------------------------------------------------

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	httpjson.Write(w, s.log, status, v)
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, code, message, field string) {
	httpjson.WriteError(w, s.log, status, requestIDFrom(r), code, message, field)
}

// writeStoreError maps store and lifecycle errors to HTTP. The real error
// goes to our logs; clients get a clean message.
func (s *Server) writeStoreError(w http.ResponseWriter, r *http.Request, err error, resource string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.writeError(w, r, http.StatusNotFound, httpjson.CodeNotFound, resource+" not found", "")
		return
	case errors.Is(err, store.ErrAlreadyExists):
		s.writeError(w, r, http.StatusConflict, httpjson.CodeAlreadyExists, resource+" already exists", "name")
		return
	case errors.Is(err, lifecycle.ErrRejected):
		s.writeError(w, r, http.StatusConflict, httpjson.CodeConflict, err.Error(), "")
		return
	case errors.Is(err, store.ErrConflict):
		s.writeError(w, r, http.StatusConflict, httpjson.CodeConflict, resource+" was modified concurrently; retry the request", "")
		return
	}
	s.log.Error("request failed", "request_id", requestIDFrom(r), "resource", resource, "error", err)
	switch {
	case errors.Is(err, store.ErrUnavailable):
		s.writeError(w, r, http.StatusServiceUnavailable, httpjson.CodeStoreUnavailable,
			"the state store (etcd) is unavailable; check GET /readyz", "")
	default:
		s.writeError(w, r, http.StatusInternalServerError, httpjson.CodeInternal, "internal server error", "")
	}
}

func (s *Server) writeValidationError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		s.writeError(w, r, http.StatusBadRequest, httpjson.CodeValidation, ve.Field+" "+ve.Message, ve.Field)
		return
	}
	s.writeError(w, r, http.StatusBadRequest, httpjson.CodeValidation, err.Error(), "")
}

// decodeJSON is strict on purpose: unknown fields, wrong types, trailing junk
// and oversized bodies all bounce.
func (s *Server) decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		s.writeError(w, r, http.StatusUnsupportedMediaType, httpjson.CodeUnsupportedMedia, "Content-Type must be application/json", "")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.opts.MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		s.writeDecodeError(w, r, err)
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			s.writeDecodeError(w, r, err)
			return false
		}
		s.writeError(w, r, http.StatusBadRequest, httpjson.CodeInvalidJSON, "request body must contain a single JSON object", "")
		return false
	}
	return true
}

func (s *Server) writeDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.As(err, &tooLarge):
		s.writeError(w, r, http.StatusRequestEntityTooLarge, httpjson.CodeTooLarge,
			fmt.Sprintf("request body must not exceed %d bytes", tooLarge.Limit), "")
	case errors.Is(err, io.EOF):
		s.writeError(w, r, http.StatusBadRequest, httpjson.CodeInvalidJSON, "request body is empty", "")
	case errors.As(err, &typeErr):
		s.writeError(w, r, http.StatusBadRequest, httpjson.CodeInvalidJSON,
			fmt.Sprintf("field %q has the wrong type (expected %s)", typeErr.Field, typeErr.Type), typeErr.Field)
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		s.writeError(w, r, http.StatusBadRequest, httpjson.CodeInvalidJSON, "request body is not valid JSON", "")
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		field := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
		s.writeError(w, r, http.StatusBadRequest, httpjson.CodeInvalidJSON, fmt.Sprintf("unknown field %q", field), field)
	default:
		s.writeError(w, r, http.StatusBadRequest, httpjson.CodeInvalidJSON, "request body could not be decoded", "")
	}
}
