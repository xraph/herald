package contract

import (
	"context"
	"strings"

	"github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/template"
)

// maxRenderBytes bounds the editor buffer a preview accepts. Previews run
// on every pause in typing; a template larger than this is not a template.
const maxRenderBytes = 256 << 10

func registerTemplates(d *dispatcher.Dispatcher, deps Deps) error {
	for _, bind := range []func() error{
		func() error { return query(d, "templates.list", templatesListHandler(deps)) },
		func() error { return query(d, "templates.detail", templatesDetailHandler(deps)) },
		func() error { return query(d, "templates.resolve", templatesResolveHandler(deps)) },
		func() error { return query(d, "templates.render", templatesRenderHandler(deps)) },
	} {
		if err := bind(); err != nil {
			return err
		}
	}
	return nil
}

func parseTemplateID(raw string) (id.TemplateID, error) {
	tid, err := id.ParseTemplateID(strings.TrimSpace(raw))
	if err != nil {
		return id.Nil, badRequest("id is not a template id")
	}
	return tid, nil
}

// ownedTemplate loads a template of appID, with its versions. Another app's
// template answers NOT_FOUND exactly like a missing one.
func ownedTemplate(ctx context.Context, deps Deps, appID, raw string) (*template.Template, error) {
	tid, err := parseTemplateID(raw)
	if err != nil {
		return nil, err
	}
	t, err := deps.Herald.Store().GetTemplate(ctx, tid)
	if err != nil {
		return nil, deps.mapError("template lookup", err)
	}
	if t.AppID != appID {
		return nil, notFound("template not found")
	}
	return t, nil
}

type templatesListRequest struct {
	Channel    string `json:"channel"`
	Category   string `json:"category"`
	NoFallback bool   `json:"noFallback"`
}

type templatesListResponse struct {
	Templates []TemplateSummary `json:"templates"`
}

func templatesListHandler(deps Deps) func(context.Context, templatesListRequest, contract.Principal) (templatesListResponse, error) {
	return func(ctx context.Context, in templatesListRequest, p contract.Principal) (templatesListResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return templatesListResponse{}, err
		}
		var list []*template.Template
		if ch := strings.TrimSpace(in.Channel); ch != "" {
			list, err = deps.Herald.Store().ListTemplatesByChannel(ctx, appID, ch)
		} else {
			list, err = deps.Herald.Store().ListTemplates(ctx, appID)
		}
		if err != nil {
			return templatesListResponse{}, deps.mapError("templates.list", err)
		}
		out := templatesListResponse{Templates: make([]TemplateSummary, 0, len(list))}
		for _, t := range list {
			if in.Category != "" && t.Category != in.Category {
				continue
			}
			if in.NoFallback && hasFallback(t) {
				continue
			}
			out.Templates = append(out.Templates, projectTemplate(t))
		}
		return out, nil
	}
}

type templatesDetailRequest struct {
	ID string `json:"id"`
}

type templatesDetailResponse struct {
	Template   TemplateDetail    `json:"template"`
	Resolution []ResolutionEntry `json:"resolution"`
}

// templatesDetailHandler answers templates.detail with the template, its
// variables and versions, and which version answers each locale it has, the
// "" fallback, and the configured default locale.
func templatesDetailHandler(deps Deps) func(context.Context, templatesDetailRequest, contract.Principal) (templatesDetailResponse, error) {
	return func(ctx context.Context, in templatesDetailRequest, p contract.Principal) (templatesDetailResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return templatesDetailResponse{}, err
		}
		t, err := ownedTemplate(ctx, deps, appID, in.ID)
		if err != nil {
			return templatesDetailResponse{}, err
		}
		seen := map[string]bool{}
		locales := []string{}
		add := func(l string) {
			if !seen[l] {
				seen[l] = true
				locales = append(locales, l)
			}
		}
		for _, v := range t.Versions {
			add(v.Locale)
		}
		add("")
		add(deps.Herald.Config().DefaultLocale)

		resolution := make([]ResolutionEntry, 0, len(locales))
		for _, l := range locales {
			v, match := template.Resolve(t, l)
			entry := ResolutionEntry{Locale: l, Match: string(match)}
			if v != nil {
				vid := v.ID.String()
				entry.VersionID = &vid
			}
			resolution = append(resolution, entry)
		}
		return templatesDetailResponse{Template: projectTemplateDetail(t), Resolution: resolution}, nil
	}
}

type templatesResolveRequest struct {
	ID     string `json:"id"`
	Locale string `json:"locale"`
}

type resolveStep struct {
	Try       string  `json:"try"`
	Match     string  `json:"match"`
	Found     bool    `json:"found"`
	VersionID *string `json:"versionId,omitempty"`
}

type templatesResolveResponse struct {
	Locale    string        `json:"locale"`
	Steps     []resolveStep `json:"steps"`
	VersionID *string       `json:"versionId"`
	Match     string        `json:"match"`
}

// templatesResolveHandler explains, step by step, which version answers any
// locale, so the locale tester doesn't refetch the whole template per key.
func templatesResolveHandler(deps Deps) func(context.Context, templatesResolveRequest, contract.Principal) (templatesResolveResponse, error) {
	return func(ctx context.Context, in templatesResolveRequest, p contract.Principal) (templatesResolveResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return templatesResolveResponse{}, err
		}
		t, err := ownedTemplate(ctx, deps, appID, in.ID)
		if err != nil {
			return templatesResolveResponse{}, err
		}
		locale := strings.TrimSpace(in.Locale)
		steps, v := template.Explain(t, locale)
		out := templatesResolveResponse{Locale: locale, Steps: make([]resolveStep, 0, len(steps)), Match: string(template.MatchNone)}
		for _, s := range steps {
			step := resolveStep{Try: s.Try, Match: string(s.Match), Found: s.Found}
			if s.VersionID != "" {
				vid := s.VersionID
				step.VersionID = &vid
			}
			out.Steps = append(out.Steps, step)
		}
		if v != nil {
			vid := v.ID.String()
			out.VersionID = &vid
			out.Match = string(steps[len(steps)-1].Match)
		}
		return out, nil
	}
}

type templatesRenderRequest struct {
	TemplateID string           `json:"templateId"`
	Content    template.Content `json:"content"`
	Variables  *[]VariableWire  `json:"variables"`
	Data       map[string]any   `json:"data"`
}

// templatesRenderHandler previews an editor buffer with Herald's own
// renderer against sample data. Nothing is sent. Every field renders on its
// own and every problem comes back as a positioned diagnostic. With a
// templateId, its stored variables apply unless the request sends unsaved
// ones.
func templatesRenderHandler(deps Deps) func(context.Context, templatesRenderRequest, contract.Principal) (template.PreviewResult, error) {
	return func(ctx context.Context, in templatesRenderRequest, p contract.Principal) (template.PreviewResult, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return template.PreviewResult{}, err
		}
		c := in.Content
		if len(c.Subject)+len(c.HTML)+len(c.Text)+len(c.Title) > maxRenderBytes {
			return template.PreviewResult{}, badRequest("the template is too large to preview")
		}
		var vars []template.Variable
		if in.TemplateID != "" {
			t, err := ownedTemplate(ctx, deps, appID, in.TemplateID)
			if err != nil {
				return template.PreviewResult{}, err
			}
			vars = t.Variables
		}
		if in.Variables != nil {
			vars = variablesFromWire(*in.Variables)
		}
		return *template.NewRenderer().Preview(c, vars, in.Data), nil
	}
}
