// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildTopology_Simple tests basic topology building
func TestBuildTopology_Simple(t *testing.T) {
	cfg := Config{
		RootContext: []interface{}{},
		Steps: []Step{
			{
				Type: "request",
				Name: "first-request",
			},
			{
				Type: "request",
				Name: "second-request",
			},
		},
	}

	topology := BuildTopology(cfg)
	require.NotNil(t, topology)

	// Check root nodes
	assert.Len(t, topology.Roots, 2)
	assert.Equal(t, 2, topology.TotalSteps)
	assert.Equal(t, 0, topology.MaxDepth)
	assert.Equal(t, 0, topology.ParallelSteps)
}

// TestBuildTopology_Nested tests topology with nested steps
func TestBuildTopology_Nested(t *testing.T) {
	cfg := Config{
		RootContext: []interface{}{},
		Steps: []Step{
			{
				Type: "forEach",
				Name: "outer",
				Path: ".items",
				As:   "item",
				Steps: []Step{
					{
						Type: "request",
						Name: "inner-request",
					},
					{
						Type: "forEach",
						Name: "inner-foreach",
						Path: ".subitems",
						As:   "subitem",
						Steps: []Step{
							{
								Type: "request",
								Name: "deepest-request",
							},
						},
					},
				},
			},
		},
	}

	topology := BuildTopology(cfg)
	require.NotNil(t, topology)

	// Check structure
	assert.Len(t, topology.Roots, 1)
	assert.Equal(t, 4, topology.TotalSteps)
	assert.Equal(t, 2, topology.MaxDepth)

	// Check root node
	root := topology.Roots[0]
	assert.Equal(t, "steps[0]", root.StepPath)
	assert.True(t, root.IsForEach)
	assert.Equal(t, 0, root.Depth)
	assert.Len(t, root.Children, 2)

	// Check nested nodes
	innerRequest := root.Children[0]
	assert.Equal(t, "steps[0].steps[0]", innerRequest.StepPath)
	assert.Equal(t, 1, innerRequest.Depth)
	assert.Equal(t, root, innerRequest.Parent)

	innerForEach := root.Children[1]
	assert.True(t, innerForEach.IsForEach)
	assert.Len(t, innerForEach.Children, 1)

	// Check deepest node
	deepest := innerForEach.Children[0]
	assert.Equal(t, "steps[0].steps[1].steps[0]", deepest.StepPath)
	assert.Equal(t, 2, deepest.Depth)
}

// TestBuildTopology_Parallel tests topology with parallel forEach
func TestBuildTopology_Parallel(t *testing.T) {
	cfg := Config{
		RootContext: []interface{}{},
		Steps: []Step{
			{
				Type: "forEach",
				Name: "parallel-foreach",
				Path: ".items",
				As:   "item",
				Parallelism: &ParallelismConfig{
					MaxConcurrency: 10,
				},
				Steps: []Step{
					{
						Type: "request",
						Name: "parallel-request",
					},
				},
			},
		},
	}

	topology := BuildTopology(cfg)
	require.NotNil(t, topology)

	assert.Equal(t, 1, topology.ParallelSteps)

	root := topology.Roots[0]
	assert.True(t, root.IsParallel)
	assert.True(t, root.IsForEach)
}

// TestBuildTopology_MergeTargets tests merge target detection
func TestBuildTopology_MergeTargets(t *testing.T) {
	tests := []struct {
		name         string
		step         Step
		wantTarget   string
		wantToRoot   bool
	}{
		{
			name: "mergeOn at root",
			step: Step{
				Type:    "request",
				Name:    "root-merge",
				MergeOn: ". = $res",
			},
			wantTarget: "current",
			wantToRoot: true,
		},
		{
			name: "mergeWithParentOn",
			step: Step{
				Type:              "request",
				Name:              "parent-merge",
				MergeWithParentOn: ".items = $res",
			},
			wantTarget: "parent",
			wantToRoot: true, // depth 0, parent is root
		},
		{
			name: "mergeWithContext to root",
			step: Step{
				Type: "request",
				Name: "context-merge-root",
				MergeWithContext: &MergeWithContextRule{
					Name: "root",
					Rule: ".items = $res",
				},
			},
			wantTarget: "root",
			wantToRoot: true,
		},
		{
			name: "mergeWithContext to named",
			step: Step{
				Type: "request",
				Name: "context-merge-named",
				MergeWithContext: &MergeWithContextRule{
					Name: "facility",
					Rule: ".details = $res",
				},
			},
			wantTarget: "facility",
			wantToRoot: false,
		},
		{
			name: "noopMerge",
			step: Step{
				Type:      "request",
				Name:      "noop-merge",
				NoopMerge: true,
			},
			wantTarget: "",
			wantToRoot: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				RootContext: []interface{}{},
				Steps:       []Step{tt.step},
			}

			topology := BuildTopology(cfg)
			require.NotNil(t, topology)
			require.Len(t, topology.Roots, 1)

			node := topology.Roots[0]
			assert.Equal(t, tt.wantTarget, node.MergeTarget, "merge target mismatch")
			assert.Equal(t, tt.wantToRoot, node.MergesToRoot, "mergesToRoot mismatch")
		})
	}
}

// TestTopology_GetNode tests node lookup by path
func TestTopology_GetNode(t *testing.T) {
	cfg := Config{
		RootContext: []interface{}{},
		Steps: []Step{
			{
				Type: "forEach",
				Name: "outer",
				Path: ".items",
				As:   "item",
				Steps: []Step{
					{
						Type: "request",
						Name: "inner",
					},
				},
			},
		},
	}

	topology := BuildTopology(cfg)

	// Find existing nodes
	outer := topology.GetNode("steps[0]")
	require.NotNil(t, outer)
	assert.Equal(t, "outer", outer.StepRef.Name)

	inner := topology.GetNode("steps[0].steps[0]")
	require.NotNil(t, inner)
	assert.Equal(t, "inner", inner.StepRef.Name)

	// Find non-existing node
	notFound := topology.GetNode("steps[99]")
	assert.Nil(t, notFound)
}

// TestTopology_WalkPreOrder tests pre-order traversal
func TestTopology_WalkPreOrder(t *testing.T) {
	cfg := Config{
		RootContext: []interface{}{},
		Steps: []Step{
			{
				Type: "forEach",
				Name: "first",
				Path: ".items",
				As:   "item",
				Steps: []Step{
					{
						Type: "request",
						Name: "first-child",
					},
				},
			},
			{
				Type: "request",
				Name: "second",
			},
		},
	}

	topology := BuildTopology(cfg)

	var visited []string
	topology.WalkPreOrder(func(node *StepNode) bool {
		visited = append(visited, node.StepRef.Name)
		return true
	})

	// Pre-order: parent before children
	assert.Equal(t, []string{"first", "first-child", "second"}, visited)
}

// TestTopology_WalkPostOrder tests post-order traversal
func TestTopology_WalkPostOrder(t *testing.T) {
	cfg := Config{
		RootContext: []interface{}{},
		Steps: []Step{
			{
				Type: "forEach",
				Name: "first",
				Path: ".items",
				As:   "item",
				Steps: []Step{
					{
						Type: "request",
						Name: "first-child",
					},
				},
			},
			{
				Type: "request",
				Name: "second",
			},
		},
	}

	topology := BuildTopology(cfg)

	var visited []string
	topology.WalkPostOrder(func(node *StepNode) bool {
		visited = append(visited, node.StepRef.Name)
		return true
	})

	// Post-order: children before parent
	assert.Equal(t, []string{"first-child", "first", "second"}, visited)
}

// TestTopology_GetStreamingPoints tests finding streaming points
func TestTopology_GetStreamingPoints(t *testing.T) {
	cfg := Config{
		RootContext: []interface{}{},
		Stream:      true,
		Steps: []Step{
			{
				Type:    "request",
				Name:    "streams-to-root",
				MergeOn: ". = $res",
			},
			{
				Type: "forEach",
				Name: "deep-foreach",
				Path: ".items",
				As:   "item",
				Steps: []Step{
					{
						Type: "forEach",
						Name: "deeper-foreach",
						Path: ".subitems",
						As:   "subitem",
						Steps: []Step{
							{
								Type:    "request",
								Name:    "deep-request",
								MergeOn: ". = $res",
							},
						},
					},
				},
			},
		},
	}

	topology := BuildTopology(cfg)
	points := topology.GetStreamingPoints()

	// Only depth <= 1 nodes that merge to root are streaming points
	var names []string
	for _, p := range points {
		names = append(names, p.StepRef.Name)
	}

	assert.Contains(t, names, "streams-to-root")
	// deep-foreach is at depth 0 but its merge is to current
	assert.Contains(t, names, "deep-foreach")
}

// TestTopology_GetParallelBranches tests finding parallel branches
func TestTopology_GetParallelBranches(t *testing.T) {
	cfg := Config{
		RootContext: []interface{}{},
		Steps: []Step{
			{
				Type: "forEach",
				Name: "parallel-1",
				Path: ".items",
				As:   "item",
				Parallelism: &ParallelismConfig{
					MaxConcurrency: 5,
				},
			},
			{
				Type: "forEach",
				Name: "sequential",
				Path: ".items",
				As:   "item",
				Steps: []Step{
					{
						Type: "forEach",
						Name: "parallel-nested",
						Path: ".subitems",
						As:   "subitem",
						Parallelism: &ParallelismConfig{
							MaxConcurrency: 3,
						},
					},
				},
			},
		},
	}

	topology := BuildTopology(cfg)
	branches := topology.GetParallelBranches()

	assert.Len(t, branches, 2)

	var names []string
	for _, b := range branches {
		names = append(names, b.StepRef.Name)
	}

	assert.Contains(t, names, "parallel-1")
	assert.Contains(t, names, "parallel-nested")
}

// TestStepNode_IsStreamingPoint tests streaming point detection
func TestStepNode_IsStreamingPoint(t *testing.T) {
	tests := []struct {
		name     string
		node     *StepNode
		expected bool
	}{
		{
			name:     "nil node",
			node:     nil,
			expected: false,
		},
		{
			name: "depth 0, merges to root",
			node: &StepNode{
				Depth:        0,
				MergesToRoot: true,
			},
			expected: true,
		},
		{
			name: "depth 1, merges to root",
			node: &StepNode{
				Depth:        1,
				MergesToRoot: true,
			},
			expected: true,
		},
		{
			name: "depth 2, merges to root",
			node: &StepNode{
				Depth:        2,
				MergesToRoot: true,
			},
			expected: false, // depth > 1
		},
		{
			name: "depth 0, does not merge to root",
			node: &StepNode{
				Depth:        0,
				MergesToRoot: false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.node.IsStreamingPoint()
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestTopology_String tests string representation
func TestTopology_String(t *testing.T) {
	cfg := Config{
		RootContext: []interface{}{},
		Steps: []Step{
			{
				Type: "request",
				Name: "test",
			},
		},
	}

	topology := BuildTopology(cfg)
	str := topology.String()

	assert.Contains(t, str, "steps=1")
	assert.Contains(t, str, "maxDepth=0")
	assert.Contains(t, str, "parallel=0")
}

// TestStepNode_String tests node string representation
func TestStepNode_String(t *testing.T) {
	node := &StepNode{
		StepPath:     "steps[0]",
		StepRef:      &Step{Name: "test-step"},
		Depth:        1,
		IsForEach:    true,
		IsParallel:   true,
		HasMerge:     true,
		MergesToRoot: true,
	}

	str := node.String()

	assert.Contains(t, str, "steps[0]")
	assert.Contains(t, str, "test-step")
	assert.Contains(t, str, "depth=1")
	assert.Contains(t, str, "F") // ForEach
	assert.Contains(t, str, "P") // Parallel
	assert.Contains(t, str, "M") // Merge
	assert.Contains(t, str, "R") // Root
}
