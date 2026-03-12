// Package mongo provides a MongoDB Store implementation using Grove ORM.
package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/xraph/grove"
	"github.com/xraph/grove/drivers/mongodriver"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/inbox"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/preference"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/template"
)

// Collection name constants.
const (
	colProviders     = "herald_providers"
	colTemplates     = "herald_templates"
	colVersions      = "herald_template_versions"
	colMessages      = "herald_messages"
	colInbox         = "herald_inbox"
	colPreferences   = "herald_preferences"
	colScopedConfigs = "herald_scoped_configs"
)

// Compile-time interface check.
var _ store.Store = (*Store)(nil)

// Store implements store.Store using MongoDB via Grove ORM.
type Store struct {
	db  *grove.DB
	mdb *mongodriver.MongoDB
}

// New creates a new MongoDB store backed by Grove ORM.
func New(db *grove.DB) *Store {
	return &Store{
		db:  db,
		mdb: mongodriver.Unwrap(db),
	}
}

// DB returns the underlying grove database for direct access.
func (s *Store) DB() *grove.DB { return s.db }

// Migrate creates indexes for all herald collections.
func (s *Store) Migrate(ctx context.Context) error {
	indexes := migrationIndexes()

	for col, models := range indexes {
		if len(models) == 0 {
			continue
		}

		_, err := s.mdb.Collection(col).Indexes().CreateMany(ctx, models)
		if err != nil {
			return fmt.Errorf("herald/mongo: migrate %s indexes: %w", col, err)
		}
	}

	return nil
}

// Ping checks database connectivity.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.Ping(ctx)
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// migrationIndexes returns the index definitions for all herald collections.
func migrationIndexes() map[string][]mongo.IndexModel {
	return map[string][]mongo.IndexModel{
		colProviders: {
			{Keys: bson.D{{Key: "app_id", Value: 1}, {Key: "channel", Value: 1}}},
		},
		colTemplates: {
			{
				Keys:    bson.D{{Key: "app_id", Value: 1}, {Key: "slug", Value: 1}, {Key: "channel", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
		},
		colVersions: {
			{
				Keys:    bson.D{{Key: "template_id", Value: 1}, {Key: "locale", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
		},
		colMessages: {
			{Keys: bson.D{{Key: "app_id", Value: 1}, {Key: "status", Value: 1}}},
			{Keys: bson.D{{Key: "created_at", Value: -1}}},
		},
		colInbox: {
			{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "read", Value: 1}, {Key: "created_at", Value: -1}}},
			{Keys: bson.D{{Key: "app_id", Value: 1}, {Key: "user_id", Value: 1}}},
		},
		colPreferences: {
			{
				Keys:    bson.D{{Key: "app_id", Value: 1}, {Key: "user_id", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
		},
		colScopedConfigs: {
			{
				Keys:    bson.D{{Key: "app_id", Value: 1}, {Key: "scope", Value: 1}, {Key: "scope_id", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
		},
	}
}

// ==================== Provider Store ====================

func (s *Store) CreateProvider(ctx context.Context, p *provider.Provider) error {
	m := toProviderModel(p)
	_, err := s.mdb.NewInsert(m).Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: create provider: %w", err)
	}
	return nil
}

func (s *Store) GetProvider(ctx context.Context, providerID id.ProviderID) (*provider.Provider, error) {
	var m providerModel
	err := s.mdb.NewFind(&m).
		Filter(bson.M{"_id": providerID.String()}).
		Scan(ctx)
	if err != nil {
		if isNoDocuments(err) {
			return nil, fmt.Errorf("herald: provider not found")
		}
		return nil, fmt.Errorf("herald/mongo: get provider: %w", err)
	}
	return fromProviderModel(&m)
}

func (s *Store) UpdateProvider(ctx context.Context, p *provider.Provider) error {
	m := toProviderModel(p)
	m.UpdatedAt = now()
	res, err := s.mdb.NewUpdate(m).
		Filter(bson.M{"_id": m.ID}).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: update provider: %w", err)
	}
	if res.MatchedCount() == 0 {
		return fmt.Errorf("herald: provider not found")
	}
	return nil
}

func (s *Store) DeleteProvider(ctx context.Context, providerID id.ProviderID) error {
	res, err := s.mdb.NewDelete((*providerModel)(nil)).
		Filter(bson.M{"_id": providerID.String()}).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: delete provider: %w", err)
	}
	if res.DeletedCount() == 0 {
		return fmt.Errorf("herald: provider not found")
	}
	return nil
}

func (s *Store) ListProviders(ctx context.Context, appID string, channel string) ([]*provider.Provider, error) {
	var models []providerModel
	filter := bson.M{"app_id": appID}
	if channel != "" {
		filter["channel"] = channel
	}

	err := s.mdb.NewFind(&models).
		Filter(filter).
		Sort(bson.D{{Key: "priority", Value: 1}, {Key: "created_at", Value: 1}}).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("herald/mongo: list providers: %w", err)
	}
	return mapProviders(models)
}

func (s *Store) ListAllProviders(ctx context.Context, appID string) ([]*provider.Provider, error) {
	var models []providerModel
	err := s.mdb.NewFind(&models).
		Filter(bson.M{"app_id": appID}).
		Sort(bson.D{{Key: "priority", Value: 1}, {Key: "created_at", Value: 1}}).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("herald/mongo: list all providers: %w", err)
	}
	return mapProviders(models)
}

func mapProviders(models []providerModel) ([]*provider.Provider, error) {
	result := make([]*provider.Provider, len(models))
	for i := range models {
		p, err := fromProviderModel(&models[i])
		if err != nil {
			return nil, err
		}
		result[i] = p
	}
	return result, nil
}

// ==================== Template Store ====================

func (s *Store) CreateTemplate(ctx context.Context, t *template.Template) error {
	m := toTemplateModel(t)
	_, err := s.mdb.NewInsert(m).Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: create template: %w", err)
	}
	return nil
}

func (s *Store) GetTemplate(ctx context.Context, templateID id.TemplateID) (*template.Template, error) {
	var m templateModel
	err := s.mdb.NewFind(&m).
		Filter(bson.M{"_id": templateID.String()}).
		Scan(ctx)
	if err != nil {
		if isNoDocuments(err) {
			return nil, fmt.Errorf("herald: template not found")
		}
		return nil, fmt.Errorf("herald/mongo: get template: %w", err)
	}
	t, err := fromTemplateModel(&m)
	if err != nil {
		return nil, err
	}
	// Load versions
	versions, err := s.ListVersions(ctx, templateID)
	if err != nil {
		return nil, err
	}
	t.Versions = make([]template.Version, len(versions))
	for i, v := range versions {
		t.Versions[i] = *v
	}
	return t, nil
}

func (s *Store) GetTemplateBySlug(ctx context.Context, appID string, slug string, channel string) (*template.Template, error) {
	var m templateModel
	err := s.mdb.NewFind(&m).
		Filter(bson.M{"app_id": appID, "slug": slug, "channel": channel}).
		Scan(ctx)
	if err != nil {
		if isNoDocuments(err) {
			return nil, fmt.Errorf("herald: template not found")
		}
		return nil, fmt.Errorf("herald/mongo: get template by slug: %w", err)
	}
	t, err := fromTemplateModel(&m)
	if err != nil {
		return nil, err
	}
	// Load versions
	versions, err := s.ListVersions(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	t.Versions = make([]template.Version, len(versions))
	for i, v := range versions {
		t.Versions[i] = *v
	}
	return t, nil
}

func (s *Store) UpdateTemplate(ctx context.Context, t *template.Template) error {
	m := toTemplateModel(t)
	m.UpdatedAt = now()
	res, err := s.mdb.NewUpdate(m).
		Filter(bson.M{"_id": m.ID}).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: update template: %w", err)
	}
	if res.MatchedCount() == 0 {
		return fmt.Errorf("herald: template not found")
	}
	return nil
}

func (s *Store) DeleteTemplate(ctx context.Context, templateID id.TemplateID) error {
	// Delete versions first
	//nolint:errcheck // best-effort cascade delete
	_, _ = s.mdb.NewDelete((*templateVersionModel)(nil)).
		Filter(bson.M{"template_id": templateID.String()}).
		Exec(ctx)

	res, err := s.mdb.NewDelete((*templateModel)(nil)).
		Filter(bson.M{"_id": templateID.String()}).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: delete template: %w", err)
	}
	if res.DeletedCount() == 0 {
		return fmt.Errorf("herald: template not found")
	}
	return nil
}

func (s *Store) ListTemplates(ctx context.Context, appID string) ([]*template.Template, error) {
	var models []templateModel
	err := s.mdb.NewFind(&models).
		Filter(bson.M{"app_id": appID}).
		Sort(bson.D{{Key: "created_at", Value: 1}}).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("herald/mongo: list templates: %w", err)
	}
	return mapTemplates(models)
}

func (s *Store) ListTemplatesByChannel(ctx context.Context, appID string, channel string) ([]*template.Template, error) {
	var models []templateModel
	err := s.mdb.NewFind(&models).
		Filter(bson.M{"app_id": appID, "channel": channel}).
		Sort(bson.D{{Key: "created_at", Value: 1}}).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("herald/mongo: list templates by channel: %w", err)
	}
	return mapTemplates(models)
}

func mapTemplates(models []templateModel) ([]*template.Template, error) {
	result := make([]*template.Template, len(models))
	for i := range models {
		t, err := fromTemplateModel(&models[i])
		if err != nil {
			return nil, err
		}
		result[i] = t
	}
	return result, nil
}

// ==================== Version Store ====================

func (s *Store) CreateVersion(ctx context.Context, v *template.Version) error {
	m := toVersionModel(v)
	_, err := s.mdb.NewInsert(m).Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: create version: %w", err)
	}
	return nil
}

func (s *Store) GetVersion(ctx context.Context, versionID id.TemplateVersionID) (*template.Version, error) {
	var m templateVersionModel
	err := s.mdb.NewFind(&m).
		Filter(bson.M{"_id": versionID.String()}).
		Scan(ctx)
	if err != nil {
		if isNoDocuments(err) {
			return nil, fmt.Errorf("herald: template version not found")
		}
		return nil, fmt.Errorf("herald/mongo: get version: %w", err)
	}
	return fromVersionModel(&m)
}

func (s *Store) UpdateVersion(ctx context.Context, v *template.Version) error {
	m := toVersionModel(v)
	m.UpdatedAt = now()
	res, err := s.mdb.NewUpdate(m).
		Filter(bson.M{"_id": m.ID}).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: update version: %w", err)
	}
	if res.MatchedCount() == 0 {
		return fmt.Errorf("herald: template version not found")
	}
	return nil
}

func (s *Store) DeleteVersion(ctx context.Context, versionID id.TemplateVersionID) error {
	res, err := s.mdb.NewDelete((*templateVersionModel)(nil)).
		Filter(bson.M{"_id": versionID.String()}).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: delete version: %w", err)
	}
	if res.DeletedCount() == 0 {
		return fmt.Errorf("herald: template version not found")
	}
	return nil
}

func (s *Store) ListVersions(ctx context.Context, templateID id.TemplateID) ([]*template.Version, error) {
	var models []templateVersionModel
	err := s.mdb.NewFind(&models).
		Filter(bson.M{"template_id": templateID.String()}).
		Sort(bson.D{{Key: "locale", Value: 1}}).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("herald/mongo: list versions: %w", err)
	}
	result := make([]*template.Version, len(models))
	for i := range models {
		v, err := fromVersionModel(&models[i])
		if err != nil {
			return nil, err
		}
		result[i] = v
	}
	return result, nil
}

// ==================== Message Store ====================

func (s *Store) CreateMessage(ctx context.Context, m *message.Message) error {
	model := toMessageModel(m)
	_, err := s.mdb.NewInsert(model).Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: create message: %w", err)
	}
	return nil
}

func (s *Store) GetMessage(ctx context.Context, messageID id.MessageID) (*message.Message, error) {
	var m messageModel
	err := s.mdb.NewFind(&m).
		Filter(bson.M{"_id": messageID.String()}).
		Scan(ctx)
	if err != nil {
		if isNoDocuments(err) {
			return nil, fmt.Errorf("herald: message not found")
		}
		return nil, fmt.Errorf("herald/mongo: get message: %w", err)
	}
	return fromMessageModel(&m)
}

func (s *Store) UpdateMessageStatus(ctx context.Context, messageID id.MessageID, status message.Status, errMsg string) error {
	res, err := s.mdb.NewUpdate((*messageModel)(nil)).
		Filter(bson.M{"_id": messageID.String()}).
		Set("status", string(status)).
		Set("error", errMsg).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: update message status: %w", err)
	}
	if res.MatchedCount() == 0 {
		return fmt.Errorf("herald: message not found")
	}
	return nil
}

func (s *Store) ListMessages(ctx context.Context, appID string, opts message.ListOptions) ([]*message.Message, error) {
	var models []messageModel
	filter := bson.M{"app_id": appID}
	if opts.Channel != "" {
		filter["channel"] = opts.Channel
	}
	if opts.Status != "" {
		filter["status"] = string(opts.Status)
	}

	q := s.mdb.NewFind(&models).
		Filter(filter).
		Sort(bson.D{{Key: "created_at", Value: -1}})

	if opts.Limit > 0 {
		q = q.Limit(int64(opts.Limit))
	}
	if opts.Offset > 0 {
		q = q.Skip(int64(opts.Offset))
	}

	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("herald/mongo: list messages: %w", err)
	}

	result := make([]*message.Message, len(models))
	for i := range models {
		msg, err := fromMessageModel(&models[i])
		if err != nil {
			return nil, err
		}
		result[i] = msg
	}
	return result, nil
}

// ==================== Inbox Store ====================

func (s *Store) CreateNotification(ctx context.Context, n *inbox.Notification) error {
	m := toNotificationModel(n)
	_, err := s.mdb.NewInsert(m).Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: create notification: %w", err)
	}
	return nil
}

func (s *Store) GetNotification(ctx context.Context, notifID id.InboxID) (*inbox.Notification, error) {
	var m notificationModel
	err := s.mdb.NewFind(&m).
		Filter(bson.M{"_id": notifID.String()}).
		Scan(ctx)
	if err != nil {
		if isNoDocuments(err) {
			return nil, fmt.Errorf("herald: notification not found")
		}
		return nil, fmt.Errorf("herald/mongo: get notification: %w", err)
	}
	return fromNotificationModel(&m)
}

func (s *Store) DeleteNotification(ctx context.Context, notifID id.InboxID) error {
	res, err := s.mdb.NewDelete((*notificationModel)(nil)).
		Filter(bson.M{"_id": notifID.String()}).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: delete notification: %w", err)
	}
	if res.DeletedCount() == 0 {
		return fmt.Errorf("herald: notification not found")
	}
	return nil
}

func (s *Store) MarkRead(ctx context.Context, notifID id.InboxID) error {
	t := now()
	res, err := s.mdb.NewUpdate((*notificationModel)(nil)).
		Filter(bson.M{"_id": notifID.String()}).
		Set("read", true).
		Set("read_at", t).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: mark read: %w", err)
	}
	if res.MatchedCount() == 0 {
		return fmt.Errorf("herald: notification not found")
	}
	return nil
}

func (s *Store) MarkAllRead(ctx context.Context, appID string, userID string) error {
	t := now()
	_, err := s.mdb.NewUpdate((*notificationModel)(nil)).
		Filter(bson.M{"app_id": appID, "user_id": userID, "read": false}).
		Set("read", true).
		Set("read_at", t).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: mark all read: %w", err)
	}
	return nil
}

func (s *Store) UnreadCount(ctx context.Context, appID string, userID string) (int, error) {
	count, err := s.mdb.Collection(colInbox).CountDocuments(ctx, bson.M{
		"app_id":  appID,
		"user_id": userID,
		"read":    false,
	})
	if err != nil {
		return 0, fmt.Errorf("herald/mongo: unread count: %w", err)
	}
	return int(count), nil
}

func (s *Store) ListNotifications(ctx context.Context, appID string, userID string, limit, offset int) ([]*inbox.Notification, error) {
	var models []notificationModel
	q := s.mdb.NewFind(&models).
		Filter(bson.M{"app_id": appID, "user_id": userID}).
		Sort(bson.D{{Key: "created_at", Value: -1}})

	if limit > 0 {
		q = q.Limit(int64(limit))
	}
	if offset > 0 {
		q = q.Skip(int64(offset))
	}

	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("herald/mongo: list notifications: %w", err)
	}

	result := make([]*inbox.Notification, len(models))
	for i := range models {
		n, err := fromNotificationModel(&models[i])
		if err != nil {
			return nil, err
		}
		result[i] = n
	}
	return result, nil
}

// ==================== Preference Store ====================

func (s *Store) GetPreference(ctx context.Context, appID string, userID string) (*preference.Preference, error) {
	var m preferenceModel
	err := s.mdb.NewFind(&m).
		Filter(bson.M{"app_id": appID, "user_id": userID}).
		Scan(ctx)
	if err != nil {
		if isNoDocuments(err) {
			return nil, nil // no preference = use defaults
		}
		return nil, fmt.Errorf("herald/mongo: get preference: %w", err)
	}
	return fromPreferenceModel(&m)
}

func (s *Store) SetPreference(ctx context.Context, p *preference.Preference) error {
	m := toPreferenceModel(p)
	_, err := s.mdb.NewUpdate(m).
		Filter(bson.M{"app_id": m.AppID, "user_id": m.UserID}).
		SetUpdate(bson.M{"$setOnInsert": bson.M{
			"_id":        m.ID,
			"app_id":     m.AppID,
			"user_id":    m.UserID,
			"created_at": m.CreatedAt,
		}, "$set": bson.M{
			"overrides":  m.Overrides,
			"updated_at": m.UpdatedAt,
		}}).
		Upsert().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: set preference: %w", err)
	}
	return nil
}

func (s *Store) DeletePreference(ctx context.Context, appID string, userID string) error {
	_, err := s.mdb.NewDelete((*preferenceModel)(nil)).
		Filter(bson.M{"app_id": appID, "user_id": userID}).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: delete preference: %w", err)
	}
	return nil
}

// ==================== ScopedConfig Store ====================

func (s *Store) GetScopedConfig(ctx context.Context, appID string, scopeType scope.ScopeType, scopeID string) (*scope.Config, error) {
	var m scopedConfigModel
	err := s.mdb.NewFind(&m).
		Filter(bson.M{"app_id": appID, "scope": string(scopeType), "scope_id": scopeID}).
		Scan(ctx)
	if err != nil {
		if isNoDocuments(err) {
			return nil, nil // no config = use parent scope
		}
		return nil, fmt.Errorf("herald/mongo: get scoped config: %w", err)
	}
	return fromScopedConfigModel(&m)
}

func (s *Store) SetScopedConfig(ctx context.Context, cfg *scope.Config) error {
	m := toScopedConfigModel(cfg)
	_, err := s.mdb.NewUpdate(m).
		Filter(bson.M{"app_id": m.AppID, "scope": m.Scope, "scope_id": m.ScopeID}).
		SetUpdate(bson.M{"$setOnInsert": bson.M{
			"_id":        m.ID,
			"app_id":     m.AppID,
			"scope":      m.Scope,
			"scope_id":   m.ScopeID,
			"created_at": m.CreatedAt,
		}, "$set": bson.M{
			"email_provider_id":   m.EmailProviderID,
			"sms_provider_id":     m.SMSProviderID,
			"push_provider_id":    m.PushProviderID,
			"webhook_provider_id": m.WebhookProviderID,
			"chat_provider_id":    m.ChatProviderID,
			"from_email":          m.FromEmail,
			"from_name":           m.FromName,
			"from_phone":          m.FromPhone,
			"default_locale":      m.DefaultLocale,
			"updated_at":          m.UpdatedAt,
		}}).
		Upsert().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: set scoped config: %w", err)
	}
	return nil
}

func (s *Store) DeleteScopedConfig(ctx context.Context, configID id.ScopedConfigID) error {
	res, err := s.mdb.NewDelete((*scopedConfigModel)(nil)).
		Filter(bson.M{"_id": configID.String()}).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("herald/mongo: delete scoped config: %w", err)
	}
	if res.DeletedCount() == 0 {
		return fmt.Errorf("herald: scoped config not found")
	}
	return nil
}

func (s *Store) ListScopedConfigs(ctx context.Context, appID string) ([]*scope.Config, error) {
	var models []scopedConfigModel
	err := s.mdb.NewFind(&models).
		Filter(bson.M{"app_id": appID}).
		Sort(bson.D{{Key: "scope", Value: 1}, {Key: "created_at", Value: 1}}).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("herald/mongo: list scoped configs: %w", err)
	}
	result := make([]*scope.Config, len(models))
	for i := range models {
		c, err := fromScopedConfigModel(&models[i])
		if err != nil {
			return nil, err
		}
		result[i] = c
	}
	return result, nil
}

// now returns the current UTC time.
func now() time.Time {
	return time.Now().UTC()
}

// isNoDocuments checks if an error wraps mongo.ErrNoDocuments.
func isNoDocuments(err error) bool {
	return errors.Is(err, mongo.ErrNoDocuments)
}
