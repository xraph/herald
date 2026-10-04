package contract

import (
	"context"
	"errors"
	"strings"

	"github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"

	"github.com/xraph/herald"
	"github.com/xraph/herald/id"
)

func registerSend(d *dispatcher.Dispatcher, deps Deps) error {
	if err := query(d, "send.resolve", sendResolveHandler(deps)); err != nil {
		return err
	}
	return command(d, "send.test", sendTestHandler(deps))
}

type sendResolveRequest struct {
	Channel    string `json:"channel"`
	ProviderID string `json:"providerId"`
	OrgID      string `json:"orgId"`
	UserID     string `json:"userId"`
}

type senderInfo struct {
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
	Phone string `json:"phone,omitempty"`
}

type sendResolveResponse struct {
	Provider *ProviderRef `json:"provider"`
	Via      string       `json:"via"`
	From     senderInfo   `json:"from"`
}

// sendResolveHandler reports which provider a send would use and why,
// through the engine's PreviewSend, so the confirmation dialog names what
// Send will actually do. Nothing handling the channel is an answer (via
// "none"), not an error.
func sendResolveHandler(deps Deps) func(context.Context, sendResolveRequest, contract.Principal) (sendResolveResponse, error) {
	return func(ctx context.Context, in sendResolveRequest, p contract.Principal) (sendResolveResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return sendResolveResponse{}, err
		}
		channel := strings.TrimSpace(in.Channel)
		if !validChannel(channel) {
			return sendResolveResponse{}, badRequest("channel is not one Herald supports")
		}
		if in.ProviderID != "" {
			if _, perr := parseProviderID(in.ProviderID); perr != nil {
				return sendResolveResponse{}, perr
			}
		}
		preview, err := deps.Herald.PreviewSend(ctx, &herald.SendRequest{
			AppID: appID, Channel: channel, ProviderID: strings.TrimSpace(in.ProviderID),
			OrgID: strings.TrimSpace(in.OrgID), UserID: strings.TrimSpace(in.UserID),
		})
		if errors.Is(err, herald.ErrNoProviderConfigured) {
			return sendResolveResponse{Via: "none"}, nil
		}
		if err != nil {
			return sendResolveResponse{}, deps.mapError("send.resolve", err)
		}
		enabled := preview.Provider.Enabled
		out := sendResolveResponse{
			Provider: &ProviderRef{ID: preview.Provider.ID.String(), Name: preview.Provider.Name, Driver: preview.Provider.Driver, Enabled: &enabled},
			Via:      preview.Via,
		}
		if channel == string(herald.ChannelSMS) {
			out.From.Phone = preview.From
		} else {
			out.From.Email, out.From.Name = preview.From, preview.FromName
		}
		return out, nil
	}
}

type sendTestRequest struct {
	Channel    string         `json:"channel"`
	Recipient  string         `json:"recipient"`
	ProviderID string         `json:"providerId"`
	Template   string         `json:"template"`
	Locale     string         `json:"locale"`
	Data       map[string]any `json:"data"`
	Subject    string         `json:"subject"`
	Body       string         `json:"body"`
	UserID     string         `json:"userId"`
}

type sendTestResponse struct {
	MessageID         string       `json:"messageId,omitempty"`
	Status            string       `json:"status"`
	Provider          *ProviderRef `json:"provider"`
	ProviderMessageID string       `json:"providerMessageId,omitempty"`
	Error             string       `json:"error,omitempty"`
	Logged            bool         `json:"logged"`
}

// sendTestHandler sends one real message to one real recipient. A provider
// failure comes back as a normal response with status "failed" and the
// provider's own error, so the page can show exactly what happened. Contract
// errors are for requests that never reached a provider: bad input, no
// provider for the channel, a missing template, a render failure.
func sendTestHandler(deps Deps) func(context.Context, sendTestRequest, contract.Principal) (sendTestResponse, error) {
	return func(ctx context.Context, in sendTestRequest, p contract.Principal) (sendTestResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return sendTestResponse{}, err
		}
		channel, recipient := strings.TrimSpace(in.Channel), strings.TrimSpace(in.Recipient)
		switch {
		case !validChannel(channel):
			return sendTestResponse{}, badRequest("channel is not one Herald supports")
		case recipient == "":
			return sendTestResponse{}, badRequest("recipient is required")
		case strings.TrimSpace(in.Template) == "" && strings.TrimSpace(in.Body) == "":
			return sendTestResponse{}, badRequest("give a template or a body")
		}
		if in.ProviderID != "" {
			if _, perr := parseProviderID(in.ProviderID); perr != nil {
				return sendTestResponse{}, perr
			}
		}
		res, err := deps.Herald.Send(ctx, &herald.SendRequest{
			AppID: appID, Channel: channel, To: []string{recipient}, ProviderID: strings.TrimSpace(in.ProviderID),
			Template: strings.TrimSpace(in.Template), Locale: strings.TrimSpace(in.Locale), Data: in.Data,
			Subject: in.Subject, Body: in.Body, UserID: strings.TrimSpace(in.UserID),
			Metadata: map[string]string{"source": "dashboard.send.test"},
		})
		if err != nil {
			return sendTestResponse{}, deps.mapError("send.test", err)
		}
		out := sendTestResponse{
			Status: string(res.Status), ProviderMessageID: res.ProviderMessageID, Error: res.Error, Logged: res.Logged,
		}
		if !res.MessageID.IsNil() {
			out.MessageID = res.MessageID.String()
		}
		if res.ProviderID != "" {
			if pid, err := id.ParseProviderID(res.ProviderID); err == nil {
				if prov, err := deps.Herald.GetProvider(ctx, appID, pid); err == nil {
					out.Provider = &ProviderRef{ID: prov.ID.String(), Name: prov.Name, Driver: prov.Driver}
				}
			}
		}
		audit(ctx, deps, p, appID, "send.test", "message", out.MessageID, map[string]string{
			"channel": channel, "status": out.Status, "provider": res.ProviderID,
		})
		return out, nil
	}
}
