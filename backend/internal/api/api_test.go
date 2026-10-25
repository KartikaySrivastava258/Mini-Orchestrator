package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/events"
	"mini-orchestrator/internal/httpjson"
	"mini-orchestrator/internal/store"
)

var t0 = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

type testServer struct {
	kv      *store.MemoryKV
	st      *store.Store
	handler http.Handler
	kicks   int
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	kv := store.NewMemoryKV()
	ts := &testServer{kv: kv, st: store.New(kv, store.DefaultPrefix, nil)}
	srv := New(ts.st, events.NewRecorder(100, nil), slog.New(slog.DiscardHandler), Options{
		Version:        "test",
		AllowedOrigins: []string{"http://localhost:5173"},
		MaxBodyBytes:   512,
		OnPodsChanged:  func() { ts.kicks++ },
	})
	srv.SetClock(func() time.Time { return t0 })
	ts.handler = srv.Handler()
	return ts
}

func (ts *testServer) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env httpjson.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not the JSON error envelope: %q", rec.Body.String())
	}
	if env.RequestID == "" {
		t.Fatal("error responses must include a request ID")
	}
	return env.Error.Code
}

func TestCreatePodValidationAndStatusCodes(t *testing.T) {
	ts := newTestServer(t)
	cases := []struct {
		name, body string
		status     int
		code       string
	}{
		{"valid", `{"name":"web","image":"nginx:stable"}`, 201, ""},
		{"duplicate", `{"name":"web","image":"nginx:stable"}`, 409, httpjson.CodeAlreadyExists},
		{"missing image", `{"name":"api"}`, 400, httpjson.CodeValidation},
		{"bad name", `{"name":"Web_1","image":"nginx"}`, 400, httpjson.CodeValidation},
		{"flag-like image", `{"name":"x","image":"--privileged"}`, 400, httpjson.CodeValidation},
		{"bad restart policy", `{"name":"x","image":"nginx","restartPolicy":"OnFailure"}`, 400, httpjson.CodeValidation},
		{"unknown field", `{"name":"x","image":"nginx","nodeName":"w1"}`, 400, httpjson.CodeInvalidJSON},
		{"wrong type", `{"name":5,"image":"nginx"}`, 400, httpjson.CodeInvalidJSON},
		{"malformed", `{"name":`, 400, httpjson.CodeInvalidJSON},
		{"trailing data", `{"name":"y","image":"nginx"}{}`, 400, httpjson.CodeInvalidJSON},
		{"too large", `{"name":"x","image":"` + strings.Repeat("a", 600) + `"}`, 413, httpjson.CodeTooLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := ts.do(t, "POST", "/api/v1/pods", c.body)
			if rec.Code != c.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, c.status, rec.Body.String())
			}
			if c.code != "" && errorCode(t, rec) != c.code {
				t.Fatalf("code %s, want %s", errorCode(t, rec), c.code)
			}
		})
	}
	if ts.kicks != 1 {
		t.Fatalf("scheduler should be kicked once (for the successful create), got %d", ts.kicks)
	}
}

func TestCreatedPodShape(t *testing.T) {
	ts := newTestServer(t)
	rec := ts.do(t, "POST", "/api/v1/pods", `{"name":"web","image":"nginx"}`)
	var p domain.Pod
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.UID == "" || p.Status.Phase != domain.PodPending || p.Spec.RestartPolicy != domain.RestartAlways || !p.CreatedAt.Equal(t0) {
		t.Fatalf("unexpected pod: %+v", p)
	}
	stored, err := ts.st.GetPod(t.Context(), "web")
	if err != nil || stored.UID != p.UID {
		t.Fatalf("pod not persisted: %v", err)
	}
}

func TestListFiltersAndEmptyList(t *testing.T) {
	ts := newTestServer(t)
	rec := ts.do(t, "GET", "/api/v1/pods", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("empty list must serialise as []: %s", rec.Body.String())
	}
	_ = ts.st.CreatePod(t.Context(), domain.Pod{Name: "a", Status: domain.PodStatus{Phase: domain.PodRunning, NodeName: "w1"}})
	_ = ts.st.CreatePod(t.Context(), domain.Pod{Name: "b", Status: domain.PodStatus{Phase: domain.PodPending}})
	var list domain.List[domain.Pod]
	_ = json.Unmarshal(ts.do(t, "GET", "/api/v1/pods?node=w1", "").Body.Bytes(), &list)
	if list.Count != 1 || list.Items[0].Name != "a" {
		t.Fatalf("node filter: %+v", list)
	}
	if rec := ts.do(t, "GET", "/api/v1/pods?phase=Bogus", ""); rec.Code != 400 {
		t.Fatalf("invalid phase filter: %d", rec.Code)
	}
}

func TestGetAndDeletePod(t *testing.T) {
	ts := newTestServer(t)
	if rec := ts.do(t, "GET", "/api/v1/pods/missing", ""); rec.Code != 404 || errorCode(t, rec) != httpjson.CodeNotFound {
		t.Fatalf("missing pod: %d", rec.Code)
	}
	ts.do(t, "POST", "/api/v1/pods", `{"name":"web","image":"nginx"}`)
	if rec := ts.do(t, "DELETE", "/api/v1/pods/web", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"Deleted"`) {
		t.Fatalf("unscheduled delete: %d %s", rec.Code, rec.Body.String())
	}

	_ = ts.st.CreateNode(t.Context(), domain.Node{Name: "w1"})
	_ = ts.st.CreatePod(t.Context(), domain.Pod{Name: "run", UID: "u1", Status: domain.PodStatus{Phase: domain.PodRunning, NodeName: "w1"}})
	if rec := ts.do(t, "DELETE", "/api/v1/pods/run", ""); rec.Code != 202 || !strings.Contains(rec.Body.String(), `"Terminating"`) {
		t.Fatalf("scheduled delete: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAgentStatusAndFinalizeFlow(t *testing.T) {
	ts := newTestServer(t)
	_ = ts.st.CreateNode(t.Context(), domain.Node{Name: "w1"})
	_ = ts.st.CreatePod(t.Context(), domain.Pod{Name: "web", UID: "u1", Status: domain.PodStatus{Phase: domain.PodScheduled, NodeName: "w1"}})
	cid := strings.Repeat("a", 64)

	ok := fmt.Sprintf(`{"uid":"u1","nodeName":"w1","phase":"Running","reason":"ContainerStarted","containerID":"%s","restartCount":0}`, cid)
	if rec := ts.do(t, "PUT", "/api/v1/pods/web/status", ok); rec.Code != 200 {
		t.Fatalf("status update: %d %s", rec.Code, rec.Body.String())
	}
	if rec := ts.do(t, "PUT", "/api/v1/pods/web/status", `{"uid":"u1","nodeName":"w2","phase":"Running","restartCount":0}`); rec.Code != 409 {
		t.Fatalf("wrong node must be rejected: %d", rec.Code)
	}
	if rec := ts.do(t, "PUT", "/api/v1/pods/web/status", `{"uid":"u1","nodeName":"w1","phase":"Pending","restartCount":0}`); rec.Code != 400 {
		t.Fatalf("agents may not set Pending: %d", rec.Code)
	}
	if rec := ts.do(t, "POST", "/api/v1/pods/web/finalize", `{"uid":"u1","nodeName":"w1"}`); rec.Code != 409 {
		t.Fatalf("finalize of a non-terminating pod must be rejected: %d", rec.Code)
	}
	ts.do(t, "DELETE", "/api/v1/pods/web", "")
	if rec := ts.do(t, "POST", "/api/v1/pods/web/finalize", `{"uid":"u1","nodeName":"w1"}`); rec.Code != 200 {
		t.Fatalf("finalize: %d %s", rec.Code, rec.Body.String())
	}
	if rec := ts.do(t, "POST", "/api/v1/pods/web/finalize", `{"uid":"u1","nodeName":"w1"}`); rec.Code != 200 {
		t.Fatalf("finalize must be idempotent: %d", rec.Code)
	}
}

func TestNodeRegistrationHeartbeatCordonDelete(t *testing.T) {
	ts := newTestServer(t)
	reg := `{"name":"w1","address":"http://127.0.0.1:9101","maxPods":5,"runtimeReady":true}`
	if rec := ts.do(t, "POST", "/api/v1/nodes", reg); rec.Code != 201 {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	if rec := ts.do(t, "POST", "/api/v1/nodes", reg); rec.Code != 200 {
		t.Fatalf("re-register must be an idempotent update: %d", rec.Code)
	}
	if rec := ts.do(t, "POST", "/api/v1/nodes", `{"name":"w2","address":"file:///etc/passwd","maxPods":5}`); rec.Code != 400 {
		t.Fatalf("bad address accepted: %d", rec.Code)
	}
	if rec := ts.do(t, "POST", "/api/v1/nodes/ghost/heartbeat", `{"runtimeReady":true}`); rec.Code != 404 {
		t.Fatalf("heartbeat for unknown node: %d", rec.Code)
	}
	rec := ts.do(t, "POST", "/api/v1/nodes/w1/heartbeat", `{"runtimeReady":false,"runtimeMessage":"docker down"}`)
	var n domain.Node
	_ = json.Unmarshal(rec.Body.Bytes(), &n)
	if n.Status.Ready || n.Status.Reason != domain.ReasonRuntimeUnavailable {
		t.Fatalf("runtime outage not reflected: %+v", n.Status)
	}
	if rec := ts.do(t, "PATCH", "/api/v1/nodes/w1", `{"unschedulable":true}`); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"unschedulable":true`) {
		t.Fatalf("cordon: %d %s", rec.Code, rec.Body.String())
	}
	if rec := ts.do(t, "PATCH", "/api/v1/nodes/w1", `{}`); rec.Code != 400 {
		t.Fatalf("empty patch: %d", rec.Code)
	}

	_ = ts.st.CreatePod(t.Context(), domain.Pod{Name: "p", UID: "u", Status: domain.PodStatus{Phase: domain.PodRunning, NodeName: "w1"}})
	if rec := ts.do(t, "DELETE", "/api/v1/nodes/w1", ""); rec.Code != 409 {
		t.Fatalf("deleting a node with pods must be refused: %d", rec.Code)
	}
	_ = ts.st.DeletePod(t.Context(), "p", nil)
	if rec := ts.do(t, "DELETE", "/api/v1/nodes/w1", ""); rec.Code != 200 {
		t.Fatalf("delete empty node: %d", rec.Code)
	}
}

func TestReplicaSetEndpoints(t *testing.T) {
	ts := newTestServer(t)
	if rec := ts.do(t, "POST", "/api/v1/replicasets", `{"name":"web","image":"nginx"}`); rec.Code != 400 {
		t.Fatalf("replicas is required: %d", rec.Code)
	}
	if rec := ts.do(t, "POST", "/api/v1/replicasets", `{"name":"web","image":"nginx","replicas":2}`); rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if rec := ts.do(t, "PATCH", "/api/v1/replicasets/web", `{"replicas":4}`); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"replicas":4`) {
		t.Fatalf("scale: %d %s", rec.Code, rec.Body.String())
	}
	if rec := ts.do(t, "PATCH", "/api/v1/replicasets/web", `{"replicas":-1}`); rec.Code != 400 {
		t.Fatalf("negative replicas: %d", rec.Code)
	}
	if rec := ts.do(t, "DELETE", "/api/v1/replicasets/web", ""); rec.Code != 200 {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := ts.do(t, "GET", "/api/v1/replicasets/web", ""); rec.Code != 404 {
		t.Fatalf("get after delete: %d", rec.Code)
	}
}

func TestHealthReadinessAndDependencyErrors(t *testing.T) {
	ts := newTestServer(t)
	if rec := ts.do(t, "GET", "/readyz", ""); rec.Code != 200 {
		t.Fatalf("readyz: %d", rec.Code)
	}
	ts.kv.SetFailure(fmt.Errorf("%w: etcd down at 10.0.0.5:2379", store.ErrUnavailable))
	if rec := ts.do(t, "GET", "/healthz", ""); rec.Code != 200 {
		t.Fatal("liveness must not depend on etcd")
	}
	if rec := ts.do(t, "GET", "/readyz", ""); rec.Code != 503 {
		t.Fatalf("readyz with etcd down: %d", rec.Code)
	}
	rec := ts.do(t, "GET", "/api/v1/pods", "")
	if rec.Code != 503 || errorCode(t, rec) != httpjson.CodeStoreUnavailable {
		t.Fatalf("list with etcd down: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Fatal("internal error details leaked to the client")
	}
}

func TestCORSAndUnknownRoutes(t *testing.T) {
	ts := newTestServer(t)
	pre := httptest.NewRequest("OPTIONS", "/api/v1/pods", nil)
	pre.Header.Set("Origin", "http://localhost:5173")
	pre.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, pre)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("allowed preflight: %d %v", rec.Code, rec.Header())
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("credentials must never be allowed")
	}

	evil := httptest.NewRequest("OPTIONS", "/api/v1/pods", nil)
	evil.Header.Set("Origin", "https://evil.example")
	evil.Header.Set("Access-Control-Request-Method", "POST")
	rec = httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, evil)
	if rec.Code != 403 || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("disallowed preflight: %d", rec.Code)
	}

	if rec := ts.do(t, "GET", "/getpods", ""); rec.Code != 404 || errorCode(t, rec) != httpjson.CodeNotFound {
		t.Fatalf("legacy route should be gone: %d", rec.Code)
	}
}

func TestRequestIDIsPropagated(t *testing.T) {
	ts := newTestServer(t)
	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	if rec.Header().Get("X-Request-ID") != "abc-123" {
		t.Fatal("request ID not echoed")
	}
	req.Header.Set("X-Request-ID", "bad id with spaces")
	rec = httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-ID"); got == "" || strings.Contains(got, " ") {
		t.Fatalf("invalid request ID should be replaced, got %q", got)
	}
}

func TestLogsRequireAStartedContainer(t *testing.T) {
	ts := newTestServer(t)
	_ = ts.st.CreatePod(t.Context(), domain.Pod{Name: "web", Status: domain.PodStatus{Phase: domain.PodPending}})
	if rec := ts.do(t, "GET", "/api/v1/pods/web/logs", ""); rec.Code != 409 {
		t.Fatalf("logs before start: %d", rec.Code)
	}
	if rec := ts.do(t, "GET", "/api/v1/pods/web/logs?tailLines=0", ""); rec.Code != 400 {
		t.Fatalf("bad tailLines: %d", rec.Code)
	}
}

func TestLogsAreProxiedFromTheAgent(t *testing.T) {
	cid := strings.Repeat("b", 64)
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/containers/"+cid+"/logs" || r.URL.Query().Get("tailLines") != "50" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(domain.LogsResponse{ContainerID: cid, TailLines: 50, Logs: "<b>hello</b>", Mode: "snapshot"})
	}))
	defer agent.Close()

	ts := newTestServer(t)
	n := domain.Node{Name: "w1"}
	n.Status.Address = agent.URL
	_ = ts.st.CreateNode(t.Context(), n)
	_ = ts.st.CreatePod(t.Context(), domain.Pod{Name: "web", Status: domain.PodStatus{Phase: domain.PodRunning, NodeName: "w1", ContainerID: cid}})
	rec := ts.do(t, "GET", "/api/v1/pods/web/logs?tailLines=50", "")
	var lr domain.LogsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &lr)
	if rec.Code != 200 || lr.Logs != "<b>hello</b>" || lr.PodName != "web" || lr.NodeName != "w1" {
		t.Fatalf("proxy: %d %s", rec.Code, rec.Body.String())
	}

	agent.Close()
	if rec := ts.do(t, "GET", "/api/v1/pods/web/logs", ""); rec.Code != 503 || errorCode(t, rec) != httpjson.CodeAgentUnreachable {
		t.Fatalf("unreachable agent: %d %s", rec.Code, rec.Body.String())
	}
}
