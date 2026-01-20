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
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

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
	stepPath          string        // Unique path to this step (e.g., "steps[0].steps[1]")
	compiledStep      *CompiledStep // Pre-compiled expressions for this step (nil if not available)
	currentContextKey string
	currentContext    *Context
	contextMap        map[string]*Context
	parentID          string // Parent step ID for profiler hierarchy
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
	compiledStep   *CompiledStep // Pre-compiled templates (nil for fallback)
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
	CompiledConfig      *CompiledConfig // Pre-compiled JQ/templates (nil for legacy mode)
	ContextMap          map[string]*Context
	globalAuthenticator Authenticator
	DataStream          chan any
	runVars             map[string]any // Runtime variables injected at execution time
	logger              Logger
	httpClient          HTTPClient
	profiler            *Profiler
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

	// Use ValidateAndCompile for fail-fast JQ/template compilation
	compiled, validationErrors, err := ValidateAndCompile(cfg)
	if err != nil {
		return nil, validationErrors, err
	}
	if len(validationErrors) != 0 {
		return nil, validationErrors, fmt.Errorf("validation failed")
	}

	c := &ApiCrawler{
		httpClient:     http.DefaultClient,
		Config:         cfg,
		CompiledConfig: compiled,
		ContextMap:     map[string]*Context{},
		logger:         NewNoopLogger(),
		profiler:       nil,
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
	a.profiler = NewProfiler(&a.mergeMutex)
	return a.profiler.Channel()
}

// getCompiledStep returns the pre-compiled step for the given path.
// Returns nil if no compiled config or step not found.
func (a *ApiCrawler) getCompiledStep(stepPath string) *CompiledStep {
	if a.CompiledConfig == nil {
		return nil
	}
	return a.CompiledConfig.GetCompiledStep(stepPath)
}

func (c *ApiCrawler) newStepExecution(step Step, stepPath string, currentContextKey string, contextMap map[string]*Context, parentID string) *stepExecution {
	return &stepExecution{
		step:              step,
		stepPath:          stepPath,
		compiledStep:      c.getCompiledStep(stepPath),
		currentContextKey: currentContextKey,
		contextMap:        contextMap,
		currentContext:    contextMap[currentContextKey],
		parentID:          parentID,
	}
}

func (c *ApiCrawler) Run(ctx context.Context, vars map[string]any) error {
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

	c.runVars = vars
	// Note: We don't reset c.runVars in defer because parallel goroutines might still be
	// reading it when Run() returns (especially on error). It will be overwritten on next Run().

	// Emit ROOT_START event
	rootID := c.profiler.EmitRootStart(c.Config, c.ContextMap)

	for i, step := range c.Config.Steps {
		stepPath := fmt.Sprintf("steps[%d]", i)
		exec := c.newStepExecution(step, stepPath, currentContext, c.ContextMap, rootID)
		if err := c.ExecuteStep(ctx, exec); err != nil {
			return err
		}
	}

	// Emit final result if not streaming
	if !c.Config.Stream {
		c.profiler.EmitFinalResult(rootID, rootCtx.Data)
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
	stepID := c.profiler.EmitRequestStepStart(exec.step, exec.parentID)

	templateCtx := c.contextMapToTemplate(exec.contextMap, c.runVars)

	// Determine authenticator (request-specific overrides global)
	authenticator := c.globalAuthenticator
	if exec.step.Request.Authentication != nil {
		authenticator = NewAuthenticator(*exec.step.Request.Authentication, c.httpClient)
	}

	// Set profiler on authenticator
	if authenticator != nil && c.profiler.Enabled() {
		authenticator.SetProfiler(c.profiler.Channel())
	}

	// Initialize paginator
	paginator, err := NewPaginator(ConfigP{exec.step.Request.Pagination})
	if err != nil {
		c.profiler.EmitError("Paginator Error", stepID, err.Error())
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
		pageID := c.profiler.EmitRequestPageStart(stepID, exec.step, pageNum)

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
			compiledStep:   exec.compiledStep,
		}

		req, urlObj, err := c.prepareHTTPRequest(reqCtx, templateCtx)
		if err != nil {
			c.profiler.EmitError("Prepare Request Error", pageID, err.Error())
			return err
		}

		// Emit URL_COMPOSITION event (only compute data if profiler enabled)
		if c.profiler.Enabled() {
			resultHeaders := make(map[string]string)
			for k, v := range req.Header {
				if len(v) > 0 {
					resultHeaders[k] = v[0]
				}
			}

			var resultBody interface{}
			if req.Body != nil {
				resultBody = next.BodyParams
			}

			c.profiler.EmitURLComposition(pageID, exec.step, URLCompositionData{
				URLTemplate:     exec.step.Request.URL,
				PageNumber:      pageNum,
				QueryParams:     next.QueryParams,
				BodyParams:      next.BodyParams,
				NextPageURL:     next.NextPageUrl,
				TemplateContext: templateCtx,
				ResultURL:       urlObj.String(),
				ResultHeaders:   resultHeaders,
				ResultBody:      resultBody,
			})
		}

		c.logger.Info("[Request] %s", urlObj.String())

		// Emit REQUEST_DETAILS event (only compute data if profiler enabled)
		if c.profiler.Enabled() {
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

			c.profiler.EmitRequestDetails(pageID, exec.step, RequestDetailsData{
				CurlCommand: curlCmd,
				Method:      req.Method,
				URL:         urlObj.String(),
				Headers:     headers,
				Body:        next.BodyParams,
			})
		}

		c.logger.Debug("[Request] Got response: status pending")

		// Execute HTTP request with timing
		requestStartTime := time.Now()
		resp, err := c.httpClient.Do(req)
		if err != nil {
			c.profiler.EmitError("Request Error", pageID, err.Error())
			return fmt.Errorf("error performing HTTP request: %w", err)
		}
		defer resp.Body.Close()
		durationMs := time.Since(requestStartTime).Milliseconds()

		// Compute response size
		responseSize := int(resp.ContentLength)
		if responseSize < 0 {
			responseSize = 0
		}

		// Capture pagination state BEFORE calling paginator.Next() (only if profiling)
		var previousPageState map[string]any
		if c.profiler.Enabled() {
			previousPageState = map[string]any{
				"pageNumber": pageNum,
				"params": map[string]any{
					"queryParams": next.QueryParams,
					"bodyParams":  next.BodyParams,
				},
				"nextPageUrl": next.NextPageUrl,
			}
		}

		// Update pagination state (reads and restores response body)
		next, stop, err = paginator.Next(resp)
		if err != nil {
			c.profiler.EmitError("Paginator Error", pageID, err.Error())
			return fmt.Errorf("paginator update error: %w", err)
		}

		// Decode JSON response
		var raw interface{}
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			c.profiler.EmitError("Response Decode Error", pageID, err.Error())
			return fmt.Errorf("error decoding response JSON: %w", err)
		}

		// Emit PAGINATION_EVAL event (if pagination is configured and this is not the first page)
		if pageNum > 0 && c.profiler.Enabled() && !stop {
			afterPageState := map[string]any{
				"pageNumber": paginator.PageNum(),
				"params": map[string]any{
					"queryParams": next.QueryParams,
					"bodyParams":  next.BodyParams,
				},
				"nextPageUrl": next.NextPageUrl,
			}

			c.profiler.EmitPaginationEval(pageID, exec.step, PaginationEvalData{
				PageNumber:           pageNum,
				PaginationConfig:     exec.step.Request.Pagination,
				PreviousResponseBody: previousResponseBody,
				PreviousHeaders:      previousResponseHeaders,
				PreviousState:        previousPageState,
				AfterState:           afterPageState,
			})
		}

		// Emit REQUEST_RESPONSE event (only compute data if profiler enabled)
		if c.profiler.Enabled() {
			responseHeaders := make(map[string]string)
			for k, v := range resp.Header {
				if len(v) > 0 {
					responseHeaders[k] = v[0]
				}
			}

			c.profiler.EmitRequestResponse(pageID, exec.step, ResponseData{
				StatusCode:   resp.StatusCode,
				Headers:      responseHeaders,
				Body:         raw,
				ResponseSize: responseSize,
				DurationMs:   durationMs,
			})
		}

		// Store response for next PAGINATION_EVAL event (only if profiling)
		if c.profiler.Enabled() {
			previousResponseBody = raw
			previousResponseHeaders = make(map[string]string)
			for k, v := range resp.Header {
				if len(v) > 0 {
					previousResponseHeaders[k] = v[0]
				}
			}
		}

		// Transform response
		transformed, err := exec.compiledStep.ExecuteResultTransformer(raw, templateCtx)
		if err != nil {
			c.profiler.EmitError("Response Transform Error", pageID, err.Error())
			return err
		}

		// Emit RESPONSE_TRANSFORM event
		c.profiler.EmitResponseTransform(pageID, exec.step, exec.step.ResultTransformer, raw, transformed)

		// Execute nested steps on transformed result
		// Note: request steps create a working context for the response data.
		// If the current context is canonical (like "root"), the working context
		// uses a unique key to avoid shadowing the original.
		cloneResult := childMapWithClonedContext(exec.contextMap, exec.currentContext, transformed, c.ContextMap)
		childContextMap := cloneResult.contextMap
		workingContextKey := cloneResult.workingKey

		// Emit CONTEXT_SELECTION event (context created for nested steps)
		if len(exec.step.Steps) > 0 {
			c.profiler.EmitContextSelection(pageID, exec.step, workingContextKey, childContextMap)
		}

		for i, step := range exec.step.Steps {
			nestedPath := fmt.Sprintf("%s.steps[%d]", exec.stepPath, i)
			newExec := c.newStepExecution(step, nestedPath, workingContextKey, childContextMap, pageID)
			if err := c.ExecuteStep(ctx, newExec); err != nil {
				return err
			}
		}

		// Get final result after nested steps from the working context
		transformed = childContextMap[workingContextKey].Data

		// Apply merge strategy (profiling is handled internally)
		if err := c.performMerge(exec, transformed, templateCtx, pageID); err != nil {
			c.profiler.EmitError("Merge Error", pageID, err.Error())
			return err
		}

		// Handle streaming at root level
		if exec.currentContext.depth == 0 && c.Config.Stream {
			if arrayData, ok := exec.currentContext.Data.([]interface{}); ok {
				for i, d := range arrayData {
					c.DataStream <- d
					c.profiler.EmitStreamResult(pageID, exec.step, d, i)
				}
				exec.currentContext.Data = []interface{}{}
			}
		}

		// Emit REQUEST_PAGE_END event
		c.profiler.EmitRequestPageEnd(pageID, stepID, exec.step, pageNum, pageStartTime)
	}

	// Emit REQUEST_STEP_END event
	c.profiler.EmitRequestStepEnd(stepID, exec.parentID, exec.step, stepStartTime)

	return nil
}

// executeForEachIteration executes a single forEach iteration
func (c *ApiCrawler) executeForEachIteration(
	ctx context.Context,
	index int,
	item any,
	exec *stepExecution,
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
	c.profiler.EmitContextSelectionWithWorker(stepID, exec.step, exec.step.As, childContextMap, workerID, workerPoolID)

	// Emit ITEM_SELECTION event with worker tracking
	itemID := c.profiler.EmitItemSelectionWithWorker(stepID, exec.step, index, item, exec.step.As, childContextMap[exec.step.As].Data, workerID, workerPoolID)

	// Execute nested steps
	for i, nested := range exec.step.Steps {
		nestedPath := fmt.Sprintf("%s.steps[%d]", exec.stepPath, i)
		newExec := c.newStepExecution(nested, nestedPath, exec.step.As, childContextMap, itemID)
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
	profilerEnabled := c.profiler.Enabled()
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
			result := c.executeForEachIteration(ctx, index, item, exec, stepID, threadID, workerPoolID)
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
				c.profiler.emit(event)
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
	stepID := c.profiler.EmitForEachStepStart(exec.step, exec.parentID)

	results := []interface{}{}
	var err error

	// Extract items to iterate over from path (forEach always uses path, validation ensures this)
	c.logger.Debug("[Foreach] Extracting from parent context with rule: %s", exec.step.Path)

	results, err = exec.compiledStep.ExecutePathExtractor(exec.currentContext.Data)
	if err != nil {
		c.profiler.EmitError("Path Extraction Error", stepID, err.Error())
		return fmt.Errorf("path extraction failed: %w", err)
	}

	// Determine execution mode and parameters
	var executionResults []interface{}

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
		if c.profiler.Enabled() {
			workerPoolID := stepID + "-pool"
			workerIDs := make([]int, maxConcurrency)
			for i := 0; i < maxConcurrency; i++ {
				workerIDs[i] = i
			}

			var rateLimit float64
			var burst int
			if rateLimiter != nil {
				rateLimit = exec.step.Parallelism.RequestsPerSecond
				burst = exec.step.Parallelism.Burst
			}

			c.profiler.EmitParallelismSetup(stepID, exec.step, ParallelismSetupData{
				MaxConcurrency: maxConcurrency,
				WorkerPoolID:   workerPoolID,
				WorkerIDs:      workerIDs,
				RateLimit:      rateLimit,
				Burst:          burst,
			})
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
			c.profiler.EmitContextSelection(stepID, exec.step, exec.step.As, childContextMap)

			// Emit ITEM_SELECTION event
			itemID := c.profiler.EmitItemSelection(stepID, exec.step, i, item, exec.step.As, childContextMap[exec.step.As].Data)

			for j, nested := range exec.step.Steps {
				nestedPath := fmt.Sprintf("%s.steps[%d]", exec.stepPath, j)
				newExec := c.newStepExecution(nested, nestedPath, exec.step.As, childContextMap, itemID)
				if err := c.ExecuteStep(ctx, newExec); err != nil {
					return err
				}
			}

			executionResults = append(executionResults, childContextMap[exec.step.As].Data)
		}
	}

	// Determine merge strategy
	templateCtx := c.contextMapToTemplate(exec.contextMap, c.runVars)

	// Check if custom merge rules are specified (compiled merge exists)
	hasCustomMerge := exec.compiledStep.Merge != nil || exec.step.NoopMerge

	if hasCustomMerge {
		// Use custom merge logic (same as request steps)
		if err := c.performMerge(exec, executionResults, templateCtx, stepID); err != nil {
			c.profiler.EmitError("Merge Error", stepID, err.Error())
			return err
		}
	} else /*if exec.step.Path != ""*/ {
		// Default: patch the array at exec.step.Path with new results
		if exec.compiledStep == nil || exec.compiledStep.SyntheticMerge == nil {
			c.profiler.EmitError("Merge Error", stepID, "synthetic merge not compiled")
			return fmt.Errorf("synthetic merge not compiled for step")
		}

		mergedData, mergeErr := exec.compiledStep.ExecuteSyntheticMerge(exec.currentContext.Data, executionResults)
		if mergeErr != nil {
			c.profiler.EmitError("Merge Error", stepID, mergeErr.Error())
			return fmt.Errorf("synthetic merge failed: %w", mergeErr)
		}

		exec.currentContext.Data = mergedData
	}

	// Handle streaming at root level
	if exec.currentContext.depth <= 1 && c.Config.Stream {
		if arrayData, ok := exec.currentContext.Data.([]interface{}); ok {
			for i, d := range arrayData {
				c.DataStream <- d
				c.profiler.EmitStreamResult(stepID, exec.step, d, i)
			}
			exec.currentContext.Data = []interface{}{}
		}
	}

	// Emit FOREACH_STEP_END event
	c.profiler.EmitForEachStepEnd(stepID, exec.parentID, exec.step, stepStartTime)

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
	stepID := c.profiler.EmitForValuesStepStart(exec.step, exec.parentID)

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
		c.profiler.EmitContextSelection(stepID, exec.step, exec.step.As, childContextMap)

		// Emit ITEM_SELECTION event (for forValues, use value selection)
		itemID := c.profiler.EmitValueSelection(stepID, exec.step, i, value, exec.step.As)

		// Execute nested steps with overlay context
		// Nested steps operate in parent context but have access to the value via 'as' key
		for j, nested := range exec.step.Steps {
			nestedPath := fmt.Sprintf("%s.steps[%d]", exec.stepPath, j)
			newExec := c.newStepExecution(nested, nestedPath, exec.currentContextKey, childContextMap, itemID)
			if err := c.ExecuteStep(ctx, newExec); err != nil {
				return err
			}
		}
	}

	// Emit FORVALUES_STEP_END event
	c.profiler.EmitForValuesStepEnd(stepID, exec.parentID, exec.step, stepStartTime)

	return nil
}

// performMerge applies the appropriate merge strategy based on step configuration.
// Handles profiling internally (captures before/after state and emits events).
// Thread-safe: Uses mutex to protect concurrent access to contexts.
func (c *ApiCrawler) performMerge(exec *stepExecution, result any, templateCtx map[string]any, pageID string) error {
	// Check for noop merge (skip merging entirely)
	if exec.step.NoopMerge {
		c.logger.Debug("[Merge] noop merge - skipping")
		return nil
	}

	merge := exec.compiledStep.Merge

	// Execute merge with mutex protection
	c.mergeMutex.Lock()
	defer c.mergeMutex.Unlock()

	// Capture data before merge (for profiling)
	var dataBefore any = nil
	if c.profiler.Enabled() {
		dataBefore = copyDataSafe(exec.currentContext)
	}
	mergeRule := "(default)"
	targetContextKey := exec.currentContext.key

	// If there's a compiled merge rule, use unified merge path
	if merge != nil && merge.Rule != nil {
		// Resolve target context
		var targetCtx *Context

		switch merge.Target {
		case MergeTargetCurrent:
			targetCtx = exec.currentContext
			targetContextKey = exec.currentContext.key
			c.logger.Debug("[Merge] merging to current context with expression: %s", merge.SourceRule)
		case MergeTargetParent:
			targetCtx = exec.contextMap[exec.currentContext.ParentContext]
			targetContextKey = exec.currentContext.ParentContext
			c.logger.Debug("[Merge] merging to parent context with expression: %s", merge.SourceRule)
		case MergeTargetNamed:
			var ok bool
			targetCtx, ok = exec.contextMap[merge.TargetName]
			if !ok {
				return fmt.Errorf("context '%s' not found", merge.TargetName)
			}
			targetContextKey = merge.TargetName
			c.logger.Debug("[Merge] merging to named context '%s' with expression: %s", merge.TargetName, merge.SourceRule)
		}

		if targetCtx == nil {
			return fmt.Errorf("merge target context is nil")
		}

		updated, err := merge.Rule.RunSingle(targetCtx.Data, result, templateCtx)
		if err != nil {
			return fmt.Errorf("merge failed: %w", err)
		}
		targetCtx.Data = updated
		mergeRule = merge.SourceRule

	} else {
		// Default merge (shallow merge for maps/arrays) - no explicit rule
		c.logger.Debug("[Merge] default merge")

		switch data := exec.currentContext.Data.(type) {
		case []interface{}:
			if resultArr, ok := result.([]interface{}); ok {
				exec.currentContext.Data = append(data, resultArr...)
			}
		case map[string]interface{}:
			if resultMap, ok := result.(map[string]interface{}); ok {
				for k, v := range resultMap {
					data[k] = v
				}
			}
		default:
			exec.currentContext.Data = result
		}
	}

	// Emit profiler event for default merge
	if c.profiler.Enabled() {
		dataAfter := copyDataSafe(exec.currentContext)
		c.profiler.EmitContextMerge(pageID, exec.step, MergeEventData{
			CurrentContextKey:   exec.currentContext.key,
			TargetContextKey:    targetContextKey,
			MergeRule:           mergeRule,
			TargetContextBefore: dataBefore,
			TargetContextAfter:  dataAfter,
			ContextMap:          exec.contextMap,
		})
	}

	return nil
}

// prepareHTTPRequest builds an HTTP request from the context and pagination parameters
func (c *ApiCrawler) prepareHTTPRequest(ctx httpRequestContext, templateCtx map[string]any) (*http.Request, *url.URL, error) {
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
		expandedURL, err := ctx.compiledStep.ExecuteURLTemplate(templateCtx, ctx.urlTemplate)
		if err != nil {
			return nil, nil, fmt.Errorf("error executing URL template: %w", err)
		}

		urlObj, err = url.Parse(expandedURL)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid URL %s: %w", expandedURL, err)
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

	// Configured body: use pre-compiled templates if available, else use raw values
	if ctx.compiledStep != nil && ctx.compiledStep.BodyTemplates != nil {
		expandedBody, err := ctx.compiledStep.ExecuteBodyTemplates(templateCtx)
		if err != nil {
			return nil, nil, fmt.Errorf("error expanding body: %w", err)
		}
		for k, v := range expandedBody {
			mergedBody[k] = v
		}
	} else {
		// No templates in body, use raw values
		for k, v := range ctx.configuredBody {
			mergedBody[k] = v
		}
	}

	// Add pagination body params (dynamic, use raw values - pagination doesn't use templates)
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

	// Apply headers (priority: global < request-specific)
	// Use direct map assignment to preserve exact header casing (Set() canonicalizes)
	// Use pre-compiled global headers
	expandedGlobalHeaders, err := c.CompiledConfig.ExecuteGlobalHeaders(templateCtx)
	if err != nil {
		return nil, nil, fmt.Errorf("error expanding global headers: %w", err)
	}
	for k, v := range expandedGlobalHeaders {
		req.Header[k] = []string{v}
	}

	// Request-specific headers: use pre-compiled templates for templated headers,
	// raw values for non-templated headers
	// Execute pre-compiled header templates
	expandedHeaders, err := ctx.compiledStep.ExecuteHeaderTemplates(templateCtx)
	if err != nil {
		return nil, nil, fmt.Errorf("error expanding headers: %w", err)
	}
	for k, v := range expandedHeaders {
		req.Header[k] = []string{v}
	}

	// Add headers without templates (use raw values)
	for k, v := range ctx.headers {
		if ctx.compiledStep == nil || ctx.compiledStep.HeaderTemplates == nil {
			// No compiled templates at all, use raw value
			req.Header[k] = []string{v}
		} else if _, hasCompiled := ctx.compiledStep.HeaderTemplates[k]; !hasCompiled {
			// This header wasn't compiled (no template markers), use raw value
			req.Header[k] = []string{v}
		}
		// If header was compiled, it was already set above
	}

	// Set Content-Type header if body is present
	if len(mergedBody) > 0 && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Apply authentication
	ctx.authenticator.PrepareRequest(req, ctx.requestID)

	return req, urlObj, nil
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
		ParentContext: currentContext.ParentContext, // Parent is the same as the original context
		key:           workingKey,
		depth:         currentContext.depth + 1,
	}

	return clonedContextResult{
		contextMap: newMap,
		workingKey: workingKey,
	}
}

// contextMapToTemplate creates a template context from the context map.
// Thread-safe: acquires mergeMutex to prevent races with concurrent merge operations.
// Also normalizes numeric values to avoid scientific notation in templates.
func (c *ApiCrawler) contextMapToTemplate(base map[string]*Context, vars map[string]any) map[string]interface{} {
	// Acquire lock to prevent races with merge operations
	c.mergeMutex.Lock()

	result := make(map[string]interface{})

	// First pass: add all contexts by their key (deep copy + normalize in one pass)
	for k, ctx := range base {
		result[k] = deepCopyAndNormalizeValue(ctx.Data)
	}

	// Second pass: spread map data from special contexts into top level
	// This allows templates to access fields directly (e.g., {{ .FacilityId }})
	// Priority: _response_* contexts first (most specific), then root
	for k, ctx := range base {
		// Spread _response_* context data (working contexts from request steps)
		if len(k) > 10 && k[:10] == "_response_" {
			if dataMap, ok := ctx.Data.(map[string]interface{}); ok {
				for field, v := range dataMap {
					result[field] = deepCopyAndNormalizeValue(v)
				}
			}
		}
	}

	// Finally spread root context data (lowest priority, won't overwrite)
	if rootCtx, ok := base["root"]; ok {
		if rootMap, ok := rootCtx.Data.(map[string]interface{}); ok {
			for k, v := range rootMap {
				if _, exists := result[k]; !exists {
					result[k] = deepCopyAndNormalizeValue(v)
				}
			}
		}
	}

	c.mergeMutex.Unlock()

	// Last: inject runtime variables at root level (highest priority, overrides everything)
	// This allows {{ .varName }} to resolve directly to the runtime variable
	// No need to lock for vars since they are read-only during execution
	// Normalize values to avoid scientific notation in templates
	for k, v := range vars {
		result[k] = deepCopyAndNormalizeValue(v)
	}

	return result
}

// deepCopyAndNormalizeValue recursively deep copies a value while normalizing floats.
// Used for template rendering to avoid scientific notation (e.g., 100024999 instead of 1.00025e+08)
func deepCopyAndNormalizeValue(v any) any {
	switch val := v.(type) {
	case float64:
		// Check if float64 is a whole number (no fractional part)
		if val == math.Trunc(val) && val >= math.MinInt64 && val <= math.MaxInt64 {
			return int64(val)
		}
		// For floats with decimals, convert to string to avoid scientific notation
		return strconv.FormatFloat(val, 'f', -1, 64)
	case map[string]interface{}:
		result := make(map[string]interface{}, len(val))
		for k, v := range val {
			result[k] = deepCopyAndNormalizeValue(v)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, v := range val {
			result[i] = deepCopyAndNormalizeValue(v)
		}
		return result
	case string, int, int64, int32, bool, nil:
		// Immutable types, safe to return as-is
		return val
	default:
		// For other types, return as-is (they should be immutable or not shared)
		return val
	}
}
