package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Validate checks a JSON request body against a Schema and returns every
// violation found, or nil when the body conforms.
//
// Supported subset: type (object/array/string/number/boolean/null),
// required, properties (nested), items (arrays), minLength/maxLength for
// strings and minimum/maximum for numbers.
func Validate(schema *Schema, body []byte) []string {
	if schema == nil {
		return nil
	}
	var value any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return []string{fmt.Sprintf("body is not valid JSON: %v", err)}
	}
	var v validator
	v.validate(schema, value, "")
	return v.errs
}

type validator struct {
	errs []string
}

func (v *validator) add(path, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if path == "" {
		message = "body" + message
	}
	v.errs = append(v.errs, path+message)
}

func (v *validator) validate(s *Schema, value any, path string) {
	if s == nil {
		return
	}
	if s.Type != "" && !typeMatches(s.Type, value) {
		v.add(path, ": must be %s, got %s", s.Type, jsonTypeName(value))
		return
	}
	switch t := value.(type) {
	case map[string]any:
		v.validateObject(s, t, path)
	case []any:
		v.validateArray(s, t, path)
	case string:
		v.validateString(s, t, path)
	case json.Number:
		v.validateNumber(s, t, path)
	}
}

func (v *validator) validateObject(s *Schema, obj map[string]any, path string) {
	for _, req := range s.Required {
		if _, ok := obj[req]; !ok {
			v.add(path, ": missing required property %q", req)
		}
	}
	for name, sub := range s.Properties {
		if val, ok := obj[name]; ok {
			v.validate(sub, val, joinPath(path, name))
		}
	}
}

func (v *validator) validateArray(s *Schema, arr []any, path string) {
	if s.Items != nil {
		for i, item := range arr {
			v.validate(s.Items, item, fmt.Sprintf("%s[%d]", path, i))
		}
	}
}

func (v *validator) validateString(s *Schema, str string, path string) {
	if s.MinLength != nil && len([]rune(str)) < *s.MinLength {
		v.add(path, ": length %d is below minLength %d", len(str), *s.MinLength)
	}
	if s.MaxLength != nil && len([]rune(str)) > *s.MaxLength {
		v.add(path, ": length %d exceeds maxLength %d", len(str), *s.MaxLength)
	}
}

func (v *validator) validateNumber(s *Schema, num json.Number, path string) {
	f, err := num.Float64()
	if err != nil {
		v.add(path, ": not a valid number: %v", err)
		return
	}
	if s.Minimum != nil && f < *s.Minimum {
		v.add(path, ": %v is below minimum %v", f, *s.Minimum)
	}
	if s.Maximum != nil && f > *s.Maximum {
		v.add(path, ": %v exceeds maximum %v", f, *s.Maximum)
	}
}

func typeMatches(want string, value any) bool {
	got := jsonTypeName(value)
	if want == got {
		return true
	}
	// JSON Schema treats integers as numbers; accept the reverse direction
	// only when the value really has no fraction.
	if want == "number" && got == "integer" {
		return true
	}
	return false
}

func jsonTypeName(value any) string {
	switch t := value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case json.Number:
		if !strings.ContainsAny(t.String(), ".eE") {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}

func joinPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

// parseSchemaJSON decodes a Schema from JSON text (used by tests and handy
// for embedding schemas in tooling).
func parseSchemaJSON(body string) (*Schema, error) {
	var s Schema
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		return nil, err
	}
	return &s, nil
}
