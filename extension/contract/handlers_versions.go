package contract

import (
	"context"
	"strings"
	"time"

	"github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/template"
)

func registerVersions(d *dispatcher.Dispatcher, deps Deps) error {
	for _, bind := range []func() error{
		func() error { return command(d, "versions.create", versionsCreateHandler(deps)) },
		func() error { return command(d, "versions.update", versionsUpdateHandler(deps)) },
		func() error { return command(d, "versions.delete", versionsDeleteHandler(deps)) },
	} {
		if err := bind(); err != nil {
			return err
		}
	}
	return nil
}

// ownedVersion loads a version through its template: the template must be
// appID's and the version must be that template's. Anything else answers
// NOT_FOUND.
func ownedVersion(ctx context.Context, deps Deps, appID, templateRaw, versionRaw string) (*template.Template, *template.Version, error) {
	t, err := ownedTemplate(ctx, deps, appID, templateRaw)
	if err != nil {
		return nil, nil, err
	}
	vid, err := id.ParseTemplateVersionID(strings.TrimSpace(versionRaw))
	if err != nil {
		return nil, nil, badRequest("versionId is not a version id")
	}
	v, err := deps.Herald.Store().GetVersion(ctx, vid)
	if err != nil {
		return nil, nil, deps.mapError("version lookup", err)
	}
	if v.TemplateID.String() != t.ID.String() {
		return nil, nil, notFound("template version not found")
	}
	return t, v, nil
}

type versionsCreateRequest struct {
	TemplateID string `json:"templateId"`
	Locale     string `json:"locale"`
	Subject    string `json:"subject"`
	HTML       string `json:"html"`
	Text       string `json:"text"`
	Title      string `json:"title"`
	Active     *bool  `json:"active"`
}

type versionResponse struct {
	Version VersionWire `json:"version"`
}

func versionsCreateHandler(deps Deps) func(context.Context, versionsCreateRequest, contract.Principal) (versionResponse, error) {
	return func(ctx context.Context, in versionsCreateRequest, p contract.Principal) (versionResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return versionResponse{}, err
		}
		t, err := ownedTemplate(ctx, deps, appID, in.TemplateID)
		if err != nil {
			return versionResponse{}, err
		}
		locale := strings.TrimSpace(in.Locale)
		if !validLocale(locale) {
			return versionResponse{}, badRequest("locale must be empty (the fallback) or a tag like en or pt-BR")
		}
		active := true
		if in.Active != nil {
			active = *in.Active
		}
		now := time.Now().UTC()
		v := &template.Version{
			ID: id.NewTemplateVersionID(), TemplateID: t.ID, Locale: locale,
			Subject: in.Subject, HTML: in.HTML, Text: in.Text, Title: in.Title,
			Active: active, CreatedAt: now, UpdatedAt: now,
		}
		if err := deps.Herald.Store().CreateVersion(ctx, v); err != nil {
			return versionResponse{}, deps.mapError("versions.create", err)
		}
		audit(ctx, deps, p, appID, "versions.create", "template_version", v.ID.String(), map[string]string{"template": t.Slug, "locale": locale})
		return versionResponse{Version: projectVersion(v)}, nil
	}
}

type versionsUpdateRequest struct {
	TemplateID string  `json:"templateId"`
	VersionID  string  `json:"versionId"`
	Subject    *string `json:"subject"`
	HTML       *string `json:"html"`
	Text       *string `json:"text"`
	Title      *string `json:"title"`
	Active     *bool   `json:"active"`
}

// versionsUpdateHandler changes content and the live switch. A version's
// locale can't change: it's part of the (template, locale) unique key.
func versionsUpdateHandler(deps Deps) func(context.Context, versionsUpdateRequest, contract.Principal) (versionResponse, error) {
	return func(ctx context.Context, in versionsUpdateRequest, p contract.Principal) (versionResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return versionResponse{}, err
		}
		t, v, err := ownedVersion(ctx, deps, appID, in.TemplateID, in.VersionID)
		if err != nil {
			return versionResponse{}, err
		}
		if in.Subject != nil {
			v.Subject = *in.Subject
		}
		if in.HTML != nil {
			v.HTML = *in.HTML
		}
		if in.Text != nil {
			v.Text = *in.Text
		}
		if in.Title != nil {
			v.Title = *in.Title
		}
		if in.Active != nil {
			v.Active = *in.Active
		}
		v.UpdatedAt = time.Now().UTC()
		if err := deps.Herald.Store().UpdateVersion(ctx, v); err != nil {
			return versionResponse{}, deps.mapError("versions.update", err)
		}
		audit(ctx, deps, p, appID, "versions.update", "template_version", v.ID.String(), map[string]string{"template": t.Slug, "locale": v.Locale})
		return versionResponse{Version: projectVersion(v)}, nil
	}
}

type versionsDeleteRequest struct {
	TemplateID string `json:"templateId"`
	VersionID  string `json:"versionId"`
}

func versionsDeleteHandler(deps Deps) func(context.Context, versionsDeleteRequest, contract.Principal) (deleteResponse, error) {
	return func(ctx context.Context, in versionsDeleteRequest, p contract.Principal) (deleteResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return deleteResponse{}, err
		}
		t, v, err := ownedVersion(ctx, deps, appID, in.TemplateID, in.VersionID)
		if err != nil {
			return deleteResponse{}, err
		}
		if err := deps.Herald.Store().DeleteVersion(ctx, v.ID); err != nil {
			return deleteResponse{}, deps.mapError("versions.delete", err)
		}
		audit(ctx, deps, p, appID, "versions.delete", "template_version", v.ID.String(), map[string]string{"template": t.Slug, "locale": v.Locale})
		return deleteResponse{OK: true, ID: v.ID.String()}, nil
	}
}
