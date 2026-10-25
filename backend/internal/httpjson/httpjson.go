// Package httpjson writes JSON responses and our one-and-only error shape,
// shared by the API and the agent:
//
//	{"error":{"code":"VALIDATION_ERROR","message":"image is required","field":"image"},"requestId":"..."}
package httpjson

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Error codes.
const (
	CodeValidation       = "VALIDATION_ERROR"
	CodeInvalidJSON      = "INVALID_JSON"
	CodeUnsupportedMedia = "UNSUPPORTED_MEDIA_TYPE"
	CodeTooLarge         = "REQUEST_TOO_LARGE"
	CodeNotFound         = "NOT_FOUND"
	CodeAlreadyExists    = "ALREADY_EXISTS"
	CodeConflict         = "CONFLICT"
	CodeStoreUnavailable = "STORE_UNAVAILABLE"
	CodeAgentUnreachable = "AGENT_UNREACHABLE"
	CodeAgentError       = "AGENT_ERROR"
	CodeRuntimeError     = "RUNTIME_ERROR"
	CodeForbiddenOrigin  = "ORIGIN_NOT_ALLOWED"
	CodeInternal         = "INTERNAL_ERROR"
)

// ErrorBody is the inside of the error envelope.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

// Envelope is the whole error response.
type Envelope struct {
	Error     ErrorBody `json:"error"`
	RequestID string    `json:"requestId,omitempty"`
}

// Write sends v as JSON with the given status.
func Write(w http.ResponseWriter, log *slog.Logger, status int, v any) {
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil && log != nil {
		log.Warn("failed to write JSON response", "error", err)
	}
}

// WriteError sends the error envelope.
func WriteError(w http.ResponseWriter, log *slog.Logger, status int, requestID, code, message, field string) {
	Write(w, log, status, Envelope{Error: ErrorBody{Code: code, Message: message, Field: field}, RequestID: requestID})
}
