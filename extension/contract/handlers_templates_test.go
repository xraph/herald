package contract

import (
	"strings"
	"testing"

	"github.com/xraph/herald/template"
)

func TestTemplatesListAndDetail(t *testing.T) {
	e := newEnv(t)
	welcome := e.template(t, appA, "auth.welcome", "email", "en")
	e.template(t, appA, "auth.goodbye", "sms", "en", "")
	e.template(t, appB, "theirs", "email", "en")

	list, err := templatesListHandler(e.deps)(bg, templatesListRequest{}, as(appA))
	if err != nil || len(list.Templates) != 2 {
		t.Fatalf("templates.list = %+v, %v", list, err)
	}
	onlyMissing, _ := templatesListHandler(e.deps)(bg, templatesListRequest{NoFallback: true}, as(appA))
	if len(onlyMissing.Templates) != 1 || onlyMissing.Templates[0].Slug != "auth.welcome" || onlyMissing.Templates[0].HasFallback {
		t.Errorf("noFallback filter = %+v", onlyMissing.Templates)
	}
	sms, _ := templatesListHandler(e.deps)(bg, templatesListRequest{Channel: "sms"}, as(appA))
	if len(sms.Templates) != 1 || sms.Templates[0].Slug != "auth.goodbye" {
		t.Errorf("channel filter = %+v", sms.Templates)
	}

	detail, err := templatesDetailHandler(e.deps)(bg, templatesDetailRequest{ID: welcome.ID.String()}, as(appA))
	if err != nil {
		t.Fatalf("templates.detail: %v", err)
	}
	if len(detail.Template.Versions) != 1 || len(detail.Template.Variables) != 1 {
		t.Errorf("detail = %+v", detail.Template)
	}
	// Resolution covers each version's locale, "" and the default locale.
	byLocale := map[string]ResolutionEntry{}
	for _, r := range detail.Resolution {
		byLocale[r.Locale] = r
	}
	if r := byLocale["en"]; r.Match != "exact" || r.VersionID == nil {
		t.Errorf("en resolves %+v", r)
	}
	if r := byLocale[""]; r.Match != "none" || r.VersionID != nil {
		t.Errorf("\"\" resolves %+v, want none: this template has no fallback", r)
	}
}

func TestTemplatesOwnership(t *testing.T) {
	e := newEnv(t)
	theirs := e.template(t, appB, "theirs", "email", "en")
	if _, err := templatesDetailHandler(e.deps)(bg, templatesDetailRequest{ID: theirs.ID.String()}, as(appA)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("templates.detail of another app's template: %v", err)
	}
	if _, err := templatesResolveHandler(e.deps)(bg, templatesResolveRequest{ID: theirs.ID.String(), Locale: "en"}, as(appA)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("templates.resolve of another app's template: %v", err)
	}
	if _, err := templatesRenderHandler(e.deps)(bg, templatesRenderRequest{TemplateID: theirs.ID.String()}, as(appA)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("templates.render of another app's template: %v", err)
	}
}

func TestTemplatesResolveExplainsEveryStep(t *testing.T) {
	e := newEnv(t)
	tmpl := e.template(t, appA, "auth.welcome", "email", "en")
	got, err := templatesResolveHandler(e.deps)(bg, templatesResolveRequest{ID: tmpl.ID.String(), Locale: "fr-CA"}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	if got.Match != "none" || got.VersionID != nil || len(got.Steps) != 3 {
		t.Fatalf("fr-CA on an en-only template = %+v", got)
	}
	if got.Steps[0].Try != "fr-CA" || got.Steps[1].Try != "fr" || got.Steps[2].Try != "" {
		t.Errorf("steps = %+v", got.Steps)
	}
}

func TestTemplatesRender(t *testing.T) {
	e := newEnv(t)
	tmpl := e.template(t, appA, "auth.welcome", "email", "en")

	// Stored variables are used when the request sends none.
	got, err := templatesRenderHandler(e.deps)(bg, templatesRenderRequest{
		TemplateID: tmpl.ID.String(),
		Content:    template.Content{Subject: "Hi {{.user_name}}", Text: "x\n  {{ index .user_name 99 }}"},
		Data:       map[string]any{"user_name": "Ada"},
	}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	if got.Fields[0].Output != "Hi Ada" || !got.Fields[0].Rendered {
		t.Errorf("subject = %+v", got.Fields[0])
	}
	var exec *template.Diagnostic
	for i := range got.Diagnostics {
		if got.Diagnostics[i].Kind == template.KindExec {
			exec = &got.Diagnostics[i]
		}
	}
	if exec == nil || exec.Field != "text" || exec.Line != 2 {
		t.Errorf("exec diagnostic = %+v", exec)
	}

	// Unsaved variable edits are honoured over the stored ones.
	unsaved := []VariableWire{{Name: "code", Type: "string", Required: true}}
	got, err = templatesRenderHandler(e.deps)(bg, templatesRenderRequest{
		TemplateID: tmpl.ID.String(),
		Content:    template.Content{Text: "{{.code}}"},
		Variables:  &unsaved,
	}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	missing := false
	for _, d := range got.Diagnostics {
		if d.Kind == template.KindMissing && strings.Contains(d.Message, `"code"`) {
			missing = true
		}
	}
	if !missing {
		t.Errorf("diagnostics = %+v, want a missing-required error for code", got.Diagnostics)
	}

	// A preview with no template at all still works (the create page uses it).
	if _, err := templatesRenderHandler(e.deps)(bg, templatesRenderRequest{Content: template.Content{Text: "plain"}}, as(appA)); err != nil {
		t.Errorf("render without a template: %v", err)
	}

	big := strings.Repeat("x", maxRenderBytes+1)
	if _, err := templatesRenderHandler(e.deps)(bg, templatesRenderRequest{Content: template.Content{HTML: big}}, as(appA)); codeOf(err) != "BAD_REQUEST" {
		t.Errorf("an oversized buffer: %v, want BAD_REQUEST", err)
	}
}
