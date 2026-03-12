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
)

// Message represents a sent or queued notification in the delivery log.
type Message struct {
	ID          id.MessageID      `json:"id"`
	AppID       string            `json:"app_id"`
	EnvID       string            `json:"env_id,omitempty"`
	TemplateID  string            `json:"template_id,omitempty"`
	ProviderID  string            `json:"provider_id,omitempty"`
	Channel     string            `json:"channel"`
	Recipient   string            `json:"recipient"`
	Subject     string            `json:"subject,omitempty"`
	Body        string            `json:"body,omitempty"`
	Status      Status            `json:"status"`
	Error       string            `json:"error,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Async       bool              `json:"async"`
	Attempts    int               `json:"attempts"`
	SentAt      *time.Time        `json:"sent_at,omitempty"`
	DeliveredAt *time.Time        `json:"delivered_at,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// ListOptions defines filtering options for listing messages.
type ListOptions struct {
	Channel string
	Status  Status
	Limit   int
	Offset  int
}
