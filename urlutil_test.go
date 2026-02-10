// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryParamEncode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"space becomes %20", "hello world", "hello%20world"},
		{"plus becomes %2B", "abc+def", "abc%2Bdef"},
		{"ampersand becomes %26", "a&b", "a%26b"},
		{"equals becomes %3D", "a=b", "a%3Db"},
		{"hash becomes %23", "abc#def", "abc%23def"},
		{"safe chars unchanged", "abcABC123-._~", "abcABC123-._~"},
		{"empty string", "", ""},
		{"unicode encoded", "caf\u00e9", "caf%C3%A9"},
		{"already-encoded-looking string is double-encoded", "%20", "%2520"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, QueryParamEncode(tt.input))
		})
	}
}

func TestSetQueryParams(t *testing.T) {
	t.Run("adds params to URL with no existing query", func(t *testing.T) {
		u, _ := url.Parse("https://api.example.com/items")
		SetQueryParams(u, map[string]string{"offset": "10"})
		assert.Equal(t, "offset=10", u.RawQuery)
	})

	t.Run("appends to URL with existing query", func(t *testing.T) {
		u, _ := url.Parse("https://api.example.com/items?format=json")
		SetQueryParams(u, map[string]string{"offset": "10"})
		assert.Contains(t, u.RawQuery, "format=json")
		assert.Contains(t, u.RawQuery, "offset=10")
	})

	t.Run("preserves literal + in existing query", func(t *testing.T) {
		u := &url.URL{
			Scheme:   "https",
			Host:     "api.example.com",
			Path:     "/items",
			RawQuery: "token=abc+def",
		}
		SetQueryParams(u, map[string]string{"page": "2"})
		assert.Contains(t, u.RawQuery, "token=abc+def")
		assert.Contains(t, u.RawQuery, "page=2")
	})

	t.Run("encodes special chars in new param values", func(t *testing.T) {
		u, _ := url.Parse("https://api.example.com/items")
		SetQueryParams(u, map[string]string{"q": "hello world"})
		assert.Equal(t, "q=hello%20world", u.RawQuery)
	})

	t.Run("empty params is no-op", func(t *testing.T) {
		u, _ := url.Parse("https://api.example.com/items?existing=yes")
		SetQueryParams(u, map[string]string{})
		assert.Equal(t, "existing=yes", u.RawQuery)
	})

	t.Run("nil params is no-op", func(t *testing.T) {
		u, _ := url.Parse("https://api.example.com/items?existing=yes")
		SetQueryParams(u, nil)
		assert.Equal(t, "existing=yes", u.RawQuery)
	})
}

func TestNormalizeRawQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"literal + preserved", "token=abc+def", "token=abc+def"},
		{"space encoded as %20", "q=hello world", "q=hello%20world"},
		{"# encoded as %23", "token=abc#def", "token=abc%23def"},
		{"existing %XX preserved", "token=abc%2Bdef", "token=abc%2Bdef"},
		{"mixed: + and space", "token=abc+def&q=hello world", "token=abc+def&q=hello%20world"},
		{"empty string", "", ""},
		{"already fully encoded", "a=1&b=%20&c=%23", "a=1&b=%20&c=%23"},
		{"base64-like with + and =", "cursor=dXNlcjE2Nw+xyz==", "cursor=dXNlcjE2Nw+xyz=="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, NormalizeRawQuery(tt.input))
		})
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "# in query value is preserved",
			input:    "https://api.example.com/items?token=abc#def",
			expected: "https://api.example.com/items?token=abc%23def",
		},
		{
			name:     "+ in query value is preserved",
			input:    "https://api.example.com/items?token=abc+def",
			expected: "https://api.example.com/items?token=abc+def",
		},
		{
			name:     "space in query value is encoded",
			input:    "https://api.example.com/items?q=hello world",
			expected: "https://api.example.com/items?q=hello%20world",
		},
		{
			name:     "# and + together in token",
			input:    "https://api.example.com/items?token=abc#def+xyz",
			expected: "https://api.example.com/items?token=abc%23def+xyz",
		},
		{
			name:     "no query params unchanged",
			input:    "https://api.example.com/items",
			expected: "https://api.example.com/items",
		},
		{
			name:     "already encoded stays unchanged",
			input:    "https://api.example.com/items?token=abc%23def%2Bxyz",
			expected: "https://api.example.com/items?token=abc%23def%2Bxyz",
		},
		{
			name:     "invalid URL returns original",
			input:    "://not-a-url",
			expected: "://not-a-url",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetQueryParamsPreservesHashInExistingQuery(t *testing.T) {
	// Simulate a URL that was already normalized (# → %23)
	// then verify SetQueryParams doesn't corrupt it
	u := &url.URL{
		Scheme:   "https",
		Host:     "api.example.com",
		Path:     "/items",
		RawQuery: "token=abc%23def+xyz",
	}
	SetQueryParams(u, map[string]string{"page": "2"})

	assert.Contains(t, u.RawQuery, "token=abc%23def+xyz")
	assert.Contains(t, u.RawQuery, "page=2")

	// Verify the full URL can be parsed back
	parsed, err := url.Parse(u.String())
	require.NoError(t, err)
	assert.Contains(t, parsed.RawQuery, "token=abc%23def+xyz")
}
