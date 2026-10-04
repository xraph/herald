package contract

import "github.com/xraph/herald/template"

// Wire types are camelCase and live in this package. Handlers never return
// a Herald domain struct, so a field added to one can't leak by accident.

// AppRef names the app a session is looking at.
type AppRef struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// FieldInfo describes one value a driver reads.
type FieldInfo struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Help      string `json:"help,omitempty"`
	Required  bool   `json:"required"`
	Secret    bool   `json:"secret"`
	Placement string `json:"placement"`
}

// DriverInfo is a registered driver. Fields is null for a driver that
// doesn't describe itself, so the form falls back to key/value rows, and
// an empty list for one that needs nothing.
type DriverInfo struct {
	Name    string      `json:"name"`
	Channel string      `json:"channel"`
	Fields  []FieldInfo `json:"fields"`
}

// hasFallback reports whether a template has an active "" version, which
// answers any locale it doesn't list.
func hasFallback(t *template.Template) bool {
	for _, v := range t.Versions {
		if v.Active && v.Locale == "" {
			return true
		}
	}
	return false
}
