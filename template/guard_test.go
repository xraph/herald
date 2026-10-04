package template

import (
	"errors"
	"fmt"
	htmltpl "html/template"
	"runtime"
	"strings"
	"testing"
	texttpl "text/template"
	"time"
)

// doubling grows a variable to 8 << 24 bytes (128 MiB) without writing it.
const doubling = `{{$x := "xxxxxxxx"}}{{range 24}}{{$x = printf "%s%s" $x $x}}{{end}}ok`

// printChain declares $v1 through $v24, each twice the one before.
func printChain() string {
	var b strings.Builder
	b.WriteString(`{{$v0 := "xxxxxxxx"}}`)
	for i := 1; i <= 24; i++ {
		fmt.Fprintf(&b, `{{$v%d := print $v%d $v%d}}`, i, i-1, i-1)
	}
	b.WriteString("ok")
	return b.String()
}

// manyCopies keeps forty 1000000-byte strings alive at once, each under the
// per-result limit.
func manyCopies() string {
	var b strings.Builder
	b.WriteString(`{{$x := printf "%1000000s" ""}}`)
	for i := range 40 {
		fmt.Fprintf(&b, `{{$c%d := print $x "%d"}}`, i, i)
	}
	b.WriteString("ok")
	return b.String()
}

// htmlManyArgs hands html three hundred 500000-byte arguments. In an HTML
// body html/template rewrites this to {{_eval_args_ ... | html}}, which would
// build 150 MB before html saw it.
func htmlManyArgs() string {
	return `{{$x := printf "%500000s" ""}}<p>{{html` + strings.Repeat(" $x", 300) + `}}</p>`
}

// bounded runs f and fails if it takes a second or more, or allocates
// 100 MB or more.
func bounded(t *testing.T, f func()) {
	t.Helper()
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	f()
	took := time.Since(start)
	runtime.ReadMemStats(&after)
	if took >= time.Second {
		t.Errorf("took %s, want well under a second", took)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew >= 100<<20 {
		t.Errorf("allocated %d MB, want far less than 100 MB", grew>>20)
	}
}

func TestStringBuildingIsCappedInPreview(t *testing.T) {
	for name, c := range map[string]Content{
		"printf doubling":        {Text: doubling},
		"print chain":            {Subject: printChain()},
		"many live copies":       {Title: manyCopies()},
		"html with many args":    {HTML: htmlManyArgs()},
		"printf width":           {Text: `{{$x := printf "%9999999s" ""}}ok`},
		"printf width on a list": {Text: `{{$x := printf "%999999v" .items}}ok`},
	} {
		t.Run(name, func(t *testing.T) {
			var res *PreviewResult
			bounded(t, func() {
				res = NewRenderer().Preview(c, []Variable{{Name: "items"}}, map[string]any{"items": make([]any, 50)})
			})
			found := false
			for _, d := range res.Diagnostics {
				if d.Kind == KindExec && strings.Contains(d.Message, "KiB limit") {
					found = true
					if d.Line == 0 || d.Column == 0 {
						t.Errorf("diagnostic has no position: %+v", d)
					}
				}
			}
			if !found {
				t.Errorf("no too-large diagnostic; got %+v", res.Diagnostics)
			}
		})
	}
}

func TestStringBuildingIsCappedInRender(t *testing.T) {
	for name, v := range map[string]*Version{
		"printf doubling":     {Text: doubling},
		"print chain":         {Subject: printChain()},
		"html with many args": {HTML: htmlManyArgs()},
	} {
		t.Run(name, func(t *testing.T) {
			var err error
			bounded(t, func() { _, err = NewRenderer().RenderVersion(v, nil, nil) })
			if !errors.Is(err, ErrRenderedTooLarge) || !errors.Is(err, ErrTemplateRenderFailed) {
				t.Errorf("err = %v, want ErrRenderedTooLarge inside ErrTemplateRenderFailed", err)
			}
		})
	}
}

// TestGuardedBuiltinsMatchTheStdlib calls each override and the function
// text/template ships under the same name with the same arguments.
func TestGuardedBuiltinsMatchTheStdlib(t *testing.T) {
	type stringer struct{ fmt.Stringer }
	var nilPtr *int
	n := 7
	argSets := [][]any{
		{},
		{"plain"},
		{"<a href=\"x\">&'</a>\x00"},
		{"a b", "c&d", "é/ü?=+"},
		{1, 2.5, true, nil},
		{"a", 1, 2, "b", nil, nil},
		{nilPtr, &n, []int{1, 2}, map[string]any{"k": "<v>"}},
		{[]any{"x", 1.5, nil}, "tail"},
		{"  \U0001F600 \x7f </script>"},
		{htmltpl.HTML("<b>"), stringer{}},
	}
	b := newBudget()
	g := guardedBuiltins(b)
	stdlib := map[string]func(...any) string{
		"print": fmt.Sprint, "println": fmt.Sprintln,
		"html": texttpl.HTMLEscaper, "js": texttpl.JSEscaper, "urlquery": texttpl.URLQueryEscaper,
	}
	for name, want := range stdlib {
		fn := g[name].(func(...any) (string, error))
		for _, args := range argSets {
			got, err := fn(append([]any(nil), args...)...)
			if err != nil {
				t.Errorf("%s%v: %v", name, args, err)
				continue
			}
			if w := want(append([]any(nil), args...)...); got != w {
				t.Errorf("%s%v = %q, stdlib %q", name, args, got, w)
			}
		}
	}

	printf := g["printf"].(func(string, ...any) (string, error))
	for _, c := range []struct {
		format string
		args   []any
	}{
		{"plain", nil},
		{"%s and %d", []any{"x", 3}},
		{"%5.2f|%-8s|%08.3f|%+d|% d", []any{3.14159, "ab", -2.5, 4, 5}},
		{"%q %x % X %#x %#q %+q", []any{"a\"b\x00é", "hi", "hi", "hi", "back`tick", "é"}},
		{"%v %+v %#v %T", []any{[]any{1, "a", nil}, map[string]any{"k": 1.5}, []any{"q"}, 1}},
		{"%[2]s %[1]s %[2]s", []any{"one", "two"}},
		{"%*d|%-*d|%.*f", []any{5, 42, 4, 7, 2, 3.14159}},
		{"%d %s", []any{"str", 5}},
		{"%s %s %s", []any{"only"}},
		{"%s", []any{"a", "extra", 3, nil}},
		{"%!%%z%", []any{1}},
		{"%[5]d %[x]d %[1]", []any{1}},
		{"%e %g %b %o %c %U", []any{1e300, 1e-7, 10, 64, 'é', 0x1F600}},
		{"%10.3v|%.2s", []any{[]any{1.23456, "abcdef"}, "xyz"}},
		{"%v %s", []any{nil, nilPtr}},
		{"%.0f", []any{1e308}},
	} {
		got, err := printf(c.format, c.args...)
		if err != nil {
			t.Errorf("printf(%q, %v): %v", c.format, c.args, err)
			continue
		}
		if want := fmt.Sprintf(c.format, c.args...); got != want {
			t.Errorf("printf(%q, %v) = %q, stdlib %q", c.format, c.args, got, want)
		}
	}
}

// TestGuardedTemplatesRenderLikeTheStdlib renders the same sources with
// Herald's renderer and with the stdlib packages, which use the real
// builtins, and compares the output byte for byte.
func TestGuardedTemplatesRenderLikeTheStdlib(t *testing.T) {
	data := map[string]any{
		"name": "Ada <Lovelace>", "n": 3, "price": 9.5, "items": []any{"a&b", 2, nil},
		"url": "https://example.com/?q=a b&c=é", "nothing": nil,
	}
	sources := []string{
		`{{print .name .n .price .nothing}}`,
		`{{println .name .items}}`,
		`{{printf "%s costs %.2f (%d) %v %q" .name .price .n .items .missing}}`,
		`{{html .name .n .nothing .missing}} {{js .name}} {{urlquery .url .n}}`,
		`{{$x := printf "%05d" .n}}{{$y := print $x "-" .name}}{{$y | html | js}}`,
		`{{range .items}}[{{printf "%v" .}}]{{end}}`,
		`{{upper .name}} {{lower .name}} {{title "ada lovelace"}} {{truncate .name 3}} {{default "x" .nothing}}`,
	}
	htmlSources := []string{
		`<p title="{{.name}}">{{html .name .n .nothing .missing}}</p>`,
		`<a href="/s?q={{urlquery .name .n}}">{{print .name .n}}</a>`,
		`<script>var s = {{js .name}}; var t = {{printf "%q" .name}};</script>`,
		`<p>{{html .name}}</p><p>{{.name | html}}</p><p>{{printf "%s" .name | urlquery}}</p>`,
	}
	r := NewRenderer()
	for _, src := range sources {
		got, err := r.renderText(src, data)
		if err != nil {
			t.Errorf("herald %q: %v", src, err)
			continue
		}
		var want strings.Builder
		if err := texttpl.Must(texttpl.New("").Funcs(r.funcMap).Parse(src)).Execute(&want, data); err != nil {
			t.Fatalf("stdlib %q: %v", src, err)
		}
		if got != want.String() {
			t.Errorf("text %q\nherald %q\nstdlib %q", src, got, want.String())
		}
	}
	for _, src := range append(sources, htmlSources...) {
		got, err := r.renderHTML(src, data)
		var want strings.Builder
		//nolint:unconvert // html/template and text/template have distinct FuncMap types
		wantErr := htmltpl.Must(htmltpl.New("").Funcs(htmltpl.FuncMap(r.funcMap)).Parse(src)).Execute(&want, data)
		if err != nil || wantErr != nil {
			// html/template refuses some sources (html piped into js), and
			// must refuse them the same way.
			if err == nil || wantErr == nil || err.Error() != wantErr.Error() {
				t.Errorf("html %q: herald err %v, stdlib err %v", src, err, wantErr)
			}
			continue
		}
		if got != want.String() {
			t.Errorf("html %q\nherald %q\nstdlib %q", src, got, want.String())
		}
	}
}

func TestFuncNamesListsOnlyTheHelpers(t *testing.T) {
	got := strings.Join(NewRenderer().FuncNames(), ",")
	if got != "default,formatDate,lower,now,title,truncate,upper" {
		t.Errorf("FuncNames = %s", got)
	}
}
