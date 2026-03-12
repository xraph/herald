package herald

import (
	"log/slog"

	"github.com/xraph/herald/bridge"
	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/template"
)

// Herald is the root notification delivery engine.
type Herald struct {
	config    Config
	store     store.Store
	drivers   *driver.Registry
	renderer  *template.Renderer
	resolver  *scope.Resolver
	chronicle bridge.Chronicle
	logger    *slog.Logger
}

// Option configures a Herald instance.
type Option func(*Herald) error

// New creates a new Herald instance with the given options.
func New(opts ...Option) (*Herald, error) {
	h := &Herald{
		config:  DefaultConfig(),
		drivers: driver.NewRegistry(),
		logger:  slog.Default(),
	}

	for _, opt := range opts {
		if err := opt(h); err != nil {
			return nil, err
		}
	}

	if h.store == nil {
		return nil, ErrNoStore
	}

	h.wireServices()

	return h, nil
}

// wireServices initializes internal services after options have been applied.
func (h *Herald) wireServices() {
	h.renderer = template.NewRenderer()
	h.resolver = scope.NewResolver(h.store, h.store, h.logger)
}

// WithStore sets the persistence backend for the Herald instance.
func WithStore(s store.Store) Option {
	return func(h *Herald) error {
		h.store = s
		return nil
	}
}

// WithLogger sets the structured logger for the Herald instance.
func WithLogger(logger *slog.Logger) Option {
	return func(h *Herald) error {
		h.logger = logger
		return nil
	}
}

// WithDriver registers a notification driver with the Herald instance.
func WithDriver(d driver.Driver) Option {
	return func(h *Herald) error {
		h.drivers.Register(d)
		return nil
	}
}

// WithDefaultLocale sets the default locale for template rendering.
func WithDefaultLocale(locale string) Option {
	return func(h *Herald) error {
		h.config.DefaultLocale = locale
		return nil
	}
}

// WithChronicle sets the audit trail backend for the Herald instance.
func WithChronicle(c bridge.Chronicle) Option {
	return func(h *Herald) error {
		h.chronicle = c
		return nil
	}
}

// WithMaxBatchSize sets the maximum number of notifications per batch send.
func WithMaxBatchSize(n int) Option {
	return func(h *Herald) error {
		h.config.MaxBatchSize = n
		return nil
	}
}

// Store returns the underlying store.
func (h *Herald) Store() store.Store {
	return h.store
}

// Drivers returns the driver registry.
func (h *Herald) Drivers() *driver.Registry {
	return h.drivers
}

// Config returns the current configuration.
func (h *Herald) Config() Config {
	return h.config
}
