// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	crawler_testing "github.com/noi-techpark/go-silky/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicAuthenticator(t *testing.T) {
	config := AuthenticatorConfig{
		Type:     "basic",
		Username: "testuser",
		Password: "testpass",
	}

	auth := NewAuthenticator(config, nil)
	req, _ := http.NewRequest("GET", "https://example.com", nil)

	err := auth.PrepareRequest(req, "")
	require.Nil(t, err)

	username, password, ok := req.BasicAuth()
	assert.True(t, ok)
	assert.Equal(t, "testuser", username)
	assert.Equal(t, "testpass", password)
}

func TestBearerAuthenticator(t *testing.T) {
	config := AuthenticatorConfig{
		Type:  "bearer",
		Token: "my-secret-token",
	}

	auth := NewAuthenticator(config, nil)
	req, _ := http.NewRequest("GET", "https://example.com", nil)

	err := auth.PrepareRequest(req, "")
	require.Nil(t, err)

	authHeader := req.Header.Get("Authorization")
	assert.Equal(t, "Bearer my-secret-token", authHeader)
}

func TestNoopAuthenticator(t *testing.T) {
	config := AuthenticatorConfig{
		Type: "",
	}

	auth := NewAuthenticator(config, nil)
	req, _ := http.NewRequest("GET", "https://example.com", nil)

	err := auth.PrepareRequest(req, "")
	require.Nil(t, err)

	// Should not add any authentication headers
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestCookieAuthenticator(t *testing.T) {
	// Setup mock HTTP client that returns a cookie
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/login": "testdata/auth/cookie_login_response.json",
	})

	// Mock needs to intercept and set cookies
	mockTransport.InterceptFunc = func(req *http.Request, resp *http.Response) {
		if req.URL.Path == "/login" {
			// Simulate cookie being set
			cookie := &http.Cookie{
				Name:  "session_id",
				Value: "abc123xyz",
			}
			resp.Header.Add("Set-Cookie", cookie.String())
		}
	}

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type: "cookie",
		LoginRequest: &RequestConfig{
			URL:    "https://api.example.com/login",
			Method: "POST",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: map[string]any{
				"username": "testuser",
				"password": "testpass",
			},
		},
		ExtractSelector: "session_id",
		MaxAgeSeconds:   3600,
	}

	auth := NewAuthenticator(config, client)

	// First request should perform login
	req1, _ := http.NewRequest("GET", "https://api.example.com/data", nil)
	err := auth.PrepareRequest(req1, "")
	require.Nil(t, err)

	// Check that cookie is set
	cookies := req1.Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "session_id", cookies[0].Name)
	assert.Equal(t, "abc123xyz", cookies[0].Value)

	// Second request should reuse the same cookie without login
	req2, _ := http.NewRequest("GET", "https://api.example.com/data2", nil)
	err = auth.PrepareRequest(req2, "")
	require.Nil(t, err)

	cookies2 := req2.Cookies()
	require.Len(t, cookies2, 1)
	assert.Equal(t, "session_id", cookies2[0].Name)
}

func TestJWTAuthenticatorFromBody(t *testing.T) {
	// Create login response with JWT in body
	loginResponse := map[string]interface{}{
		"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test",
		"user":  "testuser",
	}

	mockTransport := crawler_testing.NewMockRoundTripperWithResponse(map[string]interface{}{
		"https://api.example.com/login": loginResponse,
	})

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type: "jwt",
		LoginRequest: &RequestConfig{
			URL:    "https://api.example.com/login",
			Method: "POST",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: map[string]any{
				"username": "testuser",
				"password": "testpass",
			},
		},
		ExtractFrom:     "body",
		ExtractSelector: ".token",
		MaxAgeSeconds:   3600,
	}

	auth := NewAuthenticator(config, client)

	req, _ := http.NewRequest("GET", "https://api.example.com/data", nil)
	err := auth.PrepareRequest(req, "")
	require.Nil(t, err)

	authHeader := req.Header.Get("Authorization")
	assert.Equal(t, "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test", authHeader)
}

func TestJWTAuthenticatorFromHeader(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/login": "testdata/auth/jwt_login_response.json",
	})

	mockTransport.InterceptFunc = func(req *http.Request, resp *http.Response) {
		if req.URL.Path == "/login" {
			resp.Header.Set("X-Auth-Token", "jwt-token-from-header")
		}
	}

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type: "jwt",
		LoginRequest: &RequestConfig{
			URL:    "https://api.example.com/login",
			Method: "POST",
		},
		ExtractFrom:     "header",
		ExtractSelector: "X-Auth-Token",
	}

	auth := NewAuthenticator(config, client)

	req, _ := http.NewRequest("GET", "https://api.example.com/data", nil)
	err := auth.PrepareRequest(req, "")
	require.Nil(t, err)

	authHeader := req.Header.Get("Authorization")
	assert.Equal(t, "Bearer jwt-token-from-header", authHeader)
}

func TestJWTAuthenticatorRefresh(t *testing.T) {
	loginResponse := map[string]interface{}{
		"token": "initial-token",
	}

	mockTransport := crawler_testing.NewMockRoundTripperWithResponse(map[string]interface{}{
		"https://api.example.com/login": loginResponse,
	})

	loginCount := 0
	mockTransport.InterceptFunc = func(req *http.Request, resp *http.Response) {
		if req.URL.Path == "/login" {
			loginCount++
			// Change token on subsequent logins (after first)
			if loginCount > 1 {
				newBody := map[string]interface{}{
					"token": "refreshed-token",
				}
				bodyBytes, _ := json.Marshal(newBody)
				resp.Body = crawler_testing.CreateResponseBody(string(bodyBytes))
			}
		}
	}

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type: "jwt",
		LoginRequest: &RequestConfig{
			URL:    "https://api.example.com/login",
			Method: "POST",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: map[string]any{
				"username": "user",
				"password": "pass",
			},
		},
		ExtractFrom:     "body",
		ExtractSelector: ".token",
		MaxAgeSeconds:   1, // 1 second max age
	}

	auth := NewAuthenticator(config, client)

	// First request
	req1, _ := http.NewRequest("GET", "https://api.example.com/data", nil)
	err := auth.PrepareRequest(req1, "")
	require.Nil(t, err)
	assert.Contains(t, req1.Header.Get("Authorization"), "initial-token")

	// Wait for token to expire
	time.Sleep(2 * time.Second)

	// Second request should refresh
	req2, _ := http.NewRequest("GET", "https://api.example.com/data", nil)
	err = auth.PrepareRequest(req2, "")
	require.Nil(t, err)
	assert.Contains(t, req2.Header.Get("Authorization"), "refreshed-token")

	assert.Equal(t, 2, loginCount, "Should have logged in twice (initial + refresh)")
}

func TestCustomAuthenticatorCookieToHeader(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/login": "testdata/auth/custom_login_response.json",
	})

	mockTransport.InterceptFunc = func(req *http.Request, resp *http.Response) {
		if req.URL.Path == "/login" {
			cookie := &http.Cookie{
				Name:  "auth_cookie",
				Value: "cookie-value-123",
			}
			resp.Header.Add("Set-Cookie", cookie.String())
		}
	}

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type: "custom",
		LoginRequest: &RequestConfig{
			URL:    "https://api.example.com/login",
			Method: "POST",
		},
		ExtractFrom:     "cookie",
		ExtractSelector: "auth_cookie",
		InjectInto:      "header",
		InjectKey:       "X-Custom-Auth",
	}

	auth := NewAuthenticator(config, client)

	req, _ := http.NewRequest("GET", "https://api.example.com/data", nil)
	err := auth.PrepareRequest(req, "")
	require.Nil(t, err)

	authHeader := req.Header.Get("X-Custom-Auth")
	assert.Equal(t, "cookie-value-123", authHeader)
}

func TestCustomAuthenticatorBodyToquery(t *testing.T) {
	loginResponse := map[string]interface{}{
		"access_token": "query-param-token-456",
		"expires_in":   3600,
	}

	mockTransport := crawler_testing.NewMockRoundTripperWithResponse(map[string]interface{}{
		"https://api.example.com/auth": loginResponse,
	})

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type: "custom",
		LoginRequest: &RequestConfig{
			URL:    "https://api.example.com/auth",
			Method: "POST",
		},
		ExtractFrom:     "body",
		ExtractSelector: ".access_token",
		InjectInto:      "query",
		InjectKey:       "api_key",
	}

	auth := NewAuthenticator(config, client)

	req, _ := http.NewRequest("GET", "https://api.example.com/data", nil)
	err := auth.PrepareRequest(req, "")
	require.Nil(t, err)

	query := req.URL.Query().Get("api_key")
	assert.Equal(t, "query-param-token-456", query)
}

func TestCustomAuthenticatorHeaderToBearer(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/authenticate": "testdata/auth/custom_login_response.json",
	})

	mockTransport.InterceptFunc = func(req *http.Request, resp *http.Response) {
		if req.URL.Path == "/authenticate" {
			resp.Header.Set("Authorization", "Bearer header-token-789")
		}
	}

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type: "custom",
		LoginRequest: &RequestConfig{
			URL:    "https://api.example.com/authenticate",
			Method: "POST",
		},
		ExtractFrom:     "header",
		ExtractSelector: "Authorization",
		InjectInto:      "bearer",
	}

	auth := NewAuthenticator(config, client)

	req, _ := http.NewRequest("GET", "https://api.example.com/data", nil)
	err := auth.PrepareRequest(req, "")
	require.Nil(t, err)

	authHeader := req.Header.Get("Authorization")
	assert.Equal(t, "Bearer Bearer header-token-789", authHeader) // Note: doubled "Bearer" because we extract full header value
}

func TestCustomAuthenticatorCookieToCookie(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/login": "testdata/auth/custom_login_response.json",
	})

	mockTransport.InterceptFunc = func(req *http.Request, resp *http.Response) {
		if req.URL.Path == "/login" {
			cookie := &http.Cookie{
				Name:  "session",
				Value: "session-abc-123",
			}
			resp.Header.Add("Set-Cookie", cookie.String())
		}
	}

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type: "custom",
		LoginRequest: &RequestConfig{
			URL:    "https://api.example.com/login",
			Method: "POST",
		},
		ExtractFrom:     "cookie",
		ExtractSelector: "session",
		InjectInto:      "cookie",
		MaxAgeSeconds:   3600,
	}

	auth := NewAuthenticator(config, client)

	req, _ := http.NewRequest("GET", "https://api.example.com/data", nil)
	err := auth.PrepareRequest(req, "")
	require.Nil(t, err)

	cookies := req.Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "session", cookies[0].Name)
	assert.Equal(t, "session-abc-123", cookies[0].Value)
}

func TestOAuthAuthenticatorPasswordFlow(t *testing.T) {
	// Mock OAuth2 token endpoint
	mockTransport := crawler_testing.NewMockRoundTripperWithResponse(map[string]interface{}{
		"https://oauth.example.com/token": map[string]interface{}{
			"access_token": "oauth-access-token-123",
			"token_type":   "Bearer",
			"expires_in":   3600,
		},
	})

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type:     "oauth",
		Username: "testuser",
		Password: "testpass",
		OAuthConfig: OAuthConfig{
			Method:       "password",
			TokenURL:     "https://oauth.example.com/token",
			ClientID:     "client-id-123",
			ClientSecret: "client-secret-456",
			Scopes:       []string{"read", "write"},
		},
	}

	auth := NewAuthenticator(config, client)

	// First request should obtain token
	req1, _ := http.NewRequest("GET", "https://api.example.com/data", nil)
	err := auth.PrepareRequest(req1, "")
	require.Nil(t, err)

	authHeader := req1.Header.Get("Authorization")
	assert.Equal(t, "Bearer oauth-access-token-123", authHeader)

	// Second request should reuse cached token
	req2, _ := http.NewRequest("GET", "https://api.example.com/data2", nil)
	err = auth.PrepareRequest(req2, "")
	require.Nil(t, err)

	authHeader2 := req2.Header.Get("Authorization")
	assert.Equal(t, "Bearer oauth-access-token-123", authHeader2)
}

func TestOAuthAuthenticatorClientCredentialsFlow(t *testing.T) {
	// Mock OAuth2 token endpoint
	mockTransport := crawler_testing.NewMockRoundTripperWithResponse(map[string]interface{}{
		"https://oauth.example.com/token": map[string]interface{}{
			"access_token": "client-creds-token-789",
			"token_type":   "Bearer",
			"expires_in":   7200,
		},
	})

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type: "oauth",
		OAuthConfig: OAuthConfig{
			Method:       "client_credentials",
			TokenURL:     "https://oauth.example.com/token",
			ClientID:     "app-client-id",
			ClientSecret: "app-client-secret",
			Scopes:       []string{"api.read", "api.write"},
		},
	}

	auth := NewAuthenticator(config, client)

	req, _ := http.NewRequest("GET", "https://api.example.com/resource", nil)
	err := auth.PrepareRequest(req, "")
	require.Nil(t, err)

	authHeader := req.Header.Get("Authorization")
	assert.Equal(t, "Bearer client-creds-token-789", authHeader)
}

func TestOAuthAuthenticatorTokenRefresh(t *testing.T) {
	// Mock OAuth2 token endpoint with short-lived token
	tokenCount := 0
	mockTransport := crawler_testing.NewMockRoundTripperWithResponse(map[string]interface{}{
		"https://oauth.example.com/token": map[string]interface{}{
			"access_token": "initial-oauth-token",
			"token_type":   "Bearer",
			"expires_in":   1, // 1 second expiry
		},
	})

	mockTransport.InterceptFunc = func(req *http.Request, resp *http.Response) {
		if req.URL.Path == "/token" {
			tokenCount++
			if tokenCount > 1 {
				// Return refreshed token
				newBody := map[string]interface{}{
					"access_token": "refreshed-oauth-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				}
				bodyBytes, _ := json.Marshal(newBody)
				resp.Body = crawler_testing.CreateResponseBody(string(bodyBytes))
			}
		}
	}

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type:     "oauth",
		Username: "testuser",
		Password: "testpass",
		OAuthConfig: OAuthConfig{
			Method:       "password",
			TokenURL:     "https://oauth.example.com/token",
			ClientID:     "client-id",
			ClientSecret: "client-secret",
		},
	}

	auth := NewAuthenticator(config, client)

	// First request
	req1, _ := http.NewRequest("GET", "https://api.example.com/data", nil)
	err := auth.PrepareRequest(req1, "")
	require.Nil(t, err)
	assert.Contains(t, req1.Header.Get("Authorization"), "initial-oauth-token")

	// Wait for token to expire
	time.Sleep(2 * time.Second)

	// Second request should refresh token
	req2, _ := http.NewRequest("GET", "https://api.example.com/data", nil)
	err = auth.PrepareRequest(req2, "")
	require.Nil(t, err)
	assert.Contains(t, req2.Header.Get("Authorization"), "refreshed-oauth-token")

	assert.Equal(t, 2, tokenCount, "Should have fetched token twice (initial + refresh)")
}

func TestCustomAuthenticatorBodyInjectionJSON(t *testing.T) {
	loginResponse := map[string]interface{}{
		"api_key": "injected-token-123",
	}

	mockTransport := crawler_testing.NewMockRoundTripperWithResponse(map[string]interface{}{
		"https://api.example.com/auth": loginResponse,
	})

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type: "custom",
		LoginRequest: &RequestConfig{
			URL:    "https://api.example.com/auth",
			Method: "POST",
		},
		ExtractFrom:     "body",
		ExtractSelector: ".api_key",
		InjectInto:      "body",
		InjectKey:       "token",
	}

	auth := NewAuthenticator(config, client)

	// Create request with existing JSON body
	originalBody := map[string]any{"query": "test", "limit": float64(10)}
	bodyBytes, _ := json.Marshal(originalBody)
	req, _ := http.NewRequest("POST", "https://api.example.com/data", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	err := auth.PrepareRequest(req, "")
	require.Nil(t, err)

	// Read back the modified body
	resultBytes, err := io.ReadAll(req.Body)
	require.Nil(t, err)

	var resultBody map[string]any
	err = json.Unmarshal(resultBytes, &resultBody)
	require.Nil(t, err)

	// Original fields preserved
	assert.Equal(t, "test", resultBody["query"])
	assert.Equal(t, float64(10), resultBody["limit"])
	// Token injected
	assert.Equal(t, "injected-token-123", resultBody["token"])
	// Content-Type unchanged
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
}

func TestCustomAuthenticatorBodyInjectionFormEncoded(t *testing.T) {
	loginResponse := map[string]interface{}{
		"api_key": "form-token-456",
	}

	mockTransport := crawler_testing.NewMockRoundTripperWithResponse(map[string]interface{}{
		"https://api.example.com/auth": loginResponse,
	})

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type: "custom",
		LoginRequest: &RequestConfig{
			URL:    "https://api.example.com/auth",
			Method: "POST",
		},
		ExtractFrom:     "body",
		ExtractSelector: ".api_key",
		InjectInto:      "body",
		InjectKey:       "token",
	}

	auth := NewAuthenticator(config, client)

	// Create request with existing form-encoded body
	formData := url.Values{}
	formData.Set("query", "test")
	formData.Set("limit", "10")
	req, _ := http.NewRequest("POST", "https://api.example.com/data", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	err := auth.PrepareRequest(req, "")
	require.Nil(t, err)

	// Read back the modified body
	resultBytes, err := io.ReadAll(req.Body)
	require.Nil(t, err)

	resultValues, err := url.ParseQuery(string(resultBytes))
	require.Nil(t, err)

	// Original fields preserved
	assert.Equal(t, "test", resultValues.Get("query"))
	assert.Equal(t, "10", resultValues.Get("limit"))
	// Token injected
	assert.Equal(t, "form-token-456", resultValues.Get("token"))
}

func TestCustomAuthenticatorBodyInjectionNoExistingBody(t *testing.T) {
	loginResponse := map[string]interface{}{
		"api_key": "new-body-token-789",
	}

	mockTransport := crawler_testing.NewMockRoundTripperWithResponse(map[string]interface{}{
		"https://api.example.com/auth": loginResponse,
	})

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type: "custom",
		LoginRequest: &RequestConfig{
			URL:    "https://api.example.com/auth",
			Method: "POST",
		},
		ExtractFrom:     "body",
		ExtractSelector: ".api_key",
		InjectInto:      "body",
		InjectKey:       "token",
	}

	auth := NewAuthenticator(config, client)

	// Create request with NO body but with Content-Type set
	req, _ := http.NewRequest("POST", "https://api.example.com/data", nil)
	req.Header.Set("Content-Type", "application/json")

	err := auth.PrepareRequest(req, "")
	require.Nil(t, err)

	// Read back the new body
	resultBytes, err := io.ReadAll(req.Body)
	require.Nil(t, err)

	var resultBody map[string]any
	err = json.Unmarshal(resultBytes, &resultBody)
	require.Nil(t, err)

	// Token is the only field
	assert.Equal(t, "new-body-token-789", resultBody["token"])
	assert.Len(t, resultBody, 1)
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
}

func TestCustomAuthenticatorBodyInjectionNoContentType(t *testing.T) {
	loginResponse := map[string]interface{}{
		"api_key": "some-token",
	}

	mockTransport := crawler_testing.NewMockRoundTripperWithResponse(map[string]interface{}{
		"https://api.example.com/auth": loginResponse,
	})

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type: "custom",
		LoginRequest: &RequestConfig{
			URL:    "https://api.example.com/auth",
			Method: "POST",
		},
		ExtractFrom:     "body",
		ExtractSelector: ".api_key",
		InjectInto:      "body",
		InjectKey:       "token",
	}

	auth := NewAuthenticator(config, client)

	// Request with no body and no Content-Type → should error
	req, _ := http.NewRequest("POST", "https://api.example.com/data", nil)

	err := auth.PrepareRequest(req, "")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "Content-Type header is required")
}

func TestCustomAuthenticatorBodyInjectionEmptyToken(t *testing.T) {
	// Login returns empty token
	loginResponse := map[string]interface{}{
		"api_key": "",
	}

	mockTransport := crawler_testing.NewMockRoundTripperWithResponse(map[string]interface{}{
		"https://api.example.com/auth": loginResponse,
	})

	client := &http.Client{Transport: mockTransport}

	config := AuthenticatorConfig{
		Type: "custom",
		LoginRequest: &RequestConfig{
			URL:    "https://api.example.com/auth",
			Method: "POST",
		},
		ExtractFrom:     "body",
		ExtractSelector: ".api_key",
		InjectInto:      "body",
		InjectKey:       "token",
	}

	auth := NewAuthenticator(config, client)

	// Create request with existing JSON body
	originalBody := map[string]any{"query": "test"}
	bodyBytes, _ := json.Marshal(originalBody)
	req, _ := http.NewRequest("POST", "https://api.example.com/data", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	err := auth.PrepareRequest(req, "")
	require.Nil(t, err)

	// Body should NOT be modified (empty token = skip injection)
	resultBytes, err := io.ReadAll(req.Body)
	require.Nil(t, err)

	var resultBody map[string]any
	err = json.Unmarshal(resultBytes, &resultBody)
	require.Nil(t, err)

	// Only original field, no token injected
	assert.Equal(t, "test", resultBody["query"])
	assert.NotContains(t, resultBody, "token")
}
