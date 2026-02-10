// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky

import (
	"fmt"
	"net/url"
	"strings"
)

// QueryParamEncode percent-encodes a query parameter value per RFC 3986.
// Unlike url.QueryEscape (which uses application/x-www-form-urlencoded where
// space becomes '+'), this encodes space as '%20' and literal '+' as '%2B'.
func QueryParamEncode(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

// SetQueryParams appends query parameters to u using RFC 3986 encoding.
// Existing parameters in u.RawQuery are preserved exactly as-is — no
// decode/re-encode round-trip that would corrupt '+' signs.
func SetQueryParams(u *url.URL, params map[string]string) {
	if len(params) == 0 {
		return
	}

	var buf strings.Builder
	for k, v := range params {
		if buf.Len() > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(QueryParamEncode(k))
		buf.WriteByte('=')
		buf.WriteString(QueryParamEncode(v))
	}

	if u.RawQuery != "" {
		u.RawQuery += "&" + buf.String()
	} else {
		u.RawQuery = buf.String()
	}
}

// NormalizeRawQuery percent-encodes characters that are invalid in a URL query
// string (spaces, '#', control characters, etc.) while preserving everything
// that is valid — including literal '+' signs and existing '%XX' sequences.
//
// This operates directly on the raw query string WITHOUT a decode/re-encode
// round-trip, so '+' is never confused with space.
func NormalizeRawQuery(raw string) string {
	var buf strings.Builder
	buf.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c == '%' && i+2 < len(raw) && isHexChar(raw[i+1]) && isHexChar(raw[i+2]):
			// Already percent-encoded, preserve as-is
			buf.WriteByte(c)
			buf.WriteByte(raw[i+1])
			buf.WriteByte(raw[i+2])
			i += 2
		case c == ' ':
			buf.WriteString("%20")
		case shouldPercentEncode(c):
			fmt.Fprintf(&buf, "%%%02X", c)
		default:
			buf.WriteByte(c)
		}
	}
	return buf.String()
}

// NormalizeURL pre-encodes invalid characters in the query portion of a raw URL
// string, then parses it. This handles externally-sourced URLs (e.g., nextPageUrl
// from API responses) where query values may contain unencoded '#', '+', or spaces.
//
// The key insight: url.Parse treats '#' as a fragment separator, silently dropping
// everything after it from the query. By running NormalizeRawQuery on the query
// portion BEFORE url.Parse, we encode '#' → '%23' so it is preserved.
//
// Returns the original string unchanged if parsing fails.
func NormalizeURL(rawURL string) string {
	// Split at '?' to isolate the query portion before url.Parse sees it.
	// This prevents url.Parse from misinterpreting '#' in query values as
	// a fragment separator.
	if idx := strings.IndexByte(rawURL, '?'); idx >= 0 {
		base := rawURL[:idx]
		query := rawURL[idx+1:]
		rawURL = base + "?" + NormalizeRawQuery(query)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.String()
}

func isHexChar(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// shouldPercentEncode returns true for characters that must be percent-encoded
// in a URL query string. Characters allowed through unchanged:
//   - Unreserved: A-Z a-z 0-9 - . _ ~
//   - Sub-delimiters and pchar extras: : @ ! $ ' ( ) * , ;
//   - Query delimiters: + & =
//   - Allowed in query per RFC 3986: / ?
func shouldPercentEncode(c byte) bool {
	if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
		return false
	}
	switch c {
	case '-', '.', '_', '~': // unreserved
		return false
	case ':', '@', '!', '$', '\'', '(', ')', '*', ',', ';': // sub-delimiters + pchar extras
		return false
	case '+', '&', '=': // query-specific delimiters (preserve as-is)
		return false
	case '/', '?': // allowed in query component per RFC 3986
		return false
	}
	return true
}
