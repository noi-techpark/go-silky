// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package apigorowler

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ProfileEventType int

const (
	// Root
	EVENT_ROOT_START ProfileEventType = iota

	// Request step container
	EVENT_REQUEST_STEP_START
	EVENT_REQUEST_STEP_END

	// Request step sub-events
	EVENT_CONTEXT_SELECTION
	EVENT_REQUEST_PAGE_START
	EVENT_REQUEST_PAGE_END
	EVENT_PAGINATION_EVAL
	EVENT_URL_COMPOSITION
	EVENT_REQUEST_DETAILS
	EVENT_REQUEST_RESPONSE
	EVENT_RESPONSE_TRANSFORM
	EVENT_CONTEXT_MERGE

	// ForEach step container
	EVENT_FOREACH_STEP_START
	EVENT_FOREACH_STEP_END

	// ForValues step container
	EVENT_FORVALUES_STEP_START
	EVENT_FORVALUES_STEP_END

	// ForEach/ForValues step sub-events
	EVENT_PARALLELISM_SETUP
	EVENT_ITEM_SELECTION

	// Authentication events
	EVENT_AUTH_START
	EVENT_AUTH_CACHED
	EVENT_AUTH_LOGIN_START
	EVENT_AUTH_LOGIN_END
	EVENT_AUTH_TOKEN_EXTRACT
	EVENT_AUTH_TOKEN_INJECT
	EVENT_AUTH_END

	// Result events
	EVENT_RESULT
	EVENT_STREAM_RESULT

	// Errors
	EVENT_ERROR
)

type StepProfilerData struct {
	// Core identification
	ID       string           `json:"id"`
	ParentID string           `json:"parentId,omitempty"`
	Type     ProfileEventType `json:"type"`
	Name     string           `json:"name"`
	Step     Step             `json:"step"`

	// Timeline
	Timestamp time.Time `json:"timestamp"`
	Duration  int64     `json:"durationMs,omitempty"` // Only in END events

	// Worker tracking (for parallel execution)
	WorkerID   int    `json:"workerId,omitempty"`
	WorkerPool string `json:"workerPool,omitempty"`

	// Flexible event-specific data
	Data map[string]any `json:"data"`
}

// Profiler wraps the profiler channel and provides methods for emitting events.
// All methods are safe to call even when the profiler is disabled (nil channel).
type Profiler struct {
	ch         chan StepProfilerData
	enabled    bool
	mergeMutex *sync.Mutex // Reference to crawler's merge mutex for safe context serialization
}

// NewProfiler creates a new Profiler instance
func NewProfiler(mergeMutex *sync.Mutex) *Profiler {
	return &Profiler{
		ch:         make(chan StepProfilerData),
		enabled:    true,
		mergeMutex: mergeMutex,
	}
}

// Channel returns the underlying channel for consumers to read from
func (p *Profiler) Channel() chan StepProfilerData {
	if p == nil {
		return nil
	}
	return p.ch
}

// Enabled returns whether profiling is enabled
func (p *Profiler) Enabled() bool {
	return p != nil && p.enabled
}

// emit sends an event to the profiler channel if enabled
func (p *Profiler) emit(event StepProfilerData) {
	if p == nil || !p.enabled {
		return
	}
	p.ch <- event
}

// newEvent creates a new profiler event with common fields populated
func newEvent(eventType ProfileEventType, name string, parentID string, step Step) StepProfilerData {
	return StepProfilerData{
		ID:        uuid.New().String(),
		ParentID:  parentID,
		Type:      eventType,
		Name:      name,
		Step:      step,
		Timestamp: time.Now(),
		Data:      make(map[string]any),
	}
}

// newEventWithWorker creates a new profiler event with worker tracking
func newEventWithWorker(eventType ProfileEventType, name string, parentID string, step Step, workerID int, workerPool string) StepProfilerData {
	event := newEvent(eventType, name, parentID, step)
	if workerID >= 0 {
		event.WorkerID = workerID
		event.WorkerPool = workerPool
	}
	return event
}

// =============================================================================
// Root Events
// =============================================================================

// EmitRootStart emits the root start event and returns the root ID
func (p *Profiler) EmitRootStart(config Config, contextMap map[string]*Context) string {
	if !p.Enabled() {
		return ""
	}

	event := newEvent(EVENT_ROOT_START, "Root Start", "", Step{})
	event.Data = map[string]any{
		"contextMap": p.serializeContextMapSafe(contextMap),
		"config": map[string]any{
			"rootContext": config.RootContext,
			"stream":      config.Stream,
		},
	}
	p.emit(event)
	return event.ID
}

// EmitFinalResult emits the final result event (non-streaming mode)
func (p *Profiler) EmitFinalResult(rootID string, result any) {
	if !p.Enabled() {
		return
	}

	event := newEvent(EVENT_RESULT, "Final Result", rootID, Step{})
	event.Data = map[string]any{
		"result": copyDataSafe(result),
	}
	p.emit(event)
}

// =============================================================================
// Request Step Events
// =============================================================================

// EmitRequestStepStart emits request step start and returns the step ID
func (p *Profiler) EmitRequestStepStart(step Step, parentID string) string {
	if !p.Enabled() {
		return ""
	}

	event := newEvent(EVENT_REQUEST_STEP_START, step.Name, parentID, step)
	event.Data = map[string]any{
		"stepName":   step.Name,
		"stepConfig": step,
	}
	p.emit(event)
	return event.ID
}

// EmitRequestStepEnd emits request step end event
func (p *Profiler) EmitRequestStepEnd(stepID string, parentID string, step Step, startTime time.Time) {
	if !p.Enabled() || stepID == "" {
		return
	}

	event := StepProfilerData{
		ID:        stepID,
		ParentID:  parentID,
		Type:      EVENT_REQUEST_STEP_END,
		Name:      step.Name + " End",
		Step:      step,
		Timestamp: time.Now(),
		Duration:  time.Since(startTime).Milliseconds(),
		Data:      make(map[string]any),
	}
	p.emit(event)
}

// EmitRequestPageStart emits page start and returns the page ID
func (p *Profiler) EmitRequestPageStart(stepID string, step Step, pageNum int) string {
	if !p.Enabled() {
		return ""
	}

	event := newEvent(EVENT_REQUEST_PAGE_START, fmt.Sprintf("Page %d", pageNum), stepID, step)
	event.Data = map[string]any{
		"pageNumber": pageNum,
	}
	p.emit(event)
	return event.ID
}

// EmitRequestPageEnd emits page end event
func (p *Profiler) EmitRequestPageEnd(pageID string, stepID string, step Step, pageNum int, startTime time.Time) {
	if !p.Enabled() || pageID == "" {
		return
	}

	event := StepProfilerData{
		ID:        pageID,
		ParentID:  stepID,
		Type:      EVENT_REQUEST_PAGE_END,
		Name:      fmt.Sprintf("Page %d End", pageNum),
		Step:      step,
		Timestamp: time.Now(),
		Duration:  time.Since(startTime).Milliseconds(),
		Data:      make(map[string]any),
	}
	p.emit(event)
}

// URLCompositionData holds data for URL composition event
type URLCompositionData struct {
	URLTemplate       string
	PageNumber        int
	QueryParams       map[string]string
	BodyParams        map[string]interface{}
	NextPageURL       string
	TemplateContext   map[string]any
	ResultURL         string
	ResultHeaders     map[string]string
	ResultBody        interface{}
}

// EmitURLComposition emits URL composition event
func (p *Profiler) EmitURLComposition(pageID string, step Step, data URLCompositionData) {
	if !p.Enabled() {
		return
	}

	event := newEvent(EVENT_URL_COMPOSITION, "URL Composition", pageID, step)
	event.Data = map[string]any{
		"urlTemplate": data.URLTemplate,
		"paginationState": map[string]any{
			"pageNumber":  data.PageNumber,
			"queryParams": data.QueryParams,
			"bodyParams":  data.BodyParams,
			"nextPageUrl": data.NextPageURL,
		},
		"goTemplateContext": data.TemplateContext,
		"resultUrl":         data.ResultURL,
		"resultHeaders":     data.ResultHeaders,
		"resultBody":        data.ResultBody,
	}
	p.emit(event)
}

// RequestDetailsData holds data for request details event
type RequestDetailsData struct {
	CurlCommand string
	Method      string
	URL         string
	Headers     map[string]string
	Body        map[string]interface{}
}

// EmitRequestDetails emits request details event
func (p *Profiler) EmitRequestDetails(pageID string, step Step, data RequestDetailsData) {
	if !p.Enabled() {
		return
	}

	event := newEvent(EVENT_REQUEST_DETAILS, "Request Details", pageID, step)
	event.Data = map[string]any{
		"curl":    data.CurlCommand,
		"method":  data.Method,
		"url":     data.URL,
		"headers": data.Headers,
		"body":    data.Body,
	}
	p.emit(event)
}

// ResponseData holds data for response event
type ResponseData struct {
	StatusCode   int
	Headers      map[string]string
	Body         any
	ResponseSize int
	DurationMs   int64
}

// EmitRequestResponse emits request response event
func (p *Profiler) EmitRequestResponse(pageID string, step Step, data ResponseData) {
	if !p.Enabled() {
		return
	}

	event := newEvent(EVENT_REQUEST_RESPONSE, "Request Response", pageID, step)
	event.Data = map[string]any{
		"statusCode":   data.StatusCode,
		"headers":      data.Headers,
		"body":         copyDataSafe(data.Body),
		"responseSize": data.ResponseSize,
		"durationMs":   data.DurationMs,
	}
	p.emit(event)
}

// PaginationEvalData holds data for pagination evaluation event
type PaginationEvalData struct {
	PageNumber           int
	PaginationConfig     Pagination
	PreviousResponseBody any
	PreviousHeaders      map[string]string
	PreviousState        map[string]any
	AfterState           map[string]any
}

// EmitPaginationEval emits pagination evaluation event
func (p *Profiler) EmitPaginationEval(pageID string, step Step, data PaginationEvalData) {
	if !p.Enabled() {
		return
	}

	event := newEvent(EVENT_PAGINATION_EVAL, "Pagination Evaluation", pageID, step)
	event.Data = map[string]any{
		"pageNumber":       data.PageNumber,
		"paginationConfig": data.PaginationConfig,
		"previousResponse": map[string]any{
			"body":    copyDataSafe(data.PreviousResponseBody),
			"headers": data.PreviousHeaders,
		},
		"previousState": data.PreviousState,
		"afterState":    data.AfterState,
	}
	p.emit(event)
}

// EmitResponseTransform emits response transform event
func (p *Profiler) EmitResponseTransform(pageID string, step Step, rule string, before any, after any) {
	if !p.Enabled() || rule == "" {
		return
	}

	event := newEvent(EVENT_RESPONSE_TRANSFORM, "Response Transform", pageID, step)
	event.Data = map[string]any{
		"transformRule":  rule,
		"beforeResponse": copyDataSafe(before),
		"afterResponse":  copyDataSafe(after),
	}
	p.emit(event)
}

// =============================================================================
// Context Events
// =============================================================================

// EmitContextSelection emits context selection event
func (p *Profiler) EmitContextSelection(parentID string, step Step, contextKey string, contextMap map[string]*Context) {
	if !p.Enabled() {
		return
	}

	event := newEvent(EVENT_CONTEXT_SELECTION, "Context Selection", parentID, step)
	contextPath := buildContextPath(contextMap, contextKey)
	event.Data = map[string]any{
		"contextPath":        contextPath,
		"currentContextKey":  contextKey,
		"currentContextData": copyDataSafe(contextMap[contextKey].Data),
		"fullContextMap":     p.serializeContextMapSafe(contextMap),
	}
	p.emit(event)
}

// EmitContextSelectionWithWorker emits context selection event with worker tracking
func (p *Profiler) EmitContextSelectionWithWorker(parentID string, step Step, contextKey string, contextMap map[string]*Context, workerID int, workerPool string) {
	if !p.Enabled() {
		return
	}

	event := newEventWithWorker(EVENT_CONTEXT_SELECTION, "Context Selection", parentID, step, workerID, workerPool)
	contextPath := buildContextPath(contextMap, contextKey)
	event.Data = map[string]any{
		"contextPath":        contextPath,
		"currentContextKey":  contextKey,
		"currentContextData": copyDataSafe(contextMap[contextKey].Data),
		"fullContextMap":     p.serializeContextMapSafe(contextMap),
	}
	p.emit(event)
}

// MergeEventData holds data for context merge event
type MergeEventData struct {
	CurrentContextKey   string
	TargetContextKey    string
	MergeRule           string
	TargetContextBefore any
	TargetContextAfter  any
	ContextMap          map[string]*Context
}

// EmitContextMerge emits context merge event
func (p *Profiler) EmitContextMerge(pageID string, step Step, data MergeEventData) {
	if !p.Enabled() {
		return
	}

	event := newEvent(EVENT_CONTEXT_MERGE, "Context Merge", pageID, step)
	event.Data = map[string]any{
		"currentContextKey":   data.CurrentContextKey,
		"targetContextKey":    data.TargetContextKey,
		"mergeRule":           data.MergeRule,
		"targetContextBefore": data.TargetContextBefore,
		"targetContextAfter":  data.TargetContextAfter,
		"fullContextMap":      p.serializeContextMapSafe(data.ContextMap),
	}
	p.emit(event)
}

// =============================================================================
// ForEach Step Events
// =============================================================================

// EmitForEachStepStart emits forEach step start and returns the step ID
func (p *Profiler) EmitForEachStepStart(step Step, parentID string) string {
	if !p.Enabled() {
		return ""
	}

	event := newEvent(EVENT_FOREACH_STEP_START, step.Name, parentID, step)
	event.Data = map[string]any{
		"stepName":   step.Name,
		"stepConfig": step,
	}
	p.emit(event)
	return event.ID
}

// EmitForEachStepEnd emits forEach step end event
func (p *Profiler) EmitForEachStepEnd(stepID string, parentID string, step Step, startTime time.Time) {
	if !p.Enabled() || stepID == "" {
		return
	}

	event := StepProfilerData{
		ID:        stepID,
		ParentID:  parentID,
		Type:      EVENT_FOREACH_STEP_END,
		Name:      step.Name + " End",
		Step:      step,
		Timestamp: time.Now(),
		Duration:  time.Since(startTime).Milliseconds(),
		Data:      make(map[string]any),
	}
	p.emit(event)
}

// =============================================================================
// ForValues Step Events
// =============================================================================

// EmitForValuesStepStart emits forValues step start and returns the step ID
func (p *Profiler) EmitForValuesStepStart(step Step, parentID string) string {
	if !p.Enabled() {
		return ""
	}

	event := newEvent(EVENT_FORVALUES_STEP_START, step.Name, parentID, step)
	event.Data = map[string]any{
		"stepName":   step.Name,
		"stepConfig": step,
		"values":     step.Values,
	}
	p.emit(event)
	return event.ID
}

// EmitForValuesStepEnd emits forValues step end event
func (p *Profiler) EmitForValuesStepEnd(stepID string, parentID string, step Step, startTime time.Time) {
	if !p.Enabled() || stepID == "" {
		return
	}

	event := StepProfilerData{
		ID:        stepID,
		ParentID:  parentID,
		Type:      EVENT_FORVALUES_STEP_END,
		Name:      step.Name + " End",
		Step:      step,
		Timestamp: time.Now(),
		Duration:  time.Since(startTime).Milliseconds(),
		Data:      make(map[string]any),
	}
	p.emit(event)
}

// =============================================================================
// Item Selection Events
// =============================================================================

// EmitItemSelection emits item selection event and returns the item ID
func (p *Profiler) EmitItemSelection(parentID string, step Step, index int, item any, contextKey string, contextData any) string {
	if !p.Enabled() {
		return ""
	}

	event := newEvent(EVENT_ITEM_SELECTION, fmt.Sprintf("Item %d", index), parentID, step)
	event.Data = map[string]any{
		"iterationIndex":     index,
		"itemValue":          copyDataSafe(item),
		"currentContextKey":  contextKey,
		"currentContextData": copyDataSafe(contextData),
	}
	p.emit(event)
	return event.ID
}

// EmitItemSelectionWithWorker emits item selection event with worker tracking
func (p *Profiler) EmitItemSelectionWithWorker(parentID string, step Step, index int, item any, contextKey string, contextData any, workerID int, workerPool string) string {
	if !p.Enabled() {
		return ""
	}

	event := newEventWithWorker(EVENT_ITEM_SELECTION, fmt.Sprintf("Item %d", index), parentID, step, workerID, workerPool)
	event.Data = map[string]any{
		"iterationIndex":     index,
		"itemValue":          copyDataSafe(item),
		"currentContextKey":  contextKey,
		"currentContextData": copyDataSafe(contextData),
	}
	p.emit(event)
	return event.ID
}

// EmitValueSelection emits value selection event (for forValues) and returns the item ID
func (p *Profiler) EmitValueSelection(parentID string, step Step, index int, value any, contextKey string) string {
	if !p.Enabled() {
		return ""
	}

	event := newEvent(EVENT_ITEM_SELECTION, fmt.Sprintf("Value %d", index), parentID, step)
	event.Data = map[string]any{
		"iterationIndex":    index,
		"itemValue":         copyDataSafe(value),
		"currentContextKey": contextKey,
	}
	p.emit(event)
	return event.ID
}

// =============================================================================
// Parallelism Events
// =============================================================================

// ParallelismSetupData holds data for parallelism setup event
type ParallelismSetupData struct {
	MaxConcurrency int
	WorkerPoolID   string
	WorkerIDs      []int
	RateLimit      float64
	Burst          int
}

// EmitParallelismSetup emits parallelism setup event
func (p *Profiler) EmitParallelismSetup(parentID string, step Step, data ParallelismSetupData) {
	if !p.Enabled() {
		return
	}

	event := newEvent(EVENT_PARALLELISM_SETUP, "Parallelism Setup", parentID, step)
	event.Data = map[string]any{
		"maxConcurrency": data.MaxConcurrency,
		"workerPoolId":   data.WorkerPoolID,
		"workerIds":      data.WorkerIDs,
	}
	if data.RateLimit > 0 {
		event.Data["rateLimit"] = data.RateLimit
		event.Data["burst"] = data.Burst
	}
	p.emit(event)
}

// =============================================================================
// Stream Events
// =============================================================================

// EmitStreamResult emits stream result event
func (p *Profiler) EmitStreamResult(parentID string, step Step, entity any, index int) {
	if !p.Enabled() {
		return
	}

	event := newEvent(EVENT_STREAM_RESULT, "Stream Result", parentID, step)
	event.Data = map[string]any{
		"entity": copyDataSafe(entity),
		"index":  index,
	}
	p.emit(event)
}

// =============================================================================
// Error Events
// =============================================================================

// EmitError emits an error event
func (p *Profiler) EmitError(name string, parentID string, err string) {
	if !p.Enabled() {
		return
	}

	event := StepProfilerData{
		ID:        uuid.New().String(),
		ParentID:  parentID,
		Type:      EVENT_ERROR,
		Name:      name,
		Step:      Step{},
		Timestamp: time.Now(),
		Data: map[string]any{
			"error": err,
		},
	}
	p.emit(event)
}

// =============================================================================
// Helper Functions
// =============================================================================

// CaptureContextDataBefore captures context data before a merge operation
// Returns nil if profiling is disabled
func (p *Profiler) CaptureContextDataBefore(ctx *Context) any {
	if !p.Enabled() {
		return nil
	}
	p.mergeMutex.Lock()
	defer p.mergeMutex.Unlock()
	return copyDataSafe(ctx.Data)
}

// CaptureContextDataAfter captures context data after a merge operation
// Returns nil if profiling is disabled
func (p *Profiler) CaptureContextDataAfter(ctx *Context) any {
	if !p.Enabled() {
		return nil
	}
	p.mergeMutex.Lock()
	defer p.mergeMutex.Unlock()
	return copyDataSafe(ctx.Data)
}

// serializeContextMap converts a context map to a safe serializable format
func serializeContextMap(contextMap map[string]*Context) map[string]any {
	result := make(map[string]any)
	for key, ctx := range contextMap {
		result[key] = map[string]any{
			"data":          copyDataSafe(ctx.Data),
			"parentContext": ctx.ParentContext,
			"depth":         ctx.depth,
			"key":           ctx.key,
		}
	}
	return result
}

// serializeContextMapSafe serializes context map with proper locking
func (p *Profiler) serializeContextMapSafe(contextMap map[string]*Context) map[string]any {
	if p == nil || p.mergeMutex == nil {
		return serializeContextMap(contextMap)
	}
	p.mergeMutex.Lock()
	defer p.mergeMutex.Unlock()
	return serializeContextMap(contextMap)
}

// buildContextPath builds the hierarchical path from root to current context
func buildContextPath(contextMap map[string]*Context, currentKey string) string {
	if currentKey == "" || currentKey == "root" {
		return "root"
	}

	ctx, exists := contextMap[currentKey]
	if !exists {
		return currentKey
	}

	// Build path from root to current by traversing parents
	path := []string{currentKey}
	parentKey := ctx.ParentContext

	for parentKey != "" && parentKey != "root" {
		path = append([]string{parentKey}, path...)
		if parentCtx, ok := contextMap[parentKey]; ok {
			parentKey = parentCtx.ParentContext
		} else {
			break
		}
	}

	// Add root at the beginning
	path = append([]string{"root"}, path...)

	// Join with dots
	result := ""
	for i, p := range path {
		if i > 0 {
			result += "."
		}
		result += p
	}
	return result
}

// copyDataSafe creates a safe copy of data for profiler events
func copyDataSafe(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case []interface{}:
		cp := make([]interface{}, len(val))
		for i, item := range val {
			cp[i] = copyDataSafe(item)
		}
		return cp
	case map[string]any:
		cp := make(map[string]any, len(val))
		for k, item := range val {
			cp[k] = copyDataSafe(item)
		}
		return cp
	default:
		// Primitives and other types are safe to share
		return v
	}
}

