package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/xraph/grove/drivers/mongodriver/mongomigrate"
	"github.com/xraph/grove/migrate"
)

// Migrations is the grove migration group for the Herald mongo store.
var Migrations = migrate.NewGroup("herald")

func init() {
	Migrations.MustRegister(
		&migrate.Migration{
			Name:    "create_herald_providers",
			Version: "20240201000001",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}

				if err := mexec.CreateCollection(ctx, (*providerModel)(nil)); err != nil {
					return err
				}

				return mexec.CreateIndexes(ctx, colProviders, []mongo.IndexModel{
					{Keys: bson.D{{Key: "app_id", Value: 1}, {Key: "channel", Value: 1}}},
				})
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}
				return mexec.DropCollection(ctx, (*providerModel)(nil))
			},
		},
		&migrate.Migration{
			Name:    "create_herald_templates",
			Version: "20240201000002",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}

				if err := mexec.CreateCollection(ctx, (*templateModel)(nil)); err != nil {
					return err
				}

				return mexec.CreateIndexes(ctx, colTemplates, []mongo.IndexModel{
					{
						Keys:    bson.D{{Key: "app_id", Value: 1}, {Key: "slug", Value: 1}, {Key: "channel", Value: 1}},
						Options: options.Index().SetUnique(true),
					},
				})
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}
				return mexec.DropCollection(ctx, (*templateModel)(nil))
			},
		},
		&migrate.Migration{
			Name:    "create_herald_template_versions",
			Version: "20240201000003",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}

				if err := mexec.CreateCollection(ctx, (*templateVersionModel)(nil)); err != nil {
					return err
				}

				return mexec.CreateIndexes(ctx, colVersions, []mongo.IndexModel{
					{
						Keys:    bson.D{{Key: "template_id", Value: 1}, {Key: "locale", Value: 1}},
						Options: options.Index().SetUnique(true),
					},
				})
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}
				return mexec.DropCollection(ctx, (*templateVersionModel)(nil))
			},
		},
		&migrate.Migration{
			Name:    "create_herald_messages",
			Version: "20240201000004",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}

				if err := mexec.CreateCollection(ctx, (*messageModel)(nil)); err != nil {
					return err
				}

				return mexec.CreateIndexes(ctx, colMessages, []mongo.IndexModel{
					{Keys: bson.D{{Key: "app_id", Value: 1}, {Key: "status", Value: 1}}},
					{Keys: bson.D{{Key: "created_at", Value: -1}}},
				})
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}
				return mexec.DropCollection(ctx, (*messageModel)(nil))
			},
		},
		&migrate.Migration{
			Name:    "create_herald_inbox",
			Version: "20240201000005",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}

				if err := mexec.CreateCollection(ctx, (*notificationModel)(nil)); err != nil {
					return err
				}

				return mexec.CreateIndexes(ctx, colInbox, []mongo.IndexModel{
					{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "read", Value: 1}, {Key: "created_at", Value: -1}}},
					{Keys: bson.D{{Key: "app_id", Value: 1}, {Key: "user_id", Value: 1}}},
				})
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}
				return mexec.DropCollection(ctx, (*notificationModel)(nil))
			},
		},
		&migrate.Migration{
			Name:    "create_herald_preferences",
			Version: "20240201000006",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}

				if err := mexec.CreateCollection(ctx, (*preferenceModel)(nil)); err != nil {
					return err
				}

				return mexec.CreateIndexes(ctx, colPreferences, []mongo.IndexModel{
					{
						Keys:    bson.D{{Key: "app_id", Value: 1}, {Key: "user_id", Value: 1}},
						Options: options.Index().SetUnique(true),
					},
				})
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}
				return mexec.DropCollection(ctx, (*preferenceModel)(nil))
			},
		},
		&migrate.Migration{
			Name:    "create_herald_scoped_configs",
			Version: "20240201000007",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}

				if err := mexec.CreateCollection(ctx, (*scopedConfigModel)(nil)); err != nil {
					return err
				}

				return mexec.CreateIndexes(ctx, colScopedConfigs, []mongo.IndexModel{
					{
						Keys:    bson.D{{Key: "app_id", Value: 1}, {Key: "scope", Value: 1}, {Key: "scope_id", Value: 1}},
						Options: options.Index().SetUnique(true),
					},
				})
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}
				return mexec.DropCollection(ctx, (*scopedConfigModel)(nil))
			},
		},
		&migrate.Migration{
			Name:    "add_webhook_chat_provider_ids",
			Version: "20240201000008",
			Up: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}
				_, err := mexec.DB().Collection(colScopedConfigs).UpdateMany(ctx,
					bson.M{},
					bson.M{"$set": bson.M{
						"webhook_provider_id": "",
						"chat_provider_id":    "",
					}},
				)
				return err
			},
			Down: func(ctx context.Context, exec migrate.Executor) error {
				mexec, ok := exec.(*mongomigrate.Executor)
				if !ok {
					return fmt.Errorf("expected mongomigrate executor, got %T", exec)
				}
				_, err := mexec.DB().Collection(colScopedConfigs).UpdateMany(ctx,
					bson.M{},
					bson.M{"$unset": bson.M{
						"webhook_provider_id": "",
						"chat_provider_id":    "",
					}},
				)
				return err
			},
		},
	)
}
