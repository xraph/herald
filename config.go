package herald

// Config holds the configuration for a Herald instance.
type Config struct {
	// DefaultLocale is the default locale for template rendering (default: "en").
	DefaultLocale string

	// MaxBatchSize is the maximum number of notifications per batch send.
	MaxBatchSize int

	// TruncateBodyAt is the maximum message body length stored in the delivery log.
	// Bodies longer than this are truncated. 0 means no truncation.
	TruncateBodyAt int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		DefaultLocale:  "en",
		MaxBatchSize:   100,
		TruncateBodyAt: 4096,
	}
}
