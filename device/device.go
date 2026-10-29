// Package device defines the registry of devices a user receives push
// notifications on, and the store every backend implements for it.
package device

import (
	"encoding/hex"
	"errors"
	"time"
)

// ErrInvalidDevice is returned (wrapped) when a device or its quiet hours fail validation.
var ErrInvalidDevice = errors.New("herald: invalid device")

// Device is one registered push destination for a user.
type Device struct {
	ID         string            `json:"id"`
	AppID      string            `json:"app_id"`
	UserID     string            `json:"user_id"`
	Channel    string            `json:"channel"`  // "push"
	Provider   string            `json:"provider"` // driver name, e.g. "sealed"
	Address    string            `json:"address"`  // platform token, or gateway handle for "sealed"
	PublicKey  []byte            `json:"public_key,omitempty"`
	Platform   string            `json:"platform"` // "android" | "ios"
	Name       string            `json:"name,omitempty"`
	Locale     string            `json:"locale,omitempty"`
	TimeZone   string            `json:"time_zone,omitempty"` // IANA name
	QuietHours *QuietHours       `json:"quiet_hours,omitempty"`
	Settings   map[string]string `json:"settings,omitempty"` // owned by the application, opaque to Herald
	LastSeenAt time.Time         `json:"last_seen_at"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// UniqueKey is the identity a device is upserted on within an app and user:
// the public key when it has one, else the address.
func UniqueKey(d *Device) string {
	if len(d.PublicKey) > 0 {
		return "pk:" + hex.EncodeToString(d.PublicKey)
	}

	return "addr:" + d.Address
}
