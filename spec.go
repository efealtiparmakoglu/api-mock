package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Spec is the whole mock definition loaded from one JSON file.
type Spec struct {
	Port    int     `json:"port"`
	DelayMs int     `json:"delayMs"`
	Jitter  int     `json:"delayJitterMs"`
	Routes  []Route `json:"routes"`
}

// Route is one endpoint: matcher plus canned response and checks.
type Route struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
	// BodyFile is an alternative to Body, read relative to the spec file.
	BodyFile string  `json:"bodyFile"`
	DelayMs  int     `json:"delayMs"`
	Schema   *Schema `json:"schema"`
}

// Schema is a small validation subset: types, required fields, nested
// properties and array items.
type Schema struct {
	Type       string             `json:"type"`
	Required   []string           `json:"required"`
	Properties map[string]*Schema `json:"properties"`
	Items      *Schema            `json:"items"`
	MinLength  *int               `json:"minLength"`
	MaxLength  *int               `json:"maxLength"`
	Minimum    *float64           `json:"minimum"`
	Maximum    *float64           `json:"maximum"`
}

// LoadSpec reads and validates a spec file.
func LoadSpec(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	spec, err := ParseSpec(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i := range spec.Routes {
		if spec.Routes[i].BodyFile != "" {
			bodyPath := spec.Routes[i].BodyFile
			if !filepath.IsAbs(bodyPath) {
				bodyPath = filepath.Join(filepath.Dir(path), bodyPath)
			}
			body, err := os.ReadFile(bodyPath)
			if err != nil {
				return nil, fmt.Errorf("%s: route %d: %w", path, i, err)
			}
			spec.Routes[i].Body = json.RawMessage(body)
		}
	}
	return spec, nil
}

// ParseSpec decodes strict JSON and applies structural validation.
func ParseSpec(data []byte) (*Spec, error) {
	var spec Spec
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if len(spec.Routes) == 0 {
		return nil, fmt.Errorf("parse: spec has no routes")
	}
	seen := map[string]bool{}
	for i, r := range spec.Routes {
		if r.Method == "" {
			return nil, fmt.Errorf("route %d: missing method", i)
		}
		if !knownMethod(r.Method) {
			return nil, fmt.Errorf("route %d: unknown method %q", i, r.Method)
		}
		if r.Path == "" {
			return nil, fmt.Errorf("route %d: missing path", i)
		}
		if !validPath(r.Path) {
			return nil, fmt.Errorf("route %d: path %q must start with / and use :param or literal segments", i, r.Path)
		}
		if r.Status != 0 && (r.Status < 100 || r.Status > 599) {
			return nil, fmt.Errorf("route %d: status %d out of range", i, r.Status)
		}
		if r.Body == nil && r.BodyFile == "" && r.Status >= 200 && r.Status < 300 && !isNoContent(r.Status) {
			return nil, fmt.Errorf("route %d: %s %s has no body or bodyFile", i, r.Method, r.Path)
		}
		if r.Body != nil && r.BodyFile != "" {
			return nil, fmt.Errorf("route %d: body and bodyFile are mutually exclusive", i)
		}
		if r.Body != nil && !json.Valid(r.Body) {
			return nil, fmt.Errorf("route %d: body is not valid JSON", i)
		}
		if r.DelayMs < 0 {
			return nil, fmt.Errorf("route %d: negative delayMs", i)
		}
		key := r.Method + " " + r.Path
		if seen[key] {
			return nil, fmt.Errorf("route %d: duplicate %s", i, key)
		}
		seen[key] = true
	}
	return &spec, nil
}

func isNoContent(status int) bool { return status == 204 || status == 304 }

func knownMethod(m string) bool {
	switch m {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	}
	return false
}

func validPath(p string) bool {
	if p == "" || p[0] != '/' {
		return false
	}
	for _, seg := range splitPath(p) {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// splitPath splits "/a/:id/b" into segments, dropping the leading empty one.
func splitPath(p string) []string {
	var out []string
	start := 1
	for i := 1; i <= len(p); i++ {
		if i == len(p) || p[i] == '/' {
			if i > start {
				out = append(out, p[start:i])
			}
			start = i + 1
		}
	}
	return out
}

// matchRoute finds the first route matching method and path. Params are
// returned for ":name" segments. Linear scan keeps spec order meaningful.
func matchRoute(spec *Spec, method, path string) (*Route, map[string]string) {
	segs := splitPath(path)
	for i := range spec.Routes {
		r := &spec.Routes[i]
		if r.Method != method {
			continue
		}
		routeSegs := splitPath(r.Path)
		if len(routeSegs) != len(segs) {
			continue
		}
		params := map[string]string{}
		matched := true
		for j, rs := range routeSegs {
			if len(rs) > 0 && rs[0] == ':' {
				params[rs[1:]] = segs[j]
			} else if rs != segs[j] {
				matched = false
				break
			}
		}
		if matched {
			return r, params
		}
	}
	return nil, nil
}

// RouteDelay returns the effective artificial delay for a route.
func (s *Spec) RouteDelay(r *Route) time.Duration {
	base := r.DelayMs
	if base == 0 {
		base = s.DelayMs
	}
	jitter := s.Jitter
	if jitter <= 0 {
		return time.Duration(base) * time.Millisecond
	}
	return time.Duration(base+pseudoRandom(jitter)) * time.Millisecond
}

// pseudoRandom gives a deterministic-free but cheap 0..max-1 value from
// the clock; tests pass jitter=0 to stay deterministic.
func pseudoRandom(max int) int {
	return int(time.Now().UnixNano() % int64(max))
}
