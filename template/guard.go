package template

import (
	"fmt"
	htmltpl "html/template"
	"io"
	"net/url"
	"reflect"
	"strings"
	texttpl "text/template"
	"text/template/parse"
	"time"
	"unicode/utf8"
)

// MaxBuiltFieldBytes is the most the string-building functions (print,
// printf, println, html, js, urlquery and the string helpers) may build in
// total while one field renders. Capping each result alone isn't enough: a
// template can keep many results alive in variables without writing them.
const MaxBuiltFieldBytes = 4 * MaxRenderedFieldBytes

// evalArgsFunc is the name the HTML renderer gives its bounded replacement
// for html/template's _eval_args_ (see boundEvalArgs).
const evalArgsFunc = "_herald_eval_args_"

// buildBudget counts the bytes string-building functions produce while one
// field renders. Every render gets a fresh one.
type buildBudget struct{ left int }

func newBudget() *buildBudget { return &buildBudget{left: MaxBuiltFieldBytes} }

// room fails when a result of n bytes would pass the per-result limit or
// what is left of the field's budget.
func (b *buildBudget) room(n int) error {
	if n > MaxRenderedFieldBytes || n > b.left {
		return ErrRenderedTooLarge
	}
	return nil
}

// spend charges s to the budget and returns it.
func (b *buildBudget) spend(s string) (string, error) {
	if err := b.room(len(s)); err != nil {
		return "", err
	}
	b.left -= len(s)
	return s, nil
}

// funcs returns the function map one render parses with: the helpers, with
// the string-building ones charged to b, and text/template's string-building
// builtins replaced by bounded versions that produce the same bytes.
func (r *Renderer) funcs(b *buildBudget) texttpl.FuncMap {
	m := make(texttpl.FuncMap, len(r.funcMap)+8)
	for k, v := range r.funcMap {
		m[k] = v
	}
	for k, v := range guardedHelpers(b) {
		if _, ok := m[k]; ok {
			m[k] = v
		}
	}
	for k, v := range guardedBuiltins(b) {
		m[k] = v
	}
	return m
}

// guardedHelpers charges the helpers that build a new string.
func guardedHelpers(b *buildBudget) texttpl.FuncMap {
	casing := func(f func(string) string) func(string) (string, error) {
		return func(s string) (string, error) {
			if err := b.room(len(s)); err != nil {
				return "", err
			}
			return b.spend(f(s))
		}
	}
	return texttpl.FuncMap{
		"upper": casing(strings.ToUpper),
		"lower": casing(strings.ToLower),
		"title": casing(strings.Title), //nolint:staticcheck // simple title case is sufficient
		"truncate": func(s string, n int) (string, error) {
			if len(s) <= n {
				return s, nil
			}
			if err := b.room(n + 3); err != nil {
				return "", err
			}
			return b.spend(s[:n] + "...")
		},
		"formatDate": func(t time.Time, layout string) (string, error) {
			if err := b.room(3*len(layout) + 64); err != nil {
				return "", err
			}
			return b.spend(t.Format(layout))
		},
	}
}

// guardedBuiltins replaces text/template's string-building builtins. Each
// works out the size of its result, or an upper bound on it, before building
// anything, and refuses when that would pass the budget.
func guardedBuiltins(b *buildBudget) texttpl.FuncMap {
	return texttpl.FuncMap{
		"print": func(args ...any) (string, error) {
			if err := b.room(sprintLen(args, false)); err != nil {
				return "", err
			}
			return b.spend(fmt.Sprint(args...))
		},
		"println": func(args ...any) (string, error) {
			if err := b.room(sprintLen(args, true)); err != nil {
				return "", err
			}
			return b.spend(fmt.Sprintln(args...))
		},
		"printf": func(format string, args ...any) (string, error) {
			if err := b.room(sprintfBound(format, args)); err != nil {
				return "", err
			}
			return b.spend(fmt.Sprintf(format, args...))
		},
		"html": func(args ...any) (string, error) {
			return escapeBounded(b, args, func(w io.Writer, s string) { texttpl.HTMLEscape(w, []byte(s)) })
		},
		"js": func(args ...any) (string, error) {
			return escapeBounded(b, args, func(w io.Writer, s string) { texttpl.JSEscape(w, []byte(s)) })
		},
		"urlquery": func(args ...any) (string, error) {
			return escapeBounded(b, args, queryEscape)
		},
		evalArgsFunc: func(args ...any) (string, error) { return boundEvalArgs(b, args) },
	}
}

// sprintLen is the exact length of fmt.Sprint(args...), or of
// fmt.Sprintln(args...) when ln is set, measured one operand at a time.
func sprintLen(args []any, ln bool) int {
	n := 0
	prevString := false
	for i, arg := range args {
		isString := arg != nil && reflect.TypeOf(arg).Kind() == reflect.String
		if i > 0 && (ln || (!isString && !prevString)) {
			n++
		}
		n += operandLen(arg)
		prevString = isString
	}
	if ln {
		n++
	}
	return n
}

// operandLen is len(fmt.Sprint(arg)) without copying a plain string.
func operandLen(arg any) int {
	if s, ok := arg.(string); ok {
		return len(s)
	}
	return len(fmt.Sprint(arg))
}

// textEvalArgs is text/template's evalArgs: the string html, js and urlquery
// escape. It reports the error a too-large result would be.
func textEvalArgs(b *buildBudget, args []any) (string, error) {
	if len(args) == 1 {
		if s, ok := args[0].(string); ok {
			return s, nil
		}
	}
	for i, arg := range args {
		if a, ok := printableValue(reflect.ValueOf(arg)); ok {
			args[i] = a
		}
	}
	if err := b.room(sprintLen(args, false)); err != nil {
		return "", err
	}
	return b.spend(fmt.Sprint(args...))
}

// printableValue and indirect are text/template's, so html, js and urlquery
// stringify their arguments exactly as the builtins do.
func printableValue(v reflect.Value) (any, bool) {
	if v.Kind() == reflect.Pointer {
		v, _ = indirect(v)
	}
	if !v.IsValid() {
		return "<no value>", true
	}
	if !v.Type().Implements(errorType) && !v.Type().Implements(fmtStringerType) {
		if v.CanAddr() && (reflect.PointerTo(v.Type()).Implements(errorType) || reflect.PointerTo(v.Type()).Implements(fmtStringerType)) {
			v = v.Addr()
		} else {
			switch v.Kind() {
			case reflect.Chan, reflect.Func:
				return nil, false
			}
		}
	}
	return v.Interface(), true
}

func indirect(v reflect.Value) (rv reflect.Value, isNil bool) {
	for ; v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface; v = v.Elem() {
		if v.IsNil() {
			return v, true
		}
	}
	return v, false
}

var (
	errorType       = reflect.TypeFor[error]()
	fmtStringerType = reflect.TypeFor[fmt.Stringer]()
)

// boundEvalArgs is html/template's evalArgs with the budget applied. The HTML
// renderer calls it where html/template would call _eval_args_.
func boundEvalArgs(b *buildBudget, args []any) (string, error) {
	if len(args) == 1 {
		if s, ok := args[0].(string); ok {
			return s, nil
		}
	}
	for i, arg := range args {
		args[i] = indirectToStringerOrError(arg)
	}
	if err := b.room(sprintLen(args, false)); err != nil {
		return "", err
	}
	return b.spend(fmt.Sprint(args...))
}

// indirectToStringerOrError is html/template's.
func indirectToStringerOrError(a any) any {
	if a == nil {
		return nil
	}
	v := reflect.ValueOf(a)
	for !v.Type().Implements(fmtStringerType) && !v.Type().Implements(errorType) && v.Kind() == reflect.Pointer && !v.IsNil() {
		v = v.Elem()
	}
	return v.Interface()
}

// escapeBounded stringifies args as the builtin would, then escapes into a
// writer that stops growing at the budget. Escaping never shrinks its
// input, so an input that is already too large is refused first.
func escapeBounded(b *buildBudget, args []any, escape func(io.Writer, string)) (string, error) {
	s, err := textEvalArgs(b, args)
	if err != nil {
		return "", err
	}
	if err := b.room(len(s)); err != nil {
		return "", err
	}
	limit := min(MaxRenderedFieldBytes, b.left)
	w := &cappedWriter{limit: limit}
	escape(w, s)
	if w.over {
		return "", ErrRenderedTooLarge
	}
	return b.spend(w.buf.String())
}

// queryEscape is url.QueryEscape written in pieces. It escapes byte by byte,
// so escaping chunks and joining them is the same as escaping the whole.
func queryEscape(w io.Writer, s string) {
	const chunk = 32 << 10
	for s != "" {
		n := min(chunk, len(s))
		if _, err := io.WriteString(w, url.QueryEscape(s[:n])); err != nil {
			return
		}
		s = s[n:]
	}
}

// cappedWriter keeps what is written until the total would pass limit, then
// refuses every later write.
type cappedWriter struct {
	buf   strings.Builder
	limit int
	over  bool
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	if w.over || w.buf.Len()+len(p) > w.limit {
		w.over = true
		return 0, ErrRenderedTooLarge
	}
	return w.buf.Write(p)
}

// boundEvalArgsInHTML rewrites every output action of the form
// {{html a b ...}} or {{urlquery a b ...}} in t into
// {{_herald_eval_args_ a b ... | html}}. html/template would otherwise turn
// it into {{_eval_args_ a b ... | html}} with its own unbounded _eval_args_,
// which it installs after Herald's functions and so can't be replaced. The
// result is the same; only the joining is bounded.
func boundEvalArgsInHTML(t *htmltpl.Template) {
	for _, tt := range t.Templates() {
		if tt.Tree != nil {
			rewriteEscaperArgs(tt.Tree.Root)
		}
	}
}

func rewriteEscaperArgs(node parse.Node) {
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, c := range n.Nodes {
			rewriteEscaperArgs(c)
		}
	case *parse.ActionNode:
		p := n.Pipe
		if p == nil || len(p.Decl) != 0 || len(p.Cmds) != 1 || len(p.Cmds[0].Args) < 2 {
			return
		}
		cmd := p.Cmds[0]
		id, ok := cmd.Args[0].(*parse.IdentifierNode)
		if !ok || (id.Ident != "html" && id.Ident != "urlquery") {
			return
		}
		esc := id.Ident
		cmd.Args[0] = parse.NewIdentifier(evalArgsFunc).SetTree(nil).SetPos(id.Position())
		escCmd := &parse.CommandNode{NodeType: parse.NodeCommand, Pos: p.Position()}
		escCmd.Args = []parse.Node{parse.NewIdentifier(esc).SetTree(nil).SetPos(p.Position())}
		p.Cmds = append(p.Cmds, escCmd)
	case *parse.IfNode:
		rewriteEscaperArgs(n.List)
		rewriteEscaperArgs(n.ElseList)
	case *parse.RangeNode:
		rewriteEscaperArgs(n.List)
		rewriteEscaperArgs(n.ElseList)
	case *parse.WithNode:
		rewriteEscaperArgs(n.List)
		rewriteEscaperArgs(n.ElseList)
	}
}

// boundCeiling keeps sprintfBound's sums from overflowing; anything near it
// is refused anyway.
const boundCeiling = 1 << 50

// fmtMaxNum bounds a width or precision: fmt refuses larger ones from an
// argument and stops reading literal digits soon after.
const fmtMaxNum = 1e8

// sprintfBound is an upper bound on len(fmt.Sprintf(format, args...)) for the
// values JSON data and templates produce (strings, numbers, bools, nil, and
// lists and maps of them), worked out without formatting anything large.
// It follows fmt's verb grammar; a verb whose operand it can't place (any
// explicit [n] index) is charged as the costliest operand.
func sprintfBound(format string, args []any) int {
	sizes := make([]int, len(args))
	nodes := make([]int, len(args))
	extra := make([]int, len(args))
	for i, a := range args {
		sizes[i] = operandLen(a)
		nodes[i], extra[i] = shape(reflect.ValueOf(a), 0)
	}
	reordered := strings.Contains(format, "[")
	cost := func(i int, verb rune, sharp bool, w, p int) int {
		base := 160
		switch verb {
		case 'f', 'F':
			base = 900
		}
		factor := 5
		if !sharp && (verb == 's' || verb == 'v' || verb == 'd' || verb == 't') {
			factor = 1
		}
		one := func(j int) int { return nodes[j]*(w+2*p+base) + factor*sizes[j] + extra[j] }
		if !reordered {
			if i < len(args) {
				return one(i)
			}
			return base
		}
		worst := base
		for j := range args {
			worst = max(worst, one(j))
		}
		return worst
	}
	star := func(i int) int {
		bound := func(a any) int {
			v := reflect.ValueOf(a)
			var n int64
			switch v.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				n = v.Int()
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				return int(min(v.Uint(), fmtMaxNum)) //nolint:gosec // min caps it at fmtMaxNum
			default:
				return 0
			}
			if n > fmtMaxNum || n < -fmtMaxNum {
				return fmtMaxNum
			}
			return int(max(n, -n))
		}
		if !reordered {
			if i < len(args) {
				return bound(args[i])
			}
			return 0
		}
		worst := 0
		for _, a := range args {
			worst = max(worst, bound(a))
		}
		return worst
	}
	number := func(i int) (int, int) {
		n := 0
		for i < len(format) && '0' <= format[i] && format[i] <= '9' {
			n = min(n*10+int(format[i]-'0'), fmtMaxNum)
			i++
		}
		return n, i
	}
	index := func(i int) int {
		if i < len(format) && format[i] == '[' {
			if j := strings.IndexByte(format[i:], ']'); j > 0 {
				return i + j + 1
			}
			return i + 1
		}
		return i
	}

	total := len(format)
	argNum := 0
	for i := 0; i < len(format); {
		if format[i] != '%' {
			i++
			continue
		}
		i++
		sharp := false
		for i < len(format) && strings.IndexByte("#0+- ", format[i]) >= 0 {
			if format[i] == '#' {
				sharp = true
			}
			i++
		}
		i = index(i)
		w := 0
		if i < len(format) && format[i] == '*' {
			i++
			w = star(argNum)
			argNum++
		} else {
			w, i = number(i)
		}
		p := 0
		if i+1 < len(format) && format[i] == '.' {
			i = index(i + 1)
			if i < len(format) && format[i] == '*' {
				i++
				p = star(argNum)
				argNum++
			} else {
				p, i = number(i)
			}
		}
		i = index(i)
		total = min(total+32, boundCeiling) // fmt's own notes: BADWIDTH, MISSING, %!verb(...)
		if i >= len(format) {
			break
		}
		verb, size := utf8.DecodeRuneInString(format[i:])
		i += size
		if verb == '%' {
			continue
		}
		total = min(total+cost(argNum, verb, sharp, w, p), boundCeiling)
		argNum++
	}
	for j, a := range args { // %!(EXTRA type=value, ...)
		total = min(total+sizes[j]+4, boundCeiling)
		if a != nil {
			total = min(total+len(reflect.TypeOf(a).String()), boundCeiling)
		}
	}
	return total
}

// shape counts the values fmt visits in v (each container and each leaf) and
// the bytes of type and field names %#v and %+v may add, stopping once the
// count is already too large to matter.
func shape(v reflect.Value, depth int) (count, names int) {
	if !v.IsValid() {
		return 1, 0
	}
	if depth == 0 && v.Kind() == reflect.Pointer && !v.IsNil() {
		v = v.Elem()
	}
	count = 1
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return 1, 0
		}
		return shape(v.Elem(), depth+1)
	case reflect.Slice, reflect.Array:
		names = len(v.Type().String())
		if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
			return count + v.Len(), names
		}
		for i := 0; i < v.Len() && count < MaxRenderedFieldBytes; i++ {
			c, n := shape(v.Index(i), depth+1)
			count, names = count+c, names+n
		}
	case reflect.Map:
		names = len(v.Type().String())
		it := v.MapRange()
		for it.Next() && count < MaxRenderedFieldBytes {
			c, n := shape(it.Key(), depth+1)
			count, names = count+c, names+n
			c, n = shape(it.Value(), depth+1)
			count, names = count+c, names+n
		}
	case reflect.Struct:
		names = len(v.Type().String())
		for i := 0; i < v.NumField() && count < MaxRenderedFieldBytes; i++ {
			names += len(v.Type().Field(i).Name) + 1
			c, n := shape(v.Field(i), depth+1)
			count, names = count+c, names+n
		}
	}
	return count, names
}
