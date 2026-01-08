// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	crawler_testing "github.com/noi-techpark/go-silky/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVariablesInURLTemplate tests that runtime variables are accessible in URL templates
func TestVariablesInURLTemplate(t *testing.T) {
	// The URL contains {{ .apiKey }} and {{ .environment }} which must resolve from vars
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/data?apiKey=secret123&env=production": "testdata/crawler/variables/response.json",
	})

	craw, _, err := NewApiCrawler("testdata/crawler/variables/config.yaml")
	require.Nil(t, err)

	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	// Pass runtime variables
	vars := map[string]any{
		"apiKey":      "secret123",
		"environment": "production",
	}

	err = craw.Run(context.TODO(), vars)
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/variables/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

// TestVariablesInJQExpression tests that variables are accessible as $ctx.varName in jq
func TestVariablesInJQExpression(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/items": "testdata/crawler/variables/response_jq.json",
	})

	craw, _, err := NewApiCrawler("testdata/crawler/variables/config_jq.yaml")
	require.Nil(t, err)

	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	// Pass limit variable - jq will use $ctx.limit
	vars := map[string]any{
		"limit": 2,
	}

	err = craw.Run(context.TODO(), vars)
	require.Nil(t, err)

	data := craw.GetData()

	// Expect only first 2 items due to limit
	expected := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"id": float64(1), "name": "Item 1"},
			map[string]interface{}{"id": float64(2), "name": "Item 2"},
		},
	}

	assert.Equal(t, expected, data)
}

// TestVariablesNil tests that nil variables work correctly (no vars passed)
func TestVariablesNil(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=1": "testdata/crawler/example_foreach_value/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2": "testdata/crawler/example_foreach_value/facilities_2.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/example_foreach_value.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	// Pass nil for vars - should work fine
	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	assert.NotNil(t, data)
}

// TestVariablesEmpty tests that empty variables map works correctly
func TestVariablesEmpty(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=1": "testdata/crawler/example_foreach_value/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2": "testdata/crawler/example_foreach_value/facilities_2.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/example_foreach_value.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	// Pass empty map - should work fine
	err := craw.Run(context.TODO(), map[string]any{})
	require.Nil(t, err)

	data := craw.GetData()
	assert.NotNil(t, data)
}

// TestVariablesPrecedence tests that variables override context values
func TestVariablesPrecedence(t *testing.T) {
	// Test that variables have highest priority in template context
	// by verifying they override values that might exist in root context
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/data?apiKey=fromVars&env=production": "testdata/crawler/variables/response.json",
	})

	craw, _, err := NewApiCrawler("testdata/crawler/variables/config.yaml")
	require.Nil(t, err)

	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	// Variables should be used even if rootContext had conflicting keys
	vars := map[string]any{
		"apiKey":      "fromVars",
		"environment": "production",
	}

	err = craw.Run(context.TODO(), vars)
	require.Nil(t, err)

	data := craw.GetData()
	assert.NotNil(t, data)
}

// TestVariablesComplex tests nested/complex variable values
func TestVariablesComplex(t *testing.T) {
	// Verify complex nested variables work correctly
	vars := map[string]any{
		"auth": map[string]any{
			"user": "admin",
			"pass": "secret",
		},
		"config": map[string]any{
			"timeout": 30,
			"retries": 3,
		},
	}

	// Create a minimal crawler to test the contextMapToTemplate method
	craw, _, err := NewApiCrawler("testdata/crawler/variables/config.yaml")
	require.Nil(t, err)

	// Set up test context map
	contextMap := map[string]*Context{
		"root": {
			Data: map[string]interface{}{},
		},
	}

	result := craw.contextMapToTemplate(contextMap, vars)

	// Verify complex vars are accessible
	auth, ok := result["auth"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "admin", auth["user"])
	assert.Equal(t, "secret", auth["pass"])

	config, ok := result["config"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 30, config["timeout"])
	assert.Equal(t, 3, config["retries"])
}

// headerBodyCapturingTransport captures the request headers and body for verification
type headerBodyCapturingTransport struct {
	capturedHeaders http.Header
	capturedBody    map[string]any
}

func (t *headerBodyCapturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Capture headers
	t.capturedHeaders = req.Header.Clone()

	// Capture body
	if req.Body != nil {
		bodyBytes, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes)) // Reset for potential re-reads

		var body map[string]any
		json.Unmarshal(bodyBytes, &body)
		t.capturedBody = body
	}

	// Return a mock response
	responseBody := `{"success": true, "message": "Received"}`
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(responseBody))),
		Header:     make(http.Header),
	}, nil
}

// TestVariablesInHeaders tests that runtime variables are expanded in headers
func TestVariablesInHeaders(t *testing.T) {
	transport := &headerBodyCapturingTransport{}

	craw, _, err := NewApiCrawler("testdata/crawler/variables/config_headers_body.yaml")
	require.Nil(t, err)

	client := &http.Client{Transport: transport}
	craw.SetClient(client)

	vars := map[string]any{
		"token":      "my-secret-token",
		"tenantId":   "tenant-123",
		"searchTerm": "test query",
		"category":   "books",
		"deepValue":  "nested-value",
	}

	err = craw.Run(context.TODO(), vars)
	require.Nil(t, err)

	// Verify headers were templated correctly (using Get which canonicalizes)
	assert.Equal(t, "Bearer my-secret-token", transport.capturedHeaders.Get("Authorization"))
	assert.Equal(t, "tenant-123", transport.capturedHeaders.Get("X-Tenant-Id"))
	assert.Equal(t, "no-template", transport.capturedHeaders.Get("X-Static"))

	// Verify exact header casing is preserved (direct map access)
	assert.NotNil(t, transport.capturedHeaders["X-Tenant-Id"], "Header casing should be preserved as X-Tenant-Id")
	assert.NotNil(t, transport.capturedHeaders["X-Static"], "Header casing should be preserved as X-Static")
}

// TestVariablesHeaderCasingPreserved tests that exact header casing is preserved (e.g., X-API-KEY not X-Api-Key)
func TestVariablesHeaderCasingPreserved(t *testing.T) {
	transport := &headerBodyCapturingTransport{}

	craw, _, err := NewApiCrawler("testdata/crawler/variables/config_headers_allcaps.yaml")
	require.Nil(t, err)

	client := &http.Client{Transport: transport}
	craw.SetClient(client)

	vars := map[string]any{
		"apiKey": "secret-key-123",
	}

	err = craw.Run(context.TODO(), vars)
	require.Nil(t, err)

	// Verify exact header casing is preserved (all caps)
	assert.NotNil(t, transport.capturedHeaders["X-API-KEY"], "Header X-API-KEY should preserve exact casing")
	assert.Equal(t, []string{"secret-key-123"}, transport.capturedHeaders["X-API-KEY"])

	assert.NotNil(t, transport.capturedHeaders["X-CUSTOM-HEADER"], "Header X-CUSTOM-HEADER should preserve exact casing")
	assert.Equal(t, []string{"static-value"}, transport.capturedHeaders["X-CUSTOM-HEADER"])

	// Verify canonicalized versions don't exist
	assert.Nil(t, transport.capturedHeaders["X-Api-Key"], "Should not have canonicalized X-Api-Key")
	assert.Nil(t, transport.capturedHeaders["X-Custom-Header"], "Should not have canonicalized X-Custom-Header")
}

// TestVariablesLargeNumbers tests that large numbers are rendered without scientific notation
func TestVariablesLargeNumbers(t *testing.T) {
	transport := &headerBodyCapturingTransport{}

	craw, _, err := NewApiCrawler("testdata/crawler/variables/config_headers_body.yaml")
	require.Nil(t, err)

	client := &http.Client{Transport: transport}
	craw.SetClient(client)

	// Large number that would normally render as scientific notation (1.00025e+08)
	vars := map[string]any{
		"token":      "test",
		"tenantId":   float64(100024999), // This is how JSON unmarshals large numbers
		"searchTerm": "test",
		"category":   "test",
		"deepValue":  "test",
	}

	err = craw.Run(context.TODO(), vars)
	require.Nil(t, err)

	// Should render as "100024999" not "1.00025e+08" or "1.00025e&#43;08"
	assert.Equal(t, "100024999", transport.capturedHeaders.Get("X-Tenant-Id"))
}

// TestVariablesFloatWithDecimals tests that floats with decimals are rendered without scientific notation
func TestVariablesFloatWithDecimals(t *testing.T) {
	transport := &headerBodyCapturingTransport{}

	craw, _, err := NewApiCrawler("testdata/crawler/variables/config_headers_body.yaml")
	require.Nil(t, err)

	client := &http.Client{Transport: transport}
	craw.SetClient(client)

	vars := map[string]any{
		"token":      "test",
		"tenantId":   float64(0.000001), // Very small number that would be 1e-06
		"searchTerm": "test",
		"category":   "test",
		"deepValue":  "test",
	}

	err = craw.Run(context.TODO(), vars)
	require.Nil(t, err)

	// Should render as "0.000001" not "1e-06"
	assert.Equal(t, "0.000001", transport.capturedHeaders.Get("X-Tenant-Id"))
}

// TestVariablesInBody tests that runtime variables are expanded in body
func TestVariablesInBody(t *testing.T) {
	transport := &headerBodyCapturingTransport{}

	craw, _, err := NewApiCrawler("testdata/crawler/variables/config_headers_body.yaml")
	require.Nil(t, err)

	client := &http.Client{Transport: transport}
	craw.SetClient(client)

	vars := map[string]any{
		"token":      "my-secret-token",
		"tenantId":   "tenant-123",
		"searchTerm": "test query",
		"category":   "books",
		"deepValue":  "nested-value",
	}

	err = craw.Run(context.TODO(), vars)
	require.Nil(t, err)

	// Verify body was templated correctly
	require.NotNil(t, transport.capturedBody)

	// Top-level string field
	assert.Equal(t, "test query", transport.capturedBody["query"])

	// Nested object with templated value
	filters, ok := transport.capturedBody["filters"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "books", filters["category"])
	assert.Equal(t, float64(10), filters["limit"]) // Non-templated value preserved

	// Deeply nested
	nested, ok := transport.capturedBody["nested"].(map[string]any)
	require.True(t, ok)
	deep, ok := nested["deep"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "nested-value", deep["value"])
}
