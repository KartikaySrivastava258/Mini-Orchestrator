package agent

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/httpjson"
	"mini-orchestrator/internal/runtime"
)

// LogHandler serves container logs to the control plane:
//
//	GET /healthz
//	GET /v1/containers/{id}/logs?tailLines=N
//
// Only containers labelled for this node get served. There's no auth, so only
// the control plane should be able to reach this port.
func (a *Agent) LogHandler(maxTailLines, maxBytes int) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpjson.Write(w, a.log, http.StatusOK, map[string]string{"status": "ok", "node": a.cfg.NodeName})
	})
	mux.HandleFunc("GET /v1/containers/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !domain.ValidContainerID(id) {
			httpjson.WriteError(w, a.log, http.StatusBadRequest, "", httpjson.CodeValidation, "invalid container ID", "id")
			return
		}
		tail := 200
		if raw := r.URL.Query().Get("tailLines"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > maxTailLines {
				httpjson.WriteError(w, a.log, http.StatusBadRequest, "", httpjson.CodeValidation,
					"tailLines must be between 1 and "+strconv.Itoa(maxTailLines), "tailLines")
				return
			}
			tail = n
		}
		st, err := a.rt.Inspect(r.Context(), id)
		if errors.Is(err, runtime.ErrNotFound) || (err == nil && st.Labels[runtime.LabelNode] != a.cfg.NodeName) {
			httpjson.WriteError(w, a.log, http.StatusNotFound, "", httpjson.CodeNotFound, "container not found on this node", "")
			return
		}
		if err != nil {
			a.log.Warn("inspect for logs failed", "container", id, "error", err)
			httpjson.WriteError(w, a.log, http.StatusServiceUnavailable, "", httpjson.CodeRuntimeError, "container runtime error", "")
			return
		}
		res, err := a.rt.Logs(r.Context(), id, tail, maxBytes)
		if err != nil {
			a.log.Warn("reading logs failed", "container", id, "error", err)
			httpjson.WriteError(w, a.log, http.StatusServiceUnavailable, "", httpjson.CodeRuntimeError, "could not read container logs", "")
			return
		}
		httpjson.Write(w, a.log, http.StatusOK, domain.LogsResponse{
			ContainerID: id,
			TailLines:   tail,
			Truncated:   res.Truncated,
			Logs:        res.Text,
			FetchedAt:   time.Now().UTC(),
			Mode:        "snapshot",
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpjson.WriteError(w, a.log, http.StatusNotFound, "", httpjson.CodeNotFound, "no route for "+r.Method+" "+r.URL.Path, "")
	})
	return mux
}
