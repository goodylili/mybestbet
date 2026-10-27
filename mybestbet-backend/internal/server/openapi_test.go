package server

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const specPath = "../../api/openapi.yaml"

type openAPISpec struct {
	root map[string]any
}

func loadSpec(t *testing.T) *openAPISpec {
	t.Helper()
	raw, err := os.ReadFile(specPath)
	require.NoError(t, err)
	var root map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &root))
	return &openAPISpec{root: root}
}

func (s *openAPISpec) lookup(ref string) map[string]any {
	node := any(s.root)
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		object, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		node = object[part]
	}
	object, _ := node.(map[string]any)
	return object
}

func (s *openAPISpec) resolve(node map[string]any) map[string]any {
	for node != nil {
		ref, ok := node["$ref"].(string)
		if !ok {
			return node
		}
		node = s.lookup(ref)
	}
	return nil
}

func (s *openAPISpec) responseSchema(pathTemplate, method string, status int) (map[string]any, error) {
	operation := s.operation(pathTemplate, method)
	if operation == nil {
		return nil, fmt.Errorf("no operation %s %s", method, pathTemplate)
	}
	responses, _ := operation["responses"].(map[string]any)
	response, _ := responses[fmt.Sprint(status)].(map[string]any)
	response = s.resolve(response)
	if response == nil {
		return nil, fmt.Errorf("%s %s does not declare status %d", method, pathTemplate, status)
	}
	content, _ := response["content"].(map[string]any)
	media, _ := content["application/json"].(map[string]any)
	schema, _ := media["schema"].(map[string]any)
	if schema == nil {
		return nil, fmt.Errorf("%s %s %d has no json schema", method, pathTemplate, status)
	}
	return schema, nil
}

func (s *openAPISpec) operation(pathTemplate, method string) map[string]any {
	paths, _ := s.root["paths"].(map[string]any)
	item, _ := paths[pathTemplate].(map[string]any)
	operation, _ := item[method].(map[string]any)
	return operation
}

type objectShape struct {
	properties map[string]any
	required   []string
	additional map[string]any
}

func (s *openAPISpec) shape(schema map[string]any) objectShape {
	shape := objectShape{properties: map[string]any{}}
	s.collectShape(schema, &shape)
	return shape
}

func (s *openAPISpec) collectShape(schema map[string]any, shape *objectShape) {
	schema = s.resolve(schema)
	for name, prop := range asMap(schema["properties"]) {
		shape.properties[name] = prop
	}
	for _, name := range asSlice(schema["required"]) {
		shape.required = append(shape.required, fmt.Sprint(name))
	}
	if additional, ok := schema["additionalProperties"].(map[string]any); ok {
		shape.additional = additional
	}
	for _, part := range asSlice(schema["allOf"]) {
		s.collectShape(asMap(part), shape)
	}
}

func (s *openAPISpec) schemaType(schema map[string]any) string {
	if declared, ok := schema["type"].(string); ok {
		return declared
	}
	if _, ok := schema["allOf"]; ok {
		return "object"
	}
	return ""
}

func (s *openAPISpec) validate(value any, schema map[string]any, at string) []string {
	schema = s.resolve(schema)
	if schema == nil {
		return []string{at + ": unresolved schema"}
	}
	if enum := asSlice(schema["enum"]); enum != nil && !slices.ContainsFunc(enum, func(v any) bool { return fmt.Sprint(v) == fmt.Sprint(value) }) {
		return []string{fmt.Sprintf("%s: %v not in enum %v", at, value, enum)}
	}
	switch s.schemaType(schema) {
	case "object":
		return s.validateObject(value, schema, at)
	case "array":
		items, ok := value.([]any)
		if !ok {
			return []string{fmt.Sprintf("%s: want array, got %T", at, value)}
		}
		var problems []string
		for i, item := range items {
			problems = append(problems, s.validate(item, asMap(schema["items"]), fmt.Sprintf("%s[%d]", at, i))...)
		}
		return problems
	case "string":
		return validateString(value, schema, at)
	case "number":
		if _, ok := value.(float64); !ok {
			return []string{fmt.Sprintf("%s: want number, got %T", at, value)}
		}
	case "integer":
		if n, ok := value.(float64); !ok || n != float64(int64(n)) {
			return []string{fmt.Sprintf("%s: want integer, got %v", at, value)}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return []string{fmt.Sprintf("%s: want boolean, got %T", at, value)}
		}
	}
	return nil
}

func validateString(value any, schema map[string]any, at string) []string {
	text, ok := value.(string)
	if !ok {
		return []string{fmt.Sprintf("%s: want string, got %T", at, value)}
	}
	if schema["format"] == "date-time" {
		if _, err := time.Parse(time.RFC3339Nano, text); err != nil {
			return []string{fmt.Sprintf("%s: %q is not date-time", at, text)}
		}
	}
	return nil
}

func (s *openAPISpec) validateObject(value any, schema map[string]any, at string) []string {
	object, ok := value.(map[string]any)
	if !ok {
		return []string{fmt.Sprintf("%s: want object, got %T", at, value)}
	}
	if schema["type"] == "object" && schema["properties"] == nil && schema["additionalProperties"] == nil && schema["allOf"] == nil {
		return nil
	}
	shape := s.shape(schema)
	var problems []string
	for _, name := range shape.required {
		if _, present := object[name]; !present {
			problems = append(problems, fmt.Sprintf("%s: missing required %q", at, name))
		}
	}
	for name, field := range object {
		propSchema, declared := shape.properties[name]
		switch {
		case declared:
			problems = append(problems, s.validate(field, asMap(propSchema), at+"."+name)...)
		case shape.additional != nil:
			problems = append(problems, s.validate(field, shape.additional, at+"."+name)...)
		default:
			problems = append(problems, fmt.Sprintf("%s: undeclared property %q", at, name))
		}
	}
	return problems
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func TestSpecReferencesResolve(t *testing.T) {
	t.Parallel()
	spec := loadSpec(t)
	var unresolved []string
	var walk func(node any, at string)
	walk = func(node any, at string) {
		switch typed := node.(type) {
		case map[string]any:
			if ref, ok := typed["$ref"].(string); ok && spec.lookup(ref) == nil {
				unresolved = append(unresolved, at+" -> "+ref)
			}
			for key, child := range typed {
				walk(child, at+"/"+key)
			}
		case []any:
			for i, child := range typed {
				walk(child, fmt.Sprintf("%s[%d]", at, i))
			}
		}
	}
	walk(spec.root, "#")
	require.Empty(t, unresolved)
	require.Equal(t, "3.1.0", spec.root["openapi"])
}

func TestValidatorRejectsContractDrift(t *testing.T) {
	t.Parallel()
	spec := loadSpec(t)
	schema, err := spec.responseSchema("/v1/leagues", "get", 200)
	require.NoError(t, err)
	drifted := map[string]any{"items": []any{map[string]any{"code": 1.0, "name": "x", "country": "y", "sport": "curling", "group": "domestic", "order": 1.0, "fixtures": 2.0}}}
	problems := spec.validate(drifted, schema, "response")
	require.ElementsMatch(t, []string{
		"response.items[0]: missing required \"fixtureCount\"",
		"response.items[0].code: want string, got float64",
		"response.items[0].sport: curling not in enum [football basketball tennis american-football esports]",
		"response.items[0]: undeclared property \"fixtures\"",
	}, problems)
}
