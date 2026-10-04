package contract

import (
	"testing"
	"time"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/message"
)

func TestOverviewStats(t *testing.T) {
	e := newEnv(t, withKey())
	e.provider(t, appA, "keyed")
	e.template(t, appA, "auth.welcome", "email", "en")     // no fallback
	e.template(t, appA, "auth.goodbye", "email", "en", "") // has a fallback
	e.template(t, appB, "elsewhere", "sms", "en")          // another app

	now := time.Now().UTC()
	for _, m := range []*message.Message{
		{ID: id.NewMessageID(), AppID: appA, Channel: "email", Status: message.StatusSent, CreatedAt: now.Add(-time.Hour)},
		{ID: id.NewMessageID(), AppID: appA, Channel: "email", Status: message.StatusFailed, CreatedAt: now.Add(-time.Hour)},
		{ID: id.NewMessageID(), AppID: appA, Channel: "email", Status: message.StatusSent, CreatedAt: now.Add(-48 * time.Hour)},
		{ID: id.NewMessageID(), AppID: appB, Channel: "email", Status: message.StatusSent, CreatedAt: now.Add(-time.Hour)},
	} {
		if err := e.st.CreateMessage(bg, m); err != nil {
			t.Fatal(err)
		}
	}

	got, err := overviewStatsHandler(e.deps)(bg, overviewStatsRequest{Window: "24h"}, as(appA))
	if err != nil {
		t.Fatalf("overview.stats: %v", err)
	}
	total := 0
	for _, c := range got.Counts {
		total += c.N
	}
	if total != 2 {
		t.Errorf("24h window counted %d messages, want 2 (not the 48h-old one, not app_b's): %+v", total, got.Counts)
	}
	if got.Providers.Total != 1 || got.Providers.Enabled != 1 {
		t.Errorf("providers = %+v", got.Providers)
	}
	if got.Credentials.Encrypted != 1 || got.Credentials.Plaintext != 0 {
		t.Errorf("credentials = %+v", got.Credentials)
	}
	if len(got.TemplatesWithoutFallback) != 1 || got.TemplatesWithoutFallback[0].Slug != "auth.welcome" {
		t.Errorf("templatesWithoutFallback = %+v", got.TemplatesWithoutFallback)
	}
	if got.Since.After(now.Add(-23*time.Hour)) || got.Since.Before(now.Add(-25*time.Hour)) {
		t.Errorf("since = %v, want about 24h ago", got.Since)
	}
}

func TestOverviewStatsWindow(t *testing.T) {
	e := newEnv(t)
	if _, err := overviewStatsHandler(e.deps)(bg, overviewStatsRequest{}, as(appA)); err != nil {
		t.Errorf("an empty window should default to 7d: %v", err)
	}
	if _, err := overviewStatsHandler(e.deps)(bg, overviewStatsRequest{Window: "1y"}, as(appA)); codeOf(err) != "BAD_REQUEST" {
		t.Errorf("an unknown window: %v, want BAD_REQUEST", err)
	}
}
