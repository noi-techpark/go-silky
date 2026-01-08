// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/itchyny/gojq"
)

// CompiledJQ holds a pre-compiled JQ expression with its metadata.
// Created at validation time to enable fail-fast and avoid runtime mutex contention.
type CompiledJQ struct {
	Code       *gojq.Code
	Expression string   // Original expression for error messages
	Variables  []string // Variable names (e.g., ["$res", "$ctx"])
	UsedPaths  []string // Paths referenced in the expression (for selective context)
}

// Run executes the compiled JQ expression against the input data.
// Deep copies input data to prevent race conditions.
func (c *CompiledJQ) Run(input any, variables ...any) (any, error) {
	if c == nil || c.Code == nil {
		return input, nil
	}

	// Deep copy input to prevent race conditions from jq's normalizeNumbers
	inputCopy := deepCopyValue(input)

	iter := c.Code.Run(inputCopy, variables...)

	// Collect results
	var results []interface{}
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return nil, fmt.Errorf("jq error in '%s': %w", c.Expression, err)
		}
		results = append(results, v)
	}

	// Return single result or array based on count
	if len(results) == 0 {
		return nil, nil
	}
	if len(results) == 1 {
		return results[0], nil
	}
	return results, nil
}

// RunSingle executes the JQ expression and expects exactly one result.
func (c *CompiledJQ) RunSingle(input any, variables ...any) (any, error) {
	if c == nil || c.Code == nil {
		return input, nil
	}

	inputCopy := deepCopyValue(input)
	iter := c.Code.Run(inputCopy, variables...)

	var result interface{}
	count := 0

	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return nil, fmt.Errorf("jq error in '%s': %w", c.Expression, err)
		}
		count++
		if count > 1 {
			return nil, fmt.Errorf("expression '%s' produced %d results, expected 1", c.Expression, count)
		}
		result = v
	}

	return result, nil
}

// RunArray executes the JQ expression and returns results as an array.
// Handles jq expressions that emit items one-by-one or as arrays.
func (c *CompiledJQ) RunArray(input any) ([]interface{}, error) {
	if c == nil || c.Code == nil {
		return nil, nil
	}

	inputCopy := deepCopyValue(input)
	iter := c.Code.Run(inputCopy)

	var results []interface{}
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return nil, fmt.Errorf("jq error in '%s': %w", c.Expression, err)
		}
		results = append(results, v)
	}

	// If jq returned a single array, unwrap it
	if len(results) == 1 {
		if arr, ok := results[0].([]interface{}); ok {
			return arr, nil
		}
	}

	return results, nil
}

// CompiledTemplate holds a pre-compiled Go template with its metadata.
// Created at validation time to enable fail-fast and avoid runtime mutex contention.
type CompiledTemplate struct {
	Template   *template.Template
	Source     string   // Original template string for error messages
	UsedFields []string // Fields referenced in the template (for selective context)
}

// Execute renders the template with the given context.
func (c *CompiledTemplate) Execute(ctx map[string]any) (string, error) {
	if c == nil || c.Template == nil {
		return "", nil
	}

	var buf bytes.Buffer
	if err := c.Template.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("template error in '%s': %w", c.Source, err)
	}
	return buf.String(), nil
}

// CompiledBodyTemplates holds compiled templates for a request body.
// Structure mirrors the body map structure with templates replacing string values.
type CompiledBodyTemplates struct {
	Templates map[string]*CompiledBodyValue
}

// CompiledBodyValue represents a body value that may contain templates.
// It mirrors the recursive structure of request bodies.
type CompiledBodyValue struct {
	// One of the following will be set:
	StringTemplate *CompiledTemplate          // For string values with templates
	Literal        any                        // For non-template values (numbers, bools, etc.)
	Map            map[string]*CompiledBodyValue // For nested objects
	Array          []*CompiledBodyValue        // For arrays
}

// Execute expands all templates in the body value recursively.
func (c *CompiledBodyValue) Execute(ctx map[string]any) (any, error) {
	if c == nil {
		return nil, nil
	}

	if c.StringTemplate != nil {
		return c.StringTemplate.Execute(ctx)
	}
	if c.Literal != nil {
		return c.Literal, nil
	}
	if c.Map != nil {
		result := make(map[string]any, len(c.Map))
		for k, v := range c.Map {
			expanded, err := v.Execute(ctx)
			if err != nil {
				return nil, fmt.Errorf("error expanding body key '%s': %w", k, err)
			}
			result[k] = expanded
		}
		return result, nil
	}
	if c.Array != nil {
		result := make([]any, len(c.Array))
		for i, v := range c.Array {
			expanded, err := v.Execute(ctx)
			if err != nil {
				return nil, fmt.Errorf("error expanding body index %d: %w", i, err)
			}
			result[i] = expanded
		}
		return result, nil
	}
	return nil, nil
}

// CompiledStep holds all pre-compiled expressions for a single step.
// This eliminates runtime compilation and its associated mutex contention.
type CompiledStep struct {
	StepPath string // Unique path to this step (e.g., "steps[0].steps[1]")

	// Request step compilations
	URLTemplate     *CompiledTemplate            // URL template
	HeaderTemplates map[string]*CompiledTemplate // Header value templates
	BodyTemplates   *CompiledBodyTemplates       // Body value templates

	// Transform and merge compilations
	ResultTransformer *CompiledJQ // Response transformation (.resultTransformer)
	MergeOn           *CompiledJQ // Merge with current context (.mergeOn)
	MergeWithParentOn *CompiledJQ // Merge with parent context (.mergeWithParentOn)
	MergeWithContext  *CompiledJQ // Merge with named context (.mergeWithContext.rule)

	// ForEach compilations
	PathExtractor  *CompiledJQ // Path extraction for forEach (.path)
	SyntheticMerge *CompiledJQ // Default forEach merge: path + " = $new"

	// Aggregated fields from all templates/JQ in this step
	// Used for building selective context (only copy what's needed)
	RequiredContextFields []string

	// Nested steps (pre-compiled recursively)
	NestedSteps []*CompiledStep
}

// BuildSelectiveContext creates a minimal context containing only the fields
// this step actually needs. This is more efficient than copying the entire
// context tree for every operation.
func (cs *CompiledStep) BuildSelectiveContext(fullCtx map[string]*Context, vars map[string]any) map[string]interface{} {
	if cs == nil || len(cs.RequiredContextFields) == 0 {
		// No field requirements known, fall back to full context
		return nil
	}
	return buildSelectiveContext(cs.RequiredContextFields, fullCtx, vars)
}

// ExecuteResultTransformer transforms the response using the pre-compiled JQ expression.
func (cs *CompiledStep) ExecuteResultTransformer(input any, templateCtx map[string]any) (any, error) {
	if cs == nil || cs.ResultTransformer == nil {
		return input, nil
	}
	return cs.ResultTransformer.RunSingle(input, templateCtx)
}

// ExecuteMergeOn applies the mergeOn rule using the pre-compiled JQ expression.
func (cs *CompiledStep) ExecuteMergeOn(contextData, result any, templateCtx map[string]any) (any, error) {
	if cs == nil || cs.MergeOn == nil {
		return nil, fmt.Errorf("no mergeOn rule compiled")
	}
	return cs.MergeOn.RunSingle(contextData, result, templateCtx)
}

// ExecuteMergeWithParentOn applies the mergeWithParentOn rule.
func (cs *CompiledStep) ExecuteMergeWithParentOn(parentData, result any, templateCtx map[string]any) (any, error) {
	if cs == nil || cs.MergeWithParentOn == nil {
		return nil, fmt.Errorf("no mergeWithParentOn rule compiled")
	}
	return cs.MergeWithParentOn.RunSingle(parentData, result, templateCtx)
}

// ExecuteMergeWithContext applies the mergeWithContext rule.
func (cs *CompiledStep) ExecuteMergeWithContext(targetData, result any, templateCtx map[string]any) (any, error) {
	if cs == nil || cs.MergeWithContext == nil {
		return nil, fmt.Errorf("no mergeWithContext rule compiled")
	}
	return cs.MergeWithContext.RunSingle(targetData, result, templateCtx)
}

// ExecutePathExtractor extracts items from context data for forEach iteration.
func (cs *CompiledStep) ExecutePathExtractor(data any) ([]interface{}, error) {
	if cs == nil || cs.PathExtractor == nil {
		return nil, nil
	}
	return cs.PathExtractor.RunArray(data)
}

// ExecuteSyntheticMerge applies the default forEach merge (path = $new).
func (cs *CompiledStep) ExecuteSyntheticMerge(contextData any, newValue any) (any, error) {
	if cs == nil || cs.SyntheticMerge == nil {
		return nil, fmt.Errorf("no synthetic merge rule compiled")
	}
	return cs.SyntheticMerge.RunSingle(contextData, newValue)
}

// ExecuteURLTemplate renders the URL with the given context.
func (cs *CompiledStep) ExecuteURLTemplate(ctx map[string]any) (string, error) {
	if cs == nil || cs.URLTemplate == nil {
		return "", nil
	}
	return cs.URLTemplate.Execute(ctx)
}

// ExecuteHeaderTemplates renders all header templates with the given context.
func (cs *CompiledStep) ExecuteHeaderTemplates(ctx map[string]any) (map[string]string, error) {
	if cs == nil || cs.HeaderTemplates == nil {
		return nil, nil
	}
	result := make(map[string]string, len(cs.HeaderTemplates))
	for k, tmpl := range cs.HeaderTemplates {
		v, err := tmpl.Execute(ctx)
		if err != nil {
			return nil, fmt.Errorf("error in header '%s': %w", k, err)
		}
		result[k] = v
	}
	return result, nil
}

// ExecuteBodyTemplates expands all body templates with the given context.
func (cs *CompiledStep) ExecuteBodyTemplates(ctx map[string]any) (map[string]any, error) {
	if cs == nil || cs.BodyTemplates == nil || cs.BodyTemplates.Templates == nil {
		return nil, nil
	}
	result := make(map[string]any, len(cs.BodyTemplates.Templates))
	for k, v := range cs.BodyTemplates.Templates {
		expanded, err := v.Execute(ctx)
		if err != nil {
			return nil, fmt.Errorf("error in body field '%s': %w", k, err)
		}
		result[k] = expanded
	}
	return result, nil
}

// CompiledConfig holds the fully compiled configuration.
// This is the result of ValidateAndCompile and eliminates all runtime compilation.
type CompiledConfig struct {
	Config   Config                   // Original configuration
	Steps    map[string]*CompiledStep // Pre-compiled steps keyed by step path
	Topology *StepTopology            // Step execution topology (nil until topology.go is created)
}

// GetCompiledStep retrieves a pre-compiled step by its path.
func (cc *CompiledConfig) GetCompiledStep(path string) *CompiledStep {
	if cc == nil || cc.Steps == nil {
		return nil
	}
	return cc.Steps[path]
}

// compileJQ compiles a JQ expression with the given variables.
// Returns nil if the expression is empty.
func compileJQ(expr string, variables ...string) (*CompiledJQ, error) {
	if expr == "" {
		return nil, nil
	}

	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression '%s': %w", expr, err)
	}

	code, err := gojq.Compile(query, gojq.WithVariables(variables))
	if err != nil {
		return nil, fmt.Errorf("failed to compile jq expression '%s': %w", expr, err)
	}

	return &CompiledJQ{
		Code:       code,
		Expression: expr,
		Variables:  variables,
		UsedPaths:  extractJQPaths(expr),
	}, nil
}

// compileTemplate compiles a Go template string.
// Returns nil if the string contains no template markers.
func compileTemplate(source string) (*CompiledTemplate, error) {
	// Skip if no template markers
	if source == "" || !containsTemplateMarkers(source) {
		return nil, nil
	}

	tmpl, err := template.New("dynamic").Parse(source)
	if err != nil {
		return nil, fmt.Errorf("invalid template '%s': %w", source, err)
	}

	return &CompiledTemplate{
		Template:   tmpl,
		Source:     source,
		UsedFields: extractTemplateFields(source),
	}, nil
}

// containsTemplateMarkers checks if a string contains Go template markers.
func containsTemplateMarkers(s string) bool {
	return len(s) >= 4 && (s[0] == '{' && s[1] == '{' ||
		(len(s) > 2 && (s[len(s)-2] == '}' && s[len(s)-1] == '}')) ||
		containsSubstring(s, "{{"))
}

// containsSubstring is a simple substring check (avoiding strings import for performance).
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// compileBodyValue recursively compiles template strings in a body value.
func compileBodyValue(v any) (*CompiledBodyValue, []string, error) {
	var allFields []string

	switch val := v.(type) {
	case string:
		tmpl, err := compileTemplate(val)
		if err != nil {
			return nil, nil, err
		}
		if tmpl != nil {
			allFields = append(allFields, tmpl.UsedFields...)
			return &CompiledBodyValue{StringTemplate: tmpl}, allFields, nil
		}
		// Non-template string, store as literal
		return &CompiledBodyValue{Literal: val}, nil, nil

	case map[string]any:
		compiled := make(map[string]*CompiledBodyValue, len(val))
		for k, v := range val {
			cv, fields, err := compileBodyValue(v)
			if err != nil {
				return nil, nil, fmt.Errorf("in field '%s': %w", k, err)
			}
			compiled[k] = cv
			allFields = append(allFields, fields...)
		}
		return &CompiledBodyValue{Map: compiled}, allFields, nil

	case []any:
		compiled := make([]*CompiledBodyValue, len(val))
		for i, v := range val {
			cv, fields, err := compileBodyValue(v)
			if err != nil {
				return nil, nil, fmt.Errorf("at index %d: %w", i, err)
			}
			compiled[i] = cv
			allFields = append(allFields, fields...)
		}
		return &CompiledBodyValue{Array: compiled}, allFields, nil

	default:
		// Non-string primitives (numbers, bools, nil)
		return &CompiledBodyValue{Literal: val}, nil, nil
	}
}

// CompileStep compiles all expressions in a step and its nested steps.
// stepPath is used for error messages and step lookup.
func CompileStep(step *Step, stepPath string) (*CompiledStep, []string, error) {
	cs := &CompiledStep{
		StepPath: stepPath,
	}
	var allFields []string
	var err error

	// Compile request-specific fields
	if step.Request != nil {
		// URL template
		cs.URLTemplate, err = compileTemplate(step.Request.URL)
		if err != nil {
			return nil, nil, fmt.Errorf("URL template: %w", err)
		}
		if cs.URLTemplate != nil {
			allFields = append(allFields, cs.URLTemplate.UsedFields...)
		}

		// Header templates
		if len(step.Request.Headers) > 0 {
			cs.HeaderTemplates = make(map[string]*CompiledTemplate, len(step.Request.Headers))
			for k, v := range step.Request.Headers {
				tmpl, err := compileTemplate(v)
				if err != nil {
					return nil, nil, fmt.Errorf("header '%s': %w", k, err)
				}
				if tmpl != nil {
					cs.HeaderTemplates[k] = tmpl
					allFields = append(allFields, tmpl.UsedFields...)
				}
			}
		}

		// Body templates
		if len(step.Request.Body) > 0 {
			cs.BodyTemplates = &CompiledBodyTemplates{
				Templates: make(map[string]*CompiledBodyValue, len(step.Request.Body)),
			}
			for k, v := range step.Request.Body {
				cv, fields, err := compileBodyValue(v)
				if err != nil {
					return nil, nil, fmt.Errorf("body field '%s': %w", k, err)
				}
				cs.BodyTemplates.Templates[k] = cv
				allFields = append(allFields, fields...)
			}
		}
	}

	// Compile result transformer
	if step.ResultTransformer != "" {
		cs.ResultTransformer, err = compileJQ(step.ResultTransformer, JQ_CTX_KEY)
		if err != nil {
			return nil, nil, fmt.Errorf("resultTransformer: %w", err)
		}
		allFields = append(allFields, cs.ResultTransformer.UsedPaths...)
	}

	// Compile merge rules
	if step.MergeOn != "" {
		cs.MergeOn, err = compileJQ(step.MergeOn, JQ_RES_KEY, JQ_CTX_KEY)
		if err != nil {
			return nil, nil, fmt.Errorf("mergeOn: %w", err)
		}
		allFields = append(allFields, cs.MergeOn.UsedPaths...)
	}

	if step.MergeWithParentOn != "" {
		cs.MergeWithParentOn, err = compileJQ(step.MergeWithParentOn, JQ_RES_KEY, JQ_CTX_KEY)
		if err != nil {
			return nil, nil, fmt.Errorf("mergeWithParentOn: %w", err)
		}
		allFields = append(allFields, cs.MergeWithParentOn.UsedPaths...)
	}

	if step.MergeWithContext != nil && step.MergeWithContext.Rule != "" {
		cs.MergeWithContext, err = compileJQ(step.MergeWithContext.Rule, JQ_RES_KEY, JQ_CTX_KEY)
		if err != nil {
			return nil, nil, fmt.Errorf("mergeWithContext.rule: %w", err)
		}
		allFields = append(allFields, cs.MergeWithContext.UsedPaths...)
	}

	// Compile forEach path extractor and synthetic merge
	if step.Path != "" {
		cs.PathExtractor, err = compileJQ(step.Path)
		if err != nil {
			return nil, nil, fmt.Errorf("path: %w", err)
		}
		allFields = append(allFields, cs.PathExtractor.UsedPaths...)

		// Compile synthetic merge for default forEach merge behavior
		syntheticRule := step.Path + " = $new"
		cs.SyntheticMerge, err = compileJQ(syntheticRule, "$new")
		if err != nil {
			return nil, nil, fmt.Errorf("synthetic merge rule: %w", err)
		}
	}

	// Compile nested steps recursively
	if len(step.Steps) > 0 {
		cs.NestedSteps = make([]*CompiledStep, len(step.Steps))
		for i, nested := range step.Steps {
			nestedPath := fmt.Sprintf("%s.steps[%d]", stepPath, i)
			nestedCompiled, nestedFields, err := CompileStep(&nested, nestedPath)
			if err != nil {
				return nil, nil, fmt.Errorf("nested step %d: %w", i, err)
			}
			cs.NestedSteps[i] = nestedCompiled
			allFields = append(allFields, nestedFields...)
		}
	}

	// Deduplicate and store required fields
	cs.RequiredContextFields = mergeFields(allFields)

	return cs, allFields, nil
}

// CompileConfig compiles all steps in a configuration.
// This is the main entry point for cold-start compilation.
func CompileConfig(cfg Config) (*CompiledConfig, error) {
	cc := &CompiledConfig{
		Config: cfg,
		Steps:  make(map[string]*CompiledStep),
	}

	// Compile all top-level steps
	for i, step := range cfg.Steps {
		stepPath := fmt.Sprintf("steps[%d]", i)
		compiled, _, err := CompileStep(&step, stepPath)
		if err != nil {
			return nil, fmt.Errorf("step %d (%s): %w", i, step.Name, err)
		}

		// Store in map with path as key
		cc.Steps[stepPath] = compiled

		// Also store nested steps in the flat map for O(1) lookup
		storeNestedSteps(cc.Steps, compiled)
	}

	return cc, nil
}

// storeNestedSteps recursively stores nested compiled steps in the flat map.
func storeNestedSteps(stepMap map[string]*CompiledStep, cs *CompiledStep) {
	for _, nested := range cs.NestedSteps {
		stepMap[nested.StepPath] = nested
		storeNestedSteps(stepMap, nested)
	}
}
