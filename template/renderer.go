package template

import (
	"bytes"
	"errors"
	"fmt"
	htmltpl "html/template"
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

// Render renders a template for the given locale with the provided data.
// It finds the best matching version (exact locale match, then default "").
func (r *Renderer) Render(tmpl *Template, locale string, data map[string]any) (*RenderedContent, error) {
	version := r.findVersion(tmpl, locale)
	if version == nil {
		return nil, fmt.Errorf("%w: template=%q locale=%q", ErrNoVersionForLocale, tmpl.Slug, locale)
	}

	if err := r.validateVariables(tmpl.Variables, data); err != nil {
		return nil, err
	}

	var result RenderedContent
	var err error

	if version.Subject != "" {
		result.Subject, err = r.renderText(version.Subject, data)
		if err != nil {
			return nil, fmt.Errorf("%w: subject: %w", ErrTemplateRenderFailed, err)
		}
	}

	if version.HTML != "" {
		result.HTML, err = r.renderHTML(version.HTML, data)
		if err != nil {
			return nil, fmt.Errorf("%w: html: %w", ErrTemplateRenderFailed, err)
		}
	}

	if version.Text != "" {
		result.Text, err = r.renderText(version.Text, data)
		if err != nil {
			return nil, fmt.Errorf("%w: text: %w", ErrTemplateRenderFailed, err)
		}
	}

	if version.Title != "" {
		result.Title, err = r.renderText(version.Title, data)
		if err != nil {
			return nil, fmt.Errorf("%w: title: %w", ErrTemplateRenderFailed, err)
		}
	}

	return &result, nil
}

// findVersion returns the version matching the locale, or the default ("") locale.
func (r *Renderer) findVersion(tmpl *Template, locale string) *Version {
	var defaultVersion *Version

	for i := range tmpl.Versions {
		v := &tmpl.Versions[i]
		if !v.Active {
			continue
		}

		if v.Locale == locale {
			return v
		}

		if v.Locale == "" {
			defaultVersion = v
		}
	}

	// If exact locale not found, try language-only match (e.g., "en" from "en-US")
	if locale != "" && strings.Contains(locale, "-") {
		lang := strings.SplitN(locale, "-", 2)[0]
		for i := range tmpl.Versions {
			v := &tmpl.Versions[i]
			if v.Active && v.Locale == lang {
				return v
			}
		}
	}

	return defaultVersion
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
