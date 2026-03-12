package herald

// ChannelType represents a notification delivery channel.
type ChannelType string

// Supported notification channels.
const (
	ChannelEmail   ChannelType = "email"
	ChannelSMS     ChannelType = "sms"
	ChannelPush    ChannelType = "push"
	ChannelInApp   ChannelType = "inapp"
	ChannelWebhook ChannelType = "webhook"
	ChannelChat    ChannelType = "chat"
)

// ValidChannels returns all valid channel types.
func ValidChannels() []ChannelType {
	return []ChannelType{ChannelEmail, ChannelSMS, ChannelPush, ChannelInApp, ChannelWebhook, ChannelChat}
}

// IsValid reports whether c is a recognized channel type.
func (c ChannelType) IsValid() bool {
	switch c {
	case ChannelEmail, ChannelSMS, ChannelPush, ChannelInApp, ChannelWebhook, ChannelChat:
		return true
	default:
		return false
	}
}

// String returns the string representation.
func (c ChannelType) String() string {
	return string(c)
}
