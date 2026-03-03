// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky

import (
	"os"
	"testing"
)

func TestExpandEnv(t *testing.T) {
	os.Setenv("TEST_VAR", "test_value")
	defer os.Unsetenv("TEST_VAR")

	tests := []struct {
		input    string
		expected string
	}{
		// No expansion needed
		{"plain string", "plain string"},
		{"http://localhost:11434", "http://localhost:11434"},

		// Simple variable
		{"${TEST_VAR}", "test_value"},
		{"prefix_${TEST_VAR}_suffix", "prefix_test_value_suffix"},

		// With default - var exists
		{"${TEST_VAR:-default}", "test_value"},

		// With default - var doesn't exist
		{"${NONEXISTENT:-default_value}", "default_value"},
		{"${NONEXISTENT:-http://localhost:11434}", "http://localhost:11434"},

		// Empty default
		{"${NONEXISTENT:-}", ""},

		// Without default - var doesn't exist
		{"${NONEXISTENT}", ""},

		// Multiple variables
		{"${TEST_VAR}:${NONEXISTENT:-5432}", "test_value:5432"},

		// YAML-like content
		{"url: https://${TEST_VAR}/api", "url: https://test_value/api"},
		{"token: ${NONEXISTENT:-my-default-token}", "token: my-default-token"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ExpandEnv(tt.input)
			if result != tt.expected {
				t.Errorf("ExpandEnv(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
