package api

import (
	"errors"
	"fmt"
	"net/http"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/events"
	"mini-orchestrator/internal/httpjson"
	"mini-orchestrator/internal/lifecycle"
	"mini-orchestrator/internal/store"
)

// GET /api/v1/nodes
func (s *Server) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.store.ListNodes(r.Context())
	if err != nil {
		s.writeStoreError(w, r, err, "nodes")
		return
	}
	s.writeJSON(w, http.StatusOK, domain.NewList(nodes))
}

// GET /api/v1/nodes/{name}
func (s *Server) getNode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	n, err := s.store.GetNode(r.Context(), name)
	if err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("node %q", name))
		return
	}
	s.writeJSON(w, http.StatusOK, n)
}

// POST /api/v1/nodes (agent registration, safe to repeat)
//
// Heads up: there's no auth, so anyone who can reach the API can register a
// node. See docs/OPERATIONS.md, "Security boundaries".
func (s *Server) registerNode(w http.ResponseWriter, r *http.Request) {
	var req domain.RegisterNodeRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		s.writeValidationError(w, r, err)
		return
	}
	ctx := r.Context()
	now := s.now()
	for attempt := 0; attempt < 2; attempt++ {
		node, err := s.store.UpdateNode(ctx, req.Name, func(n *domain.Node) error {
			n.Status.Address = req.Address
			n.Status.MaxPods = req.MaxPods
			lifecycle.ApplyHeartbeat(n, req.RuntimeReady, req.RuntimeMessage, now)
			return nil
		})
		if err == nil {
			s.events.Record("api", events.TypeNormal, "Node", req.Name, "Reregistered", "node agent re-registered from "+req.Address)
			s.opts.OnPodsChanged()
			s.writeJSON(w, http.StatusOK, node)
			return
		}
		if !errors.Is(err, store.ErrNotFound) {
			s.writeStoreError(w, r, err, fmt.Sprintf("node %q", req.Name))
			return
		}

		node = domain.Node{Name: req.Name, UID: domain.NewUID(), RegisteredAt: now}
		node.Status.Address = req.Address
		node.Status.MaxPods = req.MaxPods
		lifecycle.ApplyHeartbeat(&node, req.RuntimeReady, req.RuntimeMessage, now)
		node.Status.LastTransitionTime = now
		err = s.store.CreateNode(ctx, node)
		if errors.Is(err, store.ErrAlreadyExists) {
			continue // raced with another registration; update instead
		}
		if err != nil {
			s.writeStoreError(w, r, err, fmt.Sprintf("node %q", req.Name))
			return
		}
		s.events.Record("api", events.TypeNormal, "Node", req.Name, "Registered", "node agent registered from "+req.Address)
		s.opts.OnPodsChanged()
		s.writeJSON(w, http.StatusCreated, node)
		return
	}
	s.writeError(w, r, http.StatusConflict, httpjson.CodeConflict, "node registration raced with another writer; retry", "")
}

// POST /api/v1/nodes/{name}/heartbeat (node agent only)
func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req domain.HeartbeatRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	becameReady, becameNotReady := false, false
	node, err := s.store.UpdateNode(r.Context(), name, func(n *domain.Node) error {
		wasReady := n.Status.Ready
		changed := lifecycle.ApplyHeartbeat(n, req.RuntimeReady, req.RuntimeMessage, s.now())
		becameReady = changed && !wasReady
		becameNotReady = changed && wasReady
		return nil
	})
	if err != nil {
		// a 404 here is the agent's cue to register again
		s.writeStoreError(w, r, err, fmt.Sprintf("node %q", name))
		return
	}
	switch {
	case becameReady:
		s.events.Record("node/"+name, events.TypeNormal, "Node", name, "NodeReady", node.Status.Message)
		s.opts.OnPodsChanged()
	case becameNotReady:
		s.events.Record("node/"+name, events.TypeWarning, "Node", name, node.Status.Reason, node.Status.Message)
	}
	s.writeJSON(w, http.StatusOK, node)
}

// PATCH /api/v1/nodes/{name} — cordon / uncordon
func (s *Server) patchNode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req domain.PatchNodeRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		s.writeValidationError(w, r, err)
		return
	}
	want := *req.Unschedulable
	changed := false
	node, err := s.store.UpdateNode(r.Context(), name, func(n *domain.Node) error {
		changed = false
		if n.Spec.Unschedulable == want {
			return store.ErrNoChange
		}
		n.Spec.Unschedulable = want
		changed = true
		return nil
	})
	if err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("node %q", name))
		return
	}
	if changed {
		reason, msg := "NodeUncordoned", "node accepts new pods again"
		if want {
			reason, msg = "NodeCordoned", "node will not receive new pods; existing pods keep running"
		}
		s.events.Record("api", events.TypeNormal, "Node", name, reason, msg)
		s.opts.OnPodsChanged()
	}
	s.writeJSON(w, http.StatusOK, node)
}

// DELETE /api/v1/nodes/{name}
//
// Refused while pods still live on the node, so removing a node never quietly
// orphans running containers. If the agent is still up, it simply registers
// again on its next heartbeat.
func (s *Server) deleteNode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()
	if _, err := s.store.GetNode(ctx, name); err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("node %q", name))
		return
	}
	pods, err := s.store.ListPods(ctx)
	if err != nil {
		s.writeStoreError(w, r, err, "pods")
		return
	}
	occupying := 0
	for _, p := range pods {
		if p.Status.NodeName == name && p.OccupiesNode() {
			occupying++
		}
	}
	if occupying > 0 {
		s.writeError(w, r, http.StatusConflict, httpjson.CodeConflict,
			fmt.Sprintf("node %s still has %d pod(s); cordon it and delete or wait for its pods first", name, occupying), "")
		return
	}
	if err := s.store.DeleteNode(ctx, name, nil); err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("node %q", name))
		return
	}
	s.events.Record("api", events.TypeNormal, "Node", name, "Deleted", "node record deleted")
	s.writeJSON(w, http.StatusOK, domain.DeleteResponse{
		Name:    name,
		Result:  string(lifecycle.Deleted),
		Message: "if the node agent is still running it will register again on its next heartbeat",
	})
}
