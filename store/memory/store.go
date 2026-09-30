// Package memory provides an in-memory Store implementation for testing.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/inbox"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/preference"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/template"
)

var _ store.Store = (*Store)(nil)

// Store is an in-memory implementation of herald's composite store.
type Store struct {
	mu            sync.RWMutex
	providers     map[string]*provider.Provider
	templates     map[string]*template.Template
	versions      map[string]*template.Version
	messages      map[string]*message.Message
	notifications map[string]*inbox.Notification
	preferences   map[string]*preference.Preference // key: "appID:userID"
	scopedConfigs map[string]*scope.Config          // key: "appID:scope:scopeID"
}

// New creates a new in-memory store.
func New() *Store {
	return &Store{
		providers:     make(map[string]*provider.Provider),
		templates:     make(map[string]*template.Template),
		versions:      make(map[string]*template.Version),
		messages:      make(map[string]*message.Message),
		notifications: make(map[string]*inbox.Notification),
		preferences:   make(map[string]*preference.Preference),
		scopedConfigs: make(map[string]*scope.Config),
	}
}

func (s *Store) Migrate(_ context.Context) error { return nil }
func (s *Store) Ping(_ context.Context) error    { return nil }
func (s *Store) Close() error                    { return nil }

// ─── Provider Store ──────────────────────────────

func (s *Store) CreateProvider(_ context.Context, p *provider.Provider) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers[p.ID.String()] = cloneProvider(p)
	return nil
}

func (s *Store) GetProvider(_ context.Context, providerID id.ProviderID) (*provider.Provider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.providers[providerID.String()]
	if !ok {
		return nil, store.ErrProviderNotFound
	}
	return cloneProvider(p), nil
}

func (s *Store) UpdateProvider(_ context.Context, p *provider.Provider) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.providers[p.ID.String()]; !ok {
		return store.ErrProviderNotFound
	}
	s.providers[p.ID.String()] = cloneProvider(p)
	return nil
}

func (s *Store) DeleteProvider(_ context.Context, providerID id.ProviderID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.providers[providerID.String()]; !ok {
		return store.ErrProviderNotFound
	}
	delete(s.providers, providerID.String())
	return nil
}

func (s *Store) ListProviders(_ context.Context, appID string, channel string) ([]*provider.Provider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*provider.Provider
	for _, p := range s.providers {
		if p.AppID == appID && (channel == "" || p.Channel == channel) {
			result = append(result, cloneProvider(p))
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Priority != result[j].Priority {
			return result[i].Priority < result[j].Priority
		}
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.Before(result[j].CreatedAt)
		}
		return result[i].ID.String() < result[j].ID.String()
	})
	return result, nil
}

func (s *Store) ListAllProviders(_ context.Context, appID string) ([]*provider.Provider, error) {
	return s.ListProviders(context.Background(), appID, "")
}

// ─── Template Store ──────────────────────────────

func (s *Store) CreateTemplate(_ context.Context, t *template.Template) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.slugTaken(t) {
		return store.ErrDuplicateSlug
	}
	s.templates[t.ID.String()] = cloneTemplate(t)
	return nil
}

func (s *Store) GetTemplate(_ context.Context, templateID id.TemplateID) (*template.Template, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.templates[templateID.String()]
	if !ok {
		return nil, store.ErrTemplateNotFound
	}
	return s.withVersions(t), nil
}

func (s *Store) GetTemplateBySlug(_ context.Context, appID, slug, channel string) (*template.Template, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.templates {
		if t.AppID == appID && t.Slug == slug && t.Channel == channel {
			return s.withVersions(t), nil
		}
	}
	return nil, store.ErrTemplateNotFound
}

func (s *Store) UpdateTemplate(_ context.Context, t *template.Template) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.templates[t.ID.String()]; !ok {
		return store.ErrTemplateNotFound
	}
	if s.slugTaken(t) {
		return store.ErrDuplicateSlug
	}
	s.templates[t.ID.String()] = cloneTemplate(t)
	return nil
}

func (s *Store) DeleteTemplate(_ context.Context, templateID id.TemplateID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.templates[templateID.String()]; !ok {
		return store.ErrTemplateNotFound
	}
	delete(s.templates, templateID.String())
	for k, v := range s.versions {
		if v.TemplateID.String() == templateID.String() {
			delete(s.versions, k)
		}
	}
	return nil
}

// slugTaken reports whether another template already holds t's (app, slug,
// channel), the key SQL and Mongo enforce with a unique index.
func (s *Store) slugTaken(t *template.Template) bool {
	for _, o := range s.templates {
		if o.ID.String() != t.ID.String() && o.AppID == t.AppID && o.Slug == t.Slug && o.Channel == t.Channel {
			return true
		}
	}
	return false
}

func (s *Store) ListTemplates(_ context.Context, appID string) ([]*template.Template, error) {
	return s.listTemplates(func(t *template.Template) bool { return t.AppID == appID }), nil
}

func (s *Store) ListTemplatesByChannel(_ context.Context, appID, channel string) ([]*template.Template, error) {
	return s.listTemplates(func(t *template.Template) bool { return t.AppID == appID && t.Channel == channel }), nil
}

func (s *Store) listTemplates(keep func(*template.Template) bool) []*template.Template {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*template.Template
	for _, t := range s.templates {
		if keep(t) {
			result = append(result, s.withVersions(t))
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.Before(result[j].CreatedAt)
		}
		return result[i].ID.String() < result[j].ID.String()
	})
	return result
}

// withVersions returns a copy of t carrying its versions, in locale order.
// The caller must hold s.mu.
func (s *Store) withVersions(t *template.Template) *template.Template {
	c := cloneTemplate(t)
	for _, v := range s.sortedVersions(t.ID) {
		c.Versions = append(c.Versions, *v)
	}
	return c
}

// sortedVersions returns copies of a template's versions, locale ascending.
// The caller must hold s.mu.
func (s *Store) sortedVersions(templateID id.TemplateID) []*template.Version {
	var result []*template.Version
	for _, v := range s.versions {
		if v.TemplateID.String() == templateID.String() {
			result = append(result, cloneVersion(v))
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Locale < result[j].Locale })
	return result
}

// ─── Version Store ──────────────────────────────

func (s *Store) CreateVersion(_ context.Context, v *template.Version) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.localeTaken(v) {
		return store.ErrDuplicateLocale
	}
	s.versions[v.ID.String()] = cloneVersion(v)
	return nil
}

func (s *Store) GetVersion(_ context.Context, versionID id.TemplateVersionID) (*template.Version, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.versions[versionID.String()]
	if !ok {
		return nil, store.ErrVersionNotFound
	}
	return cloneVersion(v), nil
}

func (s *Store) UpdateVersion(_ context.Context, v *template.Version) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.versions[v.ID.String()]; !ok {
		return store.ErrVersionNotFound
	}
	if s.localeTaken(v) {
		return store.ErrDuplicateLocale
	}
	s.versions[v.ID.String()] = cloneVersion(v)
	return nil
}

func (s *Store) DeleteVersion(_ context.Context, versionID id.TemplateVersionID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.versions[versionID.String()]; !ok {
		return store.ErrVersionNotFound
	}
	delete(s.versions, versionID.String())
	return nil
}

// localeTaken reports whether another version of the same template already
// holds v's locale.
func (s *Store) localeTaken(v *template.Version) bool {
	for _, o := range s.versions {
		if o.ID.String() != v.ID.String() && o.TemplateID.String() == v.TemplateID.String() && o.Locale == v.Locale {
			return true
		}
	}
	return false
}

func (s *Store) ListVersions(_ context.Context, templateID id.TemplateID) ([]*template.Version, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sortedVersions(templateID), nil
}

// ─── Message Store ──────────────────────────────

func (s *Store) CreateMessage(_ context.Context, m *message.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[m.ID.String()] = cloneMessage(m)
	return nil
}

func (s *Store) GetMessage(_ context.Context, messageID id.MessageID) (*message.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.messages[messageID.String()]
	if !ok {
		return nil, store.ErrMessageNotFound
	}
	return cloneMessage(m), nil
}

func (s *Store) RecordDelivery(_ context.Context, messageID id.MessageID, d message.Delivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.messages[messageID.String()]
	if !ok {
		return store.ErrMessageNotFound
	}
	m.Status = d.Status
	m.Error = d.Error
	m.ProviderMessageID = d.ProviderMessageID
	m.SentAt = nil
	if d.SentAt != nil {
		t := *d.SentAt
		m.SentAt = &t
	}
	return nil
}

func (s *Store) CountMessages(_ context.Context, appID string, since time.Time) ([]message.Count, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	type key struct {
		status  message.Status
		channel string
	}
	counts := map[key]int{}
	for _, m := range s.messages {
		if m.AppID == appID && !m.CreatedAt.Before(since) {
			counts[key{m.Status, m.Channel}]++
		}
	}
	result := make([]message.Count, 0, len(counts))
	for k, n := range counts {
		result = append(result, message.Count{Status: k.status, Channel: k.channel, N: n})
	}
	sortCounts(result)
	return result, nil
}

func sortCounts(cs []message.Count) {
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].Status != cs[j].Status {
			return cs[i].Status < cs[j].Status
		}
		return cs[i].Channel < cs[j].Channel
	})
}

func (s *Store) ListMessages(_ context.Context, appID string, opts message.ListOptions) ([]*message.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*message.Message
	for _, m := range s.messages {
		if m.AppID != appID || (opts.Channel != "" && m.Channel != opts.Channel) || (opts.Status != "" && m.Status != opts.Status) {
			continue
		}
		result = append(result, cloneMessage(m))
	}
	sort.SliceStable(result, func(i, j int) bool {
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.After(result[j].CreatedAt)
		}
		return result[i].ID.String() > result[j].ID.String()
	})
	return page(result, opts.Limit, opts.Offset), nil
}

// ─── Inbox Store ──────────────────────────────

func (s *Store) CreateNotification(_ context.Context, n *inbox.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notifications[n.ID.String()] = cloneNotification(n)
	return nil
}

func (s *Store) GetNotification(_ context.Context, notifID id.InboxID) (*inbox.Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.notifications[notifID.String()]
	if !ok {
		return nil, store.ErrNotificationNotFound
	}
	return cloneNotification(n), nil
}

func (s *Store) DeleteNotification(_ context.Context, notifID id.InboxID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.notifications[notifID.String()]; !ok {
		return store.ErrNotificationNotFound
	}
	delete(s.notifications, notifID.String())
	return nil
}

func (s *Store) MarkRead(_ context.Context, notifID id.InboxID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.notifications[notifID.String()]
	if !ok {
		return store.ErrNotificationNotFound
	}
	n.Read = true
	now := time.Now().UTC()
	n.ReadAt = &now
	return nil
}

func (s *Store) MarkAllRead(_ context.Context, appID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for _, n := range s.notifications {
		if n.AppID == appID && n.UserID == userID && !n.Read {
			n.Read = true
			n.ReadAt = &now
		}
	}
	return nil
}

func (s *Store) UnreadCount(_ context.Context, appID, userID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, n := range s.notifications {
		if n.AppID == appID && n.UserID == userID && !n.Read {
			count++
		}
	}
	return count, nil
}

func (s *Store) ListNotifications(_ context.Context, appID, userID string, limit, offset int) ([]*inbox.Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*inbox.Notification
	for _, n := range s.notifications {
		if n.AppID == appID && n.UserID == userID {
			result = append(result, cloneNotification(n))
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.After(result[j].CreatedAt)
		}
		return result[i].ID.String() > result[j].ID.String()
	})
	return page(result, limit, offset), nil
}

// ─── Preference Store ──────────────────────────────

func (s *Store) GetPreference(_ context.Context, appID, userID string) (*preference.Preference, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.preferences[appID+":"+userID]
	if !ok {
		return nil, store.ErrPreferenceNotFound
	}
	return clonePreference(p), nil
}

func (s *Store) SetPreference(_ context.Context, p *preference.Preference) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preferences[p.AppID+":"+p.UserID] = clonePreference(p)
	return nil
}

func (s *Store) DeletePreference(_ context.Context, appID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.preferences, appID+":"+userID)
	return nil
}

// ─── ScopedConfig Store ──────────────────────────────

func (s *Store) GetScopedConfig(_ context.Context, appID string, scopeType scope.ScopeType, scopeID string) (*scope.Config, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := appID + ":" + string(scopeType) + ":" + scopeID
	cfg, ok := s.scopedConfigs[key]
	if !ok {
		return nil, store.ErrScopedConfigNotFound
	}
	return cloneScopedConfig(cfg), nil
}

func (s *Store) SetScopedConfig(_ context.Context, cfg *scope.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := cfg.AppID + ":" + string(cfg.Scope) + ":" + cfg.ScopeID
	s.scopedConfigs[key] = cloneScopedConfig(cfg)
	return nil
}

func (s *Store) DeleteScopedConfig(_ context.Context, configID id.ScopedConfigID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.scopedConfigs {
		if v.ID.String() == configID.String() {
			delete(s.scopedConfigs, k)
			return nil
		}
	}
	return store.ErrScopedConfigNotFound
}

func (s *Store) ListScopedConfigs(_ context.Context, appID string) ([]*scope.Config, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*scope.Config
	for _, cfg := range s.scopedConfigs {
		if cfg.AppID == appID {
			result = append(result, cloneScopedConfig(cfg))
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Scope != result[j].Scope {
			return result[i].Scope < result[j].Scope
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}
