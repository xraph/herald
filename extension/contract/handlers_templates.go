package contract

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"

	"github.com/xraph/herald"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/template"
)

// maxRenderBytes bounds a whole preview request: the editor buffer plus the
// JSON size of its sample data and variables. Previews run on every pause in
// typing, and forge's transport puts no limit on a request body, so this is
// the only cap.
const maxRenderBytes = 256 << 10

// renderRequestSize is the size templates.render measures against
// maxRenderBytes. Data and variables count by their JSON encoding, the form
// they arrived in.
func renderRequestSize(in templatesRenderRequest) (int, error) {
	c := in.Content
	n := len(c.Subject) + len(c.HTML) + len(c.Text) + len(c.Title)
	data, err := json.Marshal(in.Data)
	if err != nil {
		return 0, err
	}
	n += len(data)
	if in.Variables != nil {
		vars, err := json.Marshal(*in.Variables)
		if err != nil {
			return 0, err
		}
		n += len(vars)
	}
	return n, nil
}

func registerTemplates(d *dispatcher.Dispatcher, deps Deps) error {
	for _, bind := range []func() error{
		func() error { return query(d, "templates.list", templatesListHandler(deps)) },
		func() error { return query(d, "templates.detail", templatesDetailHandler(deps)) },
		func() error { return query(d, "templates.resolve", templatesResolveHandler(deps)) },
		func() error { return query(d, "templates.render", templatesRenderHandler(deps)) },
		func() error { return command(d, "templates.create", templatesCreateHandler(deps)) },
		func() error { return command(d, "templates.update", templatesUpdateHandler(deps)) },
		func() error { return command(d, "templates.delete", templatesDeleteHandler(deps)) },
		func() error { return command(d, "templates.resetDefaults", templatesResetDefaultsHandler(deps)) },
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
		size, err := renderRequestSize(in)
		if err != nil {
			return template.PreviewResult{}, badRequest("the sample data could not be read")
		}
		if size > maxRenderBytes {
			return template.PreviewResult{}, badRequest("the template and its sample data are too large to preview")
		}
		c := in.Content
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

var (
	slugPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	localePattern   = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
	variablePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	categories      = []string{template.CategoryAuth, template.CategoryTransactional, template.CategoryMarketing, template.CategorySystem}
)

func validChannel(ch string) bool {
	return herald.ChannelType(ch).IsValid()
}

func validCategory(c string) bool { return slices.Contains(categories, c) }

// validLocale accepts "" (the fallback version) or a BCP 47 style tag.
func validLocale(l string) bool { return l == "" || localePattern.MatchString(l) }

func validVariables(vars []VariableWire) error {
	seen := map[string]bool{}
	for _, v := range vars {
		name := strings.TrimSpace(v.Name)
		if !variablePattern.MatchString(name) {
			return badRequest("variable names must be Go template field names (letters, digits, underscores)")
		}
		if seen[name] {
			return badRequest("variable " + name + " is declared twice")
		}
		seen[name] = true
	}
	return nil
}

// versionContent is one locale's content in a create request.
type versionContent struct {
	Locale  string `json:"locale"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
	Title   string `json:"title"`
}

type templatesCreateRequest struct {
	Slug      string          `json:"slug"`
	Name      string          `json:"name"`
	Channel   string          `json:"channel"`
	Category  string          `json:"category"`
	Variables []VariableWire  `json:"variables"`
	Version   *versionContent `json:"version"`
}

type templateResponse struct {
	Template TemplateSummary `json:"template"`
}

// templatesCreateHandler creates a template and, optionally, its first
// version. If the version can't be created the template is removed again,
// so a create either lands whole or not at all.
func templatesCreateHandler(deps Deps) func(context.Context, templatesCreateRequest, contract.Principal) (templateResponse, error) {
	return func(ctx context.Context, in templatesCreateRequest, p contract.Principal) (templateResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return templateResponse{}, err
		}
		slug, name, channel := strings.TrimSpace(in.Slug), strings.TrimSpace(in.Name), strings.TrimSpace(in.Channel)
		category := strings.TrimSpace(in.Category)
		if category == "" {
			category = template.CategoryTransactional
		}
		switch {
		case !slugPattern.MatchString(slug):
			return templateResponse{}, badRequest("slug must be lower-case letters, digits, dots, dashes or underscores")
		case name == "":
			return templateResponse{}, badRequest("name is required")
		case !validChannel(channel):
			return templateResponse{}, badRequest("channel is not one Herald supports")
		case !validCategory(category):
			return templateResponse{}, badRequest("category must be auth, transactional, marketing or system")
		case in.Version != nil && !validLocale(strings.TrimSpace(in.Version.Locale)):
			return templateResponse{}, badRequest("locale must be empty (the fallback) or a tag like en or pt-BR")
		}
		if verr := validVariables(in.Variables); verr != nil {
			return templateResponse{}, verr
		}

		now := time.Now().UTC()
		t := &template.Template{
			ID: id.NewTemplateID(), AppID: appID, Slug: slug, Name: name, Channel: channel, Category: category,
			Variables: variablesFromWire(in.Variables), Enabled: true, CreatedAt: now, UpdatedAt: now,
		}
		st := deps.Herald.Store()
		if err = st.CreateTemplate(ctx, t); err != nil {
			return templateResponse{}, deps.mapError("templates.create", err)
		}
		if in.Version != nil {
			v := &template.Version{
				ID: id.NewTemplateVersionID(), TemplateID: t.ID, Locale: strings.TrimSpace(in.Version.Locale),
				Subject: in.Version.Subject, HTML: in.Version.HTML, Text: in.Version.Text, Title: in.Version.Title,
				Active: true, CreatedAt: now, UpdatedAt: now,
			}
			if err = st.CreateVersion(ctx, v); err != nil {
				_ = st.DeleteTemplate(ctx, t.ID) //nolint:errcheck // best-effort rollback; the version error is what the caller needs
				return templateResponse{}, deps.mapError("templates.create", err)
			}
		}
		saved, err := st.GetTemplate(ctx, t.ID)
		if err != nil {
			return templateResponse{}, deps.mapError("templates.create", err)
		}
		audit(ctx, deps, p, appID, "templates.create", "template", t.ID.String(), map[string]string{"slug": slug, "channel": channel})
		return templateResponse{Template: projectTemplate(saved)}, nil
	}
}

type templatesUpdateRequest struct {
	ID        string          `json:"id"`
	Name      *string         `json:"name"`
	Category  *string         `json:"category"`
	Enabled   *bool           `json:"enabled"`
	Variables *[]VariableWire `json:"variables"`
}

// templatesUpdateHandler changes name, category, enabled and variables. A
// template's slug and channel can't change: callers send by slug, and the
// pair is the template's identity.
func templatesUpdateHandler(deps Deps) func(context.Context, templatesUpdateRequest, contract.Principal) (templateResponse, error) {
	return func(ctx context.Context, in templatesUpdateRequest, p contract.Principal) (templateResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return templateResponse{}, err
		}
		t, err := ownedTemplate(ctx, deps, appID, in.ID)
		if err != nil {
			return templateResponse{}, err
		}
		if in.Name != nil {
			if strings.TrimSpace(*in.Name) == "" {
				return templateResponse{}, badRequest("name is required")
			}
			t.Name = strings.TrimSpace(*in.Name)
		}
		if in.Category != nil {
			if !validCategory(*in.Category) {
				return templateResponse{}, badRequest("category must be auth, transactional, marketing or system")
			}
			t.Category = *in.Category
		}
		if in.Enabled != nil {
			t.Enabled = *in.Enabled
		}
		if in.Variables != nil {
			if err := validVariables(*in.Variables); err != nil {
				return templateResponse{}, err
			}
			t.Variables = variablesFromWire(*in.Variables)
		}
		t.UpdatedAt = time.Now().UTC()
		if err := deps.Herald.Store().UpdateTemplate(ctx, t); err != nil {
			return templateResponse{}, deps.mapError("templates.update", err)
		}
		audit(ctx, deps, p, appID, "templates.update", "template", t.ID.String(), map[string]string{"slug": t.Slug})
		return templateResponse{Template: projectTemplate(t)}, nil
	}
}

type templatesDeleteRequest struct {
	ID string `json:"id"`
}

func templatesDeleteHandler(deps Deps) func(context.Context, templatesDeleteRequest, contract.Principal) (deleteResponse, error) {
	return func(ctx context.Context, in templatesDeleteRequest, p contract.Principal) (deleteResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return deleteResponse{}, err
		}
		t, err := ownedTemplate(ctx, deps, appID, in.ID)
		if err != nil {
			return deleteResponse{}, err
		}
		if err := deps.Herald.Store().DeleteTemplate(ctx, t.ID); err != nil {
			return deleteResponse{}, deps.mapError("templates.delete", err)
		}
		audit(ctx, deps, p, appID, "templates.delete", "template", t.ID.String(), map[string]string{"slug": t.Slug})
		return deleteResponse{OK: true, ID: t.ID.String()}, nil
	}
}

type templatesResetDefaultsRequest struct{}

type templatesResetDefaultsResponse struct {
	Deleted int `json:"deleted"`
	Seeded  int `json:"seeded"`
}

// templatesResetDefaultsHandler replaces the app's system templates with the
// factory defaults. Custom templates are kept; edits to system ones are lost.
func templatesResetDefaultsHandler(deps Deps) func(context.Context, templatesResetDefaultsRequest, contract.Principal) (templatesResetDefaultsResponse, error) {
	return func(ctx context.Context, _ templatesResetDefaultsRequest, p contract.Principal) (templatesResetDefaultsResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return templatesResetDefaultsResponse{}, err
		}
		countSystem := func() (int, error) {
			list, listErr := deps.Herald.Store().ListTemplates(ctx, appID)
			if listErr != nil {
				return 0, listErr
			}
			n := 0
			for _, t := range list {
				if t.IsSystem {
					n++
				}
			}
			return n, nil
		}
		before, err := countSystem()
		if err != nil {
			return templatesResetDefaultsResponse{}, deps.mapError("templates.resetDefaults", err)
		}
		if err = deps.Herald.ResetDefaultTemplates(ctx, appID); err != nil {
			return templatesResetDefaultsResponse{}, deps.mapError("templates.resetDefaults", err)
		}
		after, err := countSystem()
		if err != nil {
			return templatesResetDefaultsResponse{}, deps.mapError("templates.resetDefaults", err)
		}
		audit(ctx, deps, p, appID, "templates.resetDefaults", "template", "", map[string]string{
			"deleted": strconv.Itoa(before), "seeded": strconv.Itoa(after),
		})
		return templatesResetDefaultsResponse{Deleted: before, Seeded: after}, nil
	}
}
