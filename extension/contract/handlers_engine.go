package contract

import (
	"context"
	"sort"

	"github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"

	"github.com/xraph/herald"
	"github.com/xraph/herald/template"
)

func registerEngine(d *dispatcher.Dispatcher, deps Deps) error {
	return query(d, "engine.info", engineInfoHandler(deps))
}

type engineInfoRequest struct{}

type encryptionInfo struct {
	Configured bool   `json:"configured"`
	KeyID      string `json:"keyId,omitempty"`
}

type engineInfoResponse struct {
	App            AppRef         `json:"app"`
	DefaultLocale  string         `json:"defaultLocale"`
	MaxBatchSize   int            `json:"maxBatchSize"`
	TruncateBodyAt int            `json:"truncateBodyAt"`
	Channels       []string       `json:"channels"`
	Drivers        []DriverInfo   `json:"drivers"`
	TemplateFuncs  []string       `json:"templateFuncs"`
	Encryption     encryptionInfo `json:"encryption"`
	APIProtected   bool           `json:"apiProtected"`
}

// engineInfoHandler answers engine.info: the app in view, engine settings,
// drivers with their field schemas, the template functions, and whether
// credentials are encrypted and the REST API is protected. It replaces the
// templ settings panel and feeds every page header.
func engineInfoHandler(deps Deps) func(context.Context, engineInfoRequest, contract.Principal) (engineInfoResponse, error) {
	return func(_ context.Context, _ engineInfoRequest, p contract.Principal) (engineInfoResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return engineInfoResponse{}, err
		}
		h := deps.Herald
		cfg := h.Config()

		channels := make([]string, 0, len(herald.ValidChannels()))
		for _, c := range herald.ValidChannels() {
			channels = append(channels, c.String())
		}

		names := h.Drivers().Names()
		sort.Strings(names)
		drivers := make([]DriverInfo, 0, len(names))
		for _, name := range names {
			drv, err := h.Drivers().Get(name)
			if err != nil {
				continue
			}
			info := DriverInfo{Name: name, Channel: drv.Channel()}
			if fields, ok := h.Drivers().Describe(name); ok {
				info.Fields = make([]FieldInfo, 0, len(fields))
				for _, f := range fields {
					info.Fields = append(info.Fields, FieldInfo{
						Key: f.Key, Label: f.Label, Help: f.Help,
						Required: f.Required, Secret: f.Secret, Placement: string(f.Placement),
					})
				}
			}
			drivers = append(drivers, info)
		}

		keyID := h.CredentialKeyID()
		protected := false
		if deps.APIProtected != nil {
			protected = deps.APIProtected()
		}
		return engineInfoResponse{
			App:            AppRef{ID: appID, Label: appLabel(appID)},
			DefaultLocale:  cfg.DefaultLocale,
			MaxBatchSize:   cfg.MaxBatchSize,
			TruncateBodyAt: cfg.TruncateBodyAt,
			Channels:       channels,
			Drivers:        drivers,
			TemplateFuncs:  template.NewRenderer().FuncNames(),
			Encryption:     encryptionInfo{Configured: keyID != "", KeyID: keyID},
			APIProtected:   protected,
		}, nil
	}
}
