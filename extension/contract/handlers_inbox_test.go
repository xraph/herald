package contract

import (
	"context"
	"testing"
	"time"

	"github.com/xraph/herald"
	"github.com/xraph/herald/bridge"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/inbox"
)

func seedInbox(t *testing.T, e *env, appID, userID string, n int) []*inbox.Notification {
	t.Helper()
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	out := make([]*inbox.Notification, 0, n)
	for i := range n {
		note := &inbox.Notification{
			ID: id.NewInboxID(), AppID: appID, UserID: userID, Type: "auth.welcome",
			Title: "Welcome", Body: "Hi", CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		if err := e.st.CreateNotification(bg, note); err != nil {
			t.Fatal(err)
		}
		out = append(out, note)
	}
	return out
}

func TestInboxListAndMarkRead(t *testing.T) {
	e := newEnv(t)
	notes := seedInbox(t, e, appA, "user-1", 3)
	seedInbox(t, e, appB, "user-1", 2)

	got, err := inboxListHandler(e.deps)(bg, inboxListRequest{UserID: "user-1"}, as(appA))
	if err != nil || len(got.Notifications) != 3 || got.Unread != 3 || got.NextCursor != "" {
		t.Fatalf("inbox.list = %+v, %v", got, err)
	}
	if _, err := inboxListHandler(e.deps)(bg, inboxListRequest{}, as(appA)); codeOf(err) != "BAD_REQUEST" {
		t.Errorf("no userId: %v", err)
	}

	if _, err := inboxMarkReadHandler(e.deps)(bg, inboxIDRequest{ID: notes[0].ID.String()}, as(appA)); err != nil {
		t.Fatal(err)
	}
	got, _ = inboxListHandler(e.deps)(bg, inboxListRequest{UserID: "user-1"}, as(appA))
	if got.Unread != 2 {
		t.Errorf("unread after markRead = %d, want 2", got.Unread)
	}

	if _, err := inboxMarkAllReadHandler(e.deps)(bg, inboxUserRequest{UserID: "user-1"}, as(appA)); err != nil {
		t.Fatal(err)
	}
	got, _ = inboxListHandler(e.deps)(bg, inboxListRequest{UserID: "user-1"}, as(appA))
	if got.Unread != 0 {
		t.Errorf("unread after markAllRead = %d, want 0", got.Unread)
	}
	theirs, _ := inboxListHandler(e.deps)(bg, inboxListRequest{UserID: "user-1"}, as(appB))
	if theirs.Unread != 2 {
		t.Errorf("markAllRead in app_a touched app_b's inbox: unread = %d", theirs.Unread)
	}
}

func TestInboxWritesStayInTheirApp(t *testing.T) {
	e := newEnv(t)
	theirs := seedInbox(t, e, appB, "user-1", 1)
	ref := inboxIDRequest{ID: theirs[0].ID.String()}
	if _, err := inboxMarkReadHandler(e.deps)(bg, ref, as(appA)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("markRead on another app's notification: %v", err)
	}
	if _, err := inboxDeleteHandler(e.deps)(bg, ref, as(appA)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("delete on another app's notification: %v", err)
	}
	if n, err := e.st.GetNotification(bg, theirs[0].ID); err != nil || n.Read {
		t.Errorf("app_b's notification after app_a's attempts: %+v, %v", n, err)
	}
	if _, err := inboxDeleteHandler(e.deps)(bg, inboxIDRequest{ID: "not-an-id"}, as(appA)); codeOf(err) != "BAD_REQUEST" {
		t.Errorf("malformed id: %v", err)
	}
}

func TestInboxListPagesByCursor(t *testing.T) {
	e := newEnv(t)
	notes := seedInbox(t, e, appA, "user-1", 5)
	seedInbox(t, e, appA, "user-2", 1)

	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		got, err := inboxListHandler(e.deps)(bg, inboxListRequest{UserID: "user-1", Cursor: cursor, Limit: 2}, as(appA))
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if got.Unread != 5 {
			t.Errorf("page %d unread = %d, want 5 across all pages", pages, got.Unread)
		}
		for _, n := range got.Notifications {
			if seen[n.ID] {
				t.Errorf("notification %s came back twice", n.ID)
			}
			seen[n.ID] = true
		}
		if got.NextCursor == "" {
			break
		}
		cursor = got.NextCursor
		if pages > 5 {
			t.Fatal("paging did not terminate")
		}
	}
	if pages != 3 || len(seen) != len(notes) {
		t.Errorf("pages = %d, distinct = %d, want 3 pages and %d", pages, len(seen), len(notes))
	}
	if _, err := inboxListHandler(e.deps)(bg, inboxListRequest{UserID: "user-1", Cursor: "!!"}, as(appA)); codeOf(err) != "BAD_REQUEST" {
		t.Errorf("bad cursor: %v", err)
	}
}

func TestInboxDeleteAndAudit(t *testing.T) {
	var events []*bridge.AuditEvent
	rec := bridge.ChronicleFunc(func(_ context.Context, ev *bridge.AuditEvent) error {
		events = append(events, ev)
		return nil
	})
	e := newEnv(t, herald.WithChronicle(rec))
	notes := seedInbox(t, e, appA, "user-1", 2)

	if _, err := inboxListHandler(e.deps)(bg, inboxListRequest{UserID: "user-1"}, as(appA)); err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("inbox.list wrote %d audit events, want none", len(events))
	}

	ref := inboxIDRequest{ID: notes[0].ID.String()}
	got, err := inboxDeleteHandler(e.deps)(bg, ref, as(appA))
	if err != nil || !got.OK || got.ID != ref.ID {
		t.Fatalf("inbox.delete = %+v, %v", got, err)
	}
	if _, err := inboxDeleteHandler(e.deps)(bg, ref, as(appA)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("delete twice: %v", err)
	}
	if _, err := inboxMarkAllReadHandler(e.deps)(bg, inboxUserRequest{UserID: "user-1"}, as(appA)); err != nil {
		t.Fatal(err)
	}
	list, _ := inboxListHandler(e.deps)(bg, inboxListRequest{UserID: "user-1"}, as(appA))
	if len(list.Notifications) != 1 {
		t.Errorf("notifications after delete = %d, want 1", len(list.Notifications))
	}

	want := []string{"dashboard.inbox.delete", "dashboard.inbox.markAllRead"}
	if len(events) != len(want) {
		t.Fatalf("got %d audit events, want %d: %+v", len(events), len(want), events)
	}
	for i, ev := range events {
		if ev.Action != want[i] || ev.ActorID != "operator-1" || ev.Tenant != appA {
			t.Errorf("event %d = %+v", i, ev)
		}
	}
}
