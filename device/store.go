package device

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a device does not exist for the app asked about.
var ErrNotFound = errors.New("herald: device not found")

// Store persists devices.
type Store interface {
	// SaveDevice upserts on (AppID, UniqueKey, UserID). An insert assigns ID and
	// CreatedAt. A conflict overwrites every other field, keeps the stored ID and
	// CreatedAt, and writes both back into d.
	SaveDevice(ctx context.Context, d *Device) error
	// GetDevice returns ErrNotFound for a missing device or another app's device.
	GetDevice(ctx context.Context, appID, id string) (*Device, error)
	// ListDevices returns a user's devices ordered by CreatedAt, then ID.
	ListDevices(ctx context.Context, appID, userID string) ([]*Device, error)
	// DeleteDevice returns ErrNotFound when no such device exists.
	DeleteDevice(ctx context.Context, appID, id string) error
	// DeleteUserDevices removes every device of a user. None is not an error.
	DeleteUserDevices(ctx context.Context, appID, userID string) error
}
