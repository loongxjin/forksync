package app

import (
	"encoding/json"
	"net/http"

	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// writeOK writes a successful ApiResponse[T] envelope to w.
func writeOK[T any](w http.ResponseWriter, data T) {
	writeEnvelope(w, types.ApiResponse[T]{Success: true, Data: data})
}

// writeErr writes a failed ApiResponse[T] envelope to w with the given error.
// The HTTP status stays 200 so the frontend's existing ApiResponse parsing
// (which reads success/error fields, not HTTP status) keeps working unchanged.
func writeErr[T any](w http.ResponseWriter, err error) {
	logger.Error("app: handler error", "error", err)
	writeEnvelope(w, types.ApiResponse[T]{Success: false, Error: err.Error()})
}

// writeEnvelope marshals one ApiResponse as a single-line JSON object.
// Single-line output preserves the existing contract the frontend expects.
func writeEnvelope[T any](w http.ResponseWriter, resp types.ApiResponse[T]) {
	w.Header().Set("Content-Type", "application/json")
	raw, err := json.Marshal(resp)
	if err != nil {
		logger.Error("app: failed to encode response", "error", err)
		http.Error(w, `{"success":false,"error":"internal encode error"}`, http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(raw)
}

// decodeJSON decodes a JSON request body into dst. Returns false (and writes
// an error response) if decoding fails.
func decodeJSON[T any](w http.ResponseWriter, r *http.Request) (dst T, ok bool) {
	if err := json.NewDecoder(r.Body).Decode(&dst); err != nil {
		writeErr[any](w, err)
		return dst, false
	}
	return dst, true
}
