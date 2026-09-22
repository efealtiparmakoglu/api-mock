package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Server serves the routes of a hot-reloadable spec.
type Server struct {
	mu       sync.RWMutex
	spec     *Spec
	specPath string
	loaded   time.Time
}

// NewServer loads the spec once; Reload takes it from here.
func NewServer(specPath string) (*Server, error) {
	s := &Server{specPath: specPath}
	return s, s.Reload()
}

// Reload re-reads the spec file, swapping it in atomically on success. A
// broken file keeps the previous routes serving and returns the error.
func (s *Server) Reload() error {
	spec, err := LoadSpec(s.specPath)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.spec = spec
	s.loaded = time.Now()
	s.mu.Unlock()
	return nil
}

// Watch polls the spec file's mtime and reloads on change. The first tick
// always reloads (lastMod starts at the zero time), so callers can rely on
// Watch having synchronized once. It never exits; errors are logged, not
// fatal — the old spec keeps serving.
func (s *Server) Watch(interval time.Duration, logger *log.Logger) {
	var lastMod time.Time
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		info, err := os.Stat(s.specPath)
		if err != nil {
			logger.Printf("watch: %v", err)
			continue
		}
		if !info.ModTime().Equal(lastMod) {
			lastMod = info.ModTime()
			if err := s.Reload(); err != nil {
				logger.Printf("hot reload FAILED (keeping previous spec): %v", err)
			} else {
				logger.Printf("hot reload ok: %d routes", len(s.current().Routes))
			}
		}
	}
}

func (s *Server) current() *Spec {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.spec
}

// ServeHTTP implements the mock behavior.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	spec := s.current()
	route, params := matchRoute(spec, r.Method, r.URL.Path)
	if route == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":  "no route matches",
			"method": r.Method,
			"path":   r.URL.Path,
		})
		return
	}
	time.Sleep(spec.RouteDelay(route))

	if route.Schema != nil && r.Body != nil {
		body, err := readBody(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if violations := Validate(route.Schema, body); len(violations) > 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error":      "request body failed schema validation",
				"violations": violations,
			})
			return
		}
	}

	for k, v := range route.Headers {
		w.Header().Set(k, v)
	}
	status := route.Status
	if status == 0 {
		status = http.StatusOK
	}
	if route.Body == nil {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(applyParams(route.Body, params))
}

// applyParams substitutes {{name}} placeholders with matched path
// parameter values (e.g. route "/users/:id" serves "{{id}}" as the id).
func applyParams(body json.RawMessage, params map[string]string) []byte {
	if len(params) == 0 || len(body) == 0 {
		return body
	}
	out := string(body)
	for name, value := range params {
		out = strings.ReplaceAll(out, "{{"+name+"}}", value)
	}
	return []byte(out)
}

func readBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 10<<20 {
		return nil, fmt.Errorf("request body exceeds 10 MiB limit")
	}
	return body, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	_ = enc.Encode(payload)
}
