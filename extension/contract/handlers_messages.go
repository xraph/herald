package contract

import (
	"context"
	"strings"

	"github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/provider"
)

const (
	defaultPageLimit = 25
	maxPageLimit     = 100
)

func registerMessages(d *dispatcher.Dispatcher, deps Deps) error {
	if err := query(d, "messages.list", messagesListHandler(deps)); err != nil {
		return err
	}
	return query(d, "messages.detail", messagesDetailHandler(deps))
}

type messagesListRequest struct {
	Channel string `json:"channel"`
	Status  string `json:"status"`
	Cursor  string `json:"cursor"`
	Limit   int    `json:"limit"`
}

type messagesListResponse struct {
	Messages   []MessageSummary `json:"messages"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

// providerNames maps every provider of appID by ID, so a page of messages
// resolves provider names with one read.
func providerNames(ctx context.Context, deps Deps, appID string) (map[string]*provider.Provider, error) {
	list, err := deps.Herald.Store().ListAllProviders(ctx, appID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*provider.Provider, len(list))
	for _, p := range list {
		out[p.ID.String()] = p
	}
	return out, nil
}

func summarizeMessage(m *message.Message, providers map[string]*provider.Provider) MessageSummary {
	s := MessageSummary{
		ID: m.ID.String(), Recipient: m.Recipient, Channel: m.Channel, Status: string(m.Status),
		TemplateSlug: m.TemplateID, Error: m.Error, CreatedAt: m.CreatedAt, SentAt: m.SentAt,
	}
	if p, ok := providers[m.ProviderID]; ok {
		s.Provider = &ProviderRef{ID: p.ID.String(), Name: p.Name, Driver: p.Driver}
	}
	return s
}

// messagesListHandler pages the delivery log newest first with an opaque
// cursor. It reads limit+1 rows to know whether there is a next page and
// never computes a total.
func messagesListHandler(deps Deps) func(context.Context, messagesListRequest, contract.Principal) (messagesListResponse, error) {
	return func(ctx context.Context, in messagesListRequest, p contract.Principal) (messagesListResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return messagesListResponse{}, err
		}
		offset, err := decodeCursor(in.Cursor)
		if err != nil {
			return messagesListResponse{}, err
		}
		limit := pageLimit(in.Limit, defaultPageLimit, maxPageLimit)
		rows, err := deps.Herald.Store().ListMessages(ctx, appID, message.ListOptions{
			Channel: strings.TrimSpace(in.Channel), Status: message.Status(strings.TrimSpace(in.Status)),
			Limit: limit + 1, Offset: offset,
		})
		if err != nil {
			return messagesListResponse{}, deps.mapError("messages.list", err)
		}
		providers, err := providerNames(ctx, deps, appID)
		if err != nil {
			return messagesListResponse{}, deps.mapError("messages.list", err)
		}
		out := messagesListResponse{Messages: make([]MessageSummary, 0, limit), NextCursor: nextCursor(offset, limit, len(rows))}
		for i, m := range rows {
			if i == limit {
				break
			}
			out.Messages = append(out.Messages, summarizeMessage(m, providers))
		}
		return out, nil
	}
}

type messagesDetailRequest struct {
	ID string `json:"id"`
}

type messagesDetailResponse struct {
	Message MessageDetail `json:"message"`
}

// messagesDetailHandler shows one message. Its template is found from the
// stored slug and channel (Herald logs the slug where the ID would go), and
// is null when that template is gone.
func messagesDetailHandler(deps Deps) func(context.Context, messagesDetailRequest, contract.Principal) (messagesDetailResponse, error) {
	return func(ctx context.Context, in messagesDetailRequest, p contract.Principal) (messagesDetailResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return messagesDetailResponse{}, err
		}
		mid, err := id.ParseMessageID(strings.TrimSpace(in.ID))
		if err != nil {
			return messagesDetailResponse{}, badRequest("id is not a message id")
		}
		m, err := deps.Herald.Store().GetMessage(ctx, mid)
		if err != nil {
			return messagesDetailResponse{}, deps.mapError("messages.detail", err)
		}
		if m.AppID != appID {
			return messagesDetailResponse{}, notFound("message not found")
		}
		providers, err := providerNames(ctx, deps, appID)
		if err != nil {
			return messagesDetailResponse{}, deps.mapError("messages.detail", err)
		}
		detail := MessageDetail{
			MessageSummary: summarizeMessage(m, providers),
			Subject:        m.Subject, Body: m.Body, Metadata: m.Metadata, Attempts: m.Attempts,
			Async: m.Async, EnvID: m.EnvID, ProviderMessageID: m.ProviderMessageID,
		}
		if detail.Metadata == nil {
			detail.Metadata = map[string]string{}
		}
		if m.TemplateID != "" {
			if t, err := deps.Herald.Store().GetTemplateBySlug(ctx, appID, m.TemplateID, m.Channel); err == nil {
				detail.Template = &TemplateRef{ID: t.ID.String(), Slug: t.Slug, Channel: t.Channel}
			}
		}
		return messagesDetailResponse{Message: detail}, nil
	}
}
