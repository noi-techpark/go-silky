// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky

import (
	"os"
	"regexp"
)

// envPattern matches ${VAR} or ${VAR:-default}
var envPattern = regexp.MustCompile(`\$\{([^}:]+)(?::-([^}]*))?\}`)

// ExpandEnv expands environment variables in a string.
// Supports ${VAR} and ${VAR:-default} syntax.
// If VAR is not set or empty:
//   - With default (${VAR:-default}): returns the default value
//   - Without default (${VAR}): returns empty string
func ExpandEnv(s string) string {
	return envPattern.ReplaceAllStringFunc(s, func(match string) string {
		parts := envPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}

		varName := parts[1]
		hasDefault := len(parts) >= 3 && parts[2] != ""
		defaultVal := ""
		if hasDefault {
			defaultVal = parts[2]
		}

		if val := os.Getenv(varName); val != "" {
			return val
		}
		return defaultVal
	})
}
