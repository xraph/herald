package storetest

import (
	"testing"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/store"
)

func testNotFound(t *testing.T, s store.Store) {
	_, err := s.GetProvider(ctx, id.NewProviderID())
	expectIs(t, "GetProvider", err, store.ErrProviderNotFound)
	expectIs(t, "UpdateProvider", s.UpdateProvider(ctx, newProvider("a", "ghost", "email", 0, Base)), store.ErrProviderNotFound)
	expectIs(t, "DeleteProvider", s.DeleteProvider(ctx, id.NewProviderID()), store.ErrProviderNotFound)

	_, err = s.GetTemplate(ctx, id.NewTemplateID())
	expectIs(t, "GetTemplate", err, store.ErrTemplateNotFound)
	_, err = s.GetTemplateBySlug(ctx, "a", "nope", "email")
	expectIs(t, "GetTemplateBySlug", err, store.ErrTemplateNotFound)
	expectIs(t, "UpdateTemplate", s.UpdateTemplate(ctx, newTemplate("a", "ghost", "email", Base)), store.ErrTemplateNotFound)
	expectIs(t, "DeleteTemplate", s.DeleteTemplate(ctx, id.NewTemplateID()), store.ErrTemplateNotFound)

	_, err = s.GetVersion(ctx, id.NewTemplateVersionID())
	expectIs(t, "GetVersion", err, store.ErrVersionNotFound)
	expectIs(t, "UpdateVersion", s.UpdateVersion(ctx, newVersion(id.NewTemplateID(), "en")), store.ErrVersionNotFound)
	expectIs(t, "DeleteVersion", s.DeleteVersion(ctx, id.NewTemplateVersionID()), store.ErrVersionNotFound)

	_, err = s.GetMessage(ctx, id.NewMessageID())
	expectIs(t, "GetMessage", err, store.ErrMessageNotFound)
	expectIs(t, "RecordDelivery", s.RecordDelivery(ctx, id.NewMessageID(), message.Delivery{Status: message.StatusSent}), store.ErrMessageNotFound)

	_, err = s.GetNotification(ctx, id.NewInboxID())
	expectIs(t, "GetNotification", err, store.ErrNotificationNotFound)
	expectIs(t, "DeleteNotification", s.DeleteNotification(ctx, id.NewInboxID()), store.ErrNotificationNotFound)
	expectIs(t, "MarkRead", s.MarkRead(ctx, id.NewInboxID()), store.ErrNotificationNotFound)

	_, err = s.GetPreference(ctx, "a", "nobody")
	expectIs(t, "GetPreference", err, store.ErrPreferenceNotFound)
	// Deleting a preference that isn't there is not an error on any backend.
	// Recorded as a fact, so a backend that starts refusing it shows up here.
	if err = s.DeletePreference(ctx, "a", "nobody"); err != nil {
		t.Errorf("DeletePreference of a missing row: got %v, want nil", err)
	}

	_, err = s.GetScopedConfig(ctx, "a", scope.ScopeApp, "a")
	expectIs(t, "GetScopedConfig", err, store.ErrScopedConfigNotFound)
	expectIs(t, "DeleteScopedConfig", s.DeleteScopedConfig(ctx, id.NewScopedConfigID()), store.ErrScopedConfigNotFound)
}

func testDuplicates(t *testing.T, s store.Store) {
	welcome := newTemplate("app_a", "welcome", "email", Base)
	must(t, "create welcome", s.CreateTemplate(ctx, welcome))

	expectIs(t, "same app, slug and channel",
		s.CreateTemplate(ctx, newTemplate("app_a", "welcome", "email", Base)), store.ErrDuplicateSlug)
	must(t, "same slug on another channel", s.CreateTemplate(ctx, newTemplate("app_a", "welcome", "sms", Base)))
	must(t, "same slug in another app", s.CreateTemplate(ctx, newTemplate("app_b", "welcome", "email", Base)))

	other := newTemplate("app_a", "other", "email", Base)
	must(t, "create other", s.CreateTemplate(ctx, other))
	other.Slug = "welcome"
	expectIs(t, "renaming onto a taken slug", s.UpdateTemplate(ctx, other), store.ErrDuplicateSlug)

	must(t, "create en", s.CreateVersion(ctx, newVersion(welcome.ID, "en")))
	expectIs(t, "second en version", s.CreateVersion(ctx, newVersion(welcome.ID, "en")), store.ErrDuplicateLocale)

	fr := newVersion(welcome.ID, "fr")
	must(t, "create fr", s.CreateVersion(ctx, fr))
	fr.Locale = "en"
	expectIs(t, "moving fr onto en", s.UpdateVersion(ctx, fr), store.ErrDuplicateLocale)
}
