// Package driver defines the notification driver interface and registry.
//
// Each notification provider (SMTP, Twilio, FCM, etc.) implements the Driver
// interface. The Registry manages available drivers by name.
package driver

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/xraph/herald/message"
)

// ErrDriverNotFound is returned when a driver is not registered.
var ErrDriverNotFound = errors.New("herald: driver not found")

// Driver is the interface that notification provider drivers must implement.
type Driver interface {
	// Name returns the driver identifier (e.g., "smtp", "twilio", "fcm").
	Name() string

	// Channel returns which notification channel this driver handles.
	Channel() string

	// Send delivers a message and returns a delivery result.
	Send(ctx context.Context, msg *OutboundMessage) (*DeliveryResult, error)

	// Validate checks if the provided credentials and settings are valid.
	Validate(credentials, settings map[string]string) error
}

// OutboundMessage is the normalized message format passed to drivers.
type OutboundMessage struct {
	To       string            `json:"to"`
	From     string            `json:"from,omitempty"`
	FromName string            `json:"from_name,omitempty"`
	Subject  string            `json:"subject,omitempty"`
	HTML     string            `json:"html,omitempty"`
	Text     string            `json:"text,omitempty"`
	Title    string            `json:"title,omitempty"`
	Data     map[string]string `json:"data,omitempty"`
}

// DeliveryResult contains the outcome of a delivery attempt.
type DeliveryResult struct {
	ProviderMessageID string         `json:"provider_message_id,omitempty"`
	Status            message.Status `json:"status"`
}

// Registry holds all available notification drivers indexed by name.
type Registry struct {
	mu      sync.RWMutex
	drivers map[string]Driver
}

// NewRegistry creates a new empty driver registry.
func NewRegistry() *Registry {
	return &Registry{
		drivers: make(map[string]Driver),
	}
}

// Register adds a driver to the registry.
func (r *Registry) Register(d Driver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drivers[d.Name()] = d
}

// Get returns a driver by name.
func (r *Registry) Get(name string) (Driver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, ok := r.drivers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrDriverNotFound, name)
	}

	return d, nil
}

// ListByChannel returns all drivers that handle the given channel.
func (r *Registry) ListByChannel(ch string) []Driver {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Driver
	for _, d := range r.drivers {
		if d.Channel() == ch {
			result = append(result, d)
		}
	}

	return result
}

// Names returns the names of all registered drivers.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.drivers))
	for name := range r.drivers {
		names = append(names, name)
	}

	return names
}
