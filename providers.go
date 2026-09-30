package herald

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/xraph/herald/credential"
	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
)

// Protection values reported per credential.
const (
	ProtectionAESGCM    = credential.ProtectionAESGCM
	ProtectionPlaintext = credential.ProtectionPlaintext
)

// CredentialState is how one stored credential is protected. It never
// carries the value.
type CredentialState struct {
	Key        string `json:"key"`
	Protection string `json:"protection"`
	KeyID      string `json:"key_id,omitempty"`
}

// ProviderUpdate changes only what it names. Nil pointers and absent keys
// leave the stored value alone, so a caller can replace one secret without
// ever reading the others. A provider's channel and driver can't change.
type ProviderUpdate struct {
	Name              *string
	Priority          *int
	Enabled           *bool
	SetCredentials    map[string]string
	RemoveCredentials []string
	SetSettings       map[string]string
	RemoveSettings    []string
}

// EncryptReport says what EncryptStoredCredentials changed.
type EncryptReport struct {
	Providers        int `json:"providers"`
	ValuesEncrypted  int `json:"values_encrypted"`
	AlreadyEncrypted int `json:"already_encrypted"`
}

// connectionTargets are the settings that say where a driver connects. The
// REST API and the dashboards never show credential values, so the right to
// edit a provider must not become the right to read its secrets. Pointing
// one of these at a server you control would do exactly that: the next send
// carries the API key or password there. Changing one therefore requires the
// secrets to be entered again in the same update.
//
// Send merges credentials under settings, so a target placed in credentials
// reaches the driver too whenever no setting shadows it. ValidateProvider
// refuses one there, and checkRetarget compares the merged value, never the
// settings alone.
var connectionTargets = []string{"base_url", "host"}

// CreateProvider validates p, encrypts its credentials when a key is
// configured, and stores it. p is left holding what was stored.
func (h *Herald) CreateProvider(ctx context.Context, p *provider.Provider) error {
	if p.ID.IsNil() {
		p.ID = id.NewProviderID()
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	p.Name = strings.TrimSpace(p.Name)
	if err := h.ValidateProvider(p); err != nil {
		return err
	}
	sealed, err := h.seal(p.ID.String(), p.Credentials, slices.Collect(maps.Keys(p.Credentials)))
	if err != nil {
		return err
	}
	p.Credentials = sealed
	return h.store.CreateProvider(ctx, p)
}

// GetProvider returns a provider of appID, with credentials as stored.
func (h *Herald) GetProvider(ctx context.Context, appID string, providerID id.ProviderID) (*provider.Provider, error) {
	p, err := h.store.GetProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if p.AppID != appID {
		return nil, ErrProviderNotFound
	}
	return p, nil
}

// UpdateProvider applies u to a provider of appID. Credentials it sets are
// encrypted; credentials it doesn't touch keep their stored form exactly. It
// refuses, and writes nothing, when an existing credential can't be decrypted
// for validation, and when it moves base_url or host to a new server without
// setting the provider's secret credentials again (see connectionTargets).
func (h *Herald) UpdateProvider(ctx context.Context, appID string, providerID id.ProviderID, u ProviderUpdate) (*provider.Provider, error) {
	existing, err := h.GetProvider(ctx, appID, providerID)
	if err != nil {
		return nil, err
	}

	next := *existing
	next.Settings = applyChanges(existing.Settings, u.SetSettings, u.RemoveSettings)
	if u.Name != nil {
		next.Name = strings.TrimSpace(*u.Name)
	}
	if u.Priority != nil {
		next.Priority = *u.Priority
	}
	if u.Enabled != nil {
		next.Enabled = *u.Enabled
	}

	// Validate against plaintext: what's stored, decrypted, with the changes.
	plain, err := h.open(existing)
	if err != nil {
		return nil, err
	}
	candidate := next
	candidate.Credentials = applyChanges(plain, u.SetCredentials, u.RemoveCredentials)
	if vErr := h.ValidateProvider(&candidate); vErr != nil {
		return nil, vErr
	}
	if rErr := h.checkRetarget(existing, plain, &candidate, u); rErr != nil {
		return nil, rErr
	}

	stored := applyChanges(existing.Credentials, u.SetCredentials, u.RemoveCredentials)
	if next.Credentials, err = h.seal(next.ID.String(), stored, slices.Collect(maps.Keys(u.SetCredentials))); err != nil {
		return nil, err
	}
	next.UpdatedAt = time.Now().UTC()
	if err := h.store.UpdateProvider(ctx, &next); err != nil {
		return nil, err
	}
	return &next, nil
}

// checkRetarget refuses an update that points a provider at a new server
// while keeping stored secrets it did not supply again. It compares the
// target a driver would actually read, credentials merged under settings the
// way driverData merges them, before and after the update, from plaintext
// (before is existing's credentials decrypted, after is the candidate's). A
// target set in credentials counts as a move whatever its value. Removing a
// target, so the vendor default applies, or keeping its current value needs
// nothing, and a secret the same update removes needn't be sent. Secret
// means secret in the driver's schema; a driver without a schema has every
// credential treated as secret. The error names keys only.
func (h *Herald) checkRetarget(existing *provider.Provider, before map[string]string, after *provider.Provider, u ProviderUpdate) error {
	var moved []string
	for _, k := range connectionTargets {
		_, smuggled := u.SetCredentials[k]
		was := mergedValue(before, existing.Settings, k)
		now := mergedValue(after.Credentials, after.Settings, k)
		if smuggled || (now != "" && now != was) {
			moved = append(moved, k)
		}
	}
	if len(moved) == 0 {
		return nil
	}
	secret := func(string) bool { return true }
	if fields, ok := h.drivers.Describe(existing.Driver); ok {
		secrets := make(map[string]bool, len(fields))
		for _, f := range fields {
			if f.Secret {
				secrets[f.Key] = true
			}
		}
		secret = func(k string) bool { return secrets[k] }
	}
	var missing []string
	for k := range existing.Credentials {
		if !secret(k) || u.SetCredentials[k] != "" || slices.Contains(u.RemoveCredentials, k) {
			continue
		}
		missing = append(missing, k)
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("%w: changing %s sends credentials to a new server; enter %s again in the same update",
		ErrInvalidProvider, strings.Join(moved, " and "), strings.Join(missing, ", "))
}

// mergedValue is the value of k a driver reads: the setting when there is
// one, even an empty one, and otherwise the credential.
func mergedValue(creds, settings map[string]string, k string) string {
	if v, ok := settings[k]; ok {
		return v
	}
	return creds[k]
}

// DeleteProvider removes a provider of appID.
func (h *Herald) DeleteProvider(ctx context.Context, appID string, providerID id.ProviderID) error {
	if _, err := h.GetProvider(ctx, appID, providerID); err != nil {
		return err
	}
	return h.store.DeleteProvider(ctx, providerID)
}

// ValidateProvider checks p the way Send will use it: the driver exists and
// handles p's channel, no secret sits in settings, no connection target
// (base_url, host) or schema setting sits in credentials, and the driver's
// own Validate accepts credentials and settings merged as Send merges them.
// Encrypted credentials are decrypted first. Errors never contain a value.
func (h *Herald) ValidateProvider(p *provider.Provider) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidProvider)
	}
	drv, err := h.drivers.Get(p.Driver)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrDriverNotFound, p.Driver)
	}
	if drv.Channel() != p.Channel {
		return fmt.Errorf("%w: driver %s sends %s, not %s", ErrInvalidChannel, p.Driver, drv.Channel(), p.Channel)
	}
	for _, k := range connectionTargets {
		if _, ok := p.Credentials[k]; ok {
			return fmt.Errorf("%w: %s says where the driver connects and belongs in settings, not credentials", ErrInvalidProvider, k)
		}
	}
	if fields, ok := h.drivers.Describe(p.Driver); ok {
		for _, f := range fields {
			if f.Secret && p.Settings[f.Key] != "" {
				return fmt.Errorf("%w: %s is a secret and belongs in credentials, not settings", ErrInvalidProvider, f.Key)
			}
			if _, inCreds := p.Credentials[f.Key]; inCreds && f.Placement == driver.PlacementSetting {
				return fmt.Errorf("%w: %s is a setting and belongs in settings, not credentials", ErrInvalidProvider, f.Key)
			}
		}
	}
	plain, err := h.open(p)
	if err != nil {
		return err
	}
	merged := make(map[string]string, len(plain)+len(p.Settings))
	maps.Copy(merged, plain)
	maps.Copy(merged, p.Settings)
	if err := drv.Validate(merged, p.Settings); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidProvider, err)
	}
	return nil
}

// CredentialStatus reports how each stored credential of p is protected.
func (h *Herald) CredentialStatus(p *provider.Provider) []CredentialState {
	keys := slices.Sorted(maps.Keys(p.Credentials))
	out := make([]CredentialState, 0, len(keys))
	for _, k := range keys {
		protection, keyID := credential.Describe(p.Credentials[k])
		out = append(out, CredentialState{Key: k, Protection: protection, KeyID: keyID})
	}
	return out
}

// EncryptStoredCredentials encrypts every plaintext credential of every
// provider in appID. It is explicit and idempotent: nothing re-encrypts on
// read, and a second run changes nothing.
func (h *Herald) EncryptStoredCredentials(ctx context.Context, appID string) (EncryptReport, error) {
	var rep EncryptReport
	if h.cipher == nil {
		return rep, ErrNoCredentialKey
	}
	providers, err := h.store.ListAllProviders(ctx, appID)
	if err != nil {
		return rep, err
	}
	for _, p := range providers {
		var names []string
		for k, v := range p.Credentials {
			if credential.IsEncrypted(v) {
				rep.AlreadyEncrypted++
			} else {
				names = append(names, k)
			}
		}
		if len(names) == 0 {
			continue
		}
		if p.Credentials, err = h.seal(p.ID.String(), p.Credentials, names); err != nil {
			return rep, err
		}
		p.UpdatedAt = time.Now().UTC()
		if err := h.store.UpdateProvider(ctx, p); err != nil {
			return rep, fmt.Errorf("herald: encrypt credentials of provider %s: %w", p.ID, err)
		}
		rep.Providers++
		rep.ValuesEncrypted += len(names)
	}
	return rep, nil
}

// seal returns a copy of creds with the named values encrypted, when a key is
// configured. Values already encrypted are left alone.
func (h *Herald) seal(providerID string, creds map[string]string, names []string) (map[string]string, error) {
	out := maps.Clone(creds)
	if h.cipher == nil || out == nil {
		return out, nil
	}
	sort.Strings(names)
	for _, name := range names {
		v, ok := out[name]
		if !ok || credential.IsEncrypted(v) {
			continue
		}
		enc, err := h.cipher.Encrypt(providerID, name, v)
		if err != nil {
			return nil, fmt.Errorf("herald: encrypt credential %q: %w", name, err)
		}
		out[name] = enc
	}
	return out, nil
}

// open returns p's credentials decrypted. Plaintext passes through.
func (h *Herald) open(p *provider.Provider) (map[string]string, error) {
	out := make(map[string]string, len(p.Credentials))
	for k, v := range p.Credentials {
		plain, err := h.cipher.Decrypt(p.ID.String(), k, v)
		if err != nil {
			return nil, fmt.Errorf("herald: credential %q of provider %s: %w", k, p.ID, err)
		}
		out[k] = plain
	}
	return out, nil
}

// applyChanges returns a copy of m with remove deleted and set applied.
func applyChanges(m, set map[string]string, remove []string) map[string]string {
	out := make(map[string]string, len(m)+len(set))
	maps.Copy(out, m)
	for _, k := range remove {
		delete(out, k)
	}
	maps.Copy(out, set)
	return out
}
