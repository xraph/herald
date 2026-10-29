package pushgateway

import (
	"errors"
	"testing"

	"github.com/xraph/herald/driver"
)

// firstArgDriver wants key_id, team_id and bundle_id in Validate's first
// argument, the way drivers/apns does.
type firstArgDriver struct{ fakeDriver }

func (d *firstArgDriver) Validate(credentials, _ map[string]string) error {
	for _, k := range []string{"key_id", "team_id", "bundle_id"} {
		if credentials[k] == "" {
			return errors.New("missing " + k)
		}
	}
	return nil
}

var _ driver.Driver = (*firstArgDriver)(nil)

func TestNewValidatesMergedCredentialsAndSettings(t *testing.T) {
	build := func(settings map[string]string) error {
		cfg := testConfig()
		cfg.IOS = &Platform{
			Driver:      &firstArgDriver{fakeDriver{name: "apns", channel: "push"}},
			Credentials: map[string]string{"private_key": "pem"},
			Settings:    settings,
		}
		_, err := New(cfg)
		return err
	}

	if err := build(map[string]string{"key_id": "K1", "team_id": "T1", "bundle_id": "com.example.app"}); err != nil {
		t.Fatalf("New with the keys in Settings: %v", err)
	}
	if err := build(map[string]string{"team_id": "T1", "bundle_id": "com.example.app"}); err == nil {
		t.Fatal("New succeeded with key_id missing from both maps")
	}
}
