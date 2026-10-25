package api

import (
	"fmt"
	"net/http"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/events"
	"mini-orchestrator/internal/lifecycle"
	"mini-orchestrator/internal/store"
)

// GET /api/v1/replicasets
func (s *Server) listReplicaSets(w http.ResponseWriter, r *http.Request) {
	sets, err := s.store.ListReplicaSets(r.Context())
	if err != nil {
		s.writeStoreError(w, r, err, "replicasets")
		return
	}
	s.writeJSON(w, http.StatusOK, domain.NewList(sets))
}

// POST /api/v1/replicasets
func (s *Server) createReplicaSet(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateReplicaSetRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	req.Normalize()
	if err := req.Validate(); err != nil {
		s.writeValidationError(w, r, err)
		return
	}
	rs := domain.ReplicaSet{
		Name:      req.Name,
		UID:       domain.NewUID(),
		CreatedAt: s.now(),
		Spec:      domain.ReplicaSetSpec{Replicas: *req.Replicas, Template: domain.PodTemplate{Image: req.Image}},
	}
	if err := s.store.CreateReplicaSet(r.Context(), rs); err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("replicaset %q", req.Name))
		return
	}
	s.events.Record("api", events.TypeNormal, domain.KindReplicaSet, rs.Name, "Created",
		fmt.Sprintf("desired replicas %d, image %s", rs.Spec.Replicas, rs.Spec.Template.Image))
	s.opts.OnReplicaSetsChanged()
	s.writeJSON(w, http.StatusCreated, rs)
}

// GET /api/v1/replicasets/{name}
func (s *Server) getReplicaSet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	rs, err := s.store.GetReplicaSet(r.Context(), name)
	if err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("replicaset %q", name))
		return
	}
	s.writeJSON(w, http.StatusOK, rs)
}

// PATCH /api/v1/replicasets/{name} — scale
func (s *Server) patchReplicaSet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req domain.PatchReplicaSetRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		s.writeValidationError(w, r, err)
		return
	}
	want := *req.Replicas
	from := 0
	changed := false
	rs, err := s.store.UpdateReplicaSet(r.Context(), name, func(cur *domain.ReplicaSet) error {
		changed = false
		from = cur.Spec.Replicas
		if cur.Spec.Replicas == want {
			return store.ErrNoChange
		}
		cur.Spec.Replicas = want
		changed = true
		return nil
	})
	if err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("replicaset %q", name))
		return
	}
	if changed {
		s.events.Record("api", events.TypeNormal, domain.KindReplicaSet, name, "Scaled", fmt.Sprintf("desired replicas %d -> %d", from, want))
		s.opts.OnReplicaSetsChanged()
	}
	s.writeJSON(w, http.StatusOK, rs)
}

// DELETE /api/v1/replicasets/{name}
//
// Deletes the ReplicaSet record. The controller notices on its next pass and
// takes the owned pods down with it.
func (s *Server) deleteReplicaSet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.store.DeleteReplicaSet(r.Context(), name, nil); err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("replicaset %q", name))
		return
	}
	s.events.Record("api", events.TypeNormal, domain.KindReplicaSet, name, "Deleted", "owned pods will be terminated by the controller")
	s.opts.OnReplicaSetsChanged()
	s.writeJSON(w, http.StatusOK, domain.DeleteResponse{
		Name:    name,
		Result:  string(lifecycle.Deleted),
		Message: "owned pods will be terminated by the ReplicaSet controller",
	})
}
