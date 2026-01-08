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


