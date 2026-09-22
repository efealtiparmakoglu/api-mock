package main

import (
	"strings"
	"testing"
)

func TestParseSpecValid(t *testing.T) {
	spec, err := ParseSpec([]byte(`{
		"port": 9999,
		"delayMs": 10,
		"routes": [
			{"method": "GET", "path": "/users/:id", "status": 200,
			 "body": {"id": "{{id}}", "name": "ada"},
			 "headers": {"X-Mock": "yes"}},
			{"method": "POST", "path": "/users", "status": 201, "body": {"ok": true},
			 "schema": {"type": "object", "required": ["name"]}},
			{"method": "DELETE", "path": "/users/:id", "status": 204}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	if spec.Port != 9999 || len(spec.Routes) != 3 {
		t.Fatalf("unexpected spec: %+v", spec)
	}
	if spec.Routes[0].Status != 200 {
		t.Fatalf("status must default through explicit value, got %d", spec.Routes[0].Status)
	}
}

func TestParseSpecRejectsUnknownFields(t *testing.T) {
	_, err := ParseSpec([]byte(`{"routes": [], "bogus": 1}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestParseSpecRejectsNoRoutes(t *testing.T) {
	if _, err := ParseSpec([]byte(`{"routes": []}`)); err == nil {
		t.Fatal("empty routes must be rejected")
	}
}

func TestParseSpecValidationErrors(t *testing.T) {
	cases := map[string]string{
		"missing method":    `{"routes": [{"path": "/x"}]}`,
		"unknown method":    `{"routes": [{"method": "FETCH", "path": "/x", "body": 1}]}`,
		"missing path":      `{"routes": [{"method": "GET"}]}`,
		"bad path":          `{"routes": [{"method": "GET", "path": "no-slash"}]}`,
		"bad status":        `{"routes": [{"method": "GET", "path": "/x", "status": 42}]}`,
		"missing body":      `{"routes": [{"method": "GET", "path": "/x", "status": 200}]}`,
		"body and bodyFile": `{"routes": [{"method": "GET", "path": "/x", "body": 1, "bodyFile": "f"}]}`,
		"negative delay":    `{"routes": [{"method": "GET", "path": "/x", "body": 1, "delayMs": -5}]}`,
		"duplicate route":   `{"routes": [{"method": "GET", "path": "/x", "body": 1}, {"method": "GET", "path": "/x", "body": 2}]}`,
	}
	for name, input := range cases {
		if _, err := ParseSpec([]byte(input)); err == nil {
			t.Errorf("%s: expected error, got none", name)
		}
	}
}

func TestParseSpecAllows204WithoutBody(t *testing.T) {
	if _, err := ParseSpec([]byte(`{"routes": [{"method": "DELETE", "path": "/x", "status": 204}]}`)); err != nil {
		t.Fatalf("204 without body must be valid: %v", err)
	}
}

func TestMatchRouteParamsAndOrder(t *testing.T) {
	spec, err := ParseSpec([]byte(`{
		"routes": [
			{"method": "GET", "path": "/users/:id", "body": {"id": "{{id}}"}},
			{"method": "GET", "path": "/users/me", "body": {"me": true}},
			{"method": "GET", "path": "/a/:x/b/:y", "body": 1}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	route, params := matchRoute(spec, "GET", "/users/42")
	if route == nil || route.Path != "/users/:id" || params["id"] != "42" {
		t.Fatalf("param route mismatch: route=%v params=%v", route, params)
	}
	route, params = matchRoute(spec, "GET", "/a/1/b/2")
	if route == nil || params["x"] != "1" || params["y"] != "2" {
		t.Fatalf("multi param mismatch: %v", params)
	}
	if route, _ := matchRoute(spec, "POST", "/users/42"); route != nil {
		t.Fatal("method must participate in matching")
	}
	if route, _ := matchRoute(spec, "GET", "/users/42/extra"); route != nil {
		t.Fatal("segment count must participate in matching")
	}
	if route, _ := matchRoute(spec, "GET", "/users"); route != nil {
		t.Fatal("partial path must not match")
	}
}

func TestApplyParams(t *testing.T) {
	got := applyParams([]byte(`{"id": "{{id}}", "q": "{{id}}/{{name}}"}`),
		map[string]string{"id": "42", "name": "ada"})
	want := `{"id": "42", "q": "42/ada"}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if out := applyParams([]byte(`{}`), nil); string(out) != `{}` {
		t.Fatalf("no params must be a no-op, got %s", out)
	}
}
