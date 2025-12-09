// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky_testing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// MockExpectation defines expected request and mock response
type MockExpectation struct {
	Request  MockRequest  `yaml:"request"`
	Response MockResponse `yaml:"response"`
}

// MockRequest defines expected request properties
type MockRequest struct {
	Method      string            `yaml:"method,omitempty"`
	URL         string            `yaml:"url"`
	Headers     map[string]string `yaml:"headers,omitempty"`
	Body        map[string]any    `yaml:"body,omitempty"`
	QueryParams map[string]string `yaml:"queryParams,omitempty"`
}

// MockResponse defines the mock response to return
type MockResponse struct {
	StatusCode int               `yaml:"statusCode,omitempty"`
	Headers    map[string]string `yaml:"headers,omitempty"`
	BodyFile   string            `yaml:"bodyFile,omitempty"`
	BodyJSON   any               `yaml:"bodyJSON,omitempty"`
}

// MockConfig contains all mock expectations
type MockConfig struct {
	Mocks []MockExpectation `yaml:"mocks"`
}

type MockRoundTripper struct {
	MockMap       map[string]string                                      // normalized URL => filepath (legacy)
	Expectations  []MockExpectation                                      // new validation-based mocks
	ValidateOnly  bool                                                   // if true, only validate without matching response
	Errors        []string                                               // validation errors
	InterceptFunc func(req *http.Request, resp *http.Response)          // function to intercept and modify responses
}

func NewMockRoundTripper(config map[string]string) *MockRoundTripper {
	return &MockRoundTripper{MockMap: normalizeMapKeys(config)}
}

func NewMockRoundTripperWithResponse(responses map[string]interface{}) *MockRoundTripper {
	expectations := make([]MockExpectation, 0)
	for url, body := range responses {
		expectations = append(expectations, MockExpectation{
			Request: MockRequest{
				URL: url,
			},
			Response: MockResponse{
				StatusCode: http.StatusOK,
				BodyJSON:   body,
			},
		})
	}
	return &MockRoundTripper{
		Expectations: expectations,
		Errors:       make([]string, 0),
	}
}

func NewMockRoundTripperFromYAML(yamlPath string) (*MockRoundTripper, error) {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read mock config: %w", err)
	}

	var config MockConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse mock config: %w", err)
	}

	return &MockRoundTripper{
		Expectations: config.Mocks,
		Errors:       make([]string, 0),
	}, nil
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// If we have expectations, try to match and validate
	if len(m.Expectations) > 0 {
		return m.roundTripWithExpectations(req)
	}

	// Legacy behavior: simple URL => file mapping
	normalized := normalizeURL(req.URL)

	filePath, ok := m.MockMap[normalized]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "mock not found"}`)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Request:    req,
		}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "failed to read mock"}`)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Request:    req,
		}, nil
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBuffer(data)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Request:    req,
	}

	// Call intercept function if set
	if m.InterceptFunc != nil {
		m.InterceptFunc(req, resp)
	}

	return resp, nil
}

func (m *MockRoundTripper) roundTripWithExpectations(req *http.Request) (*http.Response, error) {
	// Find matching expectation - try to match fully (URL + validation)
	var matchedExpectation *MockExpectation
	var validationErrors []string

	for i := range m.Expectations {
		exp := &m.Expectations[i]
		if !m.matchesURL(req, &exp.Request) {
			continue
		}

		// Try to validate - if validation passes, we found a match
		if err := m.validateRequest(req, &exp.Request); err == nil {
			matchedExpectation = exp
			break
		} else {
			// Store validation error for potential debugging
			validationErrors = append(validationErrors, fmt.Sprintf("Expectation %d: %s", i, err.Error()))
		}
	}

	if matchedExpectation == nil {
		var errMsg string
		if len(validationErrors) > 0 {
			// URL matched but validation failed
			errMsg = fmt.Sprintf("No matching expectation for %s %s. Validation errors: %v",
				req.Method, req.URL.String(), validationErrors)
		} else {
			// No URL match
			errMsg = fmt.Sprintf("No mock expectation found for %s %s", req.Method, req.URL.String())
		}
		m.Errors = append(m.Errors, errMsg)
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(bytes.NewBufferString(fmt.Sprintf(`{"error": "%s"}`, errMsg))),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Request:    req,
		}, nil
	}

	// Build response (validation already passed)
	return m.buildResponse(req, &matchedExpectation.Response)
}

func (m *MockRoundTripper) matchesURL(req *http.Request, expected *MockRequest) bool {
	expectedURL, err := url.Parse(expected.URL)
	if err != nil {
		return false
	}

	// Compare normalized URLs (without query params for now, we'll validate those separately)
	reqBase := req.URL.Scheme + "://" + req.URL.Host + strings.TrimRight(req.URL.Path, "/")
	expBase := expectedURL.Scheme + "://" + expectedURL.Host + strings.TrimRight(expectedURL.Path, "/")

	return reqBase == expBase
}

func (m *MockRoundTripper) validateRequest(req *http.Request, expected *MockRequest) error {
	// Validate method
	if expected.Method != "" && req.Method != expected.Method {
		return fmt.Errorf("method mismatch: expected %s, got %s", expected.Method, req.Method)
	}

	// Validate headers
	for key, expectedValue := range expected.Headers {
		actualValue := req.Header.Get(key)
		if actualValue != expectedValue {
			return fmt.Errorf("header %s mismatch: expected %q, got %q", key, expectedValue, actualValue)
		}
	}

	// Validate query params
	for key, expectedValue := range expected.QueryParams {
		actualValue := req.URL.Query().Get(key)
		if actualValue != expectedValue {
			return fmt.Errorf("query param %s mismatch: expected %q, got %q", key, expectedValue, actualValue)
		}
	}

	// Validate body if expected
	if len(expected.Body) > 0 {
		contentType := req.Header.Get("Content-Type")
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}
		// Restore body for potential re-reads
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		switch contentType {
		case "application/json":
			var actualBody map[string]any
			if err := json.Unmarshal(bodyBytes, &actualBody); err != nil {
				return fmt.Errorf("failed to parse JSON body: %w", err)
			}

			// Check if expected body fields are present
			for key, expectedValue := range expected.Body {
				actualValue, ok := actualBody[key]
				if !ok {
					return fmt.Errorf("body field %s missing", key)
				}
				if !deepEqual(expectedValue, actualValue) {
					return fmt.Errorf("body field %s mismatch: expected %v, got %v", key, expectedValue, actualValue)
				}
			}

		case "application/x-www-form-urlencoded":
			values, err := url.ParseQuery(string(bodyBytes))
			if err != nil {
				return fmt.Errorf("failed to parse form body: %w", err)
			}

			for key, expectedValue := range expected.Body {
				actualValue := values.Get(key)
				expectedStr := fmt.Sprintf("%v", expectedValue)
				if actualValue != expectedStr {
					return fmt.Errorf("form field %s mismatch: expected %v, got %v", key, expectedStr, actualValue)
				}
			}
		}
	}

	return nil
}

func (m *MockRoundTripper) buildResponse(req *http.Request, response *MockResponse) (*http.Response, error) {
	statusCode := response.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	// Build headers
	headers := http.Header{}
	for key, value := range response.Headers {
		headers.Set(key, value)
	}
	if headers.Get("Content-Type") == "" {
		headers.Set("Content-Type", "application/json")
	}

	// Build body
	var bodyData []byte
	var err error

	if response.BodyFile != "" {
		bodyData, err = os.ReadFile(response.BodyFile)
		if err != nil {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewBufferString(fmt.Sprintf(`{"error": "failed to read body file: %s"}`, err.Error()))),
				Header:     headers,
				Request:    req,
			}, nil
		}
	} else if response.BodyJSON != nil {
		bodyData, err = json.Marshal(response.BodyJSON)
		if err != nil {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewBufferString(fmt.Sprintf(`{"error": "failed to marshal body: %s"}`, err.Error()))),
				Header:     headers,
				Request:    req,
			}, nil
		}
	}

	resp := &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewBuffer(bodyData)),
		Header:     headers,
		Request:    req,
	}

	// Call intercept function if set
	if m.InterceptFunc != nil {
		m.InterceptFunc(req, resp)
	}

	return resp, nil
}

// deepEqual compares two values for equality (simplified version)
func deepEqual(a, b any) bool {
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}

// GetErrors returns all validation errors
func (m *MockRoundTripper) GetErrors() []string {
	return m.Errors
}

func normalizeMapKeys(input map[string]string) map[string]string {
	output := make(map[string]string)
	for raw, path := range input {
		parsed, err := url.Parse(raw)
		if err != nil {
			continue // or panic if strict
		}
		normalized := normalizeURL(parsed)
		output[normalized] = path
	}
	return output
}

// Normalize URL by sorting query params and stripping trailing slash
func normalizeURL(u *url.URL) string {
	base := u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/")
	params := u.Query()

	var sorted []string
	for k, vs := range params {
		for _, v := range vs {
			sorted = append(sorted, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	sort.Strings(sorted)

	if len(sorted) > 0 {
		return base + "?" + strings.Join(sorted, "&")
	}
	return base
}

// CreateResponseBody creates an io.ReadCloser from a string
func CreateResponseBody(body string) io.ReadCloser {
	return io.NopCloser(bytes.NewBufferString(body))
}
