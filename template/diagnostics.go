package template

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Diagnostic is one problem found while previewing a template. Positions are
// 1-based; a zero Line or Column means Go did not report one.
type Diagnostic struct {
	Field    string `json:"field"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Message  string `json:"message"`
}

// Diagnostic severities and kinds.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"

	KindParse      = "parse"
	KindExec       = "exec"
	KindEscape     = "escape"
	KindMissing    = "missing"
	KindUndeclared = "undeclared"
	KindUnprovided = "unprovided"
)

// goError matches the three shapes Go's template packages produce when the
// template is named after its field:
//
//	template: html:3: unexpected EOF                        (parse: line only)
//	template: html:2:5: executing "html" at <.x>: ...       (exec: line, byte column)
//	html/template:html:2:5: {{if}} branches end in ...      (escaper: line, byte column)
var goError = regexp.MustCompile(`^(html/template:|template: )([a-z]*)(?::(\d+))?(?::(\d+))?: (.*)$`)

var execPrefix = regexp.MustCompile(`^executing "[^"]*" at (<.*?>): (.*)$`)

// diagnose turns an error from parsing or executing one field into a
// Diagnostic, converting Go's 0-based byte column into a 1-based character
// column on the field's own source line.
func diagnose(field, src string, err error) Diagnostic {
	d := Diagnostic{Field: field, Severity: SeverityError, Kind: KindParse, Message: err.Error()}
	m := goError.FindStringSubmatch(err.Error())
	if m == nil {
		return d
	}
	d.Message = m[5]
	if m[1] == "html/template:" {
		d.Kind = KindEscape
	} else if x := execPrefix.FindStringSubmatch(d.Message); x != nil {
		d.Kind = KindExec
		d.Message = x[1] + ": " + x[2]
	}
	if m[3] != "" {
		d.Line, _ = strconv.Atoi(m[3]) //nolint:errcheck // regexp match ensures valid int
	}
	if m[4] != "" {
		byteCol, _ := strconv.Atoi(m[4]) //nolint:errcheck // regexp match ensures valid int
		d.Column = charColumn(src, d.Line, byteCol)
	}
	return d
}

// charColumn converts a 0-based byte offset within a 1-based line of src into
// a 1-based character column.
func charColumn(src string, line, byteCol int) int {
	lines := strings.Split(src, "\n")
	if line < 1 || line > len(lines) {
		return 0
	}
	l := lines[line-1]
	if byteCol > len(l) {
		byteCol = len(l)
	}
	return utf8.RuneCountInString(l[:byteCol]) + 1
}

var location = regexp.MustCompile(`:(\d+):(\d+)$`)

// position returns the 1-based line and character column of a parse-tree
// location string ("name:line:byteCol").
func position(src, loc string) (line int, col int) {
	m := location.FindStringSubmatch(loc)
	if m == nil {
		return 0, 0
	}
	line, _ = strconv.Atoi(m[1])     //nolint:errcheck // regexp match ensures valid int
	byteCol, _ := strconv.Atoi(m[2]) //nolint:errcheck // regexp match ensures valid int
	return line, charColumn(src, line, byteCol)
}
