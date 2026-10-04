package extension

import (
	"github.com/xraph/herald"
)

// Config holds configuration for the Herald Forge extension.
// Fields can be set programmatically via ExtOption functions or loaded from
// YAML configuration files (under "extensions.herald" or "herald" keys).
type Config struct {
	// Config embeds the core herald configuration.
	herald.Config `json:",inline" yaml:",inline" mapstructure:",squash"`

	// BasePath is the URL prefix for all herald routes (default: "/herald").
	BasePath string `json:"base_path" yaml:"base_path" mapstructure:"base_path"`

	// DisableRoutes disables automatic route registration with the Forge router.
	DisableRoutes bool `json:"disable_routes" yaml:"disable_routes" mapstructure:"disable_routes"`

	// DisableMigrate disables automatic database migration on Register.
	DisableMigrate bool `json:"disable_migrate" yaml:"disable_migrate" mapstructure:"disable_migrate"`

	// GroveDatabase is the name of a grove.DB registered in the DI container.
	// When set, the extension resolves this named database and auto-constructs
	// the appropriate store based on the driver type (pg/sqlite/mongo).
	// When empty and WithGroveDatabase was called, the default (unnamed) DB is used.
	GroveDatabase string `json:"grove_database" mapstructure:"grove_database" yaml:"grove_database"`

	// Providers declares notification providers seeded into the store on
	// startup (seed-if-absent). Credentials may use ${ENV} interpolation.
	Providers []ProviderConfig `json:"providers" yaml:"providers" mapstructure:"providers"`

	// CredentialsKey encrypts provider credentials at rest: 32 bytes,
	// standard base64. Without it credentials are stored in plaintext, and
	// the dashboard says so. Supports ${ENV} interpolation.
	CredentialsKey string `json:"credentials_key" yaml:"credentials_key" mapstructure:"credentials_key"`

	// CredentialsKeyID is stored beside every value the key encrypts
	// (default "k1"). Change it when you rotate to a new key.
	CredentialsKeyID string `json:"credentials_key_id" yaml:"credentials_key_id" mapstructure:"credentials_key_id"`

	// PreviousCredentialsKeys only decrypt, so values written under an older
	// key keep working after a rotation.
	PreviousCredentialsKeys []CredentialKeyConfig `json:"previous_credentials_keys" yaml:"previous_credentials_keys" mapstructure:"previous_credentials_keys"`

	// DashboardAppID is the app the dashboard shows to a session that carries
	// no app_id claim. Empty means the "" app, where standalone installs keep
	// their data. A session whose claim is present but unusable is refused,
	// never shown this app.
	DashboardAppID string `json:"dashboard_app_id" yaml:"dashboard_app_id" mapstructure:"dashboard_app_id"`

	// RequireConfig requires config to be present in YAML files.
	// If true and no config is found, Register returns an error.
	RequireConfig bool `json:"-" yaml:"-"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Config:   herald.DefaultConfig(),
		BasePath: "/herald",
	}
}

// ToHeraldOptions converts the embedded Config into herald.Option values.
func (c Config) ToHeraldOptions() []herald.Option {
	var opts []herald.Option

	if c.DefaultLocale != "" {
		opts = append(opts, herald.WithDefaultLocale(c.DefaultLocale))
	}
	if c.MaxBatchSize > 0 {
		opts = append(opts, herald.WithMaxBatchSize(c.MaxBatchSize))
	}

	return opts
}
