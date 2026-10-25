package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/events"
	"mini-orchestrator/internal/httpjson"
	"mini-orchestrator/internal/lifecycle"
	"mini-orchestrator/internal/store"
)

const (
	defaultTailLines = 200
	maxTailLines     = 2000
	// maxLogResponseBytes caps what we read from an agent. Nobody wants a 2 GB log in their browser.
	maxLogResponseBytes = 1 << 20
)

// GET /api/v1/pods?node=&phase=&owner=
func (s *Server) listPods(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	node, owner := q.Get("node"), q.Get("owner")
	phase := domain.PodPhase(q.Get("phase"))
	if phase != "" && !phase.Valid() {
		s.writeError(w, r, http.StatusBadRequest, httpjson.CodeValidation, "phase must be one of the documented pod phases", "phase")
		return
	}
	pods, err := s.store.ListPods(r.Context())
	if err != nil {
		s.writeStoreError(w, r, err, "pods")
		return
	}
	out := make([]domain.Pod, 0, len(pods))
	for _, p := range pods {
		if node != "" && p.Status.NodeName != node {
			continue
		}
		if phase != "" && p.Status.Phase != phase {
			continue
		}
		if owner != "" && (p.Owner == nil || p.Owner.Name != owner) {
			continue
		}
		out = append(out, p)
	}
	s.writeJSON(w, http.StatusOK, domain.NewList(out))
}

// POST /api/v1/pods
func (s *Server) createPod(w http.ResponseWriter, r *http.Request) {
	var req domain.CreatePodRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	req.Normalize()
	if err := req.Validate(); err != nil {
		s.writeValidationError(w, r, err)
		return
	}
	now := s.now()
	pod := domain.Pod{
		Name:      req.Name,
		UID:       domain.NewUID(),
		CreatedAt: now,
		Spec:      domain.PodSpec{Image: req.Image, RestartPolicy: req.RestartPolicy},
		Status: domain.PodStatus{
			Phase:              domain.PodPending,
			Reason:             domain.ReasonAwaitingScheduling,
			Message:            "waiting for the scheduler",
			LastTransitionTime: now,
			UpdatedAt:          now,
		},
	}
	if err := s.store.CreatePod(r.Context(), pod); err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("pod %q", req.Name))
		return
	}
	s.events.Record("api", events.TypeNormal, "Pod", pod.Name, "Created", "pod created with image "+pod.Spec.Image)
	s.opts.OnPodsChanged()
	s.writeJSON(w, http.StatusCreated, pod)
}

// GET /api/v1/pods/{name}
func (s *Server) getPod(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	pod, err := s.store.GetPod(r.Context(), name)
	if err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("pod %q", name))
		return
	}
	s.writeJSON(w, http.StatusOK, pod)
}

// DELETE /api/v1/pods/{name}
func (s *Server) deletePod(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	res, err := lifecycle.RequestPodDeletion(r.Context(), s.store, name, s.now())
	if err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("pod %q", name))
		return
	}
	s.opts.OnPodsChanged()
	s.opts.OnReplicaSetsChanged()
	if res == lifecycle.Deleted {
		s.events.Record("api", events.TypeNormal, "Pod", name, "Deleted", "pod record deleted")
		s.writeJSON(w, http.StatusOK, domain.DeleteResponse{Name: name, Result: string(res)})
		return
	}
	s.events.Record("api", events.TypeNormal, "Pod", name, domain.ReasonDeletionRequested, "waiting for the node agent to remove the container")
	s.writeJSON(w, http.StatusAccepted, domain.DeleteResponse{
		Name:    name,
		Result:  string(res),
		Message: "the node agent will remove the container and then delete the record",
	})
}

// PUT /api/v1/pods/{name}/status (node agent only)
func (s *Server) updatePodStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var u domain.PodStatusUpdate
	if !s.decodeJSON(w, r, &u) {
		return
	}
	if err := u.Validate(); err != nil {
		s.writeValidationError(w, r, err)
		return
	}
	var before domain.PodPhase
	pod, err := s.store.UpdatePod(r.Context(), name, func(p *domain.Pod) error {
		before = p.Status.Phase
		return lifecycle.ApplyAgentStatus(p, u, s.now())
	})
	if err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("pod %q", name))
		return
	}
	if before != pod.Status.Phase || u.Reason == domain.ReasonContainerRestarted {
		eventType := events.TypeNormal
		if pod.Status.Phase == domain.PodFailed || u.Reason == domain.ReasonContainerRestarted {
			eventType = events.TypeWarning
		}
		msg := pod.Status.Message
		if msg == "" {
			msg = "phase changed to " + string(pod.Status.Phase)
		}
		s.events.Record("node/"+u.NodeName, eventType, "Pod", name, u.Reason, msg)
	}
	if pod.Status.Phase.IsTerminal() {
		s.opts.OnReplicaSetsChanged()
	}
	s.writeJSON(w, http.StatusOK, pod)
}

// POST /api/v1/pods/{name}/finalize (node agent only)
//
// The agent calls this once the container is gone, and we delete the record.
// Already deleted? Still a 200, so retries are harmless.
func (s *Server) finalizePod(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req domain.FinalizePodRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		s.writeValidationError(w, r, err)
		return
	}
	err := s.store.DeletePod(r.Context(), name, func(p domain.Pod) error {
		switch {
		case p.UID != req.UID:
			return fmt.Errorf("%w: pod UID does not match", lifecycle.ErrRejected)
		case p.Status.NodeName != req.NodeName:
			return fmt.Errorf("%w: pod is not assigned to node %q", lifecycle.ErrRejected, req.NodeName)
		case !p.IsTerminating():
			return fmt.Errorf("%w: pod is not terminating", lifecycle.ErrRejected)
		}
		return nil
	})
	if errors.Is(err, store.ErrNotFound) {
		s.writeJSON(w, http.StatusOK, domain.DeleteResponse{Name: name, Result: string(lifecycle.Deleted), Message: "already deleted"})
		return
	}
	if err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("pod %q", name))
		return
	}
	s.events.Record("node/"+req.NodeName, events.TypeNormal, "Pod", name, "Deleted", "container removed and pod record deleted")
	s.opts.OnReplicaSetsChanged()
	s.writeJSON(w, http.StatusOK, domain.DeleteResponse{Name: name, Result: string(lifecycle.Deleted)})
}

// GET /api/v1/pods/{name}/logs?tailLines=N
//
// Asks the node agent running the container for a log snapshot and passes it
// along.
func (s *Server) podLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tail := defaultTailLines
	if raw := r.URL.Query().Get("tailLines"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxTailLines {
			s.writeError(w, r, http.StatusBadRequest, httpjson.CodeValidation,
				fmt.Sprintf("tailLines must be an integer between 1 and %d", maxTailLines), "tailLines")
			return
		}
		tail = n
	}
	pod, err := s.store.GetPod(r.Context(), name)
	if err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("pod %q", name))
		return
	}
	if pod.Status.ContainerID == "" || pod.Status.NodeName == "" {
		s.writeError(w, r, http.StatusConflict, httpjson.CodeConflict,
			"pod has no container yet (phase "+string(pod.Status.Phase)+"); logs are available once the node agent starts it", "")
		return
	}
	node, err := s.store.GetNode(r.Context(), pod.Status.NodeName)
	if errors.Is(err, store.ErrNotFound) {
		s.writeError(w, r, http.StatusServiceUnavailable, httpjson.CodeAgentUnreachable,
			"node "+pod.Status.NodeName+" is no longer registered; its logs cannot be fetched", "")
		return
	}
	if err != nil {
		s.writeStoreError(w, r, err, fmt.Sprintf("node %q", pod.Status.NodeName))
		return
	}

	result, status, code, msg := s.fetchAgentLogs(r.Context(), node.Status.Address, pod.Status.ContainerID, tail)
	if status != http.StatusOK {
		s.log.Warn("log fetch failed", "request_id", requestIDFrom(r), "pod", name, "node", node.Name, "detail", msg)
		s.writeError(w, r, status, code, msg, "")
		return
	}
	result.PodName = pod.Name
	result.NodeName = node.Name
	s.writeJSON(w, http.StatusOK, result)
}

func (s *Server) fetchAgentLogs(ctx context.Context, address, containerID string, tail int) (domain.LogsResponse, int, string, string) {
	var empty domain.LogsResponse
	if domain.ValidateAgentAddress(address) != nil || !domain.ValidContainerID(containerID) {
		return empty, http.StatusBadGateway, httpjson.CodeAgentError, "node has an invalid agent address or container ID"
	}
	target := strings.TrimRight(address, "/") + "/v1/containers/" + url.PathEscape(containerID) +
		"/logs?tailLines=" + strconv.Itoa(tail)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return empty, http.StatusBadGateway, httpjson.CodeAgentError, "could not build agent request"
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return empty, http.StatusServiceUnavailable, httpjson.CodeAgentUnreachable,
			"the node agent did not respond; is it running and reachable at its advertised address?"
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLogResponseBytes+1))
	if err != nil {
		return empty, http.StatusBadGateway, httpjson.CodeAgentError, "failed to read the agent response"
	}
	if len(body) > maxLogResponseBytes {
		return empty, http.StatusBadGateway, httpjson.CodeAgentError, "the agent response exceeded the size limit"
	}
	if resp.StatusCode != http.StatusOK {
		var env httpjson.Envelope
		msg := fmt.Sprintf("the node agent returned HTTP %d", resp.StatusCode)
		if json.Unmarshal(body, &env) == nil && env.Error.Message != "" {
			msg += ": " + domain.Truncate(env.Error.Message, 300)
		}
		return empty, http.StatusBadGateway, httpjson.CodeAgentError, msg
	}
	var out domain.LogsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return empty, http.StatusBadGateway, httpjson.CodeAgentError, "the node agent returned malformed JSON"
	}
	return out, http.StatusOK, "", ""
}
