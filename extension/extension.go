package extension

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/xraph/chronicle"
	"github.com/xraph/forge"
	"github.com/xraph/forge/extensions/dashboard/contributor"
	"github.com/xraph/grove"
	"github.com/xraph/vessel"

	"github.com/xraph/herald"
	"github.com/xraph/herald/api"
	"github.com/xraph/herald/bridge"
	"github.com/xraph/herald/bridge/chronicleadapter"
	heralddash "github.com/xraph/herald/dashboard"
	"github.com/xraph/herald/driver/email"
	"github.com/xraph/herald/driver/inapp"
	"github.com/xraph/herald/driver/push"
	"github.com/xraph/herald/driver/sms"
	"github.com/xraph/herald/store"
	mongostore "github.com/xraph/herald/store/mongo"
	pgstore "github.com/xraph/herald/store/postgres"
	sqlitestore "github.com/xraph/herald/store/sqlite"
)

// ExtensionName is the name registered with Forge.
const ExtensionName = "herald"

// ExtensionDescription is the human-readable description.
const ExtensionDescription = "Unified multi-channel notification delivery engine"

// ExtensionVersion is the semantic version.
const ExtensionVersion = "0.1.0"

// Ensure Extension implements forge.Extension at compile time.
var _ forge.Extension = (*Extension)(nil)

// Extension adapts Herald as a Forge extension.
// It implements the forge.Extension interface for full Forge lifecycle integration
// including registration, migration, route mounting, and graceful shutdown.
type Extension struct {
	*forge.BaseExtension

	config   Config
	h        *herald.Herald
	api      *api.ForgeAPI
	opts     []herald.Option
	useGrove bool
}

// New creates a Herald Forge extension with the given options.
func New(opts ...ExtOption) *Extension {
	ext := &Extension{
		BaseExtension: forge.NewBaseExtension(ExtensionName, ExtensionVersion, ExtensionDescription),
	}
	for _, opt := range opts {
		opt(ext)
	}
	return ext
}

// Herald returns the underlying Herald instance.
// This is nil until Register is called.
func (e *Extension) Herald() *herald.Herald {
	return e.h
}

// API returns the Forge API handler.
func (e *Extension) API() *api.ForgeAPI {
	return e.api
}

// Register implements [forge.Extension].
// It loads configuration, initializes Herald, runs migrations, registers routes,
// and provides the Herald instance to the DI container.
func (e *Extension) Register(fapp forge.App) error {
	if err := e.BaseExtension.Register(fapp); err != nil {
		return err
	}

	if err := e.loadConfiguration(); err != nil {
		return err
	}

	if err := e.Init(fapp); err != nil {
		return err
	}

	return vessel.Provide(fapp.Container(), func() (*herald.Herald, error) {
		return e.h, nil
	})
}

// Init initializes the extension. In a Forge environment, this is called
// during Register. For standalone use, call it manually.
func (e *Extension) Init(fapp forge.App) error {
	// Resolve grove database store if configured.
	if e.useGrove {
		groveDB, err := e.resolveGroveDB(fapp)
		if err != nil {
			return fmt.Errorf("herald: %w", err)
		}
		s, err := e.buildStoreFromGroveDB(groveDB)
		if err != nil {
			return err
		}
		e.opts = append(e.opts, herald.WithStore(s))
	} else if db, err := vessel.Inject[*grove.DB](fapp.Container()); err == nil {
		// Auto-discover default grove.DB from container (matches authsome/cortex pattern).
		s, err := e.buildStoreFromGroveDB(db)
		if err != nil {
			return err
		}
		e.opts = append(e.opts, herald.WithStore(s))
		e.Logger().Info("herald: auto-discovered grove.DB from container",
			forge.F("driver", db.Driver().Name()),
		)
	}

	// Auto-discover Chronicle emitter for audit logging.
	if emitter, err := vessel.Inject[chronicle.Emitter](fapp.Container()); err == nil {
		e.opts = append(e.opts, herald.WithChronicle(chronicleadapter.New(emitter)))
		e.Logger().Info("herald: auto-discovered chronicle emitter")
	} else {
		e.opts = append(e.opts, herald.WithChronicle(bridge.NewSlogChronicle(slog.Default())))
	}

	// Build herald options from extension config + user options.
	heraldOpts := make([]herald.Option, 0, len(e.opts)+4)
	heraldOpts = append(heraldOpts, e.opts...)
	heraldOpts = append(heraldOpts, e.config.ToHeraldOptions()...)

	// Register built-in drivers.
	heraldOpts = append(heraldOpts,
		herald.WithDriver(&email.SMTPDriver{}),
		herald.WithDriver(&email.ResendDriver{}),
		herald.WithDriver(&sms.TwilioDriver{}),
		herald.WithDriver(&push.FCMDriver{}),
		herald.WithDriver(&inapp.Driver{}),
	)

	// Herald core uses *slog.Logger; Forge provides forge.Logger.
	// The core defaults to slog.Default() which is fine for extension use.

	var err error
	e.h, err = herald.New(heraldOpts...)
	if err != nil {
		return err
	}

	// Run migrations if not disabled.
	if !e.config.DisableMigrate {
		if err := e.h.Store().Migrate(context.Background()); err != nil {
			return err
		}
	}

	// Seed default providers for built-in drivers (e.g. inapp).
	if err := e.h.SeedDefaultProviders(context.Background(), ""); err != nil {
		e.Logger().Warn("herald: failed to seed default providers", forge.Error(err))
	}

	// Set up Forge API.
	e.api = api.NewForgeAPI(e.h.Store(), e.h, fapp.Logger())
	if !e.config.DisableRoutes {
		basePath := e.config.BasePath
		if basePath == "" {
			basePath = "/herald"
		}
		e.api.RegisterRoutes(fapp.Router().Group(basePath))
	}

	return nil
}

// Start begins the Herald engine.
func (e *Extension) Start(ctx context.Context) error {
	e.MarkStarted()
	e.h.Start(ctx)
	return nil
}

// Stop gracefully shuts down the Herald engine.
func (e *Extension) Stop(ctx context.Context) error {
	e.h.Stop(ctx)
	e.MarkStopped()
	return nil
}

// Health implements [forge.Extension].
func (e *Extension) Health(ctx context.Context) error {
	if e.h == nil {
		return errors.New("herald extension not initialized")
	}
	return e.h.Store().Ping(ctx)
}

// DashboardContributor returns the Herald dashboard contributor for the Forge
// dashboard extension. This provides the admin UI for managing providers,
// templates, messages, and notification settings.
func (e *Extension) DashboardContributor() contributor.LocalContributor {
	return heralddash.New(heralddash.NewManifest(), e.h)
}

// RegisterRoutes registers all Herald API routes into a Forge router.
// Use this for Forge extension integration where the parent app owns the router.
func (e *Extension) RegisterRoutes(router forge.Router) {
	e.api.RegisterRoutes(router)
}

// BasePath returns the configured URL base path.
func (e *Extension) BasePath() string {
	if e.config.BasePath == "" {
		return "/herald"
	}
	return e.config.BasePath
}

// --- Config Loading (mirrors grove extension pattern) ---

// loadConfiguration loads config from YAML files or programmatic sources.
func (e *Extension) loadConfiguration() error {
	programmaticConfig := e.config

	// Try loading from config file.
	fileConfig, configLoaded := e.tryLoadFromConfigFile()

	if !configLoaded {
		if programmaticConfig.RequireConfig {
			return errors.New("herald: configuration is required but not found in config files; " +
				"ensure 'extensions.herald' or 'herald' key exists in your config")
		}

		// Use programmatic config merged with defaults.
		e.config = e.mergeWithDefaults(programmaticConfig)
	} else {
		// Config loaded from YAML -- merge with programmatic options.
		e.config = e.mergeConfigurations(fileConfig, programmaticConfig)
	}

	// Enable grove resolution if YAML config specifies grove settings.
	if e.config.GroveDatabase != "" {
		e.useGrove = true
	}

	e.Logger().Debug("herald: configuration loaded",
		forge.F("disable_routes", e.config.DisableRoutes),
		forge.F("disable_migrate", e.config.DisableMigrate),
		forge.F("base_path", e.config.BasePath),
		forge.F("grove_database", e.config.GroveDatabase),
	)

	return nil
}

// tryLoadFromConfigFile attempts to load config from YAML files.
func (e *Extension) tryLoadFromConfigFile() (Config, bool) {
	cm := e.App().Config()
	var cfg Config

	// Try "extensions.herald" first (namespaced pattern).
	if cm.IsSet("extensions.herald") {
		if err := cm.Bind("extensions.herald", &cfg); err == nil {
			e.Logger().Debug("herald: loaded config from file",
				forge.F("key", "extensions.herald"),
			)
			return cfg, true
		}
		e.Logger().Warn("herald: failed to bind extensions.herald config",
			forge.F("error", "bind failed"),
		)
	}

	// Try legacy "herald" key.
	if cm.IsSet("herald") {
		if err := cm.Bind("herald", &cfg); err == nil {
			e.Logger().Debug("herald: loaded config from file",
				forge.F("key", "herald"),
			)
			return cfg, true
		}
		e.Logger().Warn("herald: failed to bind herald config",
			forge.F("error", "bind failed"),
		)
	}

	return Config{}, false
}

// mergeWithDefaults fills zero-valued fields with defaults.
func (e *Extension) mergeWithDefaults(cfg Config) Config {
	defaults := DefaultConfig()
	if cfg.BasePath == "" {
		cfg.BasePath = defaults.BasePath
	}
	if cfg.DefaultLocale == "" {
		cfg.DefaultLocale = defaults.DefaultLocale
	}
	if cfg.MaxBatchSize == 0 {
		cfg.MaxBatchSize = defaults.MaxBatchSize
	}
	if cfg.TruncateBodyAt == 0 {
		cfg.TruncateBodyAt = defaults.TruncateBodyAt
	}
	return cfg
}

// mergeConfigurations merges YAML config with programmatic options.
// YAML config takes precedence for most fields; programmatic bool flags fill gaps.
func (e *Extension) mergeConfigurations(yamlConfig, programmaticConfig Config) Config {
	// Programmatic bool flags override when true.
	if programmaticConfig.DisableRoutes {
		yamlConfig.DisableRoutes = true
	}
	if programmaticConfig.DisableMigrate {
		yamlConfig.DisableMigrate = true
	}

	// String fields: YAML takes precedence.
	if yamlConfig.BasePath == "" && programmaticConfig.BasePath != "" {
		yamlConfig.BasePath = programmaticConfig.BasePath
	}
	if yamlConfig.GroveDatabase == "" && programmaticConfig.GroveDatabase != "" {
		yamlConfig.GroveDatabase = programmaticConfig.GroveDatabase
	}
	if yamlConfig.DefaultLocale == "" && programmaticConfig.DefaultLocale != "" {
		yamlConfig.DefaultLocale = programmaticConfig.DefaultLocale
	}

	// Int fields: YAML takes precedence, programmatic fills gaps.
	if yamlConfig.MaxBatchSize == 0 && programmaticConfig.MaxBatchSize != 0 {
		yamlConfig.MaxBatchSize = programmaticConfig.MaxBatchSize
	}
	if yamlConfig.TruncateBodyAt == 0 && programmaticConfig.TruncateBodyAt != 0 {
		yamlConfig.TruncateBodyAt = programmaticConfig.TruncateBodyAt
	}

	// Fill remaining zeros with defaults.
	return e.mergeWithDefaults(yamlConfig)
}

// resolveGroveDB resolves a *grove.DB from the DI container.
func (e *Extension) resolveGroveDB(fapp forge.App) (*grove.DB, error) {
	if e.config.GroveDatabase != "" {
		db, err := vessel.InjectNamed[*grove.DB](fapp.Container(), e.config.GroveDatabase)
		if err != nil {
			return nil, fmt.Errorf("grove database %q not found in container: %w", e.config.GroveDatabase, err)
		}
		return db, nil
	}
	db, err := vessel.Inject[*grove.DB](fapp.Container())
	if err != nil {
		return nil, fmt.Errorf("default grove database not found in container: %w", err)
	}
	return db, nil
}

// buildStoreFromGroveDB constructs the appropriate store backend
// based on the grove driver type (pg, sqlite, mongo).
func (e *Extension) buildStoreFromGroveDB(db *grove.DB) (store.Store, error) {
	driverName := db.Driver().Name()
	switch driverName {
	case "pg":
		return pgstore.New(db), nil
	case "sqlite":
		return sqlitestore.New(db), nil
	case "mongo":
		return mongostore.New(db), nil
	default:
		return nil, fmt.Errorf("herald: unsupported grove driver %q", driverName)
	}
}
