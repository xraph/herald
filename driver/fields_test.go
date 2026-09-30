package driver_test

import (
	"context"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/email"
	"github.com/xraph/herald/driver/inapp"
	"github.com/xraph/herald/driver/push"
	"github.com/xraph/herald/driver/sms"
)

type describedDriver interface {
	driver.Driver
	driver.Describer
}

// checkFields is the rule every driver's schema must satisfy. extra adds
// values for either/or requirements the schema can't mark as required.
func checkFields(t *testing.T, d describedDriver, extra map[string]string) {
	t.Helper()
	seen := map[string]bool{}
	merged := map[string]string{}
	for _, f := range d.Fields() {
		if seen[f.Key] {
			t.Errorf("%s: field %q declared twice", d.Name(), f.Key)
		}
		seen[f.Key] = true
		if f.Label == "" {
			t.Errorf("%s: field %q has no label", d.Name(), f.Key)
		}
		if f.Secret && f.Placement != driver.PlacementCredential {
			t.Errorf("%s: secret field %q must be a credential", d.Name(), f.Key)
		}
		if f.Placement != driver.PlacementCredential && f.Placement != driver.PlacementSetting {
			t.Errorf("%s: field %q has placement %q", d.Name(), f.Key, f.Placement)
		}
		if f.Required {
			merged[f.Key] = "x"
		}
	}
	for k, v := range extra {
		merged[k] = v
	}
	if err := d.Validate(merged, nil); err != nil {
		t.Errorf("%s: required fields filled, Validate still says: %v", d.Name(), err)
	}
}

func TestBuiltInSchemas(t *testing.T) {
	checkFields(t, &email.SMTPDriver{}, nil)
	checkFields(t, &email.ResendDriver{}, nil)
	checkFields(t, &sms.TwilioDriver{}, nil)
	checkFields(t, &push.FCMDriver{}, map[string]string{"access_token": "x"})
	checkFields(t, &inapp.Driver{}, nil)
}

type bare struct{}

func (bare) Name() string                          { return "bare" }
func (bare) Channel() string                       { return "email" }
func (bare) Validate(_, _ map[string]string) error { return nil }
func (bare) Send(context.Context, *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	return &driver.DeliveryResult{}, nil
}

func TestDescribe(t *testing.T) {
	r := driver.NewRegistry()
	r.Register(&email.SMTPDriver{})
	r.Register(&inapp.Driver{})
	r.Register(bare{})

	if fields, ok := r.Describe("smtp"); !ok || len(fields) == 0 {
		t.Errorf("smtp: %v %v", fields, ok)
	}
	if fields, ok := r.Describe("inapp"); !ok || fields == nil || len(fields) != 0 {
		t.Errorf("inapp needs nothing, and says so: %v %v", fields, ok)
	}
	if _, ok := r.Describe("bare"); ok {
		t.Error("a driver without Describer must report false, so the form falls back to key/value rows")
	}
	if _, ok := r.Describe("nope"); ok {
		t.Error("an unknown driver must report false")
	}
}
