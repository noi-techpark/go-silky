// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky

import (
	"fmt"
)

// StepNode represents a step in the execution topology tree.
// Used for planning execution order, determining parallelism,
// and identifying when to emit streamed results.
type StepNode struct {
	StepPath string  // Unique path (e.g., "steps[0].steps[1]")
	StepRef  *Step   // Reference to the original step definition
	Parent   *StepNode
	Children []*StepNode
	Depth    int  // Distance from root (0 = top-level step)

	// Execution characteristics
	IsParallel   bool // forEach with parallelism config
	IsForEach    bool // forEach or forValues step
	HasMerge     bool // Step has explicit merge rule

	// Streaming characteristics
	MergesToRoot bool   // Merge target is root or depth <= 1 (triggers streaming)
	MergeTarget  string // Context name that this step merges to
}

// StepTopology holds the complete step execution topology.
// Built at validation time from the configuration.
type StepTopology struct {
	// Root nodes (top-level steps)
	Roots []*StepNode

	// All nodes indexed by path for O(1) lookup
	ByPath map[string]*StepNode

	// Statistics
	MaxDepth     int
	TotalSteps   int
	ParallelSteps int
}

// GetNode retrieves a step node by its path.
func (t *StepTopology) GetNode(path string) *StepNode {
	if t == nil || t.ByPath == nil {
		return nil
	}
	return t.ByPath[path]
}

// IsStreamingPoint returns true if this step should trigger streaming
// when it completes (merges to root at depth <= 1).
func (n *StepNode) IsStreamingPoint() bool {
	if n == nil {
		return false
	}
	return n.MergesToRoot && n.Depth <= 1
}

// GetMergeTargetNode returns the node that this step merges to.
func (n *StepNode) GetMergeTargetNode(topology *StepTopology) *StepNode {
	if n == nil || n.MergeTarget == "" {
		return nil
	}
	// For root, there's no node (it's the root context)
	if n.MergeTarget == "root" {
		return nil
	}
	// Look up by context name - this requires knowing which step
	// created that context (via 'as' field)
	return nil // Would need additional mapping
}

// BuildTopology constructs the step execution topology from a configuration.
// This analyzes the step hierarchy and determines execution characteristics.
func BuildTopology(cfg Config) *StepTopology {
	topology := &StepTopology{
		Roots:  make([]*StepNode, 0, len(cfg.Steps)),
		ByPath: make(map[string]*StepNode),
	}

	// Build nodes for all top-level steps
	for i := range cfg.Steps {
		path := fmt.Sprintf("steps[%d]", i)
		node := buildStepNode(&cfg.Steps[i], nil, path, 0, topology)
		topology.Roots = append(topology.Roots, node)
	}

	return topology
}

// buildStepNode recursively builds a step node and its children.
func buildStepNode(step *Step, parent *StepNode, path string, depth int, topology *StepTopology) *StepNode {
	node := &StepNode{
		StepPath: path,
		StepRef:  step,
		Parent:   parent,
		Children: make([]*StepNode, 0, len(step.Steps)),
		Depth:    depth,

		IsForEach:  step.Type == "forEach" || step.Type == "forValues",
		IsParallel: step.Parallelism != nil,
		HasMerge:   hasMergeRule(step),
	}

	// Determine merge target
	node.MergeTarget, node.MergesToRoot = determineMergeTarget(step, depth)

	// Update topology stats
	topology.TotalSteps++
	if depth > topology.MaxDepth {
		topology.MaxDepth = depth
	}
	if node.IsParallel {
		topology.ParallelSteps++
	}

	// Store in lookup map
	topology.ByPath[path] = node

	// Build child nodes recursively
	for i := range step.Steps {
		childPath := fmt.Sprintf("%s.steps[%d]", path, i)
		child := buildStepNode(&step.Steps[i], node, childPath, depth+1, topology)
		node.Children = append(node.Children, child)
	}

	return node
}

// hasMergeRule returns true if the step has any merge rule configured.
func hasMergeRule(step *Step) bool {
	return step.MergeOn != "" ||
		step.MergeWithParentOn != "" ||
		step.MergeWithContext != nil ||
		step.NoopMerge
}

// determineMergeTarget determines what context a step will merge to.
// Returns the target name and whether it merges to root (or depth <= 1).
func determineMergeTarget(step *Step, depth int) (target string, mergesToRoot bool) {
	// NoopMerge - no merge at all
	if step.NoopMerge {
		return "", false
	}

	// Explicit mergeWithContext - named target
	if step.MergeWithContext != nil {
		target := step.MergeWithContext.Name
		// "root" explicitly merges to root
		return target, target == "root"
	}

	// mergeWithParentOn - merges to parent context
	if step.MergeWithParentOn != "" {
		// Parent is one level up
		return "parent", depth <= 2 // depth 1 -> parent is root (depth 0)
	}

	// mergeOn or default merge - merges to current context
	// For top-level steps (depth 0), current context is root
	// For forEach items, current context is the iterator context
	if step.MergeOn != "" || step.Type == "request" {
		return "current", depth <= 1
	}

	// forEach/forValues default behavior - patches path in current context
	if step.Type == "forEach" && step.Path != "" {
		return "current", depth <= 1
	}

	return "", false
}

// WalkPreOrder visits all nodes in pre-order (parent before children).
func (t *StepTopology) WalkPreOrder(fn func(*StepNode) bool) {
	if t == nil {
		return
	}
	for _, root := range t.Roots {
		if !walkPreOrder(root, fn) {
			return
		}
	}
}

func walkPreOrder(node *StepNode, fn func(*StepNode) bool) bool {
	if node == nil {
		return true
	}
	if !fn(node) {
		return false
	}
	for _, child := range node.Children {
		if !walkPreOrder(child, fn) {
			return false
		}
	}
	return true
}

// WalkPostOrder visits all nodes in post-order (children before parent).
func (t *StepTopology) WalkPostOrder(fn func(*StepNode) bool) {
	if t == nil {
		return
	}
	for _, root := range t.Roots {
		if !walkPostOrder(root, fn) {
			return
		}
	}
}

func walkPostOrder(node *StepNode, fn func(*StepNode) bool) bool {
	if node == nil {
		return true
	}
	for _, child := range node.Children {
		if !walkPostOrder(child, fn) {
			return false
		}
	}
	return fn(node)
}

// GetStreamingPoints returns all nodes that should trigger streaming.
func (t *StepTopology) GetStreamingPoints() []*StepNode {
	if t == nil {
		return nil
	}

	var points []*StepNode
	t.WalkPreOrder(func(node *StepNode) bool {
		if node.IsStreamingPoint() {
			points = append(points, node)
		}
		return true
	})
	return points
}

// GetParallelBranches returns all nodes that execute in parallel.
func (t *StepTopology) GetParallelBranches() []*StepNode {
	if t == nil {
		return nil
	}

	var branches []*StepNode
	t.WalkPreOrder(func(node *StepNode) bool {
		if node.IsParallel {
			branches = append(branches, node)
		}
		return true
	})
	return branches
}

// String returns a human-readable representation of the topology.
func (t *StepTopology) String() string {
	if t == nil {
		return "<nil topology>"
	}
	return fmt.Sprintf("StepTopology{steps=%d, maxDepth=%d, parallel=%d}",
		t.TotalSteps, t.MaxDepth, t.ParallelSteps)
}

// NodeString returns a human-readable representation of a node.
func (n *StepNode) String() string {
	if n == nil {
		return "<nil node>"
	}

	flags := ""
	if n.IsForEach {
		flags += "F"
	}
	if n.IsParallel {
		flags += "P"
	}
	if n.HasMerge {
		flags += "M"
	}
	if n.MergesToRoot {
		flags += "R"
	}

	name := ""
	if n.StepRef != nil && n.StepRef.Name != "" {
		name = n.StepRef.Name
	}

	return fmt.Sprintf("StepNode{path=%s, name=%s, depth=%d, flags=[%s]}",
		n.StepPath, name, n.Depth, flags)
}
