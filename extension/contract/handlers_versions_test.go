package contract

import "testing"

func TestVersionsLifecycle(t *testing.T) {
	e := newEnv(t)
	tmpl := e.template(t, appA, "auth.welcome", "email", "en")

	fr, err := versionsCreateHandler(e.deps)(bg, versionsCreateRequest{TemplateID: tmpl.ID.String(), Locale: "fr", Text: "Bonjour"}, as(appA))
	if err != nil || !fr.Version.Active {
		t.Fatalf("versions.create = %+v, %v", fr, err)
	}
	if _, err = versionsCreateHandler(e.deps)(bg, versionsCreateRequest{TemplateID: tmpl.ID.String(), Locale: "fr"}, as(appA)); codeOf(err) != "CONFLICT" {
		t.Errorf("duplicate locale: %v, want CONFLICT", err)
	}

	off, text := false, "Salut"
	upd, err := versionsUpdateHandler(e.deps)(bg, versionsUpdateRequest{TemplateID: tmpl.ID.String(), VersionID: fr.Version.ID, Active: &off, Text: &text}, as(appA))
	if err != nil || upd.Version.Active || upd.Version.Text != "Salut" || upd.Version.Locale != "fr" {
		t.Fatalf("versions.update = %+v, %v", upd, err)
	}

	other := e.template(t, appA, "auth.goodbye", "email", "en")
	if _, err := versionsUpdateHandler(e.deps)(bg, versionsUpdateRequest{TemplateID: other.ID.String(), VersionID: fr.Version.ID, Text: &text}, as(appA)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("a version through the wrong template: %v, want NOT_FOUND", err)
	}
	if _, err := versionsDeleteHandler(e.deps)(bg, versionsDeleteRequest{TemplateID: tmpl.ID.String(), VersionID: fr.Version.ID}, as(appB)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("delete from another app: %v, want NOT_FOUND", err)
	}
	if _, err := versionsDeleteHandler(e.deps)(bg, versionsDeleteRequest{TemplateID: tmpl.ID.String(), VersionID: fr.Version.ID}, as(appA)); err != nil {
		t.Fatalf("versions.delete: %v", err)
	}
}
