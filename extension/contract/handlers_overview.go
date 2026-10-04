package contract

import (
	"context"
	"time"

	"github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"

	"github.com/xraph/herald"
)

func registerOverview(d *dispatcher.Dispatcher, deps Deps) error {
	return query(d, "overview.stats", overviewStatsHandler(deps))
}

var overviewWindows = map[string]time.Duration{
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

type overviewStatsRequest struct {
	Window string `json:"window"`
}

// MessageCount is the number of messages with one status on one channel.
type MessageCount struct {
	Status  string `json:"status"`
	Channel string `json:"channel"`
	N       int    `json:"n"`
}

// TemplateRef points at a template.
type TemplateRef struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Channel string `json:"channel,omitempty"`
}

type providerTotals struct {
	Total   int `json:"total"`
	Enabled int `json:"enabled"`
}

type credentialTotals struct {
	Plaintext int `json:"plaintext"`
	Encrypted int `json:"encrypted"`
}

type overviewStatsResponse struct {
	Since                    time.Time        `json:"since"`
	Counts                   []MessageCount   `json:"counts"`
	Providers                providerTotals   `json:"providers"`
	Credentials              credentialTotals `json:"credentials"`
	TemplatesWithoutFallback []TemplateRef    `json:"templatesWithoutFallback"`
}

// overviewStatsHandler answers overview.stats: message counts by status and
// channel over a window, provider and credential-protection totals (counted
// per credential value, because protection is a property of each value), and
// the templates with no fallback ("") version, which fail for any locale they
// don't list.
func overviewStatsHandler(deps Deps) func(context.Context, overviewStatsRequest, contract.Principal) (overviewStatsResponse, error) {
	return func(ctx context.Context, in overviewStatsRequest, p contract.Principal) (overviewStatsResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return overviewStatsResponse{}, err
		}
		window := in.Window
		if window == "" {
			window = "7d"
		}
		span, ok := overviewWindows[window]
		if !ok {
			return overviewStatsResponse{}, badRequest("window must be 24h, 7d or 30d")
		}
		since := time.Now().UTC().Add(-span)
		st := deps.Herald.Store()

		counts, err := st.CountMessages(ctx, appID, since)
		if err != nil {
			return overviewStatsResponse{}, deps.mapError("overview.stats", err)
		}
		out := overviewStatsResponse{Since: since, Counts: make([]MessageCount, 0, len(counts)), TemplatesWithoutFallback: []TemplateRef{}}
		for _, c := range counts {
			out.Counts = append(out.Counts, MessageCount{Status: string(c.Status), Channel: c.Channel, N: c.N})
		}

		providers, err := st.ListAllProviders(ctx, appID)
		if err != nil {
			return overviewStatsResponse{}, deps.mapError("overview.stats", err)
		}
		for _, prov := range providers {
			out.Providers.Total++
			if prov.Enabled {
				out.Providers.Enabled++
			}
			for _, s := range deps.Herald.CredentialStatus(prov) {
				if s.Protection == herald.ProtectionAESGCM {
					out.Credentials.Encrypted++
				} else {
					out.Credentials.Plaintext++
				}
			}
		}

		templates, err := st.ListTemplates(ctx, appID)
		if err != nil {
			return overviewStatsResponse{}, deps.mapError("overview.stats", err)
		}
		for _, t := range templates {
			if !hasFallback(t) {
				out.TemplatesWithoutFallback = append(out.TemplatesWithoutFallback, TemplateRef{ID: t.ID.String(), Slug: t.Slug, Channel: t.Channel})
			}
		}
		return out, nil
	}
}
