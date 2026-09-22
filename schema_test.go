package main

import (
	"strings"
	"testing"
)

func mustSchema(t *testing.T, body string) *Schema {
	t.Helper()
	s, err := parseSchemaJSON(body)
	if err != nil {
		t.Fatalf("parseSchemaJSON: %v", err)
	}
	return s
}

func TestValidateAcceptsConformingBody(t *testing.T) {
	s := mustSchema(t, `{
		"type": "object",
		"required": ["name", "age"],
		"properties": {
			"name": {"type": "string", "minLength": 2},
			"age": {"type": "integer", "minimum": 0, "maximum": 150},
			"tags": {"type": "array", "items": {"type": "string"}},
			"address": {"type": "object", "required": ["city"], "properties": {"city": {"type": "string"}}}
		}
	}`)
	body := []byte(`{"name":"Ada","age":36,"tags":["x","y"],"address":{"city":"Izmir"}}`)
	if violations := Validate(s, body); len(violations) != 0 {
		t.Fatalf("expected no violations, got %v", violations)
	}
}

func TestValidateReportsAllViolations(t *testing.T) {
	s := mustSchema(t, `{
		"type": "object",
		"required": ["name", "email"],
		"properties": {
			"name": {"type": "string", "minLength": 3},
			"age": {"type": "integer"},
			"tags": {"type": "array", "items": {"type": "string"}}
		}
	}`)
	body := []byte(`{"name":"a","age":"old","tags":["ok",42]}`)
	violations := Validate(s, body)
	joined := strings.Join(violations, "; ")
	for _, want := range []string{
		`missing required property "email"`,
		"name: length 1 is below minLength 3",
		"age: must be integer, got string",
		"tags[1]: must be string, got integer",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("violations missing %q:\n got: %s", want, joined)
		}
	}
}

func TestValidateTypeErrors(t *testing.T) {
	s := mustSchema(t, `{"type": "array"}`)
	if violations := Validate(s, []byte(`{"x":1}`)); len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %v", violations)
	}
}

func TestValidateNumberRanges(t *testing.T) {
	s := mustSchema(t, `{"type": "number", "minimum": 1.5, "maximum": 9.5}`)
	if violations := Validate(s, []byte(`1.5`)); len(violations) != 0 {
		t.Errorf("minimum is inclusive, got %v", violations)
	}
	if violations := Validate(s, []byte(`9.6`)); len(violations) != 1 {
		t.Errorf("9.6 must exceed maximum, got %v", violations)
	}
}

func TestValidateInvalidJSONBody(t *testing.T) {
	s := mustSchema(t, `{"type": "object"}`)
	violations := Validate(s, []byte(`{broken`))
	if len(violations) != 1 || !strings.Contains(violations[0], "not valid JSON") {
		t.Fatalf("expected invalid-json violation, got %v", violations)
	}
}

func TestValidateNilSchemaPasses(t *testing.T) {
	if violations := Validate(nil, []byte(`anything`)); violations != nil {
		t.Fatalf("nil schema must pass, got %v", violations)
	}
}

func TestIntegerAcceptedAsNumber(t *testing.T) {
	s := mustSchema(t, `{"properties": {"n": {"type": "number"}}}`)
	if violations := Validate(s, []byte(`{"n": 3}`)); len(violations) != 0 {
		t.Fatalf("integer must satisfy number schema, got %v", violations)
	}
}
