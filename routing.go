package herald

import (
	"context"
	"errors"
	"fmt"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/scope"
)

// CheckRouting refuses a routing rule that names a provider outside cfg's
// app or on another channel. The error reads the same whether the ID is
// malformed, missing or another app's, so it never confirms that an ID exists
// elsewhere. Every writer of scoped configs calls it before saving.
func (h *Herald) CheckRouting(ctx context.Context, cfg *scope.Config) error {
	slots := []struct{ field, channel, raw string }{
		{"email_provider_id", "email", cfg.EmailProviderID},
		{"sms_provider_id", "sms", cfg.SMSProviderID},
		{"push_provider_id", "push", cfg.PushProviderID},
		{"webhook_provider_id", "webhook", cfg.WebhookProviderID},
		{"chat_provider_id", "chat", cfg.ChatProviderID},
	}
	for _, s := range slots {
		if s.raw == "" {
			continue
		}
		unusable := fmt.Errorf("%w: %s %q is not a %s provider of this app", ErrInvalidProvider, s.field, s.raw, s.channel)
		pid, err := id.ParseProviderID(s.raw)
		if err != nil {
			return unusable
		}
		p, err := h.GetProvider(ctx, cfg.AppID, pid)
		if errors.Is(err, ErrProviderNotFound) {
			return unusable
		}
		if err != nil {
			return err
		}
		if p.Channel != s.channel {
			return unusable
		}
	}
	return nil
}
