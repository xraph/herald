package template

import "strings"

// Match says how a version was chosen for a requested locale.
type Match string

// The ways Resolve can answer a locale, in the order it tries them.
const (
	MatchExact    Match = "exact"    // an active version for the locale itself
	MatchLanguage Match = "language" // "fr" answering "fr-CA"
	MatchDefault  Match = "default"  // the active version with locale ""
	MatchNone     Match = "none"
)

// Step is one lookup Resolve made: the locale it tried, which rule that was,
// and whether an active version answered.
type Step struct {
	Try       string `json:"try"`
	Match     Match  `json:"match"`
	Found     bool   `json:"found"`
	VersionID string `json:"version_id,omitempty"`
}

// Explain returns every lookup Resolve makes for locale, in order, and the
// version that answered (nil when none did). Only active versions count, and
// the configured default locale is never consulted: a locale with no exact,
// language or "" version fails.
func Explain(tmpl *Template, locale string) ([]Step, *Version) {
	var steps []Step
	try := func(want string, m Match) *Version {
		v := firstActive(tmpl, want)
		s := Step{Try: want, Match: m, Found: v != nil}
		if v != nil {
			s.VersionID = v.ID.String()
		}
		steps = append(steps, s)
		return v
	}
	if v := try(locale, MatchExact); v != nil {
		return steps, v
	}
	if lang, _, ok := strings.Cut(locale, "-"); ok && lang != "" {
		if v := try(lang, MatchLanguage); v != nil {
			return steps, v
		}
	}
	if locale != "" {
		if v := try("", MatchDefault); v != nil {
			return steps, v
		}
	}
	return steps, nil
}

// Resolve returns the version that answers locale and how it matched, or
// (nil, MatchNone).
func Resolve(tmpl *Template, locale string) (*Version, Match) {
	steps, v := Explain(tmpl, locale)
	if v == nil {
		return nil, MatchNone
	}
	return v, steps[len(steps)-1].Match
}

func firstActive(tmpl *Template, locale string) *Version {
	for i := range tmpl.Versions {
		if v := &tmpl.Versions[i]; v.Active && v.Locale == locale {
			return v
		}
	}
	return nil
}
