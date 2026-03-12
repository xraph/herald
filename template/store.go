package template

import (
	"context"

	"github.com/xraph/herald/id"
)

// Store defines persistence operations for templates and their versions.
type Store interface {
	// Template CRUD
	CreateTemplate(ctx context.Context, t *Template) error
	GetTemplate(ctx context.Context, templateID id.TemplateID) (*Template, error)
	GetTemplateBySlug(ctx context.Context, appID string, slug string, channel string) (*Template, error)
	UpdateTemplate(ctx context.Context, t *Template) error
	DeleteTemplate(ctx context.Context, templateID id.TemplateID) error
	ListTemplates(ctx context.Context, appID string) ([]*Template, error)
	ListTemplatesByChannel(ctx context.Context, appID string, channel string) ([]*Template, error)

	// Version CRUD
	CreateVersion(ctx context.Context, v *Version) error
	GetVersion(ctx context.Context, versionID id.TemplateVersionID) (*Version, error)
	UpdateVersion(ctx context.Context, v *Version) error
	DeleteVersion(ctx context.Context, versionID id.TemplateVersionID) error
	ListVersions(ctx context.Context, templateID id.TemplateID) ([]*Version, error)
}
