package contract

import (
	"testing"
	"time"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/message"
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
