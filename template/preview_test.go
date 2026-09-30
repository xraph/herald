package template

import (
	"strings"
	"testing"
)

func only(t *testing.T, res *PreviewResult, want Diagnostic) {
	t.Helper()
	for _, d := range res.Diagnostics {
		if d.Field == want.Field && d.Kind == want.Kind {
			if d.Line != want.Line || d.Column != want.Column || d.Severity != want.Severity {
				t.Errorf("%s/%s at %d:%d (%s), want %d:%d (%s): %s",
					d.Field, d.Kind, d.Line, d.Column, d.Severity, want.Line, want.Column, want.Severity, d.Message)
			}
			if want.Message != "" && d.Message != want.Message {
				t.Errorf("message = %q, want %q", d.Message, want.Message)
			}
			return
		}
	}
	t.Errorf("no %s diagnostic on %q; got %+v", want.Kind, want.Field, res.Diagnostics)
}

func output(res *PreviewResult, field string) (string, bool) {
	for _, f := range res.Fields {
		if f.Field == field {
			return f.Output, f.Rendered
		}
	}
	return "", false
}

var name = []Variable{{Name: "name"}}

func TestParseErrorHasALineAndNoColumn(t *testing.T) {
	res := NewRenderer().Preview(Content{Subject: "Hi {{.name}}", HTML: "<p>\nline2\n{{if .name}}unclosed"}, name, map[string]any{"name": "Ada"})
	only(t, res, Diagnostic{Field: "html", Kind: KindParse, Severity: SeverityError, Line: 3, Column: 0, Message: "unexpected EOF"})
	if out, ok := output(res, "subject"); !ok || out != "Hi Ada" {
		t.Errorf("a broken html field stopped the subject rendering: %q %v", out, ok)
	}
}

func TestExecErrorHasAColumn(t *testing.T) {
	res := NewRenderer().Preview(Content{Text: "x\n  {{ index .name 5 }}"}, name, map[string]any{"name": "Ada"})
	only(t, res, Diagnostic{Field: "text", Kind: KindExec, Severity: SeverityError, Line: 2, Column: 6,
		Message: "<index .name 5>: error calling index: index out of range: 5"})
}

func TestColumnsCountCharactersNotBytes(t *testing.T) {
	// "Ünïcödé " is 12 bytes and 8 characters; "index" starts at character 12.
	res := NewRenderer().Preview(Content{Text: "Ünïcödé {{ index .name 5 }}"}, name, map[string]any{"name": "Ada"})
	only(t, res, Diagnostic{Field: "text", Kind: KindExec, Severity: SeverityError, Line: 1, Column: 12})
}

func TestEscaperErrorsAreTheirOwnKind(t *testing.T) {
	res := NewRenderer().Preview(Content{HTML: "<p>\n{{if .c}}<a href=\"{{else}}<b>{{end}}\">"}, []Variable{{Name: "c"}}, map[string]any{"c": true})
	only(t, res, Diagnostic{Field: "html", Kind: KindEscape, Severity: SeverityError, Line: 2, Column: 6})
}

func TestUnknownFunctionIsAParseError(t *testing.T) {
	res := NewRenderer().Preview(Content{Subject: "{{ nosuch }}", Text: "fine"}, nil, nil)
	only(t, res, Diagnostic{Field: "subject", Kind: KindParse, Severity: SeverityError, Line: 1, Message: `function "nosuch" not defined`})
	if out, ok := output(res, "text"); !ok || out != "fine" {
		t.Errorf("text should still render: %q %v", out, ok)
	}
}

func TestUndeclaredFieldsWarnOncePerName(t *testing.T) {
	res := NewRenderer().Preview(
		Content{Text: "Hi {{.user_name}} {{.missing}} {{.missing}} {{$.other}}"},
		[]Variable{{Name: "user_name"}}, map[string]any{"user_name": "Ada"})
	var got []string
	for _, d := range res.Diagnostics {
		if d.Kind == KindUndeclared {
			got = append(got, d.Message)
		}
	}
	if len(got) != 2 {
		t.Fatalf("undeclared warnings = %v, want one for .missing and one for .other", got)
	}
	only(t, res, Diagnostic{Field: "text", Kind: KindUndeclared, Severity: SeverityWarning, Line: 1, Column: 21,
		Message: ".missing is used but not declared as a variable"})
}

func TestFieldsInsideRangeAreNotUndeclared(t *testing.T) {
	res := NewRenderer().Preview(Content{Text: "{{range .items}}{{.name}}{{end}}"},
		[]Variable{{Name: "items"}}, map[string]any{"items": []map[string]any{{"name": "a"}}})
	if len(res.Diagnostics) != 0 {
		t.Errorf("diagnostics = %+v, want none", res.Diagnostics)
	}
}

func TestMissingAndUnprovidedValues(t *testing.T) {
	res := NewRenderer().Preview(Content{Text: "{{.a}} {{.b}} {{.c}}"},
		[]Variable{{Name: "a", Required: true}, {Name: "b"}, {Name: "c", Default: "dflt"}}, map[string]any{})
	only(t, res, Diagnostic{Kind: KindMissing, Severity: SeverityError,
		Message: `required variable "a" has no value and no default`})
	only(t, res, Diagnostic{Kind: KindUnprovided, Severity: SeverityWarning})
	// Go's own behaviour, shown rather than hidden: a missing value is
	// "<no value>" in text and nothing in HTML.
	if out, _ := output(res, "text"); out != "<no value> <no value> dflt" {
		t.Errorf("text = %q", out)
	}
}

func TestHTMLIsEscaped(t *testing.T) {
	res := NewRenderer().Preview(Content{HTML: "<p>{{.v}}</p>"}, []Variable{{Name: "v"}}, map[string]any{"v": "<b>x</b>"})
	if out, _ := output(res, "html"); out != "<p>&lt;b&gt;x&lt;/b&gt;</p>" {
		t.Errorf("html = %q", out)
	}
}

func TestEveryFieldIsListedInOrder(t *testing.T) {
	res := NewRenderer().Preview(Content{Text: "t"}, nil, nil)
	fields := make([]string, 0, len(res.Fields))
	for _, f := range res.Fields {
		fields = append(fields, f.Field)
	}
	if strings.Join(fields, ",") != "subject,html,text,title" {
		t.Errorf("fields = %v", fields)
	}
	if _, ok := output(res, "subject"); ok {
		t.Error("an empty field reported Rendered")
	}
}
