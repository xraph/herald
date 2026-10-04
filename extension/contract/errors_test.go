package contract

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/xraph/forge"
	dashcontract "github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"

	"github.com/xraph/herald"
	"github.com/xraph/herald/credential"
	"github.com/xraph/herald/store"
)

func TestMapError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code dashcontract.ErrorCode
	}{
		{"provider missing", store.ErrProviderNotFound, dashcontract.CodeNotFound},
		{"template missing", fmt.Errorf("wrapped: %w", store.ErrTemplateNotFound), dashcontract.CodeNotFound},
		{"version missing", store.ErrVersionNotFound, dashcontract.CodeNotFound},
		{"message missing", store.ErrMessageNotFound, dashcontract.CodeNotFound},
		{"notification missing", store.ErrNotificationNotFound, dashcontract.CodeNotFound},
		{"routing rule missing", store.ErrScopedConfigNotFound, dashcontract.CodeNotFound},
		{"duplicate slug", store.ErrDuplicateSlug, dashcontract.CodeConflict},
		{"duplicate locale", store.ErrDuplicateLocale, dashcontract.CodeConflict},
		{"key unavailable", herald.ErrCredentialKeyUnavailable, dashcontract.CodeUnavailable},
		{"invalid provider", fmt.Errorf("%w: name is required", herald.ErrInvalidProvider), dashcontract.CodeBadRequest},
		{"no credential key", herald.ErrNoCredentialKey, dashcontract.CodeBadRequest},
		{"malformed credential", credential.ErrMalformed, dashcontract.CodeBadRequest},
		{"already a contract error", badRequest("nope"), dashcontract.CodeBadRequest},
		{"unknown", errors.New("dial tcp 10.0.0.1:5432: password=hunter2"), dashcontract.CodeInternal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mapError(c.err)
			if codeOf(got) != c.code {
				t.Fatalf("mapError(%v) code = %q, want %q", c.err, codeOf(got), c.code)
			}
			if c.code == dashcontract.CodeInternal && strings.Contains(got.Error(), "hunter2") {
				t.Errorf("an INTERNAL error leaked its cause: %v", got)
			}
		})
	}
	if mapError(nil) != nil {
		t.Error("mapError(nil) must be nil")
	}
}

// recordingLogger counts Error calls so the test can see which errors get
// logged. Only Error is exercised by Deps.mapError.
type recordingLogger struct {
	forge.Logger
	errors int
}

func (l *recordingLogger) Error(string, ...forge.Field) { l.errors++ }

func TestDepsMapErrorLogsOnlyInternal(t *testing.T) {
	log := &recordingLogger{}
	d := Deps{Logger: log}
	_ = d.mapError("x.y", store.ErrProviderNotFound)
	if log.errors != 0 {
		t.Fatalf("a NOT_FOUND was logged as an error")
	}
	_ = d.mapError("x.y", errors.New("boom"))
	if log.errors != 1 {
		t.Fatalf("an INTERNAL error was logged %d times, want 1", log.errors)
	}
	if err := (Deps{}).mapError("x.y", errors.New("boom")); codeOf(err) != dashcontract.CodeInternal {
		t.Fatalf("no logger: got %v", err)
	}
}

func TestNotFoundAndConflictCodes(t *testing.T) {
	if codeOf(notFound("x")) != dashcontract.CodeNotFound || codeOf(conflict("x")) != dashcontract.CodeConflict {
		t.Fatal("notFound and conflict must carry their codes")
	}
}

func TestCommandBindsAnIntent(t *testing.T) {
	d := dispatcher.New(nil)
	err := command(d, "thing.do", func(_ context.Context, _ struct{}, _ dashcontract.Principal) (struct{}, error) {
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatalf("command: %v", err)
	}
}

func TestFixtureHelpers(t *testing.T) {
	e := newEnv(t, withKey())
	p := e.provider(t, appA, "primary")
	if p.AppID != appA || p.Name != "primary" {
		t.Errorf("provider = %+v", p)
	}
	tm := e.template(t, appB, "welcome", "email", "en", "fr")
	if tm.AppID != appB || tm.Slug != "welcome" {
		t.Errorf("template = %+v", tm)
	}
	vs, err := e.st.ListVersions(bg, tm.ID)
	if err != nil || len(vs) != 2 {
		t.Errorf("versions = %d, %v; want 2", len(vs), err)
	}
	// The canary is the stand-in for a secret the wire must never carry.
	if strings.Contains(fmt.Sprint(mapError(errors.New(canary))), canary) {
		t.Error("an unknown error leaked its text")
	}
}
