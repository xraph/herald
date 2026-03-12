// Package sqlite provides a SQLite Store implementation using Grove ORM.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/xraph/grove"
	"github.com/xraph/grove/drivers/sqlitedriver"
	"github.com/xraph/grove/migrate"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/inbox"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/preference"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/template"
)

// compile-time interface check
var _ store.Store = (*Store)(nil)

// Store implements store.Store using SQLite via Grove ORM.
type Store struct {
	db  *grove.DB
	sdb *sqlitedriver.SqliteDB
}

// New creates a new SQLite store backed by Grove ORM.
func New(db *grove.DB) *Store {
	return &Store{
		db:  db,
		sdb: sqlitedriver.Unwrap(db),
	}
}

// DB returns the underlying grove database for direct access.
func (s *Store) DB() *grove.DB { return s.db }

// Migrate creates the required tables and indexes using the grove orchestrator.
func (s *Store) Migrate(ctx context.Context) error {
	executor, err := migrate.NewExecutorFor(s.sdb)
	if err != nil {
		return fmt.Errorf("herald/sqlite: create migration executor: %w", err)
	}
	orch := migrate.NewOrchestrator(executor, Migrations)
	if _, err := orch.Migrate(ctx); err != nil {
		return fmt.Errorf("herald/sqlite: migration failed: %w", err)
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

// ==================== Provider Store ====================

func (s *Store) CreateProvider(ctx context.Context, p *provider.Provider) error {
	m := toProviderModel(p)
	_, err := s.sdb.NewInsert(m).Exec(ctx)
	return err
}

func (s *Store) GetProvider(ctx context.Context, providerID id.ProviderID) (*provider.Provider, error) {
	m := new(providerModel)
	err := s.sdb.NewSelect(m).
		Where("id = ?", providerID.String()).
		Scan(ctx)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("herald: provider not found")
		}
		return nil, err
	}
	return fromProviderModel(m)
}

func (s *Store) UpdateProvider(ctx context.Context, p *provider.Provider) error {
	m := toProviderModel(p)
	m.UpdatedAt = now()
	res, err := s.sdb.NewUpdate(m).WherePK().Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("herald: provider not found")
	}
	return nil
}

func (s *Store) DeleteProvider(ctx context.Context, providerID id.ProviderID) error {
	res, err := s.sdb.NewDelete((*providerModel)(nil)).
		Where("id = ?", providerID.String()).
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("herald: provider not found")
	}
	return nil
}

func (s *Store) ListProviders(ctx context.Context, appID string, channel string) ([]*provider.Provider, error) {
	var models []providerModel
	q := s.sdb.NewSelect(&models).
		Where("app_id = ?", appID)
	if channel != "" {
		q = q.Where("channel = ?", channel)
	}
	q = q.OrderExpr("priority ASC, created_at ASC")

	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return mapProviders(models)
}

func (s *Store) ListAllProviders(ctx context.Context, appID string) ([]*provider.Provider, error) {
	var models []providerModel
	err := s.sdb.NewSelect(&models).
		Where("app_id = ?", appID).
		OrderExpr("priority ASC, created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
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
	_, err := s.sdb.NewInsert(m).Exec(ctx)
	return err
}

func (s *Store) GetTemplate(ctx context.Context, templateID id.TemplateID) (*template.Template, error) {
	m := new(templateModel)
	err := s.sdb.NewSelect(m).
		Where("id = ?", templateID.String()).
		Scan(ctx)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("herald: template not found")
		}
		return nil, err
	}
	t, err := fromTemplateModel(m)
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
	m := new(templateModel)
	err := s.sdb.NewSelect(m).
		Where("app_id = ?", appID).
		Where("slug = ?", slug).
		Where("channel = ?", channel).
		Scan(ctx)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("herald: template not found")
		}
		return nil, err
	}
	t, err := fromTemplateModel(m)
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
	res, err := s.sdb.NewUpdate(m).WherePK().Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("herald: template not found")
	}
	return nil
}

func (s *Store) DeleteTemplate(ctx context.Context, templateID id.TemplateID) error {
	// Delete versions first (SQLite FK support is optional)
	//nolint:errcheck // best-effort cascade delete
	_, _ = s.sdb.NewDelete((*templateVersionModel)(nil)).
		Where("template_id = ?", templateID.String()).
		Exec(ctx)

	res, err := s.sdb.NewDelete((*templateModel)(nil)).
		Where("id = ?", templateID.String()).
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("herald: template not found")
	}
	return nil
}

func (s *Store) ListTemplates(ctx context.Context, appID string) ([]*template.Template, error) {
	var models []templateModel
	err := s.sdb.NewSelect(&models).
		Where("app_id = ?", appID).
		OrderExpr("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return mapTemplates(models)
}

func (s *Store) ListTemplatesByChannel(ctx context.Context, appID string, channel string) ([]*template.Template, error) {
	var models []templateModel
	err := s.sdb.NewSelect(&models).
		Where("app_id = ?", appID).
		Where("channel = ?", channel).
		OrderExpr("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
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
	_, err := s.sdb.NewInsert(m).Exec(ctx)
	return err
}

func (s *Store) GetVersion(ctx context.Context, versionID id.TemplateVersionID) (*template.Version, error) {
	m := new(templateVersionModel)
	err := s.sdb.NewSelect(m).
		Where("id = ?", versionID.String()).
		Scan(ctx)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("herald: template version not found")
		}
		return nil, err
	}
	return fromVersionModel(m)
}

func (s *Store) UpdateVersion(ctx context.Context, v *template.Version) error {
	m := toVersionModel(v)
	m.UpdatedAt = now()
	res, err := s.sdb.NewUpdate(m).WherePK().Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("herald: template version not found")
	}
	return nil
}

func (s *Store) DeleteVersion(ctx context.Context, versionID id.TemplateVersionID) error {
	res, err := s.sdb.NewDelete((*templateVersionModel)(nil)).
		Where("id = ?", versionID.String()).
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("herald: template version not found")
	}
	return nil
}

func (s *Store) ListVersions(ctx context.Context, templateID id.TemplateID) ([]*template.Version, error) {
	var models []templateVersionModel
	err := s.sdb.NewSelect(&models).
		Where("template_id = ?", templateID.String()).
		OrderExpr("locale ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
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
	_, err := s.sdb.NewInsert(model).Exec(ctx)
	return err
}

func (s *Store) GetMessage(ctx context.Context, messageID id.MessageID) (*message.Message, error) {
	m := new(messageModel)
	err := s.sdb.NewSelect(m).
		Where("id = ?", messageID.String()).
		Scan(ctx)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("herald: message not found")
		}
		return nil, err
	}
	return fromMessageModel(m)
}

func (s *Store) UpdateMessageStatus(ctx context.Context, messageID id.MessageID, status message.Status, errMsg string) error {
	res, err := s.sdb.NewUpdate((*messageModel)(nil)).
		Set("status = ?", string(status)).
		Set("error = ?", errMsg).
		Where("id = ?", messageID.String()).
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("herald: message not found")
	}
	return nil
}

func (s *Store) ListMessages(ctx context.Context, appID string, opts message.ListOptions) ([]*message.Message, error) {
	var models []messageModel
	q := s.sdb.NewSelect(&models).Where("app_id = ?", appID)

	if opts.Channel != "" {
		q = q.Where("channel = ?", opts.Channel)
	}
	if opts.Status != "" {
		q = q.Where("status = ?", string(opts.Status))
	}
	if opts.Limit > 0 {
		q = q.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		q = q.Offset(opts.Offset)
	}
	q = q.OrderExpr("created_at DESC")

	if err := q.Scan(ctx); err != nil {
		return nil, err
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
	_, err := s.sdb.NewInsert(m).Exec(ctx)
	return err
}

func (s *Store) GetNotification(ctx context.Context, notifID id.InboxID) (*inbox.Notification, error) {
	m := new(notificationModel)
	err := s.sdb.NewSelect(m).
		Where("id = ?", notifID.String()).
		Scan(ctx)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("herald: notification not found")
		}
		return nil, err
	}
	return fromNotificationModel(m)
}

func (s *Store) DeleteNotification(ctx context.Context, notifID id.InboxID) error {
	res, err := s.sdb.NewDelete((*notificationModel)(nil)).
		Where("id = ?", notifID.String()).
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("herald: notification not found")
	}
	return nil
}

func (s *Store) MarkRead(ctx context.Context, notifID id.InboxID) error {
	t := now()
	res, err := s.sdb.NewUpdate((*notificationModel)(nil)).
		Set("read = ?", true).
		Set("read_at = ?", t).
		Where("id = ?", notifID.String()).
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("herald: notification not found")
	}
	return nil
}

func (s *Store) MarkAllRead(ctx context.Context, appID string, userID string) error {
	t := now()
	_, err := s.sdb.NewUpdate((*notificationModel)(nil)).
		Set("read = ?", true).
		Set("read_at = ?", t).
		Where("app_id = ?", appID).
		Where("user_id = ?", userID).
		Where("read = ?", false).
		Exec(ctx)
	return err
}

func (s *Store) UnreadCount(ctx context.Context, appID string, userID string) (int, error) {
	count, err := s.sdb.NewSelect((*notificationModel)(nil)).
		Where("app_id = ?", appID).
		Where("user_id = ?", userID).
		Where("read = ?", false).
		Count(ctx)
	return int(count), err
}

func (s *Store) ListNotifications(ctx context.Context, appID string, userID string, limit, offset int) ([]*inbox.Notification, error) {
	var models []notificationModel
	q := s.sdb.NewSelect(&models).
		Where("app_id = ?", appID).
		Where("user_id = ?", userID).
		OrderExpr("created_at DESC")

	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}

	if err := q.Scan(ctx); err != nil {
		return nil, err
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
	m := new(preferenceModel)
	err := s.sdb.NewSelect(m).
		Where("app_id = ?", appID).
		Where("user_id = ?", userID).
		Scan(ctx)
	if err != nil {
		if isNoRows(err) {
			return nil, nil // no preference = use defaults
		}
		return nil, err
	}
	return fromPreferenceModel(m)
}

func (s *Store) SetPreference(ctx context.Context, p *preference.Preference) error {
	m := toPreferenceModel(p)
	_, err := s.sdb.NewInsert(m).
		OnConflict("(app_id, user_id) DO UPDATE").
		Set("overrides = EXCLUDED.overrides").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (s *Store) DeletePreference(ctx context.Context, appID string, userID string) error {
	_, err := s.sdb.NewDelete((*preferenceModel)(nil)).
		Where("app_id = ?", appID).
		Where("user_id = ?", userID).
		Exec(ctx)
	return err
}

// ==================== ScopedConfig Store ====================

func (s *Store) GetScopedConfig(ctx context.Context, appID string, scopeType scope.ScopeType, scopeID string) (*scope.Config, error) {
	m := new(scopedConfigModel)
	err := s.sdb.NewSelect(m).
		Where("app_id = ?", appID).
		Where("scope = ?", string(scopeType)).
		Where("scope_id = ?", scopeID).
		Scan(ctx)
	if err != nil {
		if isNoRows(err) {
			return nil, nil // no config = use parent scope
		}
		return nil, err
	}
	return fromScopedConfigModel(m)
}

func (s *Store) SetScopedConfig(ctx context.Context, cfg *scope.Config) error {
	m := toScopedConfigModel(cfg)
	_, err := s.sdb.NewInsert(m).
		OnConflict("(app_id, scope, scope_id) DO UPDATE").
		Set("email_provider_id = EXCLUDED.email_provider_id").
		Set("sms_provider_id = EXCLUDED.sms_provider_id").
		Set("push_provider_id = EXCLUDED.push_provider_id").
		Set("from_email = EXCLUDED.from_email").
		Set("from_name = EXCLUDED.from_name").
		Set("from_phone = EXCLUDED.from_phone").
		Set("default_locale = EXCLUDED.default_locale").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (s *Store) DeleteScopedConfig(ctx context.Context, configID id.ScopedConfigID) error {
	res, err := s.sdb.NewDelete((*scopedConfigModel)(nil)).
		Where("id = ?", configID.String()).
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("herald: scoped config not found")
	}
	return nil
}

func (s *Store) ListScopedConfigs(ctx context.Context, appID string) ([]*scope.Config, error) {
	var models []scopedConfigModel
	err := s.sdb.NewSelect(&models).
		Where("app_id = ?", appID).
		OrderExpr("scope ASC, created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
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

// isNoRows checks for the standard sql.ErrNoRows sentinel.
func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
