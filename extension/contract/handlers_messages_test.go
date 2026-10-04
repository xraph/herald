package contract

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/xraph/herald"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/store/memory"
	"github.com/xraph/herald/template"
)

func seedMessages(t *testing.T, e *env, appID string, n int, providerID string) []*message.Message {
	t.Helper()
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	out := make([]*message.Message, 0, n)
	for i := range n {
		m := &message.Message{
			ID: id.NewMessageID(), AppID: appID, Channel: "email", Recipient: "ada@example.com",
			TemplateID: "auth.welcome", ProviderID: providerID, Status: message.StatusSent,
			Body: "Hello", Metadata: map[string]string{"k": "v"}, Attempts: 1,
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		if err := e.st.CreateMessage(bg, m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

func TestMessagesListPagesWithACursor(t *testing.T) {
	e := newEnv(t)
	prov := e.provider(t, appA, "primary")
	seedMessages(t, e, appA, 5, prov.ID.String())
	seedMessages(t, e, appB, 3, "")

	first, err := messagesListHandler(e.deps)(bg, messagesListRequest{Limit: 2}, as(appA))
	if err != nil || len(first.Messages) != 2 || first.NextCursor == "" {
		t.Fatalf("page 1 = %+v, %v", first, err)
	}
	if first.Messages[0].Provider == nil || first.Messages[0].Provider.Name != "primary" {
		t.Errorf("provider ref = %+v", first.Messages[0].Provider)
	}
	seen := map[string]bool{}
	cursor := first.NextCursor
	for _, m := range first.Messages {
		seen[m.ID] = true
	}
	for cursor != "" {
		page, err := messagesListHandler(e.deps)(bg, messagesListRequest{Limit: 2, Cursor: cursor}, as(appA))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range page.Messages {
			seen[m.ID] = true
		}
		cursor = page.NextCursor
	}
	if len(seen) != 5 {
		t.Errorf("paged through %d messages, want app_a's 5", len(seen))
	}
	if _, err := messagesListHandler(e.deps)(bg, messagesListRequest{Cursor: "garbage!"}, as(appA)); codeOf(err) != "BAD_REQUEST" {
		t.Errorf("bad cursor: %v", err)
	}
}

func TestMessagesDetail(t *testing.T) {
	e := newEnv(t)
	prov := e.provider(t, appA, "primary")
	tmpl := e.template(t, appA, "auth.welcome", "email", "en")
	msgs := seedMessages(t, e, appA, 1, prov.ID.String())

	got, err := messagesDetailHandler(e.deps)(bg, messagesDetailRequest{ID: msgs[0].ID.String()}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	if got.Message.Body != "Hello" || got.Message.Metadata["k"] != "v" {
		t.Errorf("message = %+v", got.Message)
	}
	if got.Message.Template == nil || got.Message.Template.ID != tmpl.ID.String() {
		t.Errorf("template = %+v, want it resolved from the stored slug", got.Message.Template)
	}
	if got.Message.Provider == nil || got.Message.Provider.Driver != "fake" {
		t.Errorf("provider = %+v", got.Message.Provider)
	}

	theirs := seedMessages(t, e, appB, 1, "")
	if _, err := messagesDetailHandler(e.deps)(bg, messagesDetailRequest{ID: theirs[0].ID.String()}, as(appA)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("another app's message: %v", err)
	}
}

// slugFailStore is a store whose template-by-slug lookup fails with a
// non-domain error.
type slugFailStore struct{ store.Store }

func (slugFailStore) GetTemplateBySlug(context.Context, string, string, string) (*template.Template, error) {
	return nil, errors.New("connection reset")
}

// seedMixed logs one message per channel and status pair, oldest first.
func seedMixed(t *testing.T, e *env, appID string, rows []message.Message) {
	t.Helper()
	base := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	for i, r := range rows {
		m := r
		m.ID, m.AppID, m.Recipient = id.NewMessageID(), appID, "ada@example.com"
		m.CreatedAt = base.Add(time.Duration(i) * time.Minute)
		if err := e.st.CreateMessage(bg, &m); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMessagesListFiltersByChannelAndStatus(t *testing.T) {
	e := newEnv(t)
	seedMixed(t, e, appA, []message.Message{
		{Channel: "email", Status: message.StatusSent},
		{Channel: "email", Status: message.StatusFailed},
		{Channel: "sms", Status: message.StatusSent},
		{Channel: "sms", Status: message.StatusFailed},
		{Channel: "email", Status: message.StatusFailed},
	})
	list := func(channel, status string) []MessageSummary {
		t.Helper()
		got, err := messagesListHandler(e.deps)(bg, messagesListRequest{Channel: channel, Status: status}, as(appA))
		if err != nil {
			t.Fatal(err)
		}
		return got.Messages
	}
	if got := list("", ""); len(got) != 5 {
		t.Errorf("no filter = %d rows, want 5", len(got))
	}
	if got := list("email", ""); len(got) != 3 {
		t.Errorf("channel=email = %d rows, want 3", len(got))
	}
	if got := list("", "failed"); len(got) != 3 {
		t.Errorf("status=failed = %d rows, want 3", len(got))
	}
	got := list(" sms ", " failed ")
	if len(got) != 1 || got[0].Channel != "sms" || got[0].Status != "failed" {
		t.Errorf("sms+failed = %+v, want the one matching row (filters are trimmed)", got)
	}
	if got := list("push", ""); len(got) != 0 {
		t.Errorf("channel=push = %d rows, want 0", len(got))
	}
}

func TestMessagesListIsNewestFirstAndEndsWithoutACursor(t *testing.T) {
	e := newEnv(t)
	seeded := seedMessages(t, e, appA, 5, "")

	var got []string
	cursor := ""
	for {
		page, err := messagesListHandler(e.deps)(bg, messagesListRequest{Limit: 2, Cursor: cursor}, as(appA))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range page.Messages {
			got = append(got, m.ID)
		}
		if page.NextCursor == "" {
			if len(page.Messages) != 1 {
				t.Errorf("last page has %d rows, want the 1 left over", len(page.Messages))
			}
			break
		}
		cursor = page.NextCursor
	}
	for i, m := range seeded {
		if want := got[len(seeded)-1-i]; m.ID.String() != want {
			t.Fatalf("position %d is %s, want %s (newest first): %v", len(seeded)-1-i, want, m.ID, got)
		}
	}

	exact, err := messagesListHandler(e.deps)(bg, messagesListRequest{Limit: 5}, as(appA))
	if err != nil || len(exact.Messages) != 5 || exact.NextCursor != "" {
		t.Errorf("a page that holds every row = %d rows, cursor %q, %v; want no next cursor", len(exact.Messages), exact.NextCursor, err)
	}
}

func TestMessagesListCapsTheLimit(t *testing.T) {
	e := newEnv(t)
	seedMessages(t, e, appA, maxPageLimit+5, "")

	got, err := messagesListHandler(e.deps)(bg, messagesListRequest{Limit: 500}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != maxPageLimit || got.NextCursor == "" {
		t.Errorf("limit 500 = %d rows, cursor %q; want %d rows and a next cursor", len(got.Messages), got.NextCursor, maxPageLimit)
	}
	def, err := messagesListHandler(e.deps)(bg, messagesListRequest{}, as(appA))
	if err != nil || len(def.Messages) != defaultPageLimit {
		t.Errorf("no limit = %d rows, %v; want the default %d", len(def.Messages), err, defaultPageLimit)
	}
}

func TestMessagesDetailWithAProviderFromAnotherAppHasNoProvider(t *testing.T) {
	e := newEnv(t)
	theirs := e.provider(t, appB, "theirs")
	msgs := seedMessages(t, e, appA, 1, theirs.ID.String())

	got, err := messagesDetailHandler(e.deps)(bg, messagesDetailRequest{ID: msgs[0].ID.String()}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	if got.Message.Provider != nil {
		t.Errorf("provider = %+v, want null for another app's provider", got.Message.Provider)
	}
}

func TestMessagesDetailRejectsAMalformedID(t *testing.T) {
	e := newEnv(t)
	for _, raw := range []string{"nope", "", id.NewTemplateID().String()} {
		if _, err := messagesDetailHandler(e.deps)(bg, messagesDetailRequest{ID: raw}, as(appA)); codeOf(err) != "BAD_REQUEST" {
			t.Errorf("id %q: %v, want BAD_REQUEST", raw, err)
		}
	}
	if _, err := messagesDetailHandler(e.deps)(bg, messagesDetailRequest{ID: id.NewMessageID().String()}, as(appA)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("unknown message: %v, want NOT_FOUND", err)
	}
}

func TestMessagesDetailTemplateIsNullOnlyWhenItIsGone(t *testing.T) {
	t.Run("a deleted template gives null", func(t *testing.T) {
		e := newEnv(t)
		msgs := seedMessages(t, e, appA, 1, "") // its slug, auth.welcome, has no template
		got, err := messagesDetailHandler(e.deps)(bg, messagesDetailRequest{ID: msgs[0].ID.String()}, as(appA))
		if err != nil {
			t.Fatal(err)
		}
		if got.Message.Template != nil || got.Message.TemplateSlug != "auth.welcome" {
			t.Errorf("template = %+v, slug = %q; want null and the stored slug kept", got.Message.Template, got.Message.TemplateSlug)
		}
	})

	t.Run("any other store error is returned, not hidden", func(t *testing.T) {
		st := memory.New()
		h, err := herald.New(herald.WithStore(slugFailStore{Store: st}))
		if err != nil {
			t.Fatal(err)
		}
		e := &env{h: h, st: st, deps: Deps{Herald: h}}
		msgs := seedMessages(t, e, appA, 1, "")
		_, err = messagesDetailHandler(e.deps)(bg, messagesDetailRequest{ID: msgs[0].ID.String()}, as(appA))
		if codeOf(err) != "INTERNAL" {
			t.Errorf("template lookup failure: %v, want INTERNAL", err)
		}
		if err != nil && strings.Contains(err.Error(), "connection reset") {
			t.Errorf("the store's error text reached the client: %v", err)
		}
	})
}
