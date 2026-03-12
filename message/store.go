package message

import (
	"context"

	"github.com/xraph/herald/id"
)

// Store defines persistence operations for the message delivery log.
type Store interface {
	CreateMessage(ctx context.Context, m *Message) error
	GetMessage(ctx context.Context, messageID id.MessageID) (*Message, error)
	UpdateMessageStatus(ctx context.Context, messageID id.MessageID, status Status, errMsg string) error
	ListMessages(ctx context.Context, appID string, opts ListOptions) ([]*Message, error)
}
