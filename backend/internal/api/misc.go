package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/events"
	"mini-orchestrator/internal/httpjson"
)

// GET /healthz: is the process alive? It never checks etcd; that is /readyz's job.
func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ReadinessResponse is returned by GET /readyz.
type ReadinessResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// GET /readyz: 200 only if etcd answers within 2 seconds.
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		s.log.Warn("readiness check failed", "check", "etcd", "error", err)
		s.writeJSON(w, http.StatusServiceUnavailable, ReadinessResponse{Status: "not ready", Checks: map[string]string{"etcd": "unavailable"}})
		return
	}
	s.writeJSON(w, http.StatusOK, ReadinessResponse{Status: "ready", Checks: map[string]string{"etcd": "ok"}})
}

// InfoResponse describes the running control plane's configuration.
type InfoResponse struct {
	Name                        string `json:"name"`
	Version                     string `json:"version"`
	APIVersion                  string `json:"apiVersion"`
	SchedulerPolicy             string `json:"schedulerPolicy"`
	NodeHeartbeatTimeoutSeconds int    `json:"nodeHeartbeatTimeoutSeconds"`
	PodEvictionTimeoutSeconds   int    `json:"podEvictionTimeoutSeconds"`
	EventsRetained              int    `json:"eventsRetained"`
	EventsPersistent            bool   `json:"eventsPersistent"`
	Authentication              string `json:"authentication"`
}

// GET /api/v1/info
func (s *Server) info(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, InfoResponse{
		Name:                        "mini-orchestrator",
		Version:                     s.opts.Version,
		APIVersion:                  "v1",
		SchedulerPolicy:             s.opts.SchedulerPolicy,
		NodeHeartbeatTimeoutSeconds: int(s.opts.HeartbeatTimeout / time.Second),
		PodEvictionTimeoutSeconds:   int(s.opts.EvictionTimeout / time.Second),
		EventsRetained:              s.events.Capacity(),
		EventsPersistent:            false,
		Authentication:              "none",
	})
}

// GET /api/v1/events?kind=&name=&type=&limit=
func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 200
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 1000 {
			s.writeError(w, r, http.StatusBadRequest, httpjson.CodeValidation, "limit must be an integer between 1 and 1000", "limit")
			return
		}
		limit = n
	}
	typ := q.Get("type")
	if typ != "" && typ != events.TypeNormal && typ != events.TypeWarning {
		s.writeError(w, r, http.StatusBadRequest, httpjson.CodeValidation, "type must be Normal or Warning", "type")
		return
	}
	list := s.events.List(events.Filter{Kind: q.Get("kind"), Name: q.Get("name"), Type: typ, Limit: limit})
	s.writeJSON(w, http.StatusOK, domain.NewList(list))
}
