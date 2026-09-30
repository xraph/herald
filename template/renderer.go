package template

import (
	"bytes"
	"errors"
	"fmt"
	htmltpl "html/template"
	"maps"
	"sort"
	"strings"
	texttpl "text/template"
	"time"
)

// Renderer errors.
var (
	ErrNoVersionForLocale      = errors.New("herald: no template version for locale")
	ErrTemplateRenderFailed    = errors.New("herald: template rendering failed")
	ErrMissingRequiredVariable = errors.New("herald: missing required template variable")
)

// RenderedContent holds the fully rendered template output.
type RenderedContent struct {
	Subject string
	HTML    string
	Text    string
	Title   string
}

// Renderer processes Go templates with data to produce rendered notification content.
type Renderer struct {
	funcMap texttpl.FuncMap
}

// NewRenderer creates a new template renderer with default helper functions.
func NewRenderer() *Renderer {
	return &Renderer{
		funcMap: defaultFuncMap(),
	}
}

// Render renders the version that answers locale (see Resolve) with data,
// after filling declared defaults. It stops at the first field that fails,
// which is what Send wants; Preview is the forgiving variant.
func (r *Renderer) Render(tmpl *Template, locale string, data map[string]any) (*RenderedContent, error) {
	version, match := Resolve(tmpl, locale)
	if match == MatchNone {
		return nil, fmt.Errorf("%w: template=%q locale=%q", ErrNoVersionForLocale, tmpl.Slug, locale)
	}
	return r.RenderVersion(version, tmpl.Variables, data)
}

// RenderVersion renders one version, active or not.
func (r *Renderer) RenderVersion(version *Version, vars []Variable, data map[string]any) (*RenderedContent, error) {
	data = withDefaults(vars, data)
	if err := r.validateVariables(vars, data); err != nil {
		return nil, err
	}

	var result RenderedContent
	var err error
	if version.Subject != "" {
		if result.Subject, err = r.renderText(version.Subject, data); err != nil {
			return nil, fmt.Errorf("%w: subject: %w", ErrTemplateRenderFailed, err)
		}
	}
	if version.HTML != "" {
		if result.HTML, err = r.renderHTML(version.HTML, data); err != nil {
			return nil, fmt.Errorf("%w: html: %w", ErrTemplateRenderFailed, err)
		}
	}
	if version.Text != "" {
		if result.Text, err = r.renderText(version.Text, data); err != nil {
			return nil, fmt.Errorf("%w: text: %w", ErrTemplateRenderFailed, err)
		}
	}
	if version.Title != "" {
		if result.Title, err = r.renderText(version.Title, data); err != nil {
			return nil, fmt.Errorf("%w: title: %w", ErrTemplateRenderFailed, err)
		}
	}
	return &result, nil
}

// validateVariables checks that all required variables are present in data.
func (r *Renderer) validateVariables(vars []Variable, data map[string]any) error {
	for _, v := range vars {
		if !v.Required {
			continue
		}

		if _, ok := data[v.Name]; !ok {
			if v.Default != "" {
				continue // has a default value
			}
			return fmt.Errorf("%w: %s", ErrMissingRequiredVariable, v.Name)
		}
	}

	return nil
}

// renderText renders a text/template string with the given data.
func (r *Renderer) renderText(tmplStr string, data map[string]any) (string, error) {
	t, err := texttpl.New("").Funcs(r.funcMap).Parse(tmplStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// renderHTML renders an html/template string with the given data (auto-escapes).
func (r *Renderer) renderHTML(tmplStr string, data map[string]any) (string, error) {
	t, err := htmltpl.New("").Funcs(r.funcMap).Parse(tmplStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// defaultFuncMap returns the built-in template helper functions.
func defaultFuncMap() texttpl.FuncMap {
	return texttpl.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": strings.Title, //nolint:staticcheck // simple title case is sufficient
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
		"default": func(def, val any) any {
			if val == nil || val == "" {
				return def
			}
			return val
		},
		"now": func() string {
			return time.Now().UTC().Format(time.RFC3339)
		},
		"formatDate": func(t time.Time, layout string) string {
			return t.Format(layout)
		},
	}
}

// FuncNames lists the helper functions templates can call, sorted.
func (r *Renderer) FuncNames() []string {
	names := make([]string, 0, len(r.funcMap))
	for name := range r.funcMap {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// withDefaults returns a copy of data with every declared variable that data
// lacks filled from its Default. The caller's map is never changed, and a
// value the caller supplied always wins.
func withDefaults(vars []Variable, data map[string]any) map[string]any {
	out := make(map[string]any, len(data)+len(vars))
	maps.Copy(out, data)
	for _, v := range vars {
		if v.Default == "" {
			continue
		}
		if _, ok := out[v.Name]; !ok {
			out[v.Name] = v.Default
		}
	}
	return out
}
