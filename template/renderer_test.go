package template

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultsFillMissingVariables(t *testing.T) {
	tmpl := &Template{
		Slug:      "otp",
		Variables: []Variable{{Name: "expires_in", Required: true, Default: "5 minutes"}},
		Versions:  []Version{{Locale: "", Active: true, Text: "Expires in {{.expires_in}}."}},
	}
	out, err := NewRenderer().Render(tmpl, "en", map[string]any{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out.Text != "Expires in 5 minutes." {
		t.Errorf("Text = %q", out.Text)
	}
}

func TestShippedMFATemplateNoLongerSaysNoValue(t *testing.T) {
	var mfa *Template
	for _, tmpl := range DefaultTemplates("app") {
		if tmpl.Slug == "auth.mfa-code" && tmpl.Channel == "sms" {
			mfa = tmpl
		}
	}
	if mfa == nil {
		t.Fatal("auth.mfa-code sms is not among the default templates")
	}
	out, err := NewRenderer().Render(mfa, "en", map[string]any{"code": "123456", "app_name": "Acme"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(out.Text, "<no value>") || !strings.Contains(out.Text, "5 minutes") {
		t.Errorf("Text = %q, want the declared default and no <no value>", out.Text)
	}
}

func TestDefaultsNeverOverrideOrMutateCallerData(t *testing.T) {
	vars := []Variable{{Name: "role", Default: "member"}}
	data := map[string]any{"role": "admin"}
	got := withDefaults(vars, data)
	if got["role"] != "admin" {
		t.Errorf("default overrode a caller value: %v", got["role"])
	}
	empty := map[string]any{}
	_ = withDefaults(vars, empty)
	if len(empty) != 0 {
		t.Errorf("withDefaults mutated the caller's map: %v", empty)
	}
}

func TestResolve(t *testing.T) {
	tmpl := &Template{Versions: []Version{
		{Locale: "fr", Active: true, Text: "fr"},
		{Locale: "", Active: true, Text: "default"},
		{Locale: "en", Active: true, Text: "en"},
		{Locale: "de", Active: false, Text: "de"},
	}}
	cases := []struct {
		locale string
		text   string
		match  Match
	}{
		{"fr", "fr", MatchExact},
		{"fr-CA", "fr", MatchLanguage},
		{"en-US", "en", MatchLanguage},
		{"de", "default", MatchDefault}, // inactive versions never answer
		{"ja", "default", MatchDefault},
		{"", "default", MatchExact},
	}
	for _, c := range cases {
		v, m := Resolve(tmpl, c.locale)
		if v == nil || v.Text != c.text || m != c.match {
			t.Errorf("Resolve(%q) = %+v, %s; want %s by %s", c.locale, v, m, c.text, c.match)
		}
	}

	enOnly := &Template{Versions: []Version{{Locale: "en", Active: true}}}
	if v, m := Resolve(enOnly, "fr"); v != nil || m != MatchNone {
		t.Errorf("fr on an en-only template = %+v, %s; want none", v, m)
	}
}

func TestExplainListsEveryStep(t *testing.T) {
	enOnly := &Template{Versions: []Version{{Locale: "en", Active: true}}}
	steps, v := Explain(enOnly, "fr-CA")
	if v != nil {
		t.Fatalf("fr-CA answered by %+v", v)
	}
	want := []Step{
		{Try: "fr-CA", Match: MatchExact},
		{Try: "fr", Match: MatchLanguage},
		{Try: "", Match: MatchDefault},
	}
	if !reflect.DeepEqual(steps, want) {
		t.Errorf("steps = %+v, want %+v", steps, want)
	}
}

func TestRenderVersionRendersInactiveVersions(t *testing.T) {
	v := &Version{Locale: "fr", Active: false, Subject: "Bonjour {{.name}}"}
	out, err := NewRenderer().RenderVersion(v, nil, map[string]any{"name": "Ada"})
	if err != nil || out.Subject != "Bonjour Ada" {
		t.Fatalf("RenderVersion = %+v, %v", out, err)
	}
}

func TestRenderStillRefusesMissingRequired(t *testing.T) {
	tmpl := &Template{
		Variables: []Variable{{Name: "code", Required: true}},
		Versions:  []Version{{Active: true, Text: "{{.code}}"}},
	}
	_, err := NewRenderer().Render(tmpl, "en", nil)
	if !errors.Is(err, ErrMissingRequiredVariable) {
		t.Errorf("err = %v, want ErrMissingRequiredVariable", err)
	}
}

func TestFuncNames(t *testing.T) {
	want := []string{"default", "formatDate", "lower", "now", "title", "truncate", "upper"}
	if got := NewRenderer().FuncNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("FuncNames = %v, want %v", got, want)
	}
}
