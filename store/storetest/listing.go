package storetest

import (
	"testing"
	"time"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/store"
)

func sameOrder(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: got %d rows %v, want %d rows %v", what, len(got), got, len(want), want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: position %d is %s, want %s (got %v, want %v)", what, i, got[i], want[i], got, want)
			return
		}
	}
}

// testEmptyAppIsExact records what an empty app ID means on this backend.
// Every backend treats it as an exact match on an empty app_id, never as "every
// app". If a backend ever starts returning other apps' rows for "", this is
// where it shows up, before a dashboard renders one tenant's data to another.
func testEmptyAppIsExact(t *testing.T, s store.Store) {
	blank := newProvider("", "blank", "inapp", 0, Base)
	other := newProvider("app_b", "other", "inapp", 0, Base)
	must(t, "create blank", s.CreateProvider(ctx, blank))
	must(t, "create other", s.CreateProvider(ctx, other))

	got, err := s.ListAllProviders(ctx, "")
	must(t, "list providers for empty app", err)
	gotIDs := make([]string, 0, len(got))
	for _, p := range got {
		gotIDs = append(gotIDs, p.ID.String())
	}
	sameOrder(t, `ListAllProviders("") is an exact match`, gotIDs, []string{blank.ID.String()})

	msgBlank := newMessage("", "email", message.StatusSent, Base)
	msgOther := newMessage("app_b", "sms", message.StatusFailed, Base)
	must(t, "create blank message", s.CreateMessage(ctx, msgBlank))
	must(t, "create other message", s.CreateMessage(ctx, msgOther))
	msgs, err := s.ListMessages(ctx, "", message.ListOptions{})
	must(t, "list messages for empty app", err)
	msgIDs := make([]string, 0, len(msgs))
	for _, m := range msgs {
		msgIDs = append(msgIDs, m.ID.String())
	}
	sameOrder(t, `ListMessages("") is an exact match`, msgIDs, []string{msgBlank.ID.String()})
}

func testOrdering(t *testing.T, s store.Store) {
	// Providers: priority ascending, then oldest first.
	late := newProvider("app_a", "late", "email", 1, Base.Add(2*time.Minute))
	early := newProvider("app_a", "early", "email", 1, Base.Add(1*time.Minute))
	first := newProvider("app_a", "first", "email", 0, Base.Add(3*time.Minute))
	must(t, "create late", s.CreateProvider(ctx, late))
	must(t, "create early", s.CreateProvider(ctx, early))
	must(t, "create first", s.CreateProvider(ctx, first))
	ps, err := s.ListAllProviders(ctx, "app_a")
	must(t, "list providers", err)
	pIDs := make([]string, 0, len(ps))
	for _, p := range ps {
		pIDs = append(pIDs, p.ID.String())
	}
	sameOrder(t, "providers by priority then created_at", pIDs,
		[]string{first.ID.String(), early.ID.String(), late.ID.String()})

	// Templates: oldest first.
	t2 := newTemplate("app_a", "second", "email", Base.Add(2*time.Minute))
	t1 := newTemplate("app_a", "first", "email", Base.Add(1*time.Minute))
	must(t, "create t2", s.CreateTemplate(ctx, t2))
	must(t, "create t1", s.CreateTemplate(ctx, t1))
	ts, err := s.ListTemplates(ctx, "app_a")
	must(t, "list templates", err)
	tIDs := make([]string, 0, len(ts))
	for _, x := range ts {
		tIDs = append(tIDs, x.ID.String())
	}
	sameOrder(t, "templates by created_at", tIDs, []string{t1.ID.String(), t2.ID.String()})

	// Versions: locale ascending.
	fr := newVersion(t1.ID, "fr")
	en := newVersion(t1.ID, "en")
	must(t, "create fr", s.CreateVersion(ctx, fr))
	must(t, "create en", s.CreateVersion(ctx, en))
	vs, err := s.ListVersions(ctx, t1.ID)
	must(t, "list versions", err)
	vIDs := make([]string, 0, len(vs))
	for _, v := range vs {
		vIDs = append(vIDs, v.ID.String())
	}
	sameOrder(t, "versions by locale", vIDs, []string{en.ID.String(), fr.ID.String()})

	// Notifications: newest first.
	n1 := newNotification("app_a", "u1", Base.Add(1*time.Minute))
	n2 := newNotification("app_a", "u1", Base.Add(2*time.Minute))
	must(t, "create n1", s.CreateNotification(ctx, n1))
	must(t, "create n2", s.CreateNotification(ctx, n2))
	ns, err := s.ListNotifications(ctx, "app_a", "u1", 0, 0)
	must(t, "list notifications", err)
	nIDs := make([]string, 0, len(ns))
	for _, n := range ns {
		nIDs = append(nIDs, n.ID.String())
	}
	sameOrder(t, "notifications newest first", nIDs, []string{n2.ID.String(), n1.ID.String()})

	// Scoped configs: scope ascending ("app" < "org" < "user"), then oldest first.
	user := &scope.Config{ID: id.NewScopedConfigID(), AppID: "app_a", Scope: scope.ScopeUser, ScopeID: "u1", CreatedAt: Base, UpdatedAt: Base}
	app := &scope.Config{ID: id.NewScopedConfigID(), AppID: "app_a", Scope: scope.ScopeApp, ScopeID: "app_a", CreatedAt: Base, UpdatedAt: Base}
	must(t, "set user scope", s.SetScopedConfig(ctx, user))
	must(t, "set app scope", s.SetScopedConfig(ctx, app))
	cs, err := s.ListScopedConfigs(ctx, "app_a")
	must(t, "list scoped configs", err)
	scopes := make([]string, 0, len(cs))
	for _, c := range cs {
		scopes = append(scopes, string(c.Scope))
	}
	sameOrder(t, "scoped configs by scope", scopes, []string{"app", "user"})
}

func testMessagePaging(t *testing.T, s store.Store) {
	created := make([]*message.Message, 0, 5)
	for i := range 5 {
		m := newMessage("app_a", "email", message.StatusSent, Base.Add(time.Duration(i)*time.Minute))
		must(t, "create message", s.CreateMessage(ctx, m))
		created = append(created, m)
	}
	// Newest first, skip one, take two: created[3], created[2].
	page, err := s.ListMessages(ctx, "app_a", message.ListOptions{Offset: 1, Limit: 2})
	must(t, "list page", err)
	got := make([]string, 0, len(page))
	for _, m := range page {
		got = append(got, m.ID.String())
	}
	sameOrder(t, "messages offset 1 limit 2", got, []string{created[3].ID.String(), created[2].ID.String()})

	past, err := s.ListMessages(ctx, "app_a", message.ListOptions{Offset: 10, Limit: 2})
	must(t, "list past the end", err)
	if len(past) != 0 {
		t.Errorf("offset past the end: got %d rows, want 0", len(past))
	}
}
