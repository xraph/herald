package memory

import (
	"maps"
	"slices"

	"github.com/xraph/herald/inbox"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/preference"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/template"
)

// The memory store copies on the way in and on the way out, so neither a
// caller mutating what it passed nor one mutating what it read can change
// stored state. SQL and Mongo behave this way by construction.

func cloneProvider(p *provider.Provider) *provider.Provider {
	c := *p
	c.Credentials = maps.Clone(p.Credentials)
	c.Settings = maps.Clone(p.Settings)
	return &c
}

func cloneTemplate(t *template.Template) *template.Template {
	c := *t
	c.Variables = slices.Clone(t.Variables)
	c.Versions = nil // attached per read from the versions map
	return &c
}

func cloneVersion(v *template.Version) *template.Version {
	c := *v
	return &c
}

func cloneMessage(m *message.Message) *message.Message {
	c := *m
	c.Metadata = maps.Clone(m.Metadata)
	if m.SentAt != nil {
		t := *m.SentAt
		c.SentAt = &t
	}
	if m.DeliveredAt != nil {
		t := *m.DeliveredAt
		c.DeliveredAt = &t
	}
	return &c
}

func cloneNotification(n *inbox.Notification) *inbox.Notification {
	c := *n
	c.Metadata = maps.Clone(n.Metadata)
	if n.ReadAt != nil {
		t := *n.ReadAt
		c.ReadAt = &t
	}
	if n.ExpiresAt != nil {
		t := *n.ExpiresAt
		c.ExpiresAt = &t
	}
	return &c
}

func clonePreference(p *preference.Preference) *preference.Preference {
	c := *p
	if p.Overrides != nil {
		c.Overrides = make(map[string]preference.ChannelPreference, len(p.Overrides))
		for k, v := range p.Overrides {
			c.Overrides[k] = preference.ChannelPreference{
				Email: cloneBool(v.Email), SMS: cloneBool(v.SMS), Push: cloneBool(v.Push), InApp: cloneBool(v.InApp),
			}
		}
	}
	return &c
}

func cloneBool(b *bool) *bool {
	if b == nil {
		return nil
	}
	v := *b
	return &v
}

func cloneScopedConfig(c *scope.Config) *scope.Config {
	x := *c
	return &x
}

// page applies offset and limit the way the SQL stores do: an offset past the
// end is an empty page, and a limit of zero means no limit.
func page[T any](xs []T, limit, offset int) []T {
	if offset >= len(xs) {
		return nil
	}
	if offset > 0 {
		xs = xs[offset:]
	}
	if limit > 0 && limit < len(xs) {
		xs = xs[:limit]
	}
	return xs
}
