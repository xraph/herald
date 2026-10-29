package herald

import (
	"context"
	"fmt"
	"time"

	"github.com/xraph/herald/device"
)

const pushChannel = "push"

// publicKeyLen is the size of an X25519 public key.
const publicKeyLen = 32

// SaveDevice registers a device, or refreshes it when the same app, user and
// public key (or address, for a device without a key) is already registered.
// It sets Channel to "push" when empty and stamps UpdatedAt and LastSeenAt.
// ID and CreatedAt are written back into d.
func (h *Herald) SaveDevice(ctx context.Context, d *device.Device) error {
	if h.devices == nil {
		return ErrNoDeviceStore
	}
	if err := validateDevice(d); err != nil {
		return err
	}

	if d.Channel == "" {
		d.Channel = pushChannel
	}
	now := h.now().UTC()
	d.UpdatedAt = now
	d.LastSeenAt = now

	return h.devices.SaveDevice(ctx, d)
}

// GetDevice returns one device of an app, or ErrDeviceNotFound.
func (h *Herald) GetDevice(ctx context.Context, appID, id string) (*device.Device, error) {
	if h.devices == nil {
		return nil, ErrNoDeviceStore
	}

	return h.devices.GetDevice(ctx, appID, id)
}

// ListDevices returns a user's devices, oldest first.
func (h *Herald) ListDevices(ctx context.Context, appID, userID string) ([]*device.Device, error) {
	if h.devices == nil {
		return nil, ErrNoDeviceStore
	}

	return h.devices.ListDevices(ctx, appID, userID)
}

// DeleteDevice removes one device, or returns ErrDeviceNotFound.
func (h *Herald) DeleteDevice(ctx context.Context, appID, id string) error {
	if h.devices == nil {
		return ErrNoDeviceStore
	}

	return h.devices.DeleteDevice(ctx, appID, id)
}

// DeleteUserDevices removes every device of a user. A user with none is not an error.
func (h *Herald) DeleteUserDevices(ctx context.Context, appID, userID string) error {
	if h.devices == nil {
		return ErrNoDeviceStore
	}

	return h.devices.DeleteUserDevices(ctx, appID, userID)
}

func validateDevice(d *device.Device) error {
	switch {
	case d.AppID == "":
		return invalidDevice("app id is required")
	case d.UserID == "":
		return invalidDevice("user id is required")
	case d.Provider == "":
		return invalidDevice("provider is required")
	case d.Address == "":
		return invalidDevice("address is required")
	case d.Channel != "" && d.Channel != pushChannel:
		return invalidDevice("channel must be %q", pushChannel)
	case d.PublicKey != nil && len(d.PublicKey) != publicKeyLen:
		return invalidDevice("public key must be %d bytes", publicKeyLen)
	}
	if d.TimeZone != "" {
		if _, err := time.LoadLocation(d.TimeZone); err != nil {
			return invalidDevice("unknown time zone")
		}
	}
	if d.QuietHours != nil {
		if err := d.QuietHours.Validate(); err != nil {
			return err
		}
	}

	return nil
}

func invalidDevice(format string, args ...any) error {
	return fmt.Errorf("%w: %s", device.ErrInvalidDevice, fmt.Sprintf(format, args...))
}
