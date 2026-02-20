// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/itchyny/gojq"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// getContentType retrieves the Content-Type from headers (case-insensitive)
func getContentType(headers map[string]string) string {
	if headers == nil {
		return ""
	}
	// Check for exact match first
	if ct, ok := headers["Content-Type"]; ok {
		return ct
	}
	// Case-insensitive search
	for key, value := range headers {
		if strings.ToLower(key) == "content-type" {
			return value
		}
	}
	return ""
}

// encodeRequestBody encodes a request body based on Content-Type header
// Returns nil reader if body is empty, error if Content-Type is missing or unsupported
func encodeRequestBody(headers map[string]string, body map[string]any) (*bytes.Reader, error) {
	if len(body) == 0 {
		return nil, nil
	}

	contentType := getContentType(headers)
	if contentType == "" {
		return nil, fmt.Errorf("Content-Type header is required when body is present")
	}

	switch contentType {
	case "application/json":
		bodyJSON, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("error encoding JSON body: %w", err)
		}
		return bytes.NewReader(bodyJSON), nil

	case "application/x-www-form-urlencoded":
		formData := url.Values{}
		for k, v := range body {
			formData.Set(k, fmt.Sprintf("%v", v))
		}
		return bytes.NewReader([]byte(formData.Encode())), nil

	default:
		return nil, fmt.Errorf("unsupported content type: %s", contentType)
	}
}

type Authenticator interface {
	PrepareRequest(req *http.Request, requestID string) error
	SetProfiler(profiler chan StepProfilerData)
}

// AuthProfiler is a helper for emitting authentication profiling events
type AuthProfiler struct {
	profiler chan StepProfilerData
	authType string
}

func (ap *AuthProfiler) emit(eventType ProfileEventType, name, requestID string, data map[string]any) string {
	if ap.profiler == nil {
		return ""
	}

	event := StepProfilerData{
		ID:        uuid.New().String(),
		ParentID:  requestID,
		Type:      eventType,
		Name:      name,
		Step:      Step{},
		Timestamp: time.Now(),
		Data:      data,
	}

	if event.Data == nil {
		event.Data = make(map[string]any)
	}
	event.Data["authType"] = ap.authType

	ap.profiler <- event
	return event.ID
}

func (ap *AuthProfiler) emitEnd(eventType ProfileEventType, name, id, parent string, start time.Time) string {
	if ap.profiler == nil {
		return ""
	}

	event := StepProfilerData{
		ID:        id,
		ParentID:  parent,
		Type:      eventType,
		Name:      name,
		Step:      Step{},
		Timestamp: time.Now(),
		Data:      map[string]any{},
		Duration:  time.Since(start).Microseconds(),
	}

	ap.profiler <- event
	return event.ID
}

type BaseAuthenticator struct {
	profiler *AuthProfiler
}

func (a *BaseAuthenticator) SetProfiler(profiler chan StepProfilerData) {
	a.profiler.profiler = profiler
}

func (a *BaseAuthenticator) GetProfiler() *AuthProfiler {
	return a.profiler
}

// NoopAuthenticator - no authentication
type NoopAuthenticator struct {
	*BaseAuthenticator
}

func (np NoopAuthenticator) PrepareRequest(req *http.Request, requestID string) error {
	return nil
}

type AuthenticatorConfig struct {
	Type string `yaml:"type,omitempty" json:"type,omitempty"` // basic | bearer | oauth | cookie | jwt | custom

	// Basic auth
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`

	// Bearer auth
	Token string `yaml:"token,omitempty" json:"token,omitempty"`

	// OAuth (inlined for backward compatibility)
	OAuthConfig `yaml:",inline" json:",inline"`

	// Cookie/JWT/Custom auth
	LoginRequest    *RequestConfig `yaml:"loginRequest,omitempty" json:"loginRequest,omitempty"`
	ExtractFrom     string         `yaml:"extractFrom,omitempty" json:"extractFrom,omitempty"`         // cookie | header | body
	ExtractSelector string         `yaml:"extractSelector,omitempty" json:"extractSelector,omitempty"` // jq for body, name for cookie/header
	InjectInto      string         `yaml:"injectInto,omitempty" json:"injectInto,omitempty"`           // cookie | header | bearer | body | query
	InjectKey       string         `yaml:"injectKey,omitempty" json:"injectKey,omitempty"`             // name for cookie/header/query/body field

	// Refresh settings
	MaxAgeSeconds int `yaml:"maxAgeSeconds,omitempty" json:"maxAgeSeconds,omitempty"` // 0 = no refresh
}

// BasicAuthenticator - HTTP Basic Authentication
type BasicAuthenticator struct {
	*BaseAuthenticator
	username string
	password string
}

func (a *BasicAuthenticator) PrepareRequest(req *http.Request, requestID string) error {
	authID := a.profiler.emit(EVENT_AUTH_START, "Basic Auth", requestID, map[string]any{
		"username": a.username,
		"password": maskToken(a.password),
	})
	startTime := time.Now()
	defer a.profiler.emitEnd(EVENT_AUTH_END, "Auth End", authID, requestID, startTime)

	req.SetBasicAuth(a.username, a.password)

	a.profiler.emit(EVENT_AUTH_TOKEN_INJECT, "Basic Auth Injected", requestID, map[string]any{
		"location": "Authorization header",
		"format":   "Basic",
	})

	a.profiler.emit(EVENT_AUTH_END, "Basic Auth Complete", requestID, nil)
	return nil
}

// BearerAuthenticator - Bearer token authentication
type BearerAuthenticator struct {
	*BaseAuthenticator
	token string
}

func (a *BearerAuthenticator) PrepareRequest(req *http.Request, requestID string) error {
	authID := a.profiler.emit(EVENT_AUTH_START, "Bearer Auth", requestID, nil)
	startTime := time.Now()
	defer a.profiler.emitEnd(EVENT_AUTH_END, "Auth End", authID, requestID, startTime)

	a.profiler.emit(EVENT_AUTH_CACHED, "Using Provided Token", requestID, map[string]any{
		"token": maskToken(a.token),
	})

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.token))

	a.profiler.emit(EVENT_AUTH_TOKEN_INJECT, "Bearer Token Injected", requestID, map[string]any{
		"location": "Authorization header",
		"format":   "Bearer",
		"token":    maskToken(a.token),
	})

	a.profiler.emit(EVENT_AUTH_END, "Bearer Auth Complete", requestID, nil)
	return nil
}

func (a *BearerAuthenticator) SetProfiler(profiler chan StepProfilerData) {
	a.profiler.profiler = profiler
}

// maskToken masks a token for display, showing only first and last 4 characters
func maskToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

type OAuthConfig struct {
	Method       string `yaml:"method,omitempty" json:"method,omitempty"` // password | client_credentials
	TokenURL     string `yaml:"tokenUrl,omitempty" json:"tokenUrl,omitempty"`
	ClientID     string `yaml:"clientId,omitempty" json:"clientId,omitempty"`
	ClientSecret string `yaml:"clientSecret,omitempty" json:"clientSecret,omitempty"`
	// usernam and password inherited from AuthenticatorConfig
	Scopes []string `yaml:"scopes,omitempty" json:"scopes,omitempty"`
}

// OAuthAuthenticator - OAuth2 authentication
type OAuthAuthenticator struct {
	*BaseAuthenticator
	conf        *oauth2.Config
	clientCreds *clientcredentials.Config
	token       *oauth2.Token
	mu          sync.Mutex
	username    string
	password    string
	method      string // password or client_credentials
	httpClient  HTTPClient
}

func (a *OAuthAuthenticator) PrepareRequest(req *http.Request, requestID string) error {
	authID := a.profiler.emit(EVENT_AUTH_START, "OAuth2 Auth", requestID, nil)
	startTime := time.Now()
	defer a.profiler.emitEnd(EVENT_AUTH_END, "Auth End", authID, requestID, startTime)

	token, fromCache, err := a.GetTokenWithCache(requestID)
	if err != nil {
		a.profiler.emit(EVENT_AUTH_END, "OAuth2 Auth Failed", requestID, map[string]any{
			"error": err.Error(),
		})
		return fmt.Errorf("could not get oauth token: %w", err)
	}

	if fromCache {
		a.profiler.emit(EVENT_AUTH_CACHED, "Using Cached OAuth Token", requestID, map[string]any{
			"token":  maskToken(token),
			"source": "cached",
		})
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	a.profiler.emit(EVENT_AUTH_TOKEN_INJECT, "OAuth Token Injected", requestID, map[string]any{
		"location": "Authorization header",
		"format":   "Bearer",
		"token":    maskToken(token),
	})

	a.profiler.emit(EVENT_AUTH_END, "OAuth2 Auth Complete", requestID, nil)
	return nil
}

// GetToken retrieves a valid access token (refreshing if necessary)
func (a *OAuthAuthenticator) GetToken(requestID string) (string, error) {
	token, _, err := a.GetTokenWithCache(requestID)
	return token, err
}

// GetTokenWithCache retrieves a valid access token and returns whether it was cached
func (a *OAuthAuthenticator) GetTokenWithCache(requestID string) (string, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Create context with HTTP client for OAuth2 library
	ctx := context.Background()
	if a.httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, a.httpClient)
	}

	// If token exists and is still valid, return it as cached
	if a.token != nil && a.token.Valid() {
		return a.token.AccessToken, true, nil
	}

	// Need to fetch new token - emit login start event
	loginID := ""
	if a.method == "password" {
		loginID = a.profiler.emit(EVENT_AUTH_LOGIN_START, "OAuth2 Login Request", requestID, map[string]any{
			"method":   a.method,
			"username": a.username,
		})
	} else if a.method == "client_credentials" && a.clientCreds != nil {
		loginID = a.profiler.emit(EVENT_AUTH_LOGIN_START, "OAuth2 Login Request", requestID, map[string]any{
			"method":   a.method,
			"clientId": a.clientCreds.ClientID,
		})
	}
	startTime := time.Now()
	defer a.profiler.emitEnd(EVENT_AUTH_LOGIN_END, "Login End", loginID, requestID, startTime)

	// Fetch new token
	var token *oauth2.Token
	var err error

	if a.conf != nil { // Password flow
		token, err = a.conf.PasswordCredentialsToken(ctx, a.username, a.password)
	} else { // Client Credentials flow
		token, err = a.clientCreds.Token(ctx)
	}

	// Emit login end event
	endData := map[string]any{
		"method": a.method,
	}
	if err != nil {
		endData["error"] = err.Error()
	} else {
		endData["token"] = maskToken(token.AccessToken)
		if !token.Expiry.IsZero() {
			endData["expiresAt"] = token.Expiry.Format(time.RFC3339)
		}
	}
	a.profiler.emit(EVENT_AUTH_TOKEN_EXTRACT, "OAuth2 Login Complete", loginID, endData)

	if err != nil {
		return "", false, err
	}

	// Store new token
	a.token = token
	return token.AccessToken, false, nil
}

// CookieAuthenticator - performs login via POST, extracts cookie, injects it
type CookieAuthenticator struct {
	*BaseAuthenticator
	loginRequest  *RequestConfig
	cookieName    string
	cookie        *http.Cookie
	maxAge        time.Duration
	acquiredAt    time.Time
	authenticated bool
	mu            sync.Mutex
	httpClient    HTTPClient
}

func (a *CookieAuthenticator) PrepareRequest(req *http.Request, requestID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	authID := a.profiler.emit(EVENT_AUTH_START, "Cookie Auth", requestID, nil)
	startTime := time.Now()
	defer a.profiler.emitEnd(EVENT_AUTH_END, "Auth End", authID, requestID, startTime)

	// Check if we need to authenticate
	needsAuth := false
	if !a.authenticated {
		needsAuth = true
	} else if a.maxAge > 0 && time.Since(a.acquiredAt) > a.maxAge {
		needsAuth = true
	}

	if needsAuth {
		if err := a.performLogin(authID); err != nil {
			a.profiler.emit(EVENT_AUTH_END, "Cookie Auth Failed", authID, map[string]any{
				"error": err.Error(),
			})
			return fmt.Errorf("cookie authentication failed: %w", err)
		}
		a.authenticated = true
	} else {
		a.profiler.emit(EVENT_AUTH_CACHED, "Using Cached Cookie", authID, map[string]any{
			"cookieName": a.cookieName,
			"age":        time.Since(a.acquiredAt).String(),
		})
	}

	// Inject cookie
	if a.cookie != nil {
		req.AddCookie(a.cookie)
		a.profiler.emit(EVENT_AUTH_TOKEN_INJECT, "Cookie Injected", authID, map[string]any{
			"location":    "Cookie header",
			"cookieName":  a.cookieName,
			"cookieValue": maskToken(a.cookie.Value),
		})
	}

	a.profiler.emit(EVENT_AUTH_END, "Cookie Auth Complete", authID, nil)
	return nil
}

func (a *CookieAuthenticator) performLogin(requestID string) error {
	loginID := a.profiler.emit(EVENT_AUTH_LOGIN_START, "Cookie Login Request", requestID, map[string]any{
		"url":    a.loginRequest.URL,
		"method": a.loginRequest.Method,
	})
	startTime := time.Now()
	defer a.profiler.emitEnd(EVENT_AUTH_LOGIN_END, "Login End", loginID, requestID, startTime)

	// Build login request
	loginReq, err := a.buildLoginRequest()
	if err != nil {
		a.profiler.emit(EVENT_ERROR, "Cookie Login Failed", loginID, map[string]any{
			"error": err.Error(),
		})
		return err
	}

	// Execute login request
	resp, err := a.httpClient.Do(loginReq)
	if err != nil {
		a.profiler.emit(EVENT_ERROR, "Cookie Login Failed", loginID, map[string]any{
			"error": err.Error(),
		})
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		a.profiler.emit(EVENT_ERROR, "Cookie Login Failed", loginID, map[string]any{
			"error":      fmt.Sprintf("login request failed with status %d", resp.StatusCode),
			"statusCode": resp.StatusCode,
		})
		return fmt.Errorf("login request failed with status %d", resp.StatusCode)
	}

	// Extract cookie
	cookies := resp.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == a.cookieName {
			a.cookie = cookie
			a.acquiredAt = time.Now()

			a.profiler.emit(EVENT_AUTH_TOKEN_EXTRACT, "Cookie Extracted", loginID, map[string]any{
				"cookieName":  a.cookieName,
				"cookieValue": maskToken(cookie.Value),
			})

			return nil
		}
	}

	a.profiler.emit(EVENT_ERROR, "Cookie Login Failed", loginID, map[string]any{
		"authType": "cookie",
		"error":    fmt.Sprintf("cookie '%s' not found in login response", a.cookieName),
	})
	return fmt.Errorf("cookie '%s' not found in login response", a.cookieName)
}

func (a *CookieAuthenticator) buildLoginRequest() (*http.Request, error) {
	// Build URL
	urlStr := a.loginRequest.URL
	urlObj, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid login URL: %w", err)
	}

	// Build body
	bodyReader, err := encodeRequestBody(a.loginRequest.Headers, a.loginRequest.Body)
	if err != nil {
		return nil, err
	}

	// Create request
	var req *http.Request
	if bodyReader != nil {
		req, err = http.NewRequest(a.loginRequest.Method, urlObj.String(), bodyReader)
	} else {
		req, err = http.NewRequest(a.loginRequest.Method, urlObj.String(), nil)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create login request: %w", err)
	}

	// Set headers
	for k, v := range a.loginRequest.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

// JWTAuthenticator - performs login via POST, extracts JWT from response
type JWTAuthenticator struct {
	*BaseAuthenticator
	loginRequest    *RequestConfig
	extractFrom     string // header | body
	extractSelector string // jq expression for body, header name for header
	token           string
	maxAge          time.Duration
	acquiredAt      time.Time
	authenticated   bool
	mu              sync.Mutex
	httpClient      HTTPClient
	jqCache         map[string]*gojq.Code
}

func (a *JWTAuthenticator) PrepareRequest(req *http.Request, requestID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	startTime := time.Now()
	authID := a.profiler.emit(EVENT_AUTH_START, "JWT Auth", requestID, nil)
	defer a.profiler.emitEnd(EVENT_AUTH_END, "Auth End", authID, requestID, startTime)

	// Check if we need to authenticate
	needsAuth := false
	if !a.authenticated {
		needsAuth = true
	} else if a.maxAge > 0 && time.Since(a.acquiredAt) > a.maxAge {
		needsAuth = true
	}

	if needsAuth {
		if err := a.performLogin(requestID); err != nil {
			a.profiler.emit(EVENT_AUTH_END, "JWT Auth Failed", requestID, map[string]any{
				"error": err.Error(),
			})
			return fmt.Errorf("jwt authentication failed: %w", err)
		}
		a.authenticated = true
	} else {
		a.profiler.emit(EVENT_AUTH_CACHED, "Using Cached JWT Token", requestID, map[string]any{
			"token": maskToken(a.token),
			"age":   time.Since(a.acquiredAt).String(),
		})
	}

	// Inject token as Bearer
	if a.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.token))
		a.profiler.emit(EVENT_AUTH_TOKEN_INJECT, "JWT Token Injected", requestID, map[string]any{
			"location": "Authorization header",
			"format":   "Bearer",
			"token":    maskToken(a.token),
		})
	}

	a.profiler.emit(EVENT_AUTH_END, "JWT Auth Complete", requestID, nil)
	return nil
}

func (a *JWTAuthenticator) performLogin(requestID string) error {
	loginID := a.profiler.emit(EVENT_AUTH_LOGIN_START, "JWT Login Request", requestID, map[string]any{
		"url":    a.loginRequest.URL,
		"method": a.loginRequest.Method,
	})

	startTime := time.Now()
	defer a.profiler.emitEnd(EVENT_AUTH_LOGIN_END, "Login End", loginID, requestID, startTime)

	// Build login request
	loginReq, err := a.buildLoginRequest()
	if err != nil {
		a.profiler.emit(EVENT_ERROR, "JWT Login Failed", loginID, map[string]any{
			"error": err.Error(),
		})
		return err
	}

	// Execute login request
	resp, err := a.httpClient.Do(loginReq)
	if err != nil {
		a.profiler.emit(EVENT_ERROR, "JWT Login Failed", loginID, map[string]any{
			"error": err.Error(),
		})
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		a.profiler.emit(EVENT_ERROR, "JWT Login Failed", loginID, map[string]any{
			"error": err.Error(),
		})
		return fmt.Errorf("login request failed with status %d", resp.StatusCode)
	}

	// Extract token
	token, err := a.extractToken(resp)
	if err != nil {
		a.profiler.emit(EVENT_ERROR, "JWT Login Failed", loginID, map[string]any{
			"error": err.Error(),
		})
		return err
	}

	a.token = token
	a.acquiredAt = time.Now()

	a.profiler.emit(EVENT_AUTH_TOKEN_EXTRACT, "JWT Token Extracted", requestID, map[string]any{
		"extractFrom":     a.extractFrom,
		"extractSelector": a.extractSelector,
		"token":           maskToken(token),
	})
	return nil
}

func (a *JWTAuthenticator) extractToken(resp *http.Response) (string, error) {
	switch a.extractFrom {
	case "header":
		token := resp.Header.Get(a.extractSelector)
		if token == "" {
			return "", fmt.Errorf("header '%s' not found in login response", a.extractSelector)
		}
		return token, nil

	case "body":
		var body interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return "", fmt.Errorf("failed to decode login response: %w", err)
		}

		// Compile or get cached jq expression
		code, err := a.getOrCompileJQ(a.extractSelector)
		if err != nil {
			return "", err
		}

		// Execute jq expression
		iter := code.Run(body)
		v, ok := iter.Next()
		if !ok {
			return "", fmt.Errorf("jq selector '%s' yielded no results", a.extractSelector)
		}
		if err, isErr := v.(error); isErr {
			return "", fmt.Errorf("jq error: %w", err)
		}

		// Convert to string
		token, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("token extracted is not a string: %T", v)
		}
		return token, nil

	default:
		return "", fmt.Errorf("unsupported extractFrom: %s", a.extractFrom)
	}
}

func (a *JWTAuthenticator) getOrCompileJQ(expression string) (*gojq.Code, error) {
	if code, ok := a.jqCache[expression]; ok {
		return code, nil
	}

	query, err := gojq.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression '%s': %w", expression, err)
	}

	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("failed to compile jq expression: %w", err)
	}

	a.jqCache[expression] = code
	return code, nil
}

func (a *JWTAuthenticator) buildLoginRequest() (*http.Request, error) {
	// Build URL
	urlStr := a.loginRequest.URL
	urlObj, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid login URL: %w", err)
	}

	// Build body
	bodyReader, err := encodeRequestBody(a.loginRequest.Headers, a.loginRequest.Body)
	if err != nil {
		return nil, err
	}

	// Create request
	var req *http.Request
	if bodyReader != nil {
		req, err = http.NewRequest(a.loginRequest.Method, urlObj.String(), bodyReader)
	} else {
		req, err = http.NewRequest(a.loginRequest.Method, urlObj.String(), nil)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create login request: %w", err)
	}

	// Set headers
	for k, v := range a.loginRequest.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

// CustomAuthenticator - fully configurable authenticator
type CustomAuthenticator struct {
	*BaseAuthenticator
	loginRequest    *RequestConfig
	extractFrom     string // cookie | header | body
	extractSelector string
	injectInto      string // cookie | header | bearer | body | query
	injectKey       string
	token           string
	cookie          *http.Cookie
	maxAge          time.Duration
	acquiredAt      time.Time
	authenticated   bool
	mu              sync.Mutex
	httpClient      HTTPClient
	jqCache         map[string]*gojq.Code
}

func (a *CustomAuthenticator) PrepareRequest(req *http.Request, requestID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	authID := a.profiler.emit(EVENT_AUTH_START, "Custom Auth", requestID, map[string]any{
		"injectInto":  a.injectInto,
		"extractFrom": a.extractFrom,
	})
	startTime := time.Now()
	defer a.profiler.emitEnd(EVENT_AUTH_END, "Auth End", authID, requestID, startTime)

	// Check if we need to authenticate
	needsAuth := false
	if !a.authenticated {
		needsAuth = true
	} else if a.maxAge > 0 && time.Since(a.acquiredAt) > a.maxAge {
		needsAuth = true
	}

	if needsAuth {
		if err := a.performLogin(requestID); err != nil {
			a.profiler.emit(EVENT_AUTH_END, "Custom Auth Failed", requestID, map[string]any{
				"error": err.Error(),
			})
			return fmt.Errorf("custom authentication failed: %w", err)
		}
		a.authenticated = true
	} else {
		cacheData := map[string]any{
			"age": time.Since(a.acquiredAt).String(),
		}
		if a.token != "" {
			cacheData["token"] = maskToken(a.token)
		}
		if a.cookie != nil {
			cacheData["cookieName"] = a.cookie.Name
			cacheData["cookieValue"] = maskToken(a.cookie.Value)
		}
		a.profiler.emit(EVENT_AUTH_CACHED, "Using Cached Credential", requestID, cacheData)
	}

	// Inject token/cookie based on injectInto
	switch a.injectInto {
	case "cookie":
		if a.cookie != nil {
			req.AddCookie(a.cookie)
			a.profiler.emit(EVENT_AUTH_TOKEN_INJECT, "Credential Injected", requestID, map[string]any{
				"location":    "Cookie header",
				"cookieName":  a.cookie.Name,
				"cookieValue": maskToken(a.cookie.Value),
			})
		}
	case "header":
		if a.token != "" {
			req.Header.Set(a.injectKey, a.token)
			a.profiler.emit(EVENT_AUTH_TOKEN_INJECT, "Credential Injected", requestID, map[string]any{
				"location":  "Header",
				"headerKey": a.injectKey,
				"token":     maskToken(a.token),
			})
		}
	case "bearer":
		if a.token != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.token))
			a.profiler.emit(EVENT_AUTH_TOKEN_INJECT, "Credential Injected", requestID, map[string]any{
				"location": "Authorization header",
				"format":   "Bearer",
				"token":    maskToken(a.token),
			})
		}
	case "query":
		if a.token != "" {
			SetQueryParams(req.URL, map[string]string{a.injectKey: a.token})
			a.profiler.emit(EVENT_AUTH_TOKEN_INJECT, "Credential Injected", requestID, map[string]any{
				"location": "Query parameter",
				"queryKey": a.injectKey,
				"token":    maskToken(a.token),
			})
		}
	case "body":
		if a.token != "" {
			if err := a.injectTokenIntoBody(req); err != nil {
				a.profiler.emit(EVENT_AUTH_END, "Custom Auth Failed", requestID, map[string]any{
					"error": err.Error(),
				})
				return fmt.Errorf("body injection failed: %w", err)
			}
			a.profiler.emit(EVENT_AUTH_TOKEN_INJECT, "Credential Injected", requestID, map[string]any{
				"location": "Request body",
				"bodyKey":  a.injectKey,
				"token":    maskToken(a.token),
			})
		}
	default:
		a.profiler.emit(EVENT_AUTH_END, "Custom Auth Failed", requestID, map[string]any{
			"error": fmt.Sprintf("unsupported injectInto: %s", a.injectInto),
		})
		return fmt.Errorf("unsupported injectInto: %s", a.injectInto)
	}

	a.profiler.emit(EVENT_AUTH_END, "Custom Auth Complete", requestID, nil)
	return nil
}

func (a *CustomAuthenticator) performLogin(requestID string) error {
	loginID := a.profiler.emit(EVENT_AUTH_LOGIN_START, "Custom Login Request", requestID, map[string]any{
		"url":    a.loginRequest.URL,
		"method": a.loginRequest.Method,
	})
	startTime := time.Now()
	defer a.profiler.emitEnd(EVENT_AUTH_LOGIN_END, "Login End", loginID, requestID, startTime)

	// Build login request
	loginReq, err := a.buildLoginRequest()
	if err != nil {
		a.profiler.emit(EVENT_AUTH_LOGIN_END, "Custom Login Failed", requestID, map[string]any{
			"error": err.Error(),
		})
		return err
	}

	// Execute login request
	resp, err := a.httpClient.Do(loginReq)
	if err != nil {
		a.profiler.emit(EVENT_ERROR, "Custom Login Failed", loginID, map[string]any{
			"error": err.Error(),
		})
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		a.profiler.emit(EVENT_ERROR, "Custom Login Failed", loginID, map[string]any{
			"error":      fmt.Sprintf("login request failed with status %d", resp.StatusCode),
			"statusCode": resp.StatusCode,
		})
		return fmt.Errorf("login request failed with status %d", resp.StatusCode)
	}

	// Extract token/cookie
	if err := a.extractCredential(resp, loginID); err != nil {
		a.profiler.emit(EVENT_ERROR, "Custom Login Failed", loginID, map[string]any{
			"error": err.Error(),
		})
		return err
	}

	a.acquiredAt = time.Now()
	return nil
}

func (a *CustomAuthenticator) extractCredential(resp *http.Response, requestID string) error {
	switch a.extractFrom {
	case "cookie":
		cookies := resp.Cookies()
		for _, cookie := range cookies {
			if cookie.Name == a.extractSelector {
				a.cookie = cookie
				// If we're not injecting as cookie, store value as token
				if a.injectInto != "cookie" {
					a.token = cookie.Value
				}
				a.profiler.emit(EVENT_AUTH_TOKEN_EXTRACT, "Credential Extracted from Cookie", requestID, map[string]any{
					"cookieName":  a.extractSelector,
					"cookieValue": maskToken(cookie.Value),
				})
				return nil
			}
		}
		return fmt.Errorf("cookie '%s' not found in login response", a.extractSelector)

	case "header":
		token := resp.Header.Get(a.extractSelector)
		if token == "" {
			return fmt.Errorf("header '%s' not found in login response", a.extractSelector)
		}
		a.token = token
		a.profiler.emit(EVENT_AUTH_TOKEN_EXTRACT, "Credential Extracted from Header", requestID, map[string]any{
			"headerName": a.extractSelector,
			"token":      maskToken(token),
		})
		return nil

	case "body":
		var body interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return fmt.Errorf("failed to decode login response: %w", err)
		}

		// Compile or get cached jq expression
		code, err := a.getOrCompileJQ(a.extractSelector)
		if err != nil {
			return err
		}

		// Execute jq expression
		iter := code.Run(body)
		v, ok := iter.Next()
		if !ok {
			return fmt.Errorf("jq selector '%s' yielded no results", a.extractSelector)
		}
		if err, isErr := v.(error); isErr {
			return fmt.Errorf("jq error: %w", err)
		}

		// Convert to string
		token, ok := v.(string)
		if !ok {
			return fmt.Errorf("token extracted is not a string: %T", v)
		}
		a.token = token
		a.profiler.emit(EVENT_AUTH_TOKEN_EXTRACT, "Credential Extracted from Body", requestID, map[string]any{
			"jqSelector": a.extractSelector,
			"token":      maskToken(token),
		})
		return nil

	default:
		return fmt.Errorf("unsupported extractFrom: %s", a.extractFrom)
	}
}

// injectTokenIntoBody reads the existing request body (if any), injects the auth token
// as a new field, re-encodes it, and replaces req.Body.
func (a *CustomAuthenticator) injectTokenIntoBody(req *http.Request) error {
	contentType := req.Header.Get("Content-Type")
	bodyMap := make(map[string]any)

	if contentType == "" {
		return fmt.Errorf("cannot inject into body: Content-Type header is required when body is present")
	}

	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}

		if len(bodyBytes) > 0 {
			switch contentType {
			case "application/json":
				if err := json.Unmarshal(bodyBytes, &bodyMap); err != nil {
					return fmt.Errorf("failed to decode JSON body: %w", err)
				}
			case "application/x-www-form-urlencoded":
				values, err := url.ParseQuery(string(bodyBytes))
				if err != nil {
					return fmt.Errorf("failed to parse form body: %w", err)
				}
				for k, v := range values {
					if len(v) == 1 {
						bodyMap[k] = v[0]
					} else {
						bodyMap[k] = v
					}
				}
			}
		}
	}

	// Inject the token
	bodyMap[a.injectKey] = a.token

	// Re-encode
	var newBytes []byte
	switch contentType {
	case "application/json":
		encoded, err := json.Marshal(bodyMap)
		if err != nil {
			return fmt.Errorf("failed to encode JSON body: %w", err)
		}
		newBytes = encoded
	case "application/x-www-form-urlencoded":
		formData := url.Values{}
		for k, v := range bodyMap {
			formData.Set(k, fmt.Sprintf("%v", v))
		}
		newBytes = []byte(formData.Encode())
	default:
		return fmt.Errorf("cannot inject into body with content type: %s", contentType)
	}

	req.Body = io.NopCloser(bytes.NewReader(newBytes))
	req.ContentLength = int64(len(newBytes))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(newBytes)), nil
	}

	return nil
}

func (a *CustomAuthenticator) getOrCompileJQ(expression string) (*gojq.Code, error) {
	if code, ok := a.jqCache[expression]; ok {
		return code, nil
	}

	query, err := gojq.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression '%s': %w", expression, err)
	}

	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("failed to compile jq expression: %w", err)
	}

	a.jqCache[expression] = code
	return code, nil
}

func (a *CustomAuthenticator) buildLoginRequest() (*http.Request, error) {
	// Build URL
	urlStr := a.loginRequest.URL
	urlObj, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid login URL: %w", err)
	}

	// Build body
	bodyReader, err := encodeRequestBody(a.loginRequest.Headers, a.loginRequest.Body)
	if err != nil {
		return nil, err
	}

	// Create request
	var req *http.Request
	if bodyReader != nil {
		req, err = http.NewRequest(a.loginRequest.Method, urlObj.String(), bodyReader)
	} else {
		req, err = http.NewRequest(a.loginRequest.Method, urlObj.String(), nil)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create login request: %w", err)
	}

	// Set headers
	for k, v := range a.loginRequest.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

// NewAuthenticator creates an authenticator based on the configuration
func NewAuthenticator(config AuthenticatorConfig, httpClient HTTPClient) Authenticator {
	if config.Type == "" {
		return &NoopAuthenticator{
			BaseAuthenticator: &BaseAuthenticator{
				profiler: &AuthProfiler{authType: "cookie"},
			},
		}
	}

	switch config.Type {
	case "basic":
		return &BasicAuthenticator{
			username: config.Username,
			password: config.Password,
			BaseAuthenticator: &BaseAuthenticator{
				profiler: &AuthProfiler{authType: "basic"},
			},
		}

	case "bearer":
		return &BearerAuthenticator{
			token: config.Token,
			BaseAuthenticator: &BaseAuthenticator{
				profiler: &AuthProfiler{authType: "bearer"},
			},
		}

	case "oauth":
		authMethod := config.Method
		tokenURL := config.TokenURL
		clientID := config.ClientID
		clientSecret := config.ClientSecret

		auth := &OAuthAuthenticator{
			username:   config.Username,
			password:   config.Password,
			method:     authMethod,
			httpClient: httpClient,
			BaseAuthenticator: &BaseAuthenticator{
				profiler: &AuthProfiler{authType: "oauth"},
			},
		}

		switch authMethod {
		case "password":
			auth.conf = &oauth2.Config{
				ClientID:     clientID,
				ClientSecret: clientSecret,
				Endpoint: oauth2.Endpoint{
					TokenURL: tokenURL,
				},
				Scopes: config.Scopes,
			}
		case "client_credentials":
			auth.clientCreds = &clientcredentials.Config{
				ClientID:     clientID,
				ClientSecret: clientSecret,
				TokenURL:     tokenURL,
				Scopes:       config.Scopes,
			}
		default:
			slog.Error("Unsupported OAUTH_METHOD. Use 'password' or 'client_credentials'")
			panic("Unsupported OAUTH_METHOD. Use 'password' or 'client_credentials'")
		}

		return auth

	case "cookie":
		maxAge := time.Duration(config.MaxAgeSeconds) * time.Second
		return &CookieAuthenticator{
			loginRequest: config.LoginRequest,
			cookieName:   config.ExtractSelector,
			maxAge:       maxAge,
			httpClient:   httpClient,
			BaseAuthenticator: &BaseAuthenticator{
				profiler: &AuthProfiler{authType: "cookie"},
			},
		}

	case "jwt":
		maxAge := time.Duration(config.MaxAgeSeconds) * time.Second
		extractFrom := config.ExtractFrom
		if extractFrom == "" {
			extractFrom = "body" // Default to body extraction
		}
		return &JWTAuthenticator{
			loginRequest:    config.LoginRequest,
			extractFrom:     extractFrom,
			extractSelector: config.ExtractSelector,
			maxAge:          maxAge,
			httpClient:      httpClient,
			jqCache:         make(map[string]*gojq.Code),
			BaseAuthenticator: &BaseAuthenticator{
				profiler: &AuthProfiler{authType: "jwt"},
			},
		}

	case "custom":
		maxAge := time.Duration(config.MaxAgeSeconds) * time.Second
		return &CustomAuthenticator{
			loginRequest:    config.LoginRequest,
			extractFrom:     config.ExtractFrom,
			extractSelector: config.ExtractSelector,
			injectInto:      config.InjectInto,
			injectKey:       config.InjectKey,
			maxAge:          maxAge,
			httpClient:      httpClient,
			jqCache:         make(map[string]*gojq.Code),
			BaseAuthenticator: &BaseAuthenticator{
				profiler: &AuthProfiler{authType: "custom"},
			},
		}

	default:
		slog.Error(fmt.Sprintf("Unsupported authentication type: %s", config.Type))
		panic(fmt.Sprintf("Unsupported authentication type: %s", config.Type))
	}
}
