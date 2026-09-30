package message

import (
	"context"
	"time"

	"github.com/xraph/herald/id"
)

// Store defines persistence operations for the message delivery log.
type Store interface {
	CreateMessage(ctx context.Context, m *Message) error
	GetMessage(ctx context.Context, messageID id.MessageID) (*Message, error)
	// RecordDelivery writes the outcome of a send: status, error, the vendor's
	// message ID and when it was sent. Every field is written, so an empty
	// value clears what was there.
	RecordDelivery(ctx context.Context, messageID id.MessageID, d Delivery) error
	ListMessages(ctx context.Context, appID string, opts ListOptions) ([]*Message, error)
	// CountMessages groups the messages created at or after since by status
	// and channel, sorted by status then channel.
	CountMessages(ctx context.Context, appID string, since time.Time) ([]Count, error)
}
