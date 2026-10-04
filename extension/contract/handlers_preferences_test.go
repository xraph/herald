package contract

import (
	"reflect"
	"sync"
	"testing"

	"github.com/xraph/herald/preference"
)

func TestPreferencesGetOffersEveryKnownType(t *testing.T) {
	e := newEnv(t)
	e.template(t, appA, "auth.welcome", "email", "en")
	e.template(t, appA, "auth.welcome", "sms", "en")
	e.template(t, appA, "billing.receipt", "email", "en")
	e.template(t, appB, "theirs.only", "email", "en")

	got, err := preferencesGetHandler(e.deps)(bg, preferencesGetRequest{UserID: "user-1"}, as(appA))
	if err != nil {
		t.Fatal(err)
	}
	if got.Preference != nil {
		t.Errorf("a user with no record has preference %+v, want null", got.Preference)
	}
	if want := []string{"auth.welcome", "billing.receipt"}; !reflect.DeepEqual(got.KnownTypes, want) {
		t.Errorf("knownTypes = %v, want %v (sorted, unique, this app only)", got.KnownTypes, want)
	}
}

func TestOptOutOnlyEverTurnsChannelsOff(t *testing.T) {
	e := newEnv(t)
	optOut := func(typ, channel string) {
		t.Helper()
		if _, err := preferencesOptOutHandler(e.deps)(bg, preferencesOptOutRequest{UserID: "user-1", Type: typ, Channel: channel}, as(appA)); err != nil {
			t.Fatalf("optOut %s/%s: %v", typ, channel, err)
		}
	}
	optOut("auth.welcome", "email")
	optOut("auth.welcome", "email") // idempotent
	optOut("auth.welcome", "sms")

	p, err := e.st.GetPreference(bg, appA, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsOptedOut("auth.welcome", "email") || !p.IsOptedOut("auth.welcome", "sms") || p.IsOptedOut("auth.welcome", "push") {
		t.Errorf("overrides = %+v", p.Overrides)
	}

	// A record the user set through the API, with a channel explicitly on,
	// keeps that channel on; opting out of another channel doesn't touch it.
	on := true
	if err := e.st.SetPreference(bg, &preference.Preference{
		ID: p.ID, AppID: appA, UserID: "user-1",
		Overrides: map[string]preference.ChannelPreference{"billing.receipt": {Push: &on}},
	}); err != nil {
		t.Fatal(err)
	}
	optOut("billing.receipt", "email")
	p, _ = e.st.GetPreference(bg, appA, "user-1")
	if got := p.Overrides["billing.receipt"]; got.Push == nil || !*got.Push || !p.IsOptedOut("billing.receipt", "email") {
		t.Errorf("billing.receipt = %+v", got)
	}
}

func TestOptOutRefusals(t *testing.T) {
	e := newEnv(t)
	for name, in := range map[string]preferencesOptOutRequest{
		"no user":      {Type: "auth.welcome", Channel: "email"},
		"no type":      {UserID: "u", Channel: "email"},
		"bad type":     {UserID: "u", Type: "Has Spaces", Channel: "email"},
		"chat channel": {UserID: "u", Type: "auth.welcome", Channel: "chat"},
	} {
		if _, err := preferencesOptOutHandler(e.deps)(bg, in, as(appA)); codeOf(err) != "BAD_REQUEST" {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestConcurrentOptOutsAreAllKept(t *testing.T) {
	e := newEnv(t)
	var wg sync.WaitGroup
	for _, ch := range []string{"email", "sms", "push", "inapp"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = preferencesOptOutHandler(e.deps)(bg, preferencesOptOutRequest{UserID: "user-1", Type: "auth.welcome", Channel: ch}, as(appA))
		}()
	}
	wg.Wait()
	p, err := e.st.GetPreference(bg, appA, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range []string{"email", "sms", "push", "inapp"} {
		if !p.IsOptedOut("auth.welcome", ch) {
			t.Errorf("concurrent opt-out of %s was lost", ch)
		}
	}
}
