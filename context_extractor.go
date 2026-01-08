// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky

import (
	"regexp"
	"strings"
)

// extractTemplateFields extracts field references from Go template strings.
// Returns unique field names that are referenced via {{ .fieldName }} syntax.
// Handles nested fields like {{ .item.id }} → "item.id"
func extractTemplateFields(tmpl string) []string {
	if tmpl == "" || !strings.Contains(tmpl, "{{") {
		return nil
	}

	// Match {{ .fieldName }} or {{ .nested.field }}
	// Also handles whitespace: {{.field}}, {{ .field }}, {{  .field  }}
	re := regexp.MustCompile(`\{\{\s*\.([a-zA-Z_][a-zA-Z0-9_.]*)`)
	matches := re.FindAllStringSubmatch(tmpl, -1)

	if len(matches) == 0 {
		return nil
	}

	// Deduplicate
	seen := make(map[string]bool)
	var fields []string
	for _, match := range matches {
		if len(match) > 1 {
			field := match[1]
			if !seen[field] {
				seen[field] = true
				fields = append(fields, field)
			}
		}
	}

	return fields
}

// extractJQPaths extracts path references from JQ expressions.
// Returns unique paths that are referenced (e.g., ".item.id" → "item.id")
// This is a simplified extraction - it finds direct path accesses but may miss
// complex computed paths.
func extractJQPaths(expr string) []string {
	if expr == "" {
		return nil
	}

	// Match .fieldName or .nested.field patterns
	// Excludes special JQ operators like .[] or .[0]
	re := regexp.MustCompile(`\.([a-zA-Z_][a-zA-Z0-9_]*)(?:\.([a-zA-Z_][a-zA-Z0-9_]*))*`)
	matches := re.FindAllString(expr, -1)

	if len(matches) == 0 {
		return nil
	}

	// Deduplicate and clean up (remove leading dot)
	seen := make(map[string]bool)
	var paths []string
	for _, match := range matches {
		// Remove leading dot
		path := strings.TrimPrefix(match, ".")
		if path != "" && !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}

	return paths
}

// mergeFields merges multiple field lists into a single deduplicated list.
func mergeFields(fieldLists ...[]string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, fields := range fieldLists {
		for _, field := range fields {
			if !seen[field] {
				seen[field] = true
				result = append(result, field)
			}
		}
	}

	return result
}

// getNestedValue retrieves a nested value from the context map.
// path is dot-separated: "item.id" retrieves ctx["item"]["id"]
func getNestedValue(ctx map[string]interface{}, path string) interface{} {
	if path == "" {
		return nil
	}

	parts := strings.Split(path, ".")
	var current interface{} = ctx

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]any:
			current = v[part]
		default:
			return nil
		}
		if current == nil {
			return nil
		}
	}

	return current
}

// setNestedValue sets a value at a nested path in the result map.
// Creates intermediate maps as needed.
// path is dot-separated: "item.id" with value "123" creates {"item": {"id": "123"}}
func setNestedValue(result map[string]interface{}, path string, value interface{}) {
	if path == "" || value == nil {
		return
	}

	parts := strings.Split(path, ".")

	// For a single part, just set directly
	if len(parts) == 1 {
		result[parts[0]] = value
		return
	}

	// For nested paths, create intermediate maps
	current := result
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		if existing, ok := current[part]; ok {
			if m, ok := existing.(map[string]interface{}); ok {
				current = m
			} else {
				// Conflict - existing value is not a map
				// Create new map and override
				newMap := make(map[string]interface{})
				current[part] = newMap
				current = newMap
			}
		} else {
			newMap := make(map[string]interface{})
			current[part] = newMap
			current = newMap
		}
	}

	// Set the final value
	current[parts[len(parts)-1]] = value
}

// buildSelectiveContext builds a minimal context containing only the required fields.
// This is more efficient than copying the entire context tree.
func buildSelectiveContext(required []string, fullCtx map[string]*Context, vars map[string]any) map[string]interface{} {
	result := make(map[string]interface{}, len(required))

	// Build a flat view of all context data for lookup
	flatCtx := make(map[string]interface{})
	for key, ctx := range fullCtx {
		flatCtx[key] = ctx.Data

		// Also spread map data for direct field access
		if dataMap, ok := ctx.Data.(map[string]interface{}); ok {
			for k, v := range dataMap {
				if _, exists := flatCtx[k]; !exists {
					flatCtx[k] = v
				}
			}
		}
	}

	// Add runtime variables (highest priority)
	for k, v := range vars {
		flatCtx[k] = v
	}

	// Extract only required fields
	for _, field := range required {
		// First part of the path is the top-level key
		parts := strings.Split(field, ".")
		topKey := parts[0]

		if val, ok := flatCtx[topKey]; ok {
			if len(parts) == 1 {
				// Single-level field
				result[topKey] = deepCopyValue(val)
			} else {
				// Nested field - get the nested value
				nestedVal := getNestedValue(flatCtx, field)
				if nestedVal != nil {
					setNestedValue(result, field, deepCopyValue(nestedVal))
				}
			}
		}
	}

	return result
}
