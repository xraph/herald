// Package memory provides an in-memory Store implementation for testing.
package memory

import (
	"context"
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
	s.providers[p.ID.String()] = p
	return nil
}

func (s *Store) GetProvider(_ context.Context, providerID id.ProviderID) (*provider.Provider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.providers[providerID.String()]
	if !ok {
		return nil, errNotFound("provider")
	}
	return p, nil
}

func (s *Store) UpdateProvider(_ context.Context, p *provider.Provider) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers[p.ID.String()] = p
	return nil
}

func (s *Store) DeleteProvider(_ context.Context, providerID id.ProviderID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.providers, providerID.String())
	return nil
}

func (s *Store) ListProviders(_ context.Context, appID string, channel string) ([]*provider.Provider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*provider.Provider
	for _, p := range s.providers {
		if p.AppID == appID && (channel == "" || p.Channel == channel) {
			result = append(result, p)
		}
	}
	return result, nil
}

func (s *Store) ListAllProviders(_ context.Context, appID string) ([]*provider.Provider, error) {
	return s.ListProviders(context.Background(), appID, "")
}

// ─── Template Store ──────────────────────────────

func (s *Store) CreateTemplate(_ context.Context, t *template.Template) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.templates[t.ID.String()] = t
	return nil
}

func (s *Store) GetTemplate(_ context.Context, templateID id.TemplateID) (*template.Template, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.templates[templateID.String()]
	if !ok {
		return nil, errNotFound("template")
	}
	// Attach versions
	t.Versions = s.versionsForTemplate(templateID)
	return t, nil
}

func (s *Store) GetTemplateBySlug(_ context.Context, appID, slug, channel string) (*template.Template, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.templates {
		if t.AppID == appID && t.Slug == slug && t.Channel == channel {
			t.Versions = s.versionsForTemplate(t.ID)
			return t, nil
		}
	}
	return nil, errNotFound("template")
}

func (s *Store) UpdateTemplate(_ context.Context, t *template.Template) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.templates[t.ID.String()] = t
	return nil
}

func (s *Store) DeleteTemplate(_ context.Context, templateID id.TemplateID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.templates, templateID.String())
	// Delete associated versions
	for k, v := range s.versions {
		if v.TemplateID == templateID {
			delete(s.versions, k)
		}
	}
	return nil
}

func (s *Store) ListTemplates(_ context.Context, appID string) ([]*template.Template, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*template.Template
	for _, t := range s.templates {
		if t.AppID == appID {
			t.Versions = s.versionsForTemplate(t.ID)
			result = append(result, t)
		}
	}
	return result, nil
}

func (s *Store) ListTemplatesByChannel(_ context.Context, appID, channel string) ([]*template.Template, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*template.Template
	for _, t := range s.templates {
		if t.AppID == appID && t.Channel == channel {
			t.Versions = s.versionsForTemplate(t.ID)
			result = append(result, t)
		}
	}
	return result, nil
}

func (s *Store) versionsForTemplate(templateID id.TemplateID) []template.Version {
	var versions []template.Version
	for _, v := range s.versions {
		if v.TemplateID == templateID {
			versions = append(versions, *v)
		}
	}
	return versions
}

// ─── Version Store ──────────────────────────────

func (s *Store) CreateVersion(_ context.Context, v *template.Version) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.versions[v.ID.String()] = v
	return nil
}

func (s *Store) GetVersion(_ context.Context, versionID id.TemplateVersionID) (*template.Version, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.versions[versionID.String()]
	if !ok {
		return nil, errNotFound("template version")
	}
	return v, nil
}

func (s *Store) UpdateVersion(_ context.Context, v *template.Version) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.versions[v.ID.String()] = v
	return nil
}

func (s *Store) DeleteVersion(_ context.Context, versionID id.TemplateVersionID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.versions, versionID.String())
	return nil
}

func (s *Store) ListVersions(_ context.Context, templateID id.TemplateID) ([]*template.Version, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*template.Version
	for _, v := range s.versions {
		if v.TemplateID == templateID {
			result = append(result, v)
		}
	}
	return result, nil
}

// ─── Message Store ──────────────────────────────

func (s *Store) CreateMessage(_ context.Context, m *message.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[m.ID.String()] = m
	return nil
}

func (s *Store) GetMessage(_ context.Context, messageID id.MessageID) (*message.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.messages[messageID.String()]
	if !ok {
		return nil, errNotFound("message")
	}
	return m, nil
}

func (s *Store) UpdateMessageStatus(_ context.Context, messageID id.MessageID, status message.Status, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.messages[messageID.String()]
	if !ok {
		return errNotFound("message")
	}
	m.Status = status
	m.Error = errMsg
	if status == message.StatusSent {
		now := time.Now().UTC()
		m.SentAt = &now
	}
	return nil
}

func (s *Store) ListMessages(_ context.Context, appID string, opts message.ListOptions) ([]*message.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*message.Message
	for _, m := range s.messages {
		if m.AppID != appID {
			continue
		}
		if opts.Channel != "" && m.Channel != opts.Channel {
			continue
		}
		if opts.Status != "" && m.Status != opts.Status {
			continue
		}
		result = append(result, m)
	}
	// Apply offset/limit
	if opts.Offset > 0 && opts.Offset < len(result) {
		result = result[opts.Offset:]
	} else if opts.Offset >= len(result) {
		return nil, nil
	}
	if opts.Limit > 0 && opts.Limit < len(result) {
		result = result[:opts.Limit]
	}
	return result, nil
}

// ─── Inbox Store ──────────────────────────────

func (s *Store) CreateNotification(_ context.Context, n *inbox.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notifications[n.ID.String()] = n
	return nil
}

func (s *Store) GetNotification(_ context.Context, notifID id.InboxID) (*inbox.Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.notifications[notifID.String()]
	if !ok {
		return nil, errNotFound("notification")
	}
	return n, nil
}

func (s *Store) DeleteNotification(_ context.Context, notifID id.InboxID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.notifications, notifID.String())
	return nil
}

func (s *Store) MarkRead(_ context.Context, notifID id.InboxID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.notifications[notifID.String()]
	if !ok {
		return errNotFound("notification")
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
			result = append(result, n)
		}
	}
	if offset > 0 && offset < len(result) {
		result = result[offset:]
	} else if offset >= len(result) {
		return nil, nil
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

// ─── Preference Store ──────────────────────────────

func (s *Store) GetPreference(_ context.Context, appID, userID string) (*preference.Preference, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.preferences[appID+":"+userID]
	if !ok {
		return nil, errNotFound("preference")
	}
	return p, nil
}

func (s *Store) SetPreference(_ context.Context, p *preference.Preference) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preferences[p.AppID+":"+p.UserID] = p
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
		return nil, errNotFound("scoped config")
	}
	return cfg, nil
}

func (s *Store) SetScopedConfig(_ context.Context, cfg *scope.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := cfg.AppID + ":" + string(cfg.Scope) + ":" + cfg.ScopeID
	s.scopedConfigs[key] = cfg
	return nil
}

func (s *Store) DeleteScopedConfig(_ context.Context, configID id.ScopedConfigID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.scopedConfigs {
		if v.ID == configID {
			delete(s.scopedConfigs, k)
			return nil
		}
	}
	return nil
}

func (s *Store) ListScopedConfigs(_ context.Context, appID string) ([]*scope.Config, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*scope.Config
	for _, cfg := range s.scopedConfigs {
		if cfg.AppID == appID {
			result = append(result, cfg)
		}
	}
	return result, nil
}

// ─── Helpers ──────────────────────────────

type notFoundError string

func (e notFoundError) Error() string { return "herald: " + string(e) + " not found" }

func errNotFound(entity string) error { return notFoundError(entity) }
