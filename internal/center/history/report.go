package history

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ipchronicle/ipchronicle/internal/probefields"
)

type JSONType = probefields.JSONType

const (
	JSONTypeString  = probefields.JSONTypeString
	JSONTypeNumber  = probefields.JSONTypeNumber
	JSONTypeBoolean = probefields.JSONTypeBoolean
	JSONTypeNull    = probefields.JSONTypeNull
	JSONTypeObject  = probefields.JSONTypeObject
	JSONTypeArray   = probefields.JSONTypeArray
)

type FieldDefinition = probefields.Definition

type FieldValue struct {
	ID            string
	Group         string
	Path          string
	ExpectedTypes []JSONType
	Status        string
	ActualType    *JSONType
	Value         *string
}

type FormatIssue struct {
	Path          string
	Kind          string
	ExpectedTypes []JSONType
	ActualType    *JSONType
}

type Report struct {
	Fields []FieldValue
	Issues []FormatIssue
}

type FieldChange struct {
	FieldID   string
	Group     string
	Path      string
	ValueType JSONType
	Before    string
	After     string
}

var ErrInvalidReportJSON = errors.New("complete-probe result is not a JSON object")

var catalog = probefields.Catalog()

func Catalog() []FieldDefinition {
	return probefields.Catalog()
}

func Interpret(raw []byte) (Report, error) {
	root, err := decodeObject(raw)
	if err != nil {
		return Report{}, err
	}
	report := Report{Fields: make([]FieldValue, 0, len(catalog))}
	for _, definition := range catalog {
		value, present := valueAt(root, definition.Path)
		field := FieldValue{
			ID: definition.ID, Group: definition.Group,
			Path:          strings.Join(definition.Path, "."),
			ExpectedTypes: append([]JSONType(nil), definition.ExpectedTypes...),
		}
		if !present {
			field.Status = "missing"
			report.Issues = append(report.Issues, FormatIssue{
				Path: field.Path, Kind: "missing",
				ExpectedTypes: append([]JSONType(nil), definition.ExpectedTypes...),
			})
			report.Fields = append(report.Fields, field)
			continue
		}
		actualType := jsonType(value)
		field.ActualType = &actualType
		if actualType == JSONTypeNull {
			field.Status = "unavailable"
			report.Fields = append(report.Fields, field)
			continue
		}
		if !containsType(definition.ExpectedTypes, actualType) {
			field.Status = "incompatible"
			report.Issues = append(report.Issues, FormatIssue{
				Path: field.Path, Kind: "incompatible",
				ExpectedTypes: append([]JSONType(nil), definition.ExpectedTypes...),
				ActualType:    &actualType,
			})
			report.Fields = append(report.Fields, field)
			continue
		}
		field.Status = "available"
		encoded := formatScalar(value)
		field.Value = &encoded
		report.Fields = append(report.Fields, field)
	}
	report.Issues = append(report.Issues, unknownIssues(root)...)
	sort.Slice(report.Issues, func(left, right int) bool {
		if report.Issues[left].Path == report.Issues[right].Path {
			return report.Issues[left].Kind < report.Issues[right].Kind
		}
		return report.Issues[left].Path < report.Issues[right].Path
	})
	return report, nil
}

func Compare(before, after Report) []FieldChange {
	beforeByID := make(map[string]FieldValue, len(before.Fields))
	for _, field := range before.Fields {
		beforeByID[field.ID] = field
	}
	comparable := make(map[string]bool, len(catalog))
	for _, definition := range catalog {
		comparable[definition.ID] = definition.Compare
	}
	changes := make([]FieldChange, 0)
	for _, current := range after.Fields {
		previous, exists := beforeByID[current.ID]
		if !exists || !comparable[current.ID] || previous.Status != "available" || current.Status != "available" ||
			previous.ActualType == nil || current.ActualType == nil || *previous.ActualType != *current.ActualType ||
			previous.Value == nil || current.Value == nil || *previous.Value == *current.Value {
			continue
		}
		changes = append(changes, FieldChange{
			FieldID: current.ID, Group: current.Group, Path: current.Path,
			ValueType: *current.ActualType, Before: *previous.Value, After: *current.Value,
		})
	}
	return changes
}

func decodeObject(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode complete-probe result: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, ErrInvalidReportJSON
		}
		return nil, fmt.Errorf("decode trailing complete-probe result data: %w", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, ErrInvalidReportJSON
	}
	return object, nil
}

func valueAt(root map[string]any, path []string) (any, bool) {
	var current any = root
	for _, segment := range path {
		if current == nil {
			return nil, true
		}
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func jsonType(value any) JSONType {
	switch value.(type) {
	case nil:
		return JSONTypeNull
	case string:
		return JSONTypeString
	case json.Number:
		return JSONTypeNumber
	case bool:
		return JSONTypeBoolean
	case map[string]any:
		return JSONTypeObject
	case []any:
		return JSONTypeArray
	default:
		panic(fmt.Sprintf("unexpected decoded JSON value %T", value))
	}
}

func formatScalar(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case string:
		return typed
	case json.Number:
		return typed.String()
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		panic(fmt.Sprintf("cannot format non-scalar JSON value %T", value))
	}
}

func containsType(types []JSONType, candidate JSONType) bool {
	for _, value := range types {
		if value == candidate {
			return true
		}
	}
	return false
}

type catalogNode struct {
	field    bool
	children map[string]*catalogNode
}

func unknownIssues(root map[string]any) []FormatIssue {
	trie := &catalogNode{children: map[string]*catalogNode{}}
	for _, definition := range catalog {
		current := trie
		for _, segment := range definition.Path {
			if current.children[segment] == nil {
				current.children[segment] = &catalogNode{children: map[string]*catalogNode{}}
			}
			current = current.children[segment]
		}
		current.field = true
	}
	issues := make([]FormatIssue, 0)
	var walk func(any, *catalogNode, []string)
	walk = func(value any, node *catalogNode, path []string) {
		if node != nil && node.field {
			return
		}
		object, isObject := value.(map[string]any)
		if isObject && len(object) > 0 {
			keys := make([]string, 0, len(object))
			for key := range object {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				var child *catalogNode
				if node != nil {
					child = node.children[key]
				}
				walk(object[key], child, append(path, key))
			}
			return
		}
		if node == nil {
			actual := jsonType(value)
			if actual == JSONTypeNull {
				return
			}
			issues = append(issues, FormatIssue{
				Path: strings.Join(path, "."), Kind: "unknown", ActualType: &actual,
			})
		}
	}
	walk(root, trie, nil)
	return issues
}
