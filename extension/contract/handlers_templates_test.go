package contract

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/xraph/forge"

	"github.com/xraph/herald"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/store/memory"
	"github.com/xraph/herald/template"
)

// hasDiagnostic reports whether res has a diagnostic of kind whose message
// contains substr.
func hasDiagnostic(res template.PreviewResult, kind, substr string) bool {
	for _, d := range res.Diagnostics {
		if d.Kind == kind && strings.Contains(d.Message, substr) {
			return true
		}
	}
	return false
}

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

	// Stored variables are used when the request sends none: the stored
	// required user_name has no value here, so only they can produce this.
	got, err := templatesRenderHandler(e.deps)(bg, templatesRenderRequest{
		TemplateID: tmpl.ID.String(),
		Content:    template.Content{Subject: "Hi"},
	}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	if !hasDiagnostic(got, template.KindMissing, `"user_name"`) {
		t.Errorf("diagnostics = %+v, want a missing-required error for the stored user_name", got.Diagnostics)
	}

	// With the value supplied, the stored declaration covers .user_name: no
	// undeclared warning, no missing error.
	got, err = templatesRenderHandler(e.deps)(bg, templatesRenderRequest{
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
	if hasDiagnostic(got, template.KindUndeclared, "user_name") || hasDiagnostic(got, template.KindMissing, "user_name") {
		t.Errorf("diagnostics = %+v, want user_name covered by the stored variables", got.Diagnostics)
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

	// An empty variables list replaces the stored ones: user_name is no
	// longer declared, so using it warns and it is no longer required.
	none := []VariableWire{}
	got, err = templatesRenderHandler(e.deps)(bg, templatesRenderRequest{
		TemplateID: tmpl.ID.String(),
		Content:    template.Content{Subject: "Hi {{.user_name}}"},
		Data:       map[string]any{"user_name": "Ada"},
		Variables:  &none,
	}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	if !hasDiagnostic(got, template.KindUndeclared, "user_name") {
		t.Errorf("diagnostics = %+v, want an undeclared warning once variables: [] replaces the stored ones", got.Diagnostics)
	}
	got, err = templatesRenderHandler(e.deps)(bg, templatesRenderRequest{
		TemplateID: tmpl.ID.String(),
		Content:    template.Content{Subject: "Hi"},
		Variables:  &none,
	}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	if hasDiagnostic(got, template.KindMissing, "user_name") {
		t.Errorf("diagnostics = %+v, want no missing error: variables: [] drops the stored required one", got.Diagnostics)
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

	bigData := map[string]any{"blob": strings.Repeat("x", maxRenderBytes)}
	if _, err := templatesRenderHandler(e.deps)(bg, templatesRenderRequest{Content: template.Content{Text: "plain"}, Data: bigData}, as(appA)); codeOf(err) != "BAD_REQUEST" {
		t.Errorf("oversized data: %v, want BAD_REQUEST", err)
	}
	bigVars := []VariableWire{{Name: "v", Description: strings.Repeat("x", maxRenderBytes)}}
	if _, err := templatesRenderHandler(e.deps)(bg, templatesRenderRequest{Content: template.Content{Text: "plain"}, Variables: &bigVars}, as(appA)); codeOf(err) != "BAD_REQUEST" {
		t.Errorf("oversized variables: %v, want BAD_REQUEST", err)
	}
	// The cap is checked before the store: an oversized request naming a
	// missing template still answers BAD_REQUEST, not NOT_FOUND.
	if _, err := templatesRenderHandler(e.deps)(bg, templatesRenderRequest{TemplateID: id.NewTemplateID().String(), Data: bigData}, as(appA)); codeOf(err) != "BAD_REQUEST" {
		t.Errorf("oversized data with an unknown template: %v, want BAD_REQUEST before any lookup", err)
	}

	big := strings.Repeat("x", maxRenderBytes+1)
	if _, err := templatesRenderHandler(e.deps)(bg, templatesRenderRequest{Content: template.Content{HTML: big}}, as(appA)); codeOf(err) != "BAD_REQUEST" {
		t.Errorf("an oversized buffer: %v, want BAD_REQUEST", err)
	}
}

func TestTemplatesCreateUpdateDelete(t *testing.T) {
	e := newEnv(t)
	created, err := templatesCreateHandler(e.deps)(bg, templatesCreateRequest{
		Slug: "auth.welcome", Name: "Welcome", Channel: "email", Category: "auth",
		Variables: []VariableWire{{Name: "user_name", Required: true}},
		Version:   &versionContent{Locale: "en", Subject: "Hi {{.user_name}}", Text: "Hello"},
	}, as(appA))
	if err != nil {
		t.Fatalf("templates.create: %v", err)
	}
	if len(created.Template.Locales) != 1 || !created.Template.Locales[0].Active {
		t.Errorf("created = %+v", created.Template)
	}
	if _, err = templatesCreateHandler(e.deps)(bg, templatesCreateRequest{Slug: "auth.welcome", Name: "Again", Channel: "email"}, as(appA)); codeOf(err) != "CONFLICT" {
		t.Errorf("duplicate slug: %v, want CONFLICT", err)
	}
	if _, err = templatesCreateHandler(e.deps)(bg, templatesCreateRequest{Slug: "auth.welcome", Name: "Theirs", Channel: "email"}, as(appB)); err != nil {
		t.Errorf("the same slug in another app: %v", err)
	}

	name, off := "Renamed", false
	updated, err := templatesUpdateHandler(e.deps)(bg, templatesUpdateRequest{ID: created.Template.ID, Name: &name, Enabled: &off}, as(appA))
	if err != nil || updated.Template.Name != "Renamed" || updated.Template.Enabled {
		t.Fatalf("templates.update = %+v, %v", updated, err)
	}
	if updated.Template.Slug != "auth.welcome" || updated.Template.Channel != "email" {
		t.Error("slug and channel must not change through update")
	}

	if _, err := templatesDeleteHandler(e.deps)(bg, templatesDeleteRequest{ID: created.Template.ID}, as(appB)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("delete from another app: %v", err)
	}
	if _, err := templatesDeleteHandler(e.deps)(bg, templatesDeleteRequest{ID: created.Template.ID}, as(appA)); err != nil {
		t.Fatalf("templates.delete: %v", err)
	}
}

func TestTemplatesCreateValidates(t *testing.T) {
	e := newEnv(t)
	for i, in := range []templatesCreateRequest{
		{Slug: "", Name: "x", Channel: "email"},
		{Slug: "Has Space", Name: "x", Channel: "email"},
		{Slug: "ok", Name: " ", Channel: "email"},
		{Slug: "ok", Name: "x", Channel: "carrier-pigeon"},
		{Slug: "ok", Name: "x", Channel: "email", Category: "spam"},
		{Slug: "ok", Name: "x", Channel: "email", Variables: []VariableWire{{Name: "a"}, {Name: "a"}}},
		{Slug: "ok", Name: "x", Channel: "email", Variables: []VariableWire{{Name: "not valid"}}},
		{Slug: "ok", Name: "x", Channel: "email", Version: &versionContent{Locale: "not a locale!"}},
	} {
		if _, err := templatesCreateHandler(e.deps)(bg, in, as(appA)); codeOf(err) != "BAD_REQUEST" {
			t.Errorf("case %d: %v, want BAD_REQUEST", i, err)
		}
	}
}

func TestTemplatesResetDefaults(t *testing.T) {
	e := newEnv(t)
	got, err := templatesResetDefaultsHandler(e.deps)(bg, templatesResetDefaultsRequest{}, as(appA))
	if err != nil || got.Seeded == 0 || got.Deleted != 0 {
		t.Fatalf("first reset = %+v, %v", got, err)
	}
	again, err := templatesResetDefaultsHandler(e.deps)(bg, templatesResetDefaultsRequest{}, as(appA))
	if err != nil || again.Deleted != got.Seeded || again.Seeded != got.Seeded {
		t.Fatalf("second reset = %+v, %v; want it to replace the %d system templates", again, err, got.Seeded)
	}
}

// failingStore is a memory store whose CreateVersion always fails, and whose
// DeleteTemplate fails too when failDelete is set. It records the context
// DeleteTemplate ran under.
type failingStore struct {
	store.Store
	failDelete bool
	deleteCtx  context.Context
}

func (f *failingStore) CreateVersion(context.Context, *template.Version) error {
	return errors.New("disk on fire")
}

func (f *failingStore) DeleteTemplate(ctx context.Context, tid id.TemplateID) error {
	f.deleteCtx = ctx
	if f.failDelete {
		return errors.New("still on fire")
	}
	return f.Store.DeleteTemplate(ctx, tid)
}

// messageLogger records the message and fields of every Error call.
type messageLogger struct {
	forge.Logger
	entries []loggedEntry
}

type loggedEntry struct {
	msg    string
	fields map[string]any
}

func (l *messageLogger) Error(msg string, fields ...forge.Field) {
	e := loggedEntry{msg: msg, fields: map[string]any{}}
	for _, f := range fields {
		e.fields[f.Key()] = f.Value()
	}
	l.entries = append(l.entries, e)
}

func TestTemplatesCreateRollsBackWhenTheVersionFails(t *testing.T) {
	create := func(e *env) (templateResponse, error) {
		cancelled, cancel := context.WithCancel(bg)
		cancel() // the request is gone by the time the second write fails
		return templatesCreateHandler(e.deps)(cancelled, templatesCreateRequest{
			Slug: "auth.welcome", Name: "Welcome", Channel: "email",
			Version: &versionContent{Locale: "en", Text: "Hello"},
		}, as(appA))
	}

	t.Run("the template is removed and the version error is what comes back", func(t *testing.T) {
		fs := &failingStore{Store: memory.New()}
		e := newEnv(t, herald.WithStore(fs))
		log := &messageLogger{}
		e.deps.Logger = log

		_, err := create(e)
		if codeOf(err) != "INTERNAL" {
			t.Fatalf("templates.create = %v, want INTERNAL (the version failure)", err)
		}
		list, lerr := fs.ListTemplates(bg, appA)
		if lerr != nil || len(list) != 0 {
			t.Fatalf("templates after a failed create = %v, %v; want none", list, lerr)
		}
		if fs.deleteCtx == nil || fs.deleteCtx.Err() != nil {
			t.Error("the rollback must run under a context that outlives the cancelled request")
		}
		for _, en := range log.entries {
			if strings.Contains(en.msg, "roll back") {
				t.Errorf("a successful rollback was logged: %+v", en)
			}
		}
	})

	t.Run("a failed rollback is logged and does not change the response", func(t *testing.T) {
		fs := &failingStore{Store: memory.New(), failDelete: true}
		e := newEnv(t, herald.WithStore(fs))
		log := &messageLogger{}
		e.deps.Logger = log

		_, err := create(e)
		if codeOf(err) != "INTERNAL" {
			t.Fatalf("templates.create = %v, want INTERNAL (the version failure)", err)
		}
		var rollback *loggedEntry
		for i := range log.entries {
			if strings.Contains(log.entries[i].msg, "roll back") {
				rollback = &log.entries[i]
			}
		}
		if rollback == nil {
			t.Fatalf("the failed rollback was not logged: %+v", log.entries)
		}
		if rollback.fields["slug"] != "auth.welcome" || rollback.fields["templateId"] == "" {
			t.Errorf("rollback entry fields = %+v, want the slug and template id", rollback.fields)
		}
		list, lerr := fs.ListTemplates(bg, appA)
		if lerr != nil || len(list) != 1 {
			t.Errorf("the orphan should be what the log reports: %v, %v", list, lerr)
		}
	})
}
