// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky

import (
	"sort"
	"sync"
)

// DeferredResult holds a single result from a forEach iteration.
// Used to collect results for atomic batch merging.
type DeferredResult struct {
	Index  int // Original iteration index (for ordered merge)
	Result any // The result from this iteration
}

// DeferredMergeCollector collects results from forEach iterations
// and merges them atomically when all iterations complete.
// This eliminates mutex contention during parallel execution.
type DeferredMergeCollector struct {
	results []DeferredResult
	mutex   sync.Mutex
	size    int // Expected number of results (for pre-allocation)
}

// NewDeferredMergeCollector creates a new collector with optional size hint.
func NewDeferredMergeCollector(sizeHint int) *DeferredMergeCollector {
	return &DeferredMergeCollector{
		results: make([]DeferredResult, 0, sizeHint),
		size:    sizeHint,
	}
}

// Collect adds a result from a forEach iteration.
// Thread-safe for concurrent collection.
func (c *DeferredMergeCollector) Collect(index int, result any) {
	c.mutex.Lock()
	c.results = append(c.results, DeferredResult{
		Index:  index,
		Result: result,
	})
	c.mutex.Unlock()
}

// Results returns all collected results sorted by index.
// This ensures deterministic merge order regardless of completion order.
func (c *DeferredMergeCollector) Results() []DeferredResult {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Sort by index for deterministic ordering
	sort.Slice(c.results, func(i, j int) bool {
		return c.results[i].Index < c.results[j].Index
	})

	return c.results
}

// ResultsAsSlice returns all collected results as an ordered slice.
// Useful for default forEach merge behavior (patch path with new array).
func (c *DeferredMergeCollector) ResultsAsSlice() []interface{} {
	sorted := c.Results()
	result := make([]interface{}, len(sorted))
	for i, r := range sorted {
		result[i] = r.Result
	}
	return result
}

// Count returns the number of collected results.
func (c *DeferredMergeCollector) Count() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return len(c.results)
}

// Clear resets the collector for reuse.
func (c *DeferredMergeCollector) Clear() {
	c.mutex.Lock()
	c.results = c.results[:0] // Keep capacity, clear contents
	c.mutex.Unlock()
}

// DeferredMergeOperation describes a deferred merge to be applied.
// Used when we need to batch multiple merge operations.
type DeferredMergeOperation struct {
	TargetContextKey string // Context key to merge into
	Result           any    // The result to merge
	MergeRule        string // JQ merge rule (if any)
}

// DeferredMergeBatch collects multiple merge operations for atomic application.
// This is used when multiple forEach iterations merge to the same context.
type DeferredMergeBatch struct {
	operations []DeferredMergeOperation
	mutex      sync.Mutex
}

// NewDeferredMergeBatch creates a new batch collector.
func NewDeferredMergeBatch(sizeHint int) *DeferredMergeBatch {
	return &DeferredMergeBatch{
		operations: make([]DeferredMergeOperation, 0, sizeHint),
	}
}

// Add queues a merge operation for later atomic application.
func (b *DeferredMergeBatch) Add(op DeferredMergeOperation) {
	b.mutex.Lock()
	b.operations = append(b.operations, op)
	b.mutex.Unlock()
}

// Operations returns all queued operations.
func (b *DeferredMergeBatch) Operations() []DeferredMergeOperation {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.operations
}

// ApplyAll applies all queued operations atomically.
// This should be called under a single mutex lock to ensure atomicity.
func (b *DeferredMergeBatch) ApplyAll(applyFn func(DeferredMergeOperation) error) error {
	b.mutex.Lock()
	ops := b.operations
	b.mutex.Unlock()

	for _, op := range ops {
		if err := applyFn(op); err != nil {
			return err
		}
	}
	return nil
}

// StreamingMergeResult holds data ready for streaming after merge.
// Used when a forEach branch completes and merges to root.
type StreamingMergeResult struct {
	Items     []interface{} // Items to stream
	StepPath  string        // Step that produced these items
	StepIndex int           // Index within parent forEach (if applicable)
}

// StreamingMergeCollector collects merge results that should trigger streaming.
// When a branch merges to root (depth <= 1), it queues items for streaming.
type StreamingMergeCollector struct {
	pending []StreamingMergeResult
	mutex   sync.Mutex
}

// NewStreamingMergeCollector creates a new streaming collector.
func NewStreamingMergeCollector() *StreamingMergeCollector {
	return &StreamingMergeCollector{
		pending: make([]StreamingMergeResult, 0),
	}
}

// Queue adds items to be streamed.
func (c *StreamingMergeCollector) Queue(stepPath string, stepIndex int, items []interface{}) {
	c.mutex.Lock()
	c.pending = append(c.pending, StreamingMergeResult{
		Items:     items,
		StepPath:  stepPath,
		StepIndex: stepIndex,
	})
	c.mutex.Unlock()
}

// Drain returns all pending items and clears the queue.
func (c *StreamingMergeCollector) Drain() []StreamingMergeResult {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	result := c.pending
	c.pending = make([]StreamingMergeResult, 0)
	return result
}

// HasPending returns true if there are items waiting to be streamed.
func (c *StreamingMergeCollector) HasPending() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return len(c.pending) > 0
}
