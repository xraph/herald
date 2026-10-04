package contract

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/preference"
	"github.com/xraph/herald/store"
)

func registerPreferences(d *dispatcher.Dispatcher, deps Deps) error {
	if err := query(d, "preferences.get", preferencesGetHandler(deps)); err != nil {
		return err
	}
	return command(d, "preferences.optOut", preferencesOptOutHandler(deps))
}

// ChannelPreferenceWire is one type's per-channel settings. A null channel
// is "never set", which Herald treats as opted in.
type ChannelPreferenceWire struct {
	Email *bool `json:"email"`
	SMS   *bool `json:"sms"`
	Push  *bool `json:"push"`
	InApp *bool `json:"inapp"`
}

// PreferenceWire is a user's stored preference record.
type PreferenceWire struct {
	ID        string                           `json:"id"`
	UserID    string                           `json:"userId"`
	Overrides map[string]ChannelPreferenceWire `json:"overrides"`
	UpdatedAt time.Time                        `json:"updatedAt"`
}

func projectPreference(p *preference.Preference) *PreferenceWire {
	out := &PreferenceWire{ID: p.ID.String(), UserID: p.UserID, UpdatedAt: p.UpdatedAt, Overrides: map[string]ChannelPreferenceWire{}}
	for typ, cp := range p.Overrides {
		out.Overrides[typ] = ChannelPreferenceWire(cp)
	}
	return out
}

type preferencesGetRequest struct {
	UserID string `json:"userId"`
}

type preferencesGetResponse struct {
	Preference *PreferenceWire `json:"preference"`
	KnownTypes []string        `json:"knownTypes"`
}

// preferencesGetHandler answers a user's record, or null when there is none,
// and the template slugs in this app so the page can offer an opt-out on a
// type the user has never touched.
func preferencesGetHandler(deps Deps) func(context.Context, preferencesGetRequest, contract.Principal) (preferencesGetResponse, error) {
	return func(ctx context.Context, in preferencesGetRequest, p contract.Principal) (preferencesGetResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return preferencesGetResponse{}, err
		}
		userID, err := requireUser(in.UserID)
		if err != nil {
			return preferencesGetResponse{}, err
		}
		out := preferencesGetResponse{KnownTypes: []string{}}
		pref, err := deps.Herald.Store().GetPreference(ctx, appID, userID)
		switch {
		case errors.Is(err, store.ErrPreferenceNotFound):
		case err != nil:
			return preferencesGetResponse{}, deps.mapError("preferences.get", err)
		default:
			out.Preference = projectPreference(pref)
		}
		templates, err := deps.Herald.Store().ListTemplates(ctx, appID)
		if err != nil {
			return preferencesGetResponse{}, deps.mapError("preferences.get", err)
		}
		for _, t := range templates {
			out.KnownTypes = append(out.KnownTypes, t.Slug)
		}
		slices.Sort(out.KnownTypes)
		out.KnownTypes = slices.Compact(out.KnownTypes)
		return out, nil
	}
}

type preferencesOptOutRequest struct {
	UserID  string `json:"userId"`
	Type    string `json:"type"`
	Channel string `json:"channel"`
}

type preferencesOptOutResponse struct {
	Preference *PreferenceWire `json:"preference"`
}

// optOutMu serialises opt-outs in this process. Preferences are stored as one
// record per user, so two opt-outs racing a read-modify-write would drop one
// of them, and a dropped opt-out sends the user mail they declined. Replicas
// can still race each other; the stores have no compare-and-set to close that.
var optOutMu sync.Mutex

// preferencesOptOutHandler turns one channel of one type off for a user,
// creating the record when there isn't one. It never turns anything on and
// leaves every other override as it was. Opting out twice is a no-op.
func preferencesOptOutHandler(deps Deps) func(context.Context, preferencesOptOutRequest, contract.Principal) (preferencesOptOutResponse, error) {
	return func(ctx context.Context, in preferencesOptOutRequest, p contract.Principal) (preferencesOptOutResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return preferencesOptOutResponse{}, err
		}
		userID, err := requireUser(in.UserID)
		if err != nil {
			return preferencesOptOutResponse{}, err
		}
		typ := strings.TrimSpace(in.Type)
		if !validType(typ) {
			return preferencesOptOutResponse{}, badRequest("type must be 1 to 256 bytes with no control characters")
		}
		channel := strings.TrimSpace(in.Channel)
		off := false
		set := map[string]func(*preference.ChannelPreference){
			"email": func(c *preference.ChannelPreference) { c.Email = &off },
			"sms":   func(c *preference.ChannelPreference) { c.SMS = &off },
			"push":  func(c *preference.ChannelPreference) { c.Push = &off },
			"inapp": func(c *preference.ChannelPreference) { c.InApp = &off },
		}[channel]
		if set == nil {
			return preferencesOptOutResponse{}, badRequest("channel must be email, sms, push or inapp")
		}

		pref, err := saveOptOut(ctx, deps, appID, userID, typ, set)
		if err != nil {
			return preferencesOptOutResponse{}, deps.mapError("preferences.optOut", err)
		}
		audit(ctx, deps, p, appID, "preferences.optOut", "preference", userID, map[string]string{"type": typ, "channel": channel})
		return preferencesOptOutResponse{Preference: projectPreference(pref)}, nil
	}
}

// maxTypeLen bounds a preference type. Types are template slugs, but a slug
// created through the REST API or Go needn't match the dashboard's slug
// pattern, so anything preferences.get can offer is accepted.
const maxTypeLen = 256

func validType(typ string) bool {
	if typ == "" || len(typ) > maxTypeLen {
		return false
	}
	return !strings.ContainsFunc(typ, unicode.IsControl)
}

// saveOptOut applies one opt-out under optOutMu and returns the stored record.
// The lock covers only the read-modify-write, so a slow audit sink (the
// caller's next step) never stalls other opt-outs.
func saveOptOut(ctx context.Context, deps Deps, appID, userID, typ string, set func(*preference.ChannelPreference)) (*preference.Preference, error) {
	optOutMu.Lock()
	defer optOutMu.Unlock()
	now := time.Now().UTC()
	pref, err := deps.Herald.Store().GetPreference(ctx, appID, userID)
	switch {
	case errors.Is(err, store.ErrPreferenceNotFound):
		pref = &preference.Preference{ID: id.NewPreferenceID(), AppID: appID, UserID: userID, CreatedAt: now}
	case err != nil:
		return nil, err
	}
	if pref.Overrides == nil {
		pref.Overrides = map[string]preference.ChannelPreference{}
	}
	cp := pref.Overrides[typ]
	set(&cp)
	pref.Overrides[typ] = cp
	pref.UpdatedAt = now
	if err := deps.Herald.Store().SetPreference(ctx, pref); err != nil {
		return nil, err
	}
	return pref, nil
}
