package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// envelopeShape is a minimal view of types.ApiResponse[T] for assertions.
type envelopeShape struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

func recJSON(t *testing.T, rec *httptest.ResponseRecorder) envelopeShape {
	t.Helper()
	var env envelopeShape
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response not a valid JSON envelope: %v (body=%q)", err, rec.Body.String())
	}
	return env
}

// --- writeOK / writeErr / writeBare / decodeJSON contract tests ---

func TestWriteOK_Envelope(t *testing.T) {
	rec := httptest.NewRecorder()
	writeOK(rec, map[string]string{"k": "v"})
	env := recJSON(t, rec)
	if !env.Success || env.Error != "" {
		t.Fatalf("writeOK envelope: success=%v error=%q", env.Success, env.Error)
	}
	if string(env.Data) != `{"k":"v"}` {
		t.Fatalf("writeOK data = %s, want {\"k\":\"v\"}", env.Data)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
}

func TestWriteErr_Envelope(t *testing.T) {
	rec := httptest.NewRecorder()
	writeErr[map[string]string](rec, errors.New("boom"))
	env := recJSON(t, rec)
	if env.Success {
		t.Fatal("writeErr should set success=false")
	}
	if env.Error != "boom" {
		t.Fatalf("writeErr error = %q, want boom", env.Error)
	}
}

func TestWriteBare_NoEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	writeBare(rec, map[string]any{"events": []string{}, "isRunning": false})
	// Bare objects must NOT have success/data wrapper — the body IS the object.
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("bare body not valid JSON: %v", err)
	}
	if _, ok := raw["success"]; ok {
		t.Fatal("bare response must not contain a 'success' field")
	}
	if _, ok := raw["events"]; !ok {
		t.Fatal("bare response missing 'events' field")
	}
}

func TestDecodeJSON_Valid(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"mode":"accept"}`))
	rec := httptest.NewRecorder()
	got, ok := decodeJSON[resolveRequest](rec, req)
	if !ok {
		t.Fatal("decodeJSON should succeed on valid JSON")
	}
	if got.Mode != "accept" {
		t.Fatalf("Mode = %q, want accept", got.Mode)
	}
}

func TestDecodeJSON_InvalidBodyWritesError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{not-json`))
	rec := httptest.NewRecorder()
	_, ok := decodeJSON[resolveRequest](rec, req)
	if ok {
		t.Fatal("decodeJSON should return ok=false on invalid JSON")
	}
	env := recJSON(t, rec)
	if env.Success {
		t.Fatal("decodeJSON error should write success=false")
	}
	if env.Error == "" {
		t.Fatal("decodeJSON error should populate the error field")
	}
}
