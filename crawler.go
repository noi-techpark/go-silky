// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package apigorowler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/itchyny/gojq"
	"golang.org/x/time/rate"
	"gopkg.in/yaml.v3"
)

// ContextData represents a single context in a snapshot
type ContextData struct {
	Data          any    `json:"data"`
	ParentContext string `json:"parentContext"`
	Depth         int    `json:"depth"`
	Key           string `json:"key"`
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

const JQ_RES_KEY = "$res"
const JQ_CTX_KEY = "$ctx"

type ParallelismConfig struct {
	MaxConcurrency    int     `yaml:"maxConcurrency,omitempty" json:"maxConcurrency,omitempty"`
	RequestsPerSecond float64 `yaml:"requestsPerSecond,omitempty" json:"requestsPerSecond,omitempty"`
	Burst             int     `yaml:"burst,omitempty" json:"burst,omitempty"`
}

type Config struct {
	Steps          []Step               `yaml:"steps" json:"steps"`
	RootContext    interface{}          `yaml:"rootContext" json:"rootContext"`
	Authentication *AuthenticatorConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
	Headers        map[string]string    `yaml:"headers,omitempty" json:"headers,omitempty"`
	Stream         bool                 `yaml:"stream,omitempty" json:"stream,omitempty"`
}

type Step struct {
	Type              string                `yaml:"type" json:"type"`
	Name              string                `yaml:"name,omitempty" json:"name,omitempty"`
	Path              string                `yaml:"path,omitempty" json:"path,omitempty"`
	As                string                `yaml:"as,omitempty" json:"as,omitempty"`
	Values            []interface{}         `yaml:"values,omitempty" json:"values,omitempty"`
	Steps             []Step                `yaml:"steps,omitempty" json:"steps,omitempty"`
	Request           *RequestConfig        `yaml:"request,omitempty" json:"request,omitempty"`
	ResultTransformer string                `yaml:"resultTransformer,omitempty" json:"resultTransformer,omitempty"`
	MergeWithParentOn string                `yaml:"mergeWithParentOn,omitempty" json:"mergeWithParentOn,omitempty"`
	MergeOn           string                `yaml:"mergeOn,omitempty" json:"mergeOn,omitempty"`
	MergeWithContext  *MergeWithContextRule `yaml:"mergeWithContext,omitempty" json:"mergeWithContext,omitempty"`
	NoopMerge         bool                  `yaml:"noopMerge,omitempty" json:"noopMerge,omitempty"`
	Parallelism       *ParallelismConfig    `yaml:"parallelism,omitempty" json:"parallelism,omitempty"`
}

type RequestConfig struct {
	URL            string               `yaml:"url" json:"url"`
	Method         string               `yaml:"method" json:"method"`
	Headers        map[string]string    `yaml:"headers,omitempty" json:"headers,omitempty"`
	Body           map[string]any       `yaml:"body,omitempty" json:"body,omitempty"`
	Pagination     Pagination           `yaml:"pagination,omitempty" json:"pagination,omitempty"`
	Authentication *AuthenticatorConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

type MergeWithContextRule struct {
	Name string `yaml:"name"`
	Rule string `yaml:"rule"`
}

type Context struct {
	Data          interface{}
	ParentContext string
	key           string
	depth         int
}

type stepExecution struct {
	step              Step
	currentContextKey string
	currentContext    *Context
	contextMap        map[string]*Context
	parentID          string // Parent step ID for profiler hierarchy
}

// mergeOperation encapsulates the parameters needed for a merge operation
type mergeOperation struct {
	step            Step
	currentContext  *Context
	contextMap      map[string]*Context
	result          any
	templateContext map[string]any
}

// httpRequestContext encapsulates HTTP request preparation parameters
type httpRequestContext struct {
	urlTemplate    string
	method         string
	requestID      string
	headers        map[string]string
	configuredBody map[string]any
	bodyParams     map[string]interface{}
	contentType    string
	queryParams    map[string]string
	nextPageURL    string
	authenticator  Authenticator
}

// forEachResult holds the result of a single forEach iteration
type forEachResult struct {
	index          int
	result         any
	profilerEvents []StepProfilerData
	err            error
	threadID       int
}

type ApiCrawler struct {
	Config              Config
	ContextMap          map[string]*Context
	globalAuthenticator Authenticator
	DataStream          chan any
	logger              Logger
	httpClient          HTTPClient
	profiler            chan StepProfilerData
	enableProfilation   bool
	templateCache       map[string]*template.Template
	jqCache             map[string]*gojq.Code
	mergeMutex          sync.Mutex // Protects concurrent merge operations
}

func NewApiCrawler(configPath string) (*ApiCrawler, []ValidationError, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, err
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, nil, err
	}

	errors := ValidateConfig(cfg)
	if len(errors) != 0 {
		return nil, errors, fmt.Errorf("validation failed")
	}

	c := &ApiCrawler{
		httpClient:    http.DefaultClient,
		Config:        cfg,
		ContextMap:    map[string]*Context{},
		logger:        NewNoopLogger(),
		profiler:      nil,
		templateCache: make(map[string]*template.Template),
		jqCache:       make(map[string]*gojq.Code),
	}

	// handle stream channel
	if cfg.Stream {
		c.DataStream = make(chan any)
	}

	return c, nil, nil
}

func (a *ApiCrawler) GetDataStream() chan interface{} {
	return a.DataStream
}

func (a *ApiCrawler) GetData() interface{} {
	return a.ContextMap["root"].Data
}

func (a *ApiCrawler) SetLogger(logger Logger) {
	a.logger = logger
}

func (a *ApiCrawler) SetClient(client HTTPClient) {
	a.httpClient = client
}

func (a *ApiCrawler) EnableProfiler() chan StepProfilerData {
	a.enableProfilation = true
	a.profiler = make(chan StepProfilerData)
	return a.profiler
}

// getOrCompileTemplate retrieves a pre-compiled template from the cache,
// or compiles, caches, and returns it if not found.
func (a *ApiCrawler) getOrCompileTemplate(tmplString string) (*template.Template, error) {
	if tmpl, ok := a.templateCache[tmplString]; ok {
		return tmpl, nil
	}

	tmpl, err := template.New("dynamic").Parse(tmplString)
	if err != nil {
		return nil, fmt.Errorf("error parsing template: %w", err)
	}

	a.templateCache[tmplString] = tmpl
	return tmpl, nil
}

// getOrCompileJQRule retrieves a pre-compiled JQ rule from the cache,
// or compiles, caches, and returns it if not found.
func (a *ApiCrawler) getOrCompileJQRule(ruleString string, variables ...string) (*gojq.Code, error) {
	cacheKey := ruleString
	if len(variables) > 0 {
		// Use a unique key for rules with variables
		// to avoid collisions with rules without variables.
		cacheKey += fmt.Sprintf("$$vars:%v", variables)
	}

	if code, ok := a.jqCache[cacheKey]; ok {
		return code, nil
	}

	query, err := gojq.Parse(ruleString)
	if err != nil {
		return nil, fmt.Errorf("invalid jq rule '%s': %w", ruleString, err)
	}

	code, err := gojq.Compile(query, gojq.WithVariables(variables))
	if err != nil {
		return nil, fmt.Errorf("failed to compile jq rule: %w", err)
	}

	a.jqCache[cacheKey] = code
	return code, nil
}

func newStepExecution(step Step, currentContextKey string, contextMap map[string]*Context, parentID string) *stepExecution {
	return &stepExecution{
		step:              step,
		currentContextKey: currentContextKey,
		contextMap:        contextMap,
		currentContext:    contextMap[currentContextKey],
		parentID:          parentID,
	}
}

func (c *ApiCrawler) Run(ctx context.Context) error {
	// instantiate global authenticator at Run time so we force the authenticator to refresh
	if c.Config.Authentication != nil {
		c.globalAuthenticator = NewAuthenticator(*c.Config.Authentication, c.httpClient)
	} else {
		c.globalAuthenticator = NoopAuthenticator{
			BaseAuthenticator: &BaseAuthenticator{
				profiler: &AuthProfiler{authType: "cookie"},
			},
		}
	}

	rootCtx := &Context{
		Data:          c.Config.RootContext,
		ParentContext: "",
		depth:         0,
		key:           "root",
	}

	c.ContextMap["root"] = rootCtx
	currentContext := "root"

	// Emit ROOT_START event
	var rootID string
	if c.profiler != nil {
		event := newProfilerEvent(EVENT_ROOT_START, "Root Start", "", Step{})
		event.Data = map[string]any{
			"contextMap": c.serializeContextMapSafe(c.ContextMap),
			"config": map[string]any{
				"rootContext": c.Config.RootContext,
				"stream":      c.Config.Stream,
			},
		}
		c.profiler <- event
		rootID = event.ID
	}

	for _, step := range c.Config.Steps {
		ecxec := newStepExecution(step, currentContext, c.ContextMap, rootID)
		if err := c.ExecuteStep(ctx, ecxec); err != nil {
			return err
		}
	}

	// Emit final result if not streaming
	if c.profiler != nil && !c.Config.Stream {
		resultEvent := newProfilerEvent(EVENT_RESULT, "Final Result", rootID, Step{})
		c.mergeMutex.Lock()
		finalResult := copyDataSafe(rootCtx.Data)
		c.mergeMutex.Unlock()
		resultEvent.Data = map[string]any{
			"result": finalResult,
		}
		c.profiler <- resultEvent
	}

	return nil
}

func (c *ApiCrawler) ExecuteStep(ctx context.Context, exec *stepExecution) error {
	switch exec.step.Type {
	case "request":
		return c.handleRequest(ctx, exec)
	case "forEach":
		return c.handleForEach(ctx, exec)
	case "forValues":
		return c.handleForValues(ctx, exec)
	default:
		return fmt.Errorf("unknown step type: %s", exec.step.Type)
	}
}

func (c *ApiCrawler) handleRequest(ctx context.Context, exec *stepExecution) error {
	c.logger.Info("[Request] Preparing %s", exec.step.Name)

	// Emit REQUEST_STEP_START event
	stepStartTime := time.Now()
	var stepID string
	if c.profiler != nil {
		event := newProfilerEvent(EVENT_REQUEST_STEP_START, exec.step.Name, exec.parentID, exec.step)
		event.Data = map[string]any{
			"stepName":   exec.step.Name,
			"stepConfig": exec.step,
		}
		c.profiler <- event
		stepID = event.ID
	}

	templateCtx := contextMapToTemplate(exec.contextMap)

	// Determine authenticator (request-specific overrides global)
	authenticator := c.globalAuthenticator
	if exec.step.Request.Authentication != nil {
		authenticator = NewAuthenticator(*exec.step.Request.Authentication, c.httpClient)
	}

	// Set profiler on authenticator
	if authenticator != nil && c.profiler != nil {
		authenticator.SetProfiler(c.profiler)
	}

	// Initialize paginator
	paginator, err := NewPaginator(ConfigP{exec.step.Request.Pagination})
	if err != nil {
		emitProfilerError(c.profiler, "Paginator Error", stepID, err.Error())
		return fmt.Errorf("error creating request paginator: %w", err)
	}

	stop := false
	next := paginator.NextFromCtx()

	// Track previous response for PAGINATION_EVAL event
	var previousResponseBody interface{}
	var previousResponseHeaders map[string]string

	// Pagination loop
	for !stop {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		pageNum := paginator.PageNum()

		// Emit REQUEST_PAGE_START event
		pageStartTime := time.Now()
		var pageID string
		if c.profiler != nil {
			event := newProfilerEvent(EVENT_REQUEST_PAGE_START, fmt.Sprintf("Page %d", pageNum), stepID, exec.step)
			event.Data = map[string]any{
				"pageNumber": pageNum,
			}
			c.profiler <- event
			pageID = event.ID
		}

		// Prepare HTTP request
		reqCtx := httpRequestContext{
			requestID:      pageID,
			urlTemplate:    exec.step.Request.URL,
			method:         exec.step.Request.Method,
			headers:        exec.step.Request.Headers,
			configuredBody: exec.step.Request.Body,
			bodyParams:     next.BodyParams,
			contentType:    getContentType(exec.step.Request.Headers),
			queryParams:    next.QueryParams,
			nextPageURL:    next.NextPageUrl,
			authenticator:  authenticator,
		}

		req, urlObj, err := c.prepareHTTPRequest(reqCtx, templateCtx, c.Config.Headers)
		if err != nil {
			emitProfilerError(c.profiler, "Prepare Request Error", pageID, err.Error())
			return err
		}

		// Emit URL_COMPOSITION event
		if c.profiler != nil {
			event := newProfilerEvent(EVENT_URL_COMPOSITION, "URL Composition", pageID, exec.step)

			// Build result headers and body from request
			resultHeaders := make(map[string]string)
			for k, v := range req.Header {
				if len(v) > 0 {
					resultHeaders[k] = v[0]
				}
			}

			var resultBody interface{}
			if req.Body != nil {
				// Note: Request body is already set, we don't read it here to avoid consuming it
				resultBody = next.BodyParams
			}

			event.Data = map[string]any{
				"urlTemplate": exec.step.Request.URL,
				"paginationState": map[string]any{
					"pageNumber":  pageNum,
					"queryParams": next.QueryParams,
					"bodyParams":  next.BodyParams,
					"nextPageUrl": next.NextPageUrl,
				},
				"goTemplateContext": templateCtx,
				"resultUrl":         urlObj.String(),
				"resultHeaders":     resultHeaders,
				"resultBody":        resultBody,
			}
			c.profiler <- event
		}

		c.logger.Info("[Request] %s", urlObj.String())

		// Emit REQUEST_DETAILS event
		if c.profiler != nil {
			event := newProfilerEvent(EVENT_REQUEST_DETAILS, "Request Details", pageID, exec.step)

			// Build curl command
			curlCmd := fmt.Sprintf("curl -X %s '%s'", req.Method, urlObj.String())
			for k, v := range req.Header {
				if len(v) > 0 {
					curlCmd += fmt.Sprintf(" -H '%s: %s'", k, v[0])
				}
			}
			if req.Body != nil && len(next.BodyParams) > 0 {
				bodyJSON, _ := json.Marshal(next.BodyParams)
				curlCmd += fmt.Sprintf(" -d '%s'", string(bodyJSON))
			}

			// Build headers map
			headers := make(map[string]string)
			for k, v := range req.Header {
				if len(v) > 0 {
					headers[k] = v[0]
				}
			}

			event.Data = map[string]any{
				"curl":    curlCmd,
				"method":  req.Method,
				"url":     urlObj.String(),
				"headers": headers,
				"body":    next.BodyParams,
			}
			c.profiler <- event
		}

		c.logger.Debug("[Request] Got response: status pending")

		// Execute HTTP request with timing
		requestStartTime := time.Now()
		resp, err := c.httpClient.Do(req)
		if err != nil {
			emitProfilerError(c.profiler, "Request Error", pageID, err.Error())
			return fmt.Errorf("error performing HTTP request: %w", err)
		}
		defer resp.Body.Close()
		durationMs := time.Since(requestStartTime).Milliseconds()

		// Emit HTTP response event with metadata and duration
		responseSize := int(resp.ContentLength)
		if responseSize < 0 {
			responseSize = 0
		}

		// Capture pagination state BEFORE calling paginator.Next()
		previousPageState := map[string]any{
			"pageNumber": pageNum,
			"params": map[string]any{
				"queryParams": next.QueryParams,
				"bodyParams":  next.BodyParams,
			},
			"nextPageUrl": next.NextPageUrl,
		}

		// Update pagination state (reads and restores response body)
		next, stop, err = paginator.Next(resp)
		if err != nil {
			emitProfilerError(c.profiler, "Paginator Error", pageID, err.Error())
			return fmt.Errorf("paginator update error: %w", err)
		}

		// Decode JSON response
		var raw interface{}
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			emitProfilerError(c.profiler, "Response Decode Error", pageID, err.Error())
			return fmt.Errorf("error decoding response JSON: %w", err)
		}

		// Emit PAGINATION_EVAL event (if pagination is configured and this is not the first page)
		if pageNum > 0 && c.profiler != nil && !stop {
			event := newProfilerEvent(EVENT_PAGINATION_EVAL, "Pagination Evaluation", pageID, exec.step)

			afterPageState := map[string]any{
				"pageNumber": paginator.PageNum(),
				"params": map[string]any{
					"queryParams": next.QueryParams,
					"bodyParams":  next.BodyParams,
				},
				"nextPageUrl": next.NextPageUrl,
			}

			event.Data = map[string]any{
				"pageNumber":       pageNum,
				"paginationConfig": exec.step.Request.Pagination,
				"previousResponse": map[string]any{
					"body":    copyDataSafe(previousResponseBody),
					"headers": previousResponseHeaders,
				},
				"previousState": previousPageState,
				"afterState":    afterPageState,
			}
			c.profiler <- event
		}

		// Emit REQUEST_RESPONSE event
		if c.profiler != nil {
			event := newProfilerEvent(EVENT_REQUEST_RESPONSE, "Request Response", pageID, exec.step)

			// Build response headers map
			responseHeaders := make(map[string]string)
			for k, v := range resp.Header {
				if len(v) > 0 {
					responseHeaders[k] = v[0]
				}
			}

			event.Data = map[string]any{
				"statusCode":   resp.StatusCode,
				"headers":      responseHeaders,
				"body":         copyDataSafe(raw),
				"responseSize": responseSize,
				"durationMs":   durationMs,
			}
			c.profiler <- event
		}

		// Store response for next PAGINATION_EVAL event
		previousResponseBody = raw
		previousResponseHeaders = make(map[string]string)
		for k, v := range resp.Header {
			if len(v) > 0 {
				previousResponseHeaders[k] = v[0]
			}
		}

		// Transform response
		transformed, err := c.transformResult(raw, exec.step.ResultTransformer, templateCtx)
		if err != nil {
			emitProfilerError(c.profiler, "Response Transform Error", pageID, err.Error())
			return err
		}

		// Emit RESPONSE_TRANSFORM event
		if c.profiler != nil && exec.step.ResultTransformer != "" {
			event := newProfilerEvent(EVENT_RESPONSE_TRANSFORM, "Response Transform", pageID, exec.step)

			event.Data = map[string]any{
				"transformRule":  exec.step.ResultTransformer,
				"beforeResponse": copyDataSafe(raw),
				"afterResponse":  copyDataSafe(transformed),
				// TODO: Add diff computation
			}
			c.profiler <- event
		}

		// Execute nested steps on transformed result
		// Note: request steps create a working context for the response data.
		// If the current context is canonical (like "root"), the working context
		// uses a unique key to avoid shadowing the original.
		cloneResult := childMapWithClonedContext(exec.contextMap, exec.currentContext, transformed, c.ContextMap)
		childContextMap := cloneResult.contextMap
		workingContextKey := cloneResult.workingKey

		// Emit CONTEXT_SELECTION event (context created for nested steps)
		if c.profiler != nil && len(exec.step.Steps) > 0 {
			event := newProfilerEvent(EVENT_CONTEXT_SELECTION, "Context Selection", pageID, exec.step)
			contextPath := buildContextPath(childContextMap, workingContextKey)
			event.Data = map[string]any{
				"contextPath":        contextPath,
				"currentContextKey":  workingContextKey,
				"currentContextData": copyDataSafe(childContextMap[workingContextKey].Data),
				"fullContextMap":     c.serializeContextMapSafe(childContextMap),
			}
			c.profiler <- event
		}

		for _, step := range exec.step.Steps {
			newExec := newStepExecution(step, workingContextKey, childContextMap, pageID)
			if err := c.ExecuteStep(ctx, newExec); err != nil {
				return err
			}
		}

		// Get final result after nested steps from the working context
		transformed = childContextMap[workingContextKey].Data

		// Apply merge strategy
		mergeOp := mergeOperation{
			step:            exec.step,
			currentContext:  exec.currentContext,
			contextMap:      exec.contextMap,
			result:          transformed,
			templateContext: templateCtx,
		}

		var dataBefore any = nil
		if c.profiler != nil {
			// Capture data before merge with mutex protection
			c.mergeMutex.Lock()
			dataBefore = copyDataSafe(exec.currentContext.Data)
			c.mergeMutex.Unlock()
		}

		mergeStepName, err := c.performMerge(mergeOp)
		if err != nil {
			emitProfilerError(c.profiler, "Merge Error", pageID, err.Error())
			return err
		}

		// Emit CONTEXT_MERGE event if merge happened
		if mergeStepName != "" && c.profiler != nil {
			event := newProfilerEvent(EVENT_CONTEXT_MERGE, "Context Merge", pageID, exec.step)

			// Determine merge rule and target context
			mergeRule := ""
			currentContextKey := exec.currentContext.key
			targetContextKey := exec.currentContext.key

			if exec.step.MergeOn != "" {
				mergeRule = exec.step.MergeOn
			} else if exec.step.MergeWithParentOn != "" {
				mergeRule = exec.step.MergeWithParentOn
				targetContextKey = exec.currentContext.ParentContext
			} else if exec.step.MergeWithContext != nil {
				mergeRule = exec.step.MergeWithContext.Rule
				targetContextKey = exec.step.MergeWithContext.Name
			}

			// Capture data after merge with mutex protection
			c.mergeMutex.Lock()
			dataAfter := copyDataSafe(exec.currentContext.Data)
			c.mergeMutex.Unlock()

			event.Data = map[string]any{
				"currentContextKey":   currentContextKey,
				"targetContextKey":    targetContextKey,
				"mergeRule":           mergeRule,
				"targetContextBefore": dataBefore,
				"targetContextAfter":  dataAfter,
				"fullContextMap":      c.serializeContextMapSafe(exec.contextMap),
				// TODO: Add diff computation
			}
			c.profiler <- event
		}

		// Handle streaming at root level
		if exec.currentContext.depth == 0 && c.Config.Stream {
			array_data := exec.currentContext.Data.([]interface{})
			for _, d := range array_data {
				c.DataStream <- d

				// Emit STREAM_RESULT event
				if c.profiler != nil {
					streamEvent := newProfilerEvent(EVENT_STREAM_RESULT, "Stream Result", pageID, exec.step)
					streamEvent.Data = map[string]any{
						"entity": copyDataSafe(d),
						"index":  len(array_data),
					}
					c.profiler <- streamEvent
				}
			}
			exec.currentContext.Data = []interface{}{}
		}

		// Emit REQUEST_PAGE_END event (reuse START event ID)
		if c.profiler != nil && pageID != "" {
			event := StepProfilerData{
				ID:        pageID, // Reuse START event ID
				ParentID:  stepID,
				Type:      EVENT_REQUEST_PAGE_END,
				Name:      fmt.Sprintf("Page %d End", pageNum),
				Step:      exec.step,
				Timestamp: time.Now(),
				Duration:  time.Since(pageStartTime).Milliseconds(),
				Data:      make(map[string]any),
			}
			c.profiler <- event
		}
	}

	// Emit REQUEST_STEP_END event (reuse START event ID)
	if c.profiler != nil && stepID != "" {
		event := StepProfilerData{
			ID:        stepID, // Reuse START event ID
			ParentID:  exec.parentID,
			Type:      EVENT_REQUEST_STEP_END,
			Name:      exec.step.Name + " End",
			Step:      exec.step,
			Timestamp: time.Now(),
			Duration:  time.Since(stepStartTime).Milliseconds(),
			Data:      make(map[string]any),
		}
		c.profiler <- event
	}

	return nil
}

// executeForEachIteration executes a single forEach iteration
func (c *ApiCrawler) executeForEachIteration(
	ctx context.Context,
	index int,
	item any,
	exec *stepExecution,
	profilerEnabled bool,
	stepID string,
	workerID int,
	workerPoolID string,
) forEachResult {
	result := forEachResult{
		index:          index,
		profilerEvents: make([]StepProfilerData, 0),
	}

	c.logger.Info("[ForEach] Iteration %d as '%s'", index, exec.step.As, "item", item)

	childContextMap := childMapWith(exec.contextMap, exec.currentContext, exec.step.As, item)

	// Emit CONTEXT_SELECTION event with worker tracking (context created for iteration)
	if c.profiler != nil {
		event := newProfilerEventWithWorker(EVENT_CONTEXT_SELECTION, "Context Selection", stepID, exec.step, workerID, workerPoolID)
		contextPath := buildContextPath(childContextMap, exec.step.As)
		event.Data = map[string]any{
			"contextPath":        contextPath,
			"currentContextKey":  exec.step.As,
			"currentContextData": copyDataSafe(childContextMap[exec.step.As].Data),
			"fullContextMap":     c.serializeContextMapSafe(childContextMap),
		}
		c.profiler <- event
	}

	// Emit ITEM_SELECTION event with worker tracking
	var itemID string
	if c.profiler != nil {
		event := newProfilerEventWithWorker(EVENT_ITEM_SELECTION, fmt.Sprintf("Item %d", index), stepID, exec.step, workerID, workerPoolID)
		event.Data = map[string]any{
			"iterationIndex":     index,
			"itemValue":          copyDataSafe(item),
			"currentContextKey":  exec.step.As,
			"currentContextData": copyDataSafe(childContextMap[exec.step.As].Data),
		}
		c.profiler <- event
		itemID = event.ID
	}

	// Execute nested steps
	for _, nested := range exec.step.Steps {
		newExec := newStepExecution(nested, exec.step.As, childContextMap, itemID)
		if err := c.ExecuteStep(ctx, newExec); err != nil {
			result.err = err
			return result
		}
	}

	result.result = childContextMap[exec.step.As].Data

	return result
}

// executeForEachParallel executes forEach iterations in parallel
func (c *ApiCrawler) executeForEachParallel(
	ctx context.Context,
	exec *stepExecution,
	items []interface{},
	maxConcurrency int,
	rateLimiter *rate.Limiter,
	stepID string,
) ([]interface{}, error) {
	profilerEnabled := c.profiler != nil
	numItems := len(items)

	// Results channel sized to hold all results
	resultsChan := make(chan forEachResult, numItems)

	// Error group for managing goroutines
	var wg sync.WaitGroup

	// Semaphore for concurrency control
	semaphore := make(chan struct{}, maxConcurrency)

	// Launch workers for each item
	for i, item := range items {
		wg.Add(1)

		go func(index int, item any, threadID int) {
			defer wg.Done()

			// Acquire semaphore slot
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Check context cancellation
			select {
			case <-ctx.Done():
				resultsChan <- forEachResult{index: index, err: ctx.Err()}
				return
			default:
			}

			// Apply rate limiting if configured
			if rateLimiter != nil {
				if err := rateLimiter.Wait(ctx); err != nil {
					resultsChan <- forEachResult{index: index, err: err}
					return
				}
			}

			// Execute iteration
			workerPoolID := stepID + "-pool"
			result := c.executeForEachIteration(ctx, index, item, exec, profilerEnabled, stepID, threadID, workerPoolID)
			result.threadID = threadID
			resultsChan <- result
		}(i, item, i%maxConcurrency)
	}

	// Close results channel when all workers complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results and maintain order
	results := make([]forEachResult, numItems)
	for result := range resultsChan {
		if result.err != nil {
			return nil, result.err
		}
		results[result.index] = result
	}

	// Emit profiler events in order if profiling is enabled
	if profilerEnabled {
		for _, result := range results {
			for _, event := range result.profilerEvents {
				c.profiler <- event
			}
		}
	}

	// Extract execution results in order
	executionResults := make([]interface{}, numItems)
	for i, result := range results {
		executionResults[i] = result.result
	}

	return executionResults, nil
}

func (c *ApiCrawler) handleForEach(ctx context.Context, exec *stepExecution) error {
	c.logger.Info("[Foreach] Preparing %s", exec.step.Name)

	// Emit FOREACH_STEP_START event
	stepStartTime := time.Now()
	var stepID string
	if c.profiler != nil {
		event := newProfilerEvent(EVENT_FOREACH_STEP_START, exec.step.Name, exec.parentID, exec.step)
		event.Data = map[string]any{
			"stepName":   exec.step.Name,
			"stepConfig": exec.step,
		}
		c.profiler <- event
		stepID = event.ID
	}

	results := []interface{}{}

	// Extract items to iterate over from path (forEach always uses path, validation ensures this)
	if len(exec.step.Path) != 0 {
		c.logger.Debug("[Foreach] Extracting from parent context with rule: %s", exec.step.Path)

		code, err := c.getOrCompileJQRule(exec.step.Path)
		if err != nil {
			emitProfilerError(c.profiler, "Path Extraction Error", stepID, err.Error())
			return fmt.Errorf("failed to get/compile jq path: %w", err)
		}

		iter := code.Run(exec.currentContext.Data)
		for {
			v, ok := iter.Next()
			if !ok {
				break
			}
			if err, isErr := v.(error); isErr {
				return fmt.Errorf("jq error: %w", err)
			}
			results = append(results, v)
		}

		// Make sure the result is an array (jq might emit one-by-one items)
		if len(results) == 1 {
			if arr, ok := results[0].([]interface{}); ok {
				results = arr
			}
		}
	}

	// Determine execution mode and parameters
	var executionResults []interface{}
	var err error

	if exec.step.Parallelism != nil {
		// Determine max concurrency (step setting or default)
		maxConcurrency := exec.step.Parallelism.MaxConcurrency
		if maxConcurrency == 0 {
			maxConcurrency = 10 // Default concurrency
		}

		// Create rate limiter if configured
		var rateLimiter *rate.Limiter
		if exec.step.Parallelism.RequestsPerSecond > 0 {
			burst := exec.step.Parallelism.Burst
			if burst == 0 {
				burst = 1 // Default burst
			}
			rateLimiter = rate.NewLimiter(rate.Limit(exec.step.Parallelism.RequestsPerSecond), burst)
		}

		// Emit PARALLELISM_SETUP event
		if c.profiler != nil {
			workerPoolID := stepID + "-pool"
			workerIDs := make([]int, maxConcurrency)
			for i := 0; i < maxConcurrency; i++ {
				workerIDs[i] = i
			}

			event := newProfilerEvent(EVENT_PARALLELISM_SETUP, "Parallelism Setup", stepID, exec.step)
			event.Data = map[string]any{
				"maxConcurrency": maxConcurrency,
				"workerPoolId":   workerPoolID,
				"workerIds":      workerIDs,
			}
			if rateLimiter != nil {
				event.Data["rateLimit"] = exec.step.Parallelism.RequestsPerSecond
				event.Data["burst"] = exec.step.Parallelism.Burst
			}
			c.profiler <- event
		}

		c.logger.Info("[ForEach] Executing %d iterations in parallel (max concurrency: %d)", len(results), maxConcurrency)

		// Execute in parallel
		executionResults, err = c.executeForEachParallel(ctx, exec, results, maxConcurrency, rateLimiter, stepID)
		if err != nil {
			return err
		}
	} else {
		// Execute sequentially (original behavior)
		executionResults = make([]interface{}, 0)
		for i, item := range results {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			c.logger.Info("[ForEach] Iteration %d as '%s'", i, exec.step.As, "item", item)

			childContextMap := childMapWith(exec.contextMap, exec.currentContext, exec.step.As, item)

			// Emit CONTEXT_SELECTION event (context created for iteration)
			if c.profiler != nil {
				event := newProfilerEvent(EVENT_CONTEXT_SELECTION, "Context Selection", stepID, exec.step)
				contextPath := buildContextPath(childContextMap, exec.step.As)
				event.Data = map[string]any{
					"contextPath":        contextPath,
					"currentContextKey":  exec.step.As,
					"currentContextData": copyDataSafe(childContextMap[exec.step.As].Data),
					"fullContextMap":     c.serializeContextMapSafe(childContextMap),
				}
				c.profiler <- event
			}

			// Emit ITEM_SELECTION event
			var itemID string
			if c.profiler != nil {
				event := newProfilerEvent(EVENT_ITEM_SELECTION, fmt.Sprintf("Item %d", i), stepID, exec.step)
				event.Data = map[string]any{
					"iterationIndex":     i,
					"itemValue":          copyDataSafe(item),
					"currentContextKey":  exec.step.As,
					"currentContextData": copyDataSafe(childContextMap[exec.step.As].Data),
				}
				c.profiler <- event
				itemID = event.ID
			}

			for _, nested := range exec.step.Steps {
				newExec := newStepExecution(nested, exec.step.As, childContextMap, itemID)
				if err := c.ExecuteStep(ctx, newExec); err != nil {
					return err
				}
			}

			executionResults = append(executionResults, childContextMap[exec.step.As].Data)
		}
	}

	// Determine merge strategy
	templateCtx := contextMapToTemplate(exec.contextMap)

	// Check if custom merge rules are specified
	hasCustomMerge := exec.step.MergeOn != "" || exec.step.MergeWithParentOn != "" || exec.step.MergeWithContext != nil || exec.step.NoopMerge

	if hasCustomMerge {
		// Use custom merge logic (same as request steps)
		mergeOp := mergeOperation{
			step:            exec.step,
			currentContext:  exec.currentContext,
			contextMap:      exec.contextMap,
			result:          executionResults,
			templateContext: templateCtx,
		}

		_, err := c.performMerge(mergeOp)
		if err != nil {
			emitProfilerError(c.profiler, "Merge Error", stepID, err.Error())
			return err
		}

	} else /*if exec.step.Path != ""*/ {
		// Default: patch the array at exec.step.Path with new results
		code, err := c.getOrCompileJQRule(exec.step.Path+" = $new", "$new")
		if err != nil {
			emitProfilerError(c.profiler, "Merge Error", stepID, err.Error())
			return fmt.Errorf("failed to get/compile merge rule: %w", err)
		}

		iter := code.Run(exec.currentContext.Data, executionResults)

		v, ok := iter.Next()
		if !ok {
			return fmt.Errorf("patch yielded nothing")
		}
		if err, isErr := v.(error); isErr {
			return err
		}

		exec.currentContext.Data = v
	}
	// If Path is empty (using values) and no custom merge, skip merge
	// The nested steps are expected to handle merging

	// Handle streaming at root level
	if exec.currentContext.depth <= 1 && c.Config.Stream {
		array_data := exec.currentContext.Data.([]interface{})
		for _, d := range array_data {
			c.DataStream <- d

			// Emit STREAM_RESULT event
			if c.profiler != nil {
				streamEvent := newProfilerEvent(EVENT_STREAM_RESULT, "Stream Result", stepID, exec.step)
				streamEvent.Data = map[string]any{
					"entity": copyDataSafe(d),
					"index":  len(array_data),
				}
				c.profiler <- streamEvent
			}
		}
		exec.currentContext.Data = []interface{}{}
	}

	// Emit FOREACH_STEP_END event (reuse START event ID)
	if c.profiler != nil && stepID != "" {
		event := StepProfilerData{
			ID:        stepID, // Reuse START event ID
			ParentID:  exec.parentID,
			Type:      EVENT_FOREACH_STEP_END,
			Name:      exec.step.Name + " End",
			Step:      exec.step,
			Timestamp: time.Now(),
			Duration:  time.Since(stepStartTime).Milliseconds(),
			Data:      make(map[string]any),
		}
		c.profiler <- event
	}

	return nil
}

// handleForValues iterates over literal values and creates an overlay context for each.
// Unlike forEach, forValues:
// - Only accepts literal values (no path extraction)
// - Creates overlay context (value assigned directly to 'as' key, no .value wrapper)
// - Does NOT merge results back - nested steps handle their own merging
// - Preserves parent context accessibility
func (c *ApiCrawler) handleForValues(ctx context.Context, exec *stepExecution) error {
	c.logger.Info("[ForValues] Preparing %s", exec.step.Name)

	// Emit FORVALUES_STEP_START event
	stepStartTime := time.Now()
	var stepID string
	if c.profiler != nil {
		event := newProfilerEvent(EVENT_FORVALUES_STEP_START, exec.step.Name, exec.parentID, exec.step)
		event.Data = map[string]any{
			"stepName":   exec.step.Name,
			"stepConfig": exec.step,
			"values":     exec.step.Values,
		}
		c.profiler <- event
		stepID = event.ID
	}

	// Iterate over values
	for i, value := range exec.step.Values {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c.logger.Info("[ForValues] Iteration %d as '%s' = %v", i, exec.step.As, value)

		// Create overlay context map - value is assigned directly (no .value wrapper)
		// This preserves parent context while adding the new value
		childContextMap := childMapWithOverlay(exec.contextMap, exec.currentContext, exec.step.As, value)

		// Emit CONTEXT_SELECTION event
		if c.profiler != nil {
			event := newProfilerEvent(EVENT_CONTEXT_SELECTION, "Context Selection", stepID, exec.step)
			contextPath := buildContextPath(childContextMap, exec.step.As)
			event.Data = map[string]any{
				"contextPath":        contextPath,
				"currentContextKey":  exec.step.As,
				"currentContextData": copyDataSafe(value),
				"fullContextMap":     c.serializeContextMapSafe(childContextMap),
			}
			c.profiler <- event
		}

		// Emit ITEM_SELECTION event
		var itemID string
		if c.profiler != nil {
			event := newProfilerEvent(EVENT_ITEM_SELECTION, fmt.Sprintf("Value %d", i), stepID, exec.step)
			event.Data = map[string]any{
				"iterationIndex":    i,
				"itemValue":         copyDataSafe(value),
				"currentContextKey": exec.step.As,
			}
			c.profiler <- event
			itemID = event.ID
		}

		// Execute nested steps with overlay context
		// Nested steps operate in parent context but have access to the value via 'as' key
		for _, nested := range exec.step.Steps {
			newExec := newStepExecution(nested, exec.currentContextKey, childContextMap, itemID)
			if err := c.ExecuteStep(ctx, newExec); err != nil {
				return err
			}
		}
	}

	// Emit FORVALUES_STEP_END event
	if c.profiler != nil && stepID != "" {
		event := StepProfilerData{
			ID:        stepID,
			ParentID:  exec.parentID,
			Type:      EVENT_FORVALUES_STEP_END,
			Name:      exec.step.Name + " End",
			Step:      exec.step,
			Timestamp: time.Now(),
			Duration:  time.Since(stepStartTime).Milliseconds(),
			Data:      make(map[string]any),
		}
		c.profiler <- event
	}

	return nil
}

func applyMergeRule(c *ApiCrawler, contextData any, rule string, result any, templateCtx map[string]any) (interface{}, error) {
	// Parse the JQ expression
	code, err := c.getOrCompileJQRule(rule, JQ_RES_KEY, JQ_CTX_KEY)
	if err != nil {
		return nil, fmt.Errorf("failed to get/compile merge rule: %w", err)
	}

	// Run the query against contextData, passing $res as a variable
	iter := code.Run(contextData, result, templateCtx)

	// Collect the results, expecting exactly one
	var values []interface{}
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if errVal, isErr := v.(error); isErr {
			return nil, fmt.Errorf("error running JQ: %w", errVal)
		}
		values = append(values, v)
	}

	// Enforce exactly one result
	if len(values) != 1 {
		return nil, fmt.Errorf("merge rule must produce exactly one result, got %d", len(values))
	}

	return values[0], nil
}

// performMerge applies the appropriate merge strategy based on step configuration
// Returns the profiler step name and error if any. The actual context is modified in place.
// Thread-safe: Uses mutex to protect concurrent access to contexts.
func (c *ApiCrawler) performMerge(op mergeOperation) (profilerStepName string, err error) {
	// Lock for thread-safe merge operations
	c.mergeMutex.Lock()
	defer c.mergeMutex.Unlock()

	// Check for noop merge (skip merging entirely)
	if op.step.NoopMerge {
		c.logger.Debug("[Merge] noop merge - skipping")
		return "", nil
	}

	// 1. Explicit merge rule (merge with ancestor context)
	if op.step.MergeOn != "" {
		c.logger.Debug("[Merge] merging-on with expression: %s", op.step.MergeOn)
		updated, err := applyMergeRule(c, op.currentContext.Data, op.step.MergeOn, op.result, op.templateContext)
		if err != nil {
			return "", fmt.Errorf("mergeOn failed: %w", err)
		}
		op.currentContext.Data = updated
		return "Merge-On", nil
	}

	// 2. Merge with parent context
	if op.step.MergeWithParentOn != "" {
		c.logger.Debug("[Merge] merging-with-parent with expression: %s", op.step.MergeWithParentOn)
		parentCtx := op.contextMap[op.currentContext.ParentContext]
		updated, err := applyMergeRule(c, parentCtx.Data, op.step.MergeWithParentOn, op.result, op.templateContext)
		if err != nil {
			return "", fmt.Errorf("mergeWithParentOn failed: %w", err)
		}
		parentCtx.Data = updated
		return "Merge-Parent", nil
	}

	// 3. Named context merge (cross-scope update)
	// Note: Since childMapWithClonedContext now uses unique working keys for
	// canonical contexts (like "root"), the original canonical contexts are
	// preserved in the context map and can be found by name.
	if op.step.MergeWithContext != nil {
		c.logger.Debug("[Merge] merging-with-context with expression: %s:%s",
			op.step.MergeWithContext.Name, op.step.MergeWithContext.Rule)

		targetCtx, ok := op.contextMap[op.step.MergeWithContext.Name]
		if !ok {
			return "", fmt.Errorf("context '%s' not found", op.step.MergeWithContext.Name)
		}
		updated, err := applyMergeRule(c, targetCtx.Data, op.step.MergeWithContext.Rule, op.result, op.templateContext)
		if err != nil {
			return "", fmt.Errorf("mergeWithContext failed: %w", err)
		}
		targetCtx.Data = updated
		return "Merge-Context", nil
	}

	// 4. Default merge (shallow merge for maps/arrays)
	c.logger.Debug("[Merge] default merge")
	switch data := op.currentContext.Data.(type) {
	case []interface{}:
		op.currentContext.Data = append(data, op.result.([]interface{})...)
	case map[string]interface{}:
		if transformedMap, ok := op.result.(map[string]interface{}); ok {
			for k, v := range transformedMap {
				data[k] = v
			}
		}
	default:
		op.currentContext.Data = op.result
	}
	return "Merge-Default", nil
}

// prepareHTTPRequest builds an HTTP request from the context and pagination parameters
func (c *ApiCrawler) prepareHTTPRequest(ctx httpRequestContext, templateCtx map[string]any, globalHeaders map[string]string) (*http.Request, *url.URL, error) {
	var urlObj *url.URL
	var err error

	// Determine base URL
	if ctx.nextPageURL != "" {
		urlObj, err = url.Parse(ctx.nextPageURL)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid nextPageURL %s: %w", ctx.nextPageURL, err)
		}
	} else {
		// Template URL expansion
		tmpl, err := c.getOrCompileTemplate(ctx.urlTemplate)
		if err != nil {
			return nil, nil, fmt.Errorf("error getting/compiling URL template: %w", err)
		}

		var urlBuf bytes.Buffer
		if err := tmpl.Execute(&urlBuf, templateCtx); err != nil {
			return nil, nil, fmt.Errorf("error executing URL template: %w", err)
		}

		urlObj, err = url.Parse(urlBuf.String())
		if err != nil {
			return nil, nil, fmt.Errorf("invalid URL %s: %w", urlBuf.String(), err)
		}
	}

	// Add query parameters
	query := urlObj.Query()
	for k, v := range ctx.queryParams {
		query.Set(k, v)
	}
	urlObj.RawQuery = query.Encode()

	// Merge configured body with pagination body params
	mergedBody := make(map[string]any)
	for k, v := range ctx.configuredBody {
		mergedBody[k] = v
	}
	for k, v := range ctx.bodyParams {
		mergedBody[k] = v
	}

	// Determine content type
	contentType := ctx.contentType

	// Prepare request body based on content type
	var reqBody io.Reader
	if len(mergedBody) > 0 {
		switch contentType {
		case "application/json":
			bodyJSON, err := json.Marshal(mergedBody)
			if err != nil {
				return nil, nil, fmt.Errorf("error encoding JSON body: %w", err)
			}
			reqBody = bytes.NewReader(bodyJSON)

		case "application/x-www-form-urlencoded":
			formData := url.Values{}
			for k, v := range mergedBody {
				formData.Set(k, fmt.Sprintf("%v", v))
			}
			reqBody = bytes.NewReader([]byte(formData.Encode()))

		default:
			return nil, nil, fmt.Errorf("unsupported content type: %s", contentType)
		}
	}

	// Create HTTP request
	req, err := http.NewRequest(ctx.method, urlObj.String(), reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating HTTP request: %w", err)
	}

	// Apply headers (priority: global < request-specific < pagination)
	for k, v := range globalHeaders {
		req.Header.Set(k, v)
	}
	for k, v := range ctx.headers {
		req.Header.Set(k, v)
	}

	// Set Content-Type header if body is present
	if len(mergedBody) > 0 && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Apply authentication
	ctx.authenticator.PrepareRequest(req, ctx.requestID)

	return req, urlObj, nil
}

// transformResult applies the jq transformer to the raw response
func (c *ApiCrawler) transformResult(raw any, transformer string, templateCtx map[string]any) (any, error) {
	if transformer == "" {
		return raw, nil
	}

	c.logger.Debug("[Transform] transforming with expression: %s", transformer)

	code, err := c.getOrCompileJQRule(transformer, JQ_CTX_KEY)
	if err != nil {
		return nil, fmt.Errorf("failed to get/compile transform rule: %w", err)
	}

	iter := code.Run(raw, templateCtx)
	var singleResult interface{}
	count := 0

	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return nil, fmt.Errorf("jq error: %w", err)
		}

		count++
		if count > 1 {
			return nil, fmt.Errorf("resultTransformer yielded more than one value")
		}

		singleResult = v
	}
	return singleResult, nil
}

func childMapWith(base map[string]*Context, currentCotnext *Context, key string, value interface{}) map[string]*Context {
	newMap := make(map[string]*Context, len(base)+1)
	for k, v := range base {
		newMap[k] = v
	}
	newMap[key] = &Context{
		Data:          value,
		ParentContext: currentCotnext.key,
		key:           key,
		depth:         currentCotnext.depth + 1,
	}
	return newMap
}

// childMapWithOverlay creates an overlay context for forValues.
// Unlike childMapWith, this:
// - Assigns the value directly (no wrapper)
// - Keeps the same parent reference (overlay, not child)
// - Nested steps operate on the PARENT context but have access to the value via 'as' key
func childMapWithOverlay(base map[string]*Context, currentContext *Context, key string, value interface{}) map[string]*Context {
	newMap := make(map[string]*Context, len(base)+1)
	for k, v := range base {
		newMap[k] = v
	}
	// Create overlay context - same depth as parent, parent points to parent's parent
	newMap[key] = &Context{
		Data:          value, // Value assigned directly, no wrapper
		ParentContext: currentContext.ParentContext,
		key:           key,
		depth:         currentContext.depth, // Same depth - it's an overlay, not a child
	}
	return newMap
}

// clonedContextResult holds the result of cloning a context, including
// the new context map and the key where the working context was placed
type clonedContextResult struct {
	contextMap map[string]*Context
	workingKey string // The key where the cloned/working context is stored
}

// childMapWithClonedContext creates a new context map with a cloned working context.
// If the current context is a canonical context (exists in canonicalMap), the clone
// is stored under a unique working key to avoid shadowing the original.
// This ensures that mergeWithContext can always find the original canonical contexts.
func childMapWithClonedContext(base map[string]*Context, currentContext *Context, value interface{}, canonicalMap map[string]*Context) clonedContextResult {
	newMap := make(map[string]*Context, len(base)+2)

	// Copy all existing contexts
	for k, v := range base {
		newMap[k] = v
	}

	// Determine the working key - if current context is canonical, use a unique key
	workingKey := currentContext.key
	if _, isCanonical := canonicalMap[currentContext.key]; isCanonical {
		// Create a unique working key that won't shadow the canonical context
		workingKey = "_response_" + currentContext.key
	}

	// Create the working context with the response data
	newMap[workingKey] = &Context{
		Data:          value,
		ParentContext: currentContext.key, // Parent is the original context
		key:           workingKey,
		depth:         currentContext.depth + 1,
	}

	return clonedContextResult{
		contextMap: newMap,
		workingKey: workingKey,
	}
}

func contextMapToTemplate(base map[string]*Context) map[string]interface{} {
	result := make(map[string]interface{})

	// First pass: add all contexts by their key
	for k, c := range base {
		result[k] = c.Data
	}

	// Second pass: spread map data from special contexts into top level
	// This allows templates to access fields directly (e.g., {{ .FacilityId }})
	// Priority: _response_* contexts first (most specific), then root
	for k, c := range base {
		// Spread _response_* context data (working contexts from request steps)
		if len(k) > 10 && k[:10] == "_response_" {
			if dataMap, ok := c.Data.(map[string]interface{}); ok {
				for field, v := range dataMap {
					result[field] = v
				}
			}
		}
	}

	// Finally spread root context data (lowest priority, won't overwrite)
	if rootCtx, ok := base["root"]; ok {
		if rootMap, ok := rootCtx.Data.(map[string]interface{}); ok {
			for k, v := range rootMap {
				if _, exists := result[k]; !exists {
					result[k] = v
				}
			}
		}
	}

	return result
}

// copyDataSafe creates a safe copy of data for profiler events (non-JSON approach)
func copyDataSafe(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case []interface{}:
		copy := make([]interface{}, len(val))
		for i, item := range val {
			copy[i] = copyDataSafe(item)
		}
		return copy
	case map[string]any:
		copy := make(map[string]any, len(val))
		for k, item := range val {
			copy[k] = copyDataSafe(item)
		}
		return copy
	default:
		// Primitives and other types are safe to share
		return v
	}
}
