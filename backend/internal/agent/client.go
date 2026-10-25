package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"mini-orchestrator/internal/domain"
	"mini-orchestrator/internal/httpjson"
)

// Sentinel errors matched by APIError.Is.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// APIError is a non-2xx answer from the control plane.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("control plane returned HTTP %d %s: %s", e.Status, e.Code, e.Message)
}

// Is lets callers write errors.Is(err, ErrNotFound) and friends.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrNotFound:
		return e.Status == http.StatusNotFound
	case ErrConflict:
		return e.Status == http.StatusConflict
	}
	return false
}

// ControlPlane is the slice of the API the agent actually uses. Client does it
// over HTTP; the tests swap in a fake.
type ControlPlane interface {
	Register(ctx context.Context, req domain.RegisterNodeRequest) error
	Heartbeat(ctx context.Context, node string, req domain.HeartbeatRequest) error
	ListPods(ctx context.Context, node string) ([]domain.Pod, error)
	UpdatePodStatus(ctx context.Context, pod string, u domain.PodStatusUpdate) error
	FinalizePod(ctx context.Context, pod string, req domain.FinalizePodRequest) error
}

// Client talks to the control plane's REST API.
type Client struct {
	base string
	http *http.Client
}

// NewClient returns a Client with a per-request timeout.
func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{base: baseURL, http: &http.Client{Timeout: timeout}}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("%s %s: read response: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &APIError{Status: resp.StatusCode}
		var env httpjson.Envelope
		if json.Unmarshal(data, &env) == nil {
			apiErr.Code, apiErr.Message = env.Error.Code, env.Error.Message
		}
		return apiErr
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("%s %s: decode response: %w", method, path, err)
		}
	}
	return nil
}

func (c *Client) Register(ctx context.Context, req domain.RegisterNodeRequest) error {
	return c.do(ctx, http.MethodPost, "/api/v1/nodes", req, nil)
}

func (c *Client) Heartbeat(ctx context.Context, node string, req domain.HeartbeatRequest) error {
	return c.do(ctx, http.MethodPost, "/api/v1/nodes/"+url.PathEscape(node)+"/heartbeat", req, nil)
}

func (c *Client) ListPods(ctx context.Context, node string) ([]domain.Pod, error) {
	var list domain.List[domain.Pod]
	if err := c.do(ctx, http.MethodGet, "/api/v1/pods?node="+url.QueryEscape(node), nil, &list); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) UpdatePodStatus(ctx context.Context, pod string, u domain.PodStatusUpdate) error {
	return c.do(ctx, http.MethodPut, "/api/v1/pods/"+url.PathEscape(pod)+"/status", u, nil)
}

func (c *Client) FinalizePod(ctx context.Context, pod string, req domain.FinalizePodRequest) error {
	return c.do(ctx, http.MethodPost, "/api/v1/pods/"+url.PathEscape(pod)+"/finalize", req, nil)
}

var _ ControlPlane = (*Client)(nil)
