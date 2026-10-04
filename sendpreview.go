package herald

import (
	"context"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/provider"
)

// SendPreview is what a send would use, without sending: the provider, why
// it was picked, and the sender it would go out as.
type SendPreview struct {
	Provider *provider.Provider
	Via      string
	From     string
	FromName string
}

// PreviewSend resolves the provider and sender for req exactly as Send would,
// through the same code, and sends nothing. It returns ErrNoProviderConfigured
// (wrapped) when nothing handles the channel, and Send's own errors for a
// chosen provider that's missing, in another app or on another channel, and
// ErrDriverNotFound when the provider's driver isn't registered.
func (h *Herald) PreviewSend(ctx context.Context, req *SendRequest) (*SendPreview, error) {
	res, err := h.resolveForSend(ctx, req)
	if err != nil {
		return nil, err
	}
	if _, err := h.driverFor(res.Provider); err != nil {
		return nil, err
	}
	out := &driver.OutboundMessage{}
	applyFrom(out, res, req.Channel, res.Provider)
	return &SendPreview{Provider: res.Provider, Via: res.Via, From: out.From, FromName: out.FromName}, nil
}
