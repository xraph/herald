package extension

import (
	"errors"
	"fmt"

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
