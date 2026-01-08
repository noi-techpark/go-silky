// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompileJQ_ValidExpression tests that valid JQ expressions compile correctly
func TestCompileJQ_ValidExpression(t *testing.T) {
	tests := []struct {
		name       string
		expr       string
		variables  []string
		wantErr    bool
		wantPaths  []string
	}{
		{
			name:      "simple path",
			expr:      ".items",
			variables: nil,
			wantErr:   false,
			wantPaths: []string{"items"},
		},
		{
			name:      "nested path",
			expr:      ".data.items",
			variables: nil,
			wantErr:   false,
			wantPaths: []string{"data"},
		},
		{
			name:      "with variable",
			expr:      ".items = $res",
			variables: []string{"$res"},
			wantErr:   false,
			wantPaths: []string{"items"},
		},
		{
			name:      "multiple variables",
			expr:      ".items = $res | .ctx = $ctx",
			variables: []string{"$res", "$ctx"},
			wantErr:   false,
			wantPaths: []string{"items", "ctx"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled, err := compileJQ(tt.expr, tt.variables...)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, compiled)
			assert.NotNil(t, compiled.Code)
			assert.Equal(t, tt.expr, compiled.Expression)
			assert.Equal(t, tt.variables, compiled.Variables)
		})
	}
}

// TestCompileJQ_InvalidExpression tests that invalid JQ expressions fail at compile time
func TestCompileJQ_InvalidExpression(t *testing.T) {
	tests := []struct {
		name      string
		expr      string
		variables []string
	}{
		{
			name:      "syntax error",
			expr:      ".items[",
			variables: nil,
		},
		{
			name:      "unbalanced braces",
			expr:      ".items | { foo",
			variables: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled, err := compileJQ(tt.expr, tt.variables...)
			require.Error(t, err, "expected compilation to fail for invalid expression")
			assert.Nil(t, compiled)
		})
	}
}

// TestCompileJQ_Empty tests that empty expressions return nil
func TestCompileJQ_Empty(t *testing.T) {
	compiled, err := compileJQ("")
	require.NoError(t, err)
	assert.Nil(t, compiled)
}

// TestCompiledJQ_Run tests the Run method
func TestCompiledJQ_Run(t *testing.T) {
	compiled, err := compileJQ(".items")
	require.NoError(t, err)

	input := map[string]interface{}{
		"items": []interface{}{1, 2, 3},
	}

	result, err := compiled.Run(input)
	require.NoError(t, err)
	assert.Equal(t, []interface{}{1, 2, 3}, result)
}

// TestCompiledJQ_RunSingle tests the RunSingle method
func TestCompiledJQ_RunSingle(t *testing.T) {
	compiled, err := compileJQ(".name")
	require.NoError(t, err)

	input := map[string]interface{}{
		"name": "test",
	}

	result, err := compiled.RunSingle(input)
	require.NoError(t, err)
	assert.Equal(t, "test", result)
}

// TestCompiledJQ_RunArray tests the RunArray method
func TestCompiledJQ_RunArray(t *testing.T) {
	compiled, err := compileJQ(".items[]")
	require.NoError(t, err)

	input := map[string]interface{}{
		"items": []interface{}{"a", "b", "c"},
	}

	result, err := compiled.RunArray(input)
	require.NoError(t, err)
	assert.Equal(t, []interface{}{"a", "b", "c"}, result)
}

// TestCompileTemplate_Valid tests that valid templates compile correctly
func TestCompileTemplate_Valid(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantNil    bool
		wantFields []string
	}{
		{
			name:    "no template markers",
			source:  "just a plain string",
			wantNil: true,
		},
		{
			name:       "simple field",
			source:     "https://api.example.com/{{ .id }}",
			wantNil:    false,
			wantFields: []string{"id"},
		},
		{
			name:       "multiple fields",
			source:     "{{ .name }} - {{ .value }}",
			wantNil:    false,
			wantFields: []string{"name", "value"},
		},
		{
			name:       "nested field",
			source:     "{{ .item.id }}",
			wantNil:    false,
			wantFields: []string{"item.id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled, err := compileTemplate(tt.source)
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, compiled)
				return
			}
			require.NotNil(t, compiled)
			assert.NotNil(t, compiled.Template)
			assert.Equal(t, tt.source, compiled.Source)
			assert.ElementsMatch(t, tt.wantFields, compiled.UsedFields)
		})
	}
}

// TestCompileTemplate_Invalid tests that invalid templates fail at compile time
func TestCompileTemplate_Invalid(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "unclosed action",
			source: "{{ .field",
		},
		{
			name:   "invalid syntax",
			source: "{{ range }}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled, err := compileTemplate(tt.source)
			require.Error(t, err, "expected compilation to fail for invalid template")
			assert.Nil(t, compiled)
		})
	}
}

// TestCompiledTemplate_Execute tests the Execute method
func TestCompiledTemplate_Execute(t *testing.T) {
	compiled, err := compileTemplate("Hello {{ .name }}!")
	require.NoError(t, err)

	result, err := compiled.Execute(map[string]any{"name": "World"})
	require.NoError(t, err)
	assert.Equal(t, "Hello World!", result)
}

// TestExtractTemplateFields tests field extraction from templates
func TestExtractTemplateFields(t *testing.T) {
	tests := []struct {
		name   string
		tmpl   string
		want   []string
	}{
		{
			name: "empty",
			tmpl: "",
			want: nil,
		},
		{
			name: "no template",
			tmpl: "plain text",
			want: nil,
		},
		{
			name: "single field",
			tmpl: "{{ .name }}",
			want: []string{"name"},
		},
		{
			name: "multiple fields",
			tmpl: "{{ .first }} and {{ .second }}",
			want: []string{"first", "second"},
		},
		{
			name: "nested field",
			tmpl: "{{ .item.id }}",
			want: []string{"item.id"},
		},
		{
			name: "deduplicated",
			tmpl: "{{ .name }} {{ .name }}",
			want: []string{"name"},
		},
		{
			name: "compact syntax",
			tmpl: "{{.name}}",
			want: []string{"name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTemplateFields(tt.tmpl)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExtractJQPaths tests path extraction from JQ expressions
func TestExtractJQPaths(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want []string
	}{
		{
			name: "empty",
			expr: "",
			want: nil,
		},
		{
			name: "single path",
			expr: ".items",
			want: []string{"items"},
		},
		{
			name: "nested path",
			expr: ".data.items",
			want: []string{"data.items"},
		},
		{
			name: "multiple paths",
			expr: ".items = .newItems",
			want: []string{"items", "newItems"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJQPaths(tt.expr)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildSelectiveContext tests selective context building
func TestBuildSelectiveContext(t *testing.T) {
	fullCtx := map[string]*Context{
		"root": {
			Data: map[string]interface{}{
				"id":   123,
				"name": "test",
				"nested": map[string]interface{}{
					"value": "inner",
				},
			},
		},
		"item": {
			Data: map[string]interface{}{
				"itemId": 456,
			},
		},
	}

	vars := map[string]any{
		"apiKey": "secret",
	}

	tests := []struct {
		name     string
		required []string
		want     map[string]interface{}
	}{
		{
			name:     "single field from root",
			required: []string{"id"},
			want: map[string]interface{}{
				"id": 123,
			},
		},
		{
			name:     "field from named context",
			required: []string{"item"},
			want: map[string]interface{}{
				"item": map[string]interface{}{
					"itemId": 456,
				},
			},
		},
		{
			name:     "runtime variable",
			required: []string{"apiKey"},
			want: map[string]interface{}{
				"apiKey": "secret",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSelectiveContext(tt.required, fullCtx, vars)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestCompileStep tests step compilation
func TestCompileStep(t *testing.T) {
	step := &Step{
		Type: "request",
		Name: "test-request",
		Request: &RequestConfig{
			URL:    "https://api.example.com/{{ .id }}",
			Method: "GET",
			Headers: map[string]string{
				"Authorization": "Bearer {{ .token }}",
			},
		},
		ResultTransformer: ".data",
		MergeOn:           ".items = $res",
	}

	compiled, fields, err := CompileStep(step, "steps[0]")
	require.NoError(t, err)
	require.NotNil(t, compiled)

	// Check compiled URL template
	assert.NotNil(t, compiled.URLTemplate)
	assert.Contains(t, compiled.URLTemplate.UsedFields, "id")

	// Check compiled header templates
	assert.NotNil(t, compiled.HeaderTemplates)
	assert.NotNil(t, compiled.HeaderTemplates["Authorization"])

	// Check compiled JQ expressions
	assert.NotNil(t, compiled.ResultTransformer)
	assert.NotNil(t, compiled.MergeOn)

	// Check required fields collected
	assert.NotEmpty(t, fields)
}

// TestCompileStep_NestedSteps tests that nested steps are compiled
func TestCompileStep_NestedSteps(t *testing.T) {
	step := &Step{
		Type: "forEach",
		Name: "test-foreach",
		Path: ".items",
		As:   "item",
		Steps: []Step{
			{
				Type: "request",
				Name: "nested-request",
				Request: &RequestConfig{
					URL:    "https://api.example.com/{{ .item.id }}",
					Method: "GET",
				},
			},
		},
	}

	compiled, _, err := CompileStep(step, "steps[0]")
	require.NoError(t, err)
	require.NotNil(t, compiled)

	// Check path extractor
	assert.NotNil(t, compiled.PathExtractor)

	// Check nested steps
	assert.Len(t, compiled.NestedSteps, 1)
	assert.NotNil(t, compiled.NestedSteps[0].URLTemplate)
}

// TestCompileConfig tests full config compilation
func TestCompileConfig(t *testing.T) {
	cfg := Config{
		RootContext: []interface{}{},
		Steps: []Step{
			{
				Type: "request",
				Name: "first-request",
				Request: &RequestConfig{
					URL:    "https://api.example.com/data",
					Method: "GET",
				},
			},
			{
				Type: "forEach",
				Name: "iterate",
				Path: ".items",
				As:   "item",
				Steps: []Step{
					{
						Type: "request",
						Name: "nested-request",
						Request: &RequestConfig{
							URL:    "https://api.example.com/item/{{ .item.id }}",
							Method: "GET",
						},
					},
				},
			},
		},
	}

	compiled, err := CompileConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, compiled)

	// Check steps are accessible by path
	assert.NotNil(t, compiled.GetCompiledStep("steps[0]"))
	assert.NotNil(t, compiled.GetCompiledStep("steps[1]"))
	assert.NotNil(t, compiled.GetCompiledStep("steps[1].steps[0]"))
}

// TestCompileConfig_InvalidJQ tests that invalid JQ fails at compile time
func TestCompileConfig_InvalidJQ(t *testing.T) {
	cfg := Config{
		RootContext: []interface{}{},
		Steps: []Step{
			{
				Type: "forEach",
				Name: "bad-foreach",
				Path: ".items[",  // Invalid JQ
				As:   "item",
			},
		},
	}

	compiled, err := CompileConfig(cfg)
	require.Error(t, err, "expected compilation to fail for invalid JQ")
	assert.Nil(t, compiled)
}

// TestCompileConfig_InvalidTemplate tests that invalid templates fail at compile time
func TestCompileConfig_InvalidTemplate(t *testing.T) {
	cfg := Config{
		RootContext: []interface{}{},
		Steps: []Step{
			{
				Type: "request",
				Name: "bad-request",
				Request: &RequestConfig{
					URL:    "https://api.example.com/{{ .id",  // Invalid template
					Method: "GET",
				},
			},
		},
	}

	compiled, err := CompileConfig(cfg)
	require.Error(t, err, "expected compilation to fail for invalid template")
	assert.Nil(t, compiled)
}

// TestValidateAndCompile_FullIntegration tests the full validation and compilation flow
func TestValidateAndCompile_FullIntegration(t *testing.T) {
	cfg := Config{
		RootContext: []interface{}{},
		Steps: []Step{
			{
				Type: "request",
				Name: "test-request",
				Request: &RequestConfig{
					URL:    "https://api.example.com/{{ .id }}",  // URL with template
					Method: "GET",
				},
				ResultTransformer: ".data.items",
				MergeOn:           ". = $res",
			},
		},
	}

	compiled, errs, err := ValidateAndCompile(cfg)
	require.NoError(t, err)
	require.Empty(t, errs)
	require.NotNil(t, compiled)

	// Check compiled config
	assert.NotNil(t, compiled.Config)
	assert.NotNil(t, compiled.Steps)
	assert.NotNil(t, compiled.Topology)

	// Check step is compiled
	step := compiled.GetCompiledStep("steps[0]")
	require.NotNil(t, step)
	assert.NotNil(t, step.URLTemplate, "URLTemplate should be compiled for template URLs")
	assert.NotNil(t, step.ResultTransformer)
	assert.NotNil(t, step.MergeOn)
}

// TestValidateAndCompile_InvalidConfig tests that invalid configs return validation errors
func TestValidateAndCompile_InvalidConfig(t *testing.T) {
	cfg := Config{
		// Missing RootContext
		Steps: []Step{
			{
				Type: "invalid-type",  // Invalid step type
			},
		},
	}

	compiled, errs, err := ValidateAndCompile(cfg)
	require.NoError(t, err, "should return validation errors, not error")
	require.NotEmpty(t, errs)
	assert.Nil(t, compiled)
}
