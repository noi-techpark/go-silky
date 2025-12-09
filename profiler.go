// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package apigorowler

import (
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

// Helper to create profiler events with UUID4
func newProfilerEvent(eventType ProfileEventType, name string, parentID string, step Step) StepProfilerData {
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

// Helper to create profiler events with UUID4
func emitProfilerError(profiler chan StepProfilerData, name string, parentID string, err string) {
	if nil == profiler {
		return
	}
	profiler <- StepProfilerData{
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
}

// Helper to create profiler events with worker tracking
func newProfilerEventWithWorker(eventType ProfileEventType, name string, parentID string, step Step, workerID int, workerPool string) StepProfilerData {
	event := newProfilerEvent(eventType, name, parentID, step)
	if workerID >= 0 {
		event.WorkerID = workerID
		event.WorkerPool = workerPool
	}
	return event
}

// serializeContextMap converts a context map to a safe serializable format
// NOTE: Caller must hold c.mergeMutex if accessing shared context maps
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

// serializeContextMapSafe serializes context map with proper locking for concurrent access
func (c *ApiCrawler) serializeContextMapSafe(contextMap map[string]*Context) map[string]any {
	c.mergeMutex.Lock()
	defer c.mergeMutex.Unlock()
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
