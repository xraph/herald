package sqlite

import (
	"context"

	"github.com/xraph/grove/migrate"
)

// Migrations is the grove migration group for the Herald store (SQLite).
var Migrations = migrate.NewGroup("herald")

func init() {
	Migrations.MustRegister(
		&migrate.Migration{
			Name:    "create_herald_providers",
			Version: "20240201000001",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				_, err := exec.Exec(ctx, `
CREATE TABLE IF NOT EXISTS herald_providers (
    id          TEXT PRIMARY KEY,
    app_id      TEXT NOT NULL,
    name        TEXT NOT NULL,
    channel     TEXT NOT NULL,
    driver      TEXT NOT NULL,
    credentials TEXT NOT NULL DEFAULT '{}',
    settings    TEXT NOT NULL DEFAULT '{}',
    priority    INTEGER NOT NULL DEFAULT 0,
    enabled     INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_herald_providers_app_channel ON herald_providers (app_id, channel);
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
    variables  TEXT,
    is_system  INTEGER NOT NULL DEFAULT 0,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(app_id, slug, channel)
);
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
    template_id TEXT NOT NULL,
    locale      TEXT NOT NULL DEFAULT '',
    subject     TEXT,
    html        TEXT,
    text        TEXT,
    title       TEXT,
    active      INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(template_id, locale)
);
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
    metadata     TEXT NOT NULL DEFAULT '{}',
    async        INTEGER NOT NULL DEFAULT 0,
    attempts     INTEGER NOT NULL DEFAULT 0,
    sent_at      TEXT,
    delivered_at TEXT,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
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
    read       INTEGER NOT NULL DEFAULT 0,
    read_at    TEXT,
    metadata   TEXT NOT NULL DEFAULT '{}',
    expires_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_herald_inbox_user ON herald_inbox (user_id, read, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_herald_inbox_app ON herald_inbox (app_id, user_id);
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
    overrides  TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
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
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT NOT NULL DEFAULT (datetime('now')),
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
				if _, err := exec.Exec(ctx, `ALTER TABLE herald_scoped_configs ADD COLUMN webhook_provider_id TEXT NOT NULL DEFAULT ''`); err != nil {
					return err
				}
				_, err := exec.Exec(ctx, `ALTER TABLE herald_scoped_configs ADD COLUMN chat_provider_id TEXT NOT NULL DEFAULT ''`)
				return err
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				// SQLite does not support DROP COLUMN before 3.35.0; best-effort.
				if _, err := exec.Exec(ctx, `ALTER TABLE herald_scoped_configs DROP COLUMN webhook_provider_id`); err != nil {
					return err
				}
				_, err := exec.Exec(ctx, `ALTER TABLE herald_scoped_configs DROP COLUMN chat_provider_id`)
				return err
			},
		},
	)
}
