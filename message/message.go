// Package message defines the notification delivery log entity.
package message

import (
	"time"

	"github.com/xraph/herald/id"
)

// Status represents the delivery state of a message.
type Status string

// Message status constants.
const (
	StatusQueued    Status = "queued"
	StatusSending   Status = "sending"
	StatusSent      Status = "sent"
	StatusFailed    Status = "failed"
	StatusBounced   Status = "bounced"
	StatusDelivered Status = "delivered"

	// StatusSuppressed is a send Herald chose not to make, because the
	// recipient opted out of this notification type on this channel.
	StatusSuppressed Status = "suppressed"
)

// Message represents a sent or queued notification in the delivery log.
type Message struct {
	ID                id.MessageID      `json:"id"`
	AppID             string            `json:"app_id"`
	EnvID             string            `json:"env_id,omitempty"`
	TemplateID        string            `json:"template_id,omitempty"`
	ProviderID        string            `json:"provider_id,omitempty"`
	ProviderMessageID string            `json:"provider_message_id,omitempty"`
	Channel           string            `json:"channel"`
	Recipient         string            `json:"recipient"`
	Subject           string            `json:"subject,omitempty"`
	Body              string            `json:"body,omitempty"`
	Status            Status            `json:"status"`
	Error             string            `json:"error,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	Async             bool              `json:"async"`
	Attempts          int               `json:"attempts"`
	SentAt            *time.Time        `json:"sent_at,omitempty"`
	DeliveredAt       *time.Time        `json:"delivered_at,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
}

// ListOptions defines filtering options for listing messages.
type ListOptions struct {
	Channel string
	Status  Status
	Limit   int
	Offset  int
}

// Delivery is the outcome of one send attempt, written by RecordDelivery.
type Delivery struct {
	Status            Status
	Error             string
	ProviderMessageID string
	SentAt            *time.Time
}

// Count is the number of messages with one status on one channel.
type Count struct {
	Status  Status `json:"status"`
	Channel string `json:"channel"`
	N       int    `json:"n"`
}
