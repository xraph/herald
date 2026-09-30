package herald

import (
	"errors"
	"testing"

	"github.com/xraph/herald/template"
)

// The root package's renderer errors must be the renderer's own values, or
// errors.Is against the root name never matches what Render returns.
func TestRendererErrorsAreTheSameValues(t *testing.T) {
	_, err := template.NewRenderer().Render(&template.Template{Slug: "x"}, "en", nil)
	if !errors.Is(err, ErrNoVersionForLocale) {
		t.Errorf("errors.Is(%v, herald.ErrNoVersionForLocale) = false", err)
	}
	//nolint:errorlint // testing that errors are the same sentinel value, not wrapped
	if ErrTemplateRenderFailed != template.ErrTemplateRenderFailed ||
		ErrMissingRequiredVariable != template.ErrMissingRequiredVariable {
		t.Error("root renderer errors are separate values from the renderer's")
	}
}
