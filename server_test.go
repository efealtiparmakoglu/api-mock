package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSpec(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spec.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const simpleSpec = `{
	"routes": [
		{"method": "GET",  "path": "/hello",       "status": 200, "body": {"msg": "hi"}, "headers": {"X-Mock": "v1"}},
		{"method": "POST", "path": "/users",       "status": 201, "body": {"created": true},
		 "schema": {"type": "object", "required": ["name"]}},
		{"method": "GET",  "path": "/users/:id",   "status": 200, "body": {"id": "{{id}}"}},
		{"method": "GET",  "path": "/slow",        "status": 200, "body": {}, "delayMs": 120},
		{"method": "GET",  "path": "/no-body",     "status": 204}
	]
}`

func do(t *testing.T, s *Server, method, path, body string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec.Result()
}

func TestServerServesConfiguredRoutes(t *testing.T) {
	s, err := NewServer(writeSpec(t, simpleSpec))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	res := do(t, s, "GET", "/hello", "")
	if res.StatusCode != 200 {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if res.Header.Get("X-Mock") != "v1" {
		t.Fatalf("custom header missing")
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["msg"] != "hi" {
		t.Fatalf("payload = %v", payload)
	}
}

func TestServerUnknownRouteIs404(t *testing.T) {
	s, _ := NewServer(writeSpec(t, simpleSpec))
	res := do(t, s, "GET", "/nope", "")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestServerSchemaValidationReturns400(t *testing.T) {
	s, _ := NewServer(writeSpec(t, simpleSpec))
	res := do(t, s, "POST", "/users", `{"age": 3}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var payload struct {
		Violations []string `json:"violations"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Violations) == 0 {
		t.Fatal("violations must be listed")
	}
	if res := do(t, s, "POST", "/users", `{"name": "ada"}`); res.StatusCode != 201 {
		t.Fatalf("valid body must pass, got %d", res.StatusCode)
	}
}

func TestServerPathParamSubstitution(t *testing.T) {
	s, _ := NewServer(writeSpec(t, simpleSpec))
	res := do(t, s, "GET", "/users/77", "")
	body, _ := io.ReadAll(res.Body)
	if !bytes.Contains(body, []byte(`"77"`)) {
		t.Fatalf("{{id}} must be substituted, got %s", body)
	}
}

func TestServerRouteDelayIsApplied(t *testing.T) {
	s, _ := NewServer(writeSpec(t, simpleSpec))
	start := time.Now()
	do(t, s, "GET", "/slow", "")
	if elapsed := time.Since(start); elapsed < 110*time.Millisecond {
		t.Fatalf("delay not applied: %v", elapsed)
	}
}

func TestServer204HasNoBody(t *testing.T) {
	s, _ := NewServer(writeSpec(t, simpleSpec))
	res := do(t, s, "GET", "/no-body", "")
	if res.StatusCode != 204 {
		t.Fatalf("status = %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if len(body) != 0 {
		t.Fatalf("204 must have empty body, got %q", body)
	}
}

func TestHotReloadKeepsServingOnBrokenSpec(t *testing.T) {
	specPath := writeSpec(t, simpleSpec)
	s, err := NewServer(specPath)
	if err != nil {
		t.Fatal(err)
	}
	// break the spec and simulate what Watch does: Reload fails, old spec serves
	if err := os.WriteFile(specPath, []byte(`{"routes": [ {"method":`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(); err == nil {
		t.Fatal("broken spec must fail reload")
	}
	if res := do(t, s, "GET", "/hello", ""); res.StatusCode != 200 {
		t.Fatalf("old spec must keep serving after failed reload, got %d", res.StatusCode)
	}

	// fix the spec with a new route; Watch (already polling) picks it up
	logger := log.New(io.Discard, "", 0)
	go s.Watch(20*time.Millisecond, logger)
	updated := `{"routes": [{"method": "GET", "path": "/v2", "body": {"v": 2}}]}`
	if err := os.WriteFile(specPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		res := do(t, s, "GET", "/v2", "")
		if res.StatusCode == 200 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("hot reload did not pick up the new route within 3s")
}

func TestBodyFileIsLoaded(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "payload.json"), []byte(`{"from": "file"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(dir, "spec.json")
	if err := os.WriteFile(specPath, []byte(`{
		"routes": [{"method": "GET", "path": "/f", "bodyFile": "payload.json"}]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(specPath)
	if err != nil {
		t.Fatalf("LoadSpec with bodyFile: %v", err)
	}
	body, _ := io.ReadAll(do(t, s, "GET", "/f", "").Body)
	if !bytes.Contains(body, []byte("file")) {
		t.Fatalf("bodyFile not served: %s", body)
	}
}
