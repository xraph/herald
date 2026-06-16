package postgres

import (
	"testing"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/inbox"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/provider"
)

// The herald_* tables declare their jsonb columns as NOT NULL DEFAULT '{}'.
// grove always lists every column in the INSERT, so the DEFAULT never applies —
// a nil Go map is sent as an explicit SQL NULL (pgx encodes a nil map as NULL)
// and trips the not-null constraint. The *ToModel functions must coalesce nil
// collections to empty (but non-nil) values so pgx serializes '{}' instead of
// NULL. These tests exercise the pure toModel functions, no DB required.

func TestProviderToModel_nilCollectionsBecomeEmpty(t *testing.T) {
	m := toProviderModel(&provider.Provider{ID: id.NewProviderID()})
	if m.Credentials == nil {
		t.Error("toProviderModel: Credentials is nil; want non-nil empty map (would insert SQL NULL)")
	}
	if m.Settings == nil {
		t.Error("toProviderModel: Settings is nil; want non-nil empty map (would insert SQL NULL)")
	}
}

func TestMessageToModel_nilMetadataBecomesEmpty(t *testing.T) {
	m := toMessageModel(&message.Message{ID: id.NewMessageID(), Metadata: nil})
	if m.Metadata == nil {
		t.Fatal("toMessageModel: Metadata is nil; want non-nil empty map")
	}
}

func TestNotificationToModel_nilMetadataBecomesEmpty(t *testing.T) {
	m := toNotificationModel(&inbox.Notification{ID: id.NewInboxID(), Metadata: nil})
	if m.Metadata == nil {
		t.Fatal("toNotificationModel: Metadata is nil; want non-nil empty map")
	}
}
