package storetest

import (
	"reflect"
	"testing"
	"time"

	"github.com/xraph/herald/message"
	"github.com/xraph/herald/store"
)

func testRecordDelivery(t *testing.T, s store.Store) {
	m := newMessage("app_a", "sms", message.StatusSending, Base)
	must(t, "create message", s.CreateMessage(ctx, m))

	sentAt := Base.Add(3 * time.Second)
	must(t, "record delivery", s.RecordDelivery(ctx, m.ID, message.Delivery{
		Status: message.StatusSent, ProviderMessageID: "SM123", SentAt: &sentAt,
	}))
	got, err := s.GetMessage(ctx, m.ID)
	must(t, "get message", err)
	if got.Status != message.StatusSent || got.ProviderMessageID != "SM123" || got.Error != "" {
		t.Errorf("after sent: status=%q vendor=%q error=%q", got.Status, got.ProviderMessageID, got.Error)
	}
	if got.SentAt == nil || !got.SentAt.Equal(sentAt) {
		t.Errorf("sent_at: got %v, want %v", got.SentAt, sentAt)
	}

	// Every field is written, so an empty value clears what was there.
	must(t, "record retry failure", s.RecordDelivery(ctx, m.ID, message.Delivery{
		Status: message.StatusFailed, Error: "retry failed",
	}))
	cleared, err := s.GetMessage(ctx, m.ID)
	must(t, "get cleared message", err)
	if cleared.Status != message.StatusFailed || cleared.SentAt != nil || cleared.ProviderMessageID != "" {
		t.Errorf("after retry failure: status=%q sent_at=%v vendor=%q, want failed, nil, empty",
			cleared.Status, cleared.SentAt, cleared.ProviderMessageID)
	}

	failed := newMessage("app_a", "sms", message.StatusSending, Base)
	must(t, "create failing message", s.CreateMessage(ctx, failed))
	must(t, "record failure", s.RecordDelivery(ctx, failed.ID, message.Delivery{
		Status: message.StatusFailed, Error: "twilio: API error 400: bad number",
	}))
	gotF, err := s.GetMessage(ctx, failed.ID)
	must(t, "get failed message", err)
	if gotF.Status != message.StatusFailed || gotF.Error != "twilio: API error 400: bad number" || gotF.SentAt != nil {
		t.Errorf("after failed: status=%q error=%q sent_at=%v", gotF.Status, gotF.Error, gotF.SentAt)
	}
}

func testCountMessages(t *testing.T, s store.Store) {
	since := Base
	rows := []struct {
		app     string
		channel string
		status  message.Status
		at      time.Time
	}{
		{"app_a", "email", message.StatusSent, Base.Add(-time.Hour)}, // before the window
		{"app_a", "email", message.StatusSent, Base},                 // on the boundary: counted
		{"app_a", "email", message.StatusSent, Base.Add(time.Minute)},
		{"app_a", "email", message.StatusFailed, Base.Add(time.Minute)},
		{"app_a", "sms", message.StatusSent, Base.Add(2 * time.Minute)},
		{"app_b", "email", message.StatusSent, Base.Add(time.Minute)}, // another app
	}
	for _, r := range rows {
		must(t, "create message", s.CreateMessage(ctx, newMessage(r.app, r.channel, r.status, r.at)))
	}

	got, err := s.CountMessages(ctx, "app_a", since)
	must(t, "count messages", err)
	want := []message.Count{
		{Status: message.StatusFailed, Channel: "email", N: 1},
		{Status: message.StatusSent, Channel: "email", N: 2},
		{Status: message.StatusSent, Channel: "sms", N: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CountMessages: got %+v, want %+v", got, want)
	}

	empty, err := s.CountMessages(ctx, "app_a", Base.Add(time.Hour))
	must(t, "count an empty window", err)
	if len(empty) != 0 {
		t.Errorf("empty window: got %+v, want no rows", empty)
	}
}
