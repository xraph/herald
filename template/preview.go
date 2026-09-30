package template

import (
	"bytes"
	"fmt"
	htmltpl "html/template"
	texttpl "text/template"
	"text/template/parse"
)

// Content is the editable text of one version: a saved version or an unsaved
// editor buffer.
type Content struct {
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
	Title   string `json:"title"`
}

// FieldOutput is one field's rendered text. Rendered is false when the field
// was empty or failed.
type FieldOutput struct {
	Field    string `json:"field"`
	Output   string `json:"output"`
	Rendered bool   `json:"rendered"`
}

// PreviewResult is every field's output and every problem found.
type PreviewResult struct {
	Fields      []FieldOutput `json:"fields"`
	Diagnostics []Diagnostic  `json:"diagnostics"`
}

// Preview renders every field of c independently against data, with variable
// defaults applied, and reports every problem as a Diagnostic. It never
// returns an error and never stops at the first failure, so a broken HTML body
// doesn't hide a broken subject. Nothing is sent.
func (r *Renderer) Preview(c Content, vars []Variable, data map[string]any) *PreviewResult {
	data = withDefaults(vars, data)
	declared := make(map[string]bool, len(vars))
	for _, v := range vars {
		declared[v.Name] = true
	}

	res := &PreviewResult{Fields: []FieldOutput{}, Diagnostics: []Diagnostic{}}
	for _, f := range []struct {
		name, src string
		html      bool
	}{
		{"subject", c.Subject, false},
		{"html", c.HTML, true},
		{"text", c.Text, false},
		{"title", c.Title, false},
	} {
		out := FieldOutput{Field: f.name}
		if f.src != "" {
			rendered, tree, err := r.renderField(f.name, f.src, f.html, data)
			if err != nil {
				res.Diagnostics = append(res.Diagnostics, diagnose(f.name, f.src, err))
			} else {
				out.Output, out.Rendered = rendered, true
			}
			if tree != nil {
				res.Diagnostics = append(res.Diagnostics, undeclared(f.name, f.src, tree, declared)...)
			}
		}
		res.Fields = append(res.Fields, out)
	}
	res.Diagnostics = append(res.Diagnostics, missingValues(vars, data)...)
	return res
}

// renderField parses and executes one field. The template is named after the
// field so Go's error messages say where they came from. The parse tree is
// returned whenever parsing succeeded, even if execution failed.
func (r *Renderer) renderField(name, src string, html bool, data map[string]any) (string, *parse.Tree, error) {
	var buf bytes.Buffer
	if html {
		//nolint:unconvert // html/template and text/template have distinct FuncMap types; conversion is required
		t, err := htmltpl.New(name).Funcs(htmltpl.FuncMap(r.funcMap)).Parse(src)
		if err != nil {
			return "", nil, err
		}
		if err := t.Execute(&buf, data); err != nil {
			return "", t.Tree, err
		}
		return buf.String(), t.Tree, nil
	}
	t, err := texttpl.New(name).Funcs(r.funcMap).Parse(src)
	if err != nil {
		return "", nil, err
	}
	if err := t.Execute(&buf, data); err != nil {
		return "", t.Tree, err
	}
	return buf.String(), t.Tree, nil
}

// undeclared warns once per name about a top-level field (.name or $.name)
// that no declared variable covers. Fields inside range and with bodies are
// skipped, because the dot has moved there.
func undeclared(field, src string, tree *parse.Tree, declared map[string]bool) []Diagnostic {
	var out []Diagnostic
	seen := map[string]bool{}
	walk(tree.Root, func(name string, n parse.Node) {
		if declared[name] || seen[name] {
			return
		}
		seen[name] = true
		loc, _ := tree.ErrorContext(n)
		line, col := position(src, loc)
		out = append(out, Diagnostic{
			Field: field, Line: line, Column: col,
			Severity: SeverityWarning, Kind: KindUndeclared,
			Message: fmt.Sprintf(".%s is used but not declared as a variable", name),
		})
	})
	return out
}

func walk(node parse.Node, visit func(name string, n parse.Node)) {
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, c := range n.Nodes {
			walk(c, visit)
		}
	case *parse.ActionNode:
		walk(n.Pipe, visit)
	case *parse.PipeNode:
		if n == nil {
			return
		}
		for _, c := range n.Cmds {
			walk(c, visit)
		}
	case *parse.CommandNode:
		for _, a := range n.Args {
			walk(a, visit)
		}
	case *parse.FieldNode:
		visit(n.Ident[0], n)
	case *parse.VariableNode:
		if len(n.Ident) > 1 && n.Ident[0] == "$" {
			visit(n.Ident[1], n)
		}
	case *parse.ChainNode:
		walk(n.Node, visit)
	case *parse.IfNode:
		walk(n.Pipe, visit)
		walk(n.List, visit)
		walk(n.ElseList, visit)
	case *parse.RangeNode:
		walk(n.Pipe, visit)
		walk(n.ElseList, visit)
	case *parse.WithNode:
		walk(n.Pipe, visit)
		walk(n.ElseList, visit)
	case *parse.TemplateNode:
		walk(n.Pipe, visit)
	}
}

// missingValues reports declared variables with no value in data after
// defaults were applied: an error for a required one, because Render would
// refuse it, and a warning for an optional one.
func missingValues(vars []Variable, data map[string]any) []Diagnostic {
	var out []Diagnostic
	for _, v := range vars {
		if _, ok := data[v.Name]; ok {
			continue
		}
		if v.Required {
			out = append(out, Diagnostic{
				Severity: SeverityError, Kind: KindMissing,
				Message: fmt.Sprintf("required variable %q has no value and no default", v.Name),
			})
			continue
		}
		out = append(out, Diagnostic{
			Severity: SeverityWarning, Kind: KindUnprovided,
			Message: fmt.Sprintf("%q has no sample value, so it renders as <no value> in text and nothing in HTML", v.Name),
		})
	}
	return out
}
