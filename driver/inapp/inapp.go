// Package inapp provides the in-app notification driver.
// This is a no-op driver since in-app notifications are handled
// directly by the Herald engine (stored in the inbox table).
package inapp

import (
	"context"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/message"
)

// Driver is the in-app notification driver.
// It is a no-op since the Herald engine handles inbox storage directly.
type Driver struct{}

var _ driver.Driver = (*Driver)(nil)

func (d *Driver) Name() string    { return "inapp" }
func (d *Driver) Channel() string { return "inapp" }

func (d *Driver) Validate(_, _ map[string]string) error { return nil }

func (d *Driver) Send(_ context.Context, _ *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	return &driver.DeliveryResult{Status: message.StatusDelivered}, nil
}
