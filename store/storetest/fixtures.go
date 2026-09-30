package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/inbox"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/template"
)

// Base is the reference instant for fixtures. Whole seconds in UTC, because
// Mongo keeps milliseconds and Postgres microseconds.
var Base = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

var ctx = context.Background()

func newProvider(appID, name, channel string, priority int, created time.Time) *provider.Provider {
	return &provider.Provider{
		ID:          id.NewProviderID(),
		AppID:       appID,
		Name:        name,
		Channel:     channel,
		Driver:      "smtp",
		Credentials: map[string]string{"password": "pw-" + name, "username": "user-" + name},
		Settings:    map[string]string{"host": "smtp.example.com", "port": "587"},
		Priority:    priority,
		Enabled:     true,
		CreatedAt:   created,
		UpdatedAt:   created,
	}
}

func newTemplate(appID, slug, channel string, created time.Time) *template.Template {
	return &template.Template{
		ID:       id.NewTemplateID(),
		AppID:    appID,
		Slug:     slug,
		Name:     "Template " + slug,
		Channel:  channel,
		Category: template.CategoryTransactional,
		Variables: []template.Variable{
			{Name: "user_name", Type: "string", Required: true, Description: "who it is for"},
			{Name: "expires_in", Type: "string", Default: "1 hour"},
		},
		Enabled:   true,
		CreatedAt: created,
		UpdatedAt: created,
	}
}

func newVersion(templateID id.TemplateID, locale string) *template.Version {
	return &template.Version{
		ID:         id.NewTemplateVersionID(),
		TemplateID: templateID,
		Locale:     locale,
		Subject:    "Hello {{.user_name}}",
		HTML:       "<p>Hello {{.user_name}}</p>",
		Text:       "Hello {{.user_name}}",
		Title:      "Hi",
		Active:     true,
		CreatedAt:  Base,
		UpdatedAt:  Base,
	}
}

func newMessage(appID, channel string, status message.Status, created time.Time) *message.Message {
	return &message.Message{
		ID:         id.NewMessageID(),
		AppID:      appID,
		EnvID:      "env_1",
		TemplateID: "auth.welcome",
		ProviderID: "hpvd_fixture",
		Channel:    channel,
		Recipient:  "ada@example.com",
		Subject:    "Welcome",
		Body:       "Hello Ada",
		Status:     status,
		Metadata:   map[string]string{"trace": "t-1", "source": "suite"},
		Attempts:   1,
		CreatedAt:  created,
	}
}

func newNotification(appID, userID string, created time.Time) *inbox.Notification {
	return &inbox.Notification{
		ID:        id.NewInboxID(),
		AppID:     appID,
		UserID:    userID,
		Type:      "auth.welcome",
		Title:     "Welcome",
		Body:      "Hello",
		ActionURL: "https://example.com/a",
		Metadata:  map[string]string{"k": "v"},
		CreatedAt: created,
	}
}

func must(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

func expectIs(t *testing.T, what string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("%s: got error %v, want errors.Is(%v)", what, err, want)
	}
}
