package driver

import (
	"errors"
	"time"
)

// Urgency says how hard a push should try to reach the user.
type Urgency string

// Push urgency levels, from quietest to loudest.
const (
	UrgencyPassive       Urgency = "passive"
	UrgencyActive        Urgency = "active"
	UrgencyTimeSensitive Urgency = "time_sensitive"
	UrgencyCritical      Urgency = "critical"
)

// Valid reports whether u is one of the four urgency levels. The empty
// string is not valid.
func (u Urgency) Valid() bool {
	switch u {
	case UrgencyPassive, UrgencyActive, UrgencyTimeSensitive, UrgencyCritical:
		return true
	}
	return false
}

// Push kinds: show a notification, or withdraw one shown earlier.
const (
	PushKindShow     = "show"
	PushKindWithdraw = "withdraw"
)

// PushOptions are the per-message push settings a driver honours. A nil
// *PushOptions means the v1.8 behaviour.
type PushOptions struct {
	// Urgency is the delivery priority. Empty means UrgencyActive.
	Urgency Urgency
	// Kind is PushKindShow or PushKindWithdraw. Empty means PushKindShow.
	Kind string
	// CollapseID lets a later push replace an earlier one.
	CollapseID string
	// TTL is how long the provider may hold the push. Zero leaves it to the provider.
	TTL time.Duration
	// ThreadID groups notifications on the device.
	ThreadID string
	// Category names the action set the device shows with the notification.
	Category string
	// Data is extra payload. It wins over provider-level data settings.
	Data map[string]string
	// DataOnly sends no visible notification block.
	DataOnly bool
	// MutableContent lets a device extension rewrite the notification.
	MutableContent bool
	// ContentAvailable wakes the app for a background update.
	ContentAvailable bool
}

// EffectiveUrgency returns the urgency, defaulting to UrgencyActive. It is
// safe on a nil receiver.
func (o *PushOptions) EffectiveUrgency() Urgency {
	if o == nil || o.Urgency == "" {
		return UrgencyActive
	}
	return o.Urgency
}

// EffectiveKind returns the kind, defaulting to PushKindShow. It is safe on a
// nil receiver.
func (o *PushOptions) EffectiveKind() string {
	if o == nil || o.Kind == "" {
		return PushKindShow
	}
	return o.Kind
}

// ErrAddressGone means the provider says the recipient address no longer
// exists, so the caller should stop sending to it.
var ErrAddressGone = errors.New("herald: address gone")
