package postgres

import (
	"context"

	"github.com/xraph/grove/migrate"
)

// Migrations is the grove migration group for the Herald store.
// It can be registered with the grove extension for orchestrated migration
// management (locking, version tracking, rollback support).
var Migrations = migrate.NewGroup("herald")

func init() {
	Migrations.MustRegister(
		&migrate.Migration{
			Name:    "create_herald_providers",
			Version: "20240201000001",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `
CREATE TABLE IF NOT EXISTS herald_providers (
    id           TEXT PRIMARY KEY,
    app_id       TEXT NOT NULL,
    name         TEXT NOT NULL,
    channel      TEXT NOT NULL,
    driver       TEXT NOT NULL,
    credentials  JSONB NOT NULL DEFAULT '{}',
    settings     JSONB NOT NULL DEFAULT '{}',
    priority     INTEGER NOT NULL DEFAULT 0,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_herald_providers_app_channel ON herald_providers (app_id, channel);
CREATE INDEX IF NOT EXISTS idx_herald_providers_app ON herald_providers (app_id);
`)
				return err
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `DROP TABLE IF EXISTS herald_providers`)
				return err
			},
		},
		&migrate.Migration{
			Name:    "create_herald_templates",
			Version: "20240201000002",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `
CREATE TABLE IF NOT EXISTS herald_templates (
    id         TEXT PRIMARY KEY,
    app_id     TEXT NOT NULL,
    slug       TEXT NOT NULL,
    name       TEXT NOT NULL,
    channel    TEXT NOT NULL,
    category   TEXT NOT NULL DEFAULT 'transactional',
    variables  JSONB,
    is_system  BOOLEAN NOT NULL DEFAULT FALSE,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(app_id, slug, channel)
);

CREATE INDEX IF NOT EXISTS idx_herald_templates_app ON herald_templates (app_id);
CREATE INDEX IF NOT EXISTS idx_herald_templates_app_channel ON herald_templates (app_id, channel);
`)
				return err
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `DROP TABLE IF EXISTS herald_templates`)
				return err
			},
		},
		&migrate.Migration{
			Name:    "create_herald_template_versions",
			Version: "20240201000003",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `
CREATE TABLE IF NOT EXISTS herald_template_versions (
    id          TEXT PRIMARY KEY,
    template_id TEXT NOT NULL REFERENCES herald_templates(id) ON DELETE CASCADE,
    locale      TEXT NOT NULL DEFAULT '',
    subject     TEXT,
    html        TEXT,
    text        TEXT,
    title       TEXT,
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(template_id, locale)
);

CREATE INDEX IF NOT EXISTS idx_herald_template_versions_template ON herald_template_versions (template_id);
`)
				return err
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `DROP TABLE IF EXISTS herald_template_versions`)
				return err
			},
		},
		&migrate.Migration{
			Name:    "create_herald_messages",
			Version: "20240201000004",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `
CREATE TABLE IF NOT EXISTS herald_messages (
    id           TEXT PRIMARY KEY,
    app_id       TEXT NOT NULL,
    env_id       TEXT NOT NULL DEFAULT '',
    template_id  TEXT NOT NULL DEFAULT '',
    provider_id  TEXT NOT NULL DEFAULT '',
    channel      TEXT NOT NULL,
    recipient    TEXT NOT NULL,
    subject      TEXT NOT NULL DEFAULT '',
    body         TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'queued',
    error        TEXT NOT NULL DEFAULT '',
    metadata     JSONB NOT NULL DEFAULT '{}',
    async        BOOLEAN NOT NULL DEFAULT FALSE,
    attempts     INTEGER NOT NULL DEFAULT 0,
    sent_at      TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_herald_messages_app_status ON herald_messages (app_id, status);
CREATE INDEX IF NOT EXISTS idx_herald_messages_created ON herald_messages (created_at DESC);
`)
				return err
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `DROP TABLE IF EXISTS herald_messages`)
				return err
			},
		},
		&migrate.Migration{
			Name:    "create_herald_inbox",
			Version: "20240201000005",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `
CREATE TABLE IF NOT EXISTS herald_inbox (
    id         TEXT PRIMARY KEY,
    app_id     TEXT NOT NULL,
    env_id     TEXT NOT NULL DEFAULT '',
    user_id    TEXT NOT NULL,
    type       TEXT NOT NULL,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    action_url TEXT NOT NULL DEFAULT '',
    image_url  TEXT NOT NULL DEFAULT '',
    read       BOOLEAN NOT NULL DEFAULT FALSE,
    read_at    TIMESTAMPTZ,
    metadata   JSONB NOT NULL DEFAULT '{}',
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_herald_inbox_user ON herald_inbox (user_id, read, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_herald_inbox_app_user ON herald_inbox (app_id, user_id);
`)
				return err
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `DROP TABLE IF EXISTS herald_inbox`)
				return err
			},
		},
		&migrate.Migration{
			Name:    "create_herald_preferences",
			Version: "20240201000006",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `
CREATE TABLE IF NOT EXISTS herald_preferences (
    id         TEXT PRIMARY KEY,
    app_id     TEXT NOT NULL,
    user_id    TEXT NOT NULL,
    overrides  JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(app_id, user_id)
);
`)
				return err
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `DROP TABLE IF EXISTS herald_preferences`)
				return err
			},
		},
		&migrate.Migration{
			Name:    "create_herald_scoped_configs",
			Version: "20240201000007",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `
CREATE TABLE IF NOT EXISTS herald_scoped_configs (
    id                TEXT PRIMARY KEY,
    app_id            TEXT NOT NULL,
    scope             TEXT NOT NULL,
    scope_id          TEXT NOT NULL,
    email_provider_id TEXT NOT NULL DEFAULT '',
    sms_provider_id   TEXT NOT NULL DEFAULT '',
    push_provider_id  TEXT NOT NULL DEFAULT '',
    from_email        TEXT NOT NULL DEFAULT '',
    from_name         TEXT NOT NULL DEFAULT '',
    from_phone        TEXT NOT NULL DEFAULT '',
    default_locale    TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(app_id, scope, scope_id)
);
`)
				return err
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `DROP TABLE IF EXISTS herald_scoped_configs`)
				return err
			},
		},
		&migrate.Migration{
			Name:    "add_webhook_chat_provider_ids",
			Version: "20240201000008",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `
ALTER TABLE herald_scoped_configs ADD COLUMN IF NOT EXISTS webhook_provider_id TEXT NOT NULL DEFAULT '';
ALTER TABLE herald_scoped_configs ADD COLUMN IF NOT EXISTS chat_provider_id TEXT NOT NULL DEFAULT '';
`)
				return err
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `
ALTER TABLE herald_scoped_configs DROP COLUMN IF EXISTS webhook_provider_id;
ALTER TABLE herald_scoped_configs DROP COLUMN IF EXISTS chat_provider_id;
`)
				return err
			},
		},
	)
}
