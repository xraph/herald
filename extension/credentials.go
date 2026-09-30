package extension

import (
	"errors"
	"fmt"

	"github.com/xraph/forge"

	"github.com/xraph/herald"
	"github.com/xraph/herald/credential"
)

// CredentialKeyConfig is one previous credential key.
type CredentialKeyConfig struct {
	ID  string `json:"id" yaml:"id" mapstructure:"id"`
	Key string `json:"key" yaml:"key" mapstructure:"key"`
}

// credentialOptions turns the key settings into herald options. Errors name
// the setting and never echo its value.
func (c Config) credentialOptions() ([]herald.Option, error) {
	if c.CredentialsKey == "" {
		if len(c.PreviousCredentialsKeys) > 0 {
			return nil, errors.New("herald: previous_credentials_keys is set but credentials_key is not")
		}
		return nil, nil
	}
	key, err := credential.ParseKey(c.CredentialsKey)
	if err != nil {
		return nil, fmt.Errorf("herald: credentials_key: %w", err)
	}
	keyID := c.CredentialsKeyID
	if keyID == "" {
		keyID = "k1"
	}
	opts := []herald.Option{herald.WithCredentialKey(keyID, key)}
	for i, pk := range c.PreviousCredentialsKeys {
		b, err := credential.ParseKey(pk.Key)
		if err != nil {
			return nil, fmt.Errorf("herald: previous_credentials_keys[%d]: %w", i, err)
		}
		opts = append(opts, herald.WithPreviousCredentialKey(pk.ID, b))
	}
	return opts, nil
}

// logCredentialKey says at startup whether provider credentials are encrypted
// at rest. It names the key ID and never logs key material. A standalone Init
// before Register has no logger yet, so nil logs nothing.
func logCredentialKey(logger forge.Logger, keyID string) {
	if logger == nil {
		return
	}
	if keyID != "" {
		logger.Info("herald: provider credentials are encrypted at rest", forge.F("key_id", keyID))
		return
	}
	logger.Warn("herald: provider credentials are stored in plaintext; set credentials_key to encrypt them")
}
