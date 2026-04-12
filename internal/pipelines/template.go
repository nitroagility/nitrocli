package pipelines

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"
)

// TemplateContext holds all namespaced data available in command templates.
// Templates use {{ .Env.VARIABLE }} syntax.
type TemplateContext struct {
	Env map[string]string `json:"env"`
}

// TemplateEngine evaluates Go templates in command arguments.
type TemplateEngine struct {
	ctx TemplateContext
}

// NewTemplateEngine creates a template engine with the given provider variables.
func NewTemplateEngine(envVars map[string]string) *TemplateEngine {
	return &TemplateEngine{
		ctx: TemplateContext{
			Env: envVars,
		},
	}
}

// Eval evaluates a single string as a Go template.
// Non-template strings are returned unchanged.
func (t *TemplateEngine) Eval(s string) (string, error) {
	return t.eval(s)
}

// EvalArgs evaluates templates in each argument string.
// Non-template strings are returned unchanged.
func (t *TemplateEngine) EvalArgs(args []string) ([]string, error) {
	result := make([]string, len(args))
	for i, arg := range args {
		resolved, err := t.eval(arg)
		if err != nil {
			return nil, fmt.Errorf("template error in arg %q: %w", arg, err)
		}
		result[i] = resolved
	}
	return result, nil
}

// safeFuncs is a restricted function map — blocks dangerous builtins like `call`.
var safeFuncs = template.FuncMap{
	"call": func(...any) (string, error) {
		return "", errors.New("call is not allowed in templates")
	},
}

func (t *TemplateEngine) eval(s string) (string, error) {
	tmpl, err := template.New("").Funcs(safeFuncs).Option("missingkey=error").Parse(s)
	if err != nil {
		return s, nil //nolint:nilerr // not a template, return as-is
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, t.ctx); err != nil {
		return "", fmt.Errorf("cannot resolve %q: %w", s, err)
	}

	return buf.String(), nil
}
