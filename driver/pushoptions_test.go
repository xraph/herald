package driver_test

import (
	"testing"

	"github.com/xraph/herald/driver"
)

func TestPushOptionsDefaults(t *testing.T) {
	var nilOpts *driver.PushOptions
	if nilOpts.EffectiveUrgency() != driver.UrgencyActive || nilOpts.EffectiveKind() != "show" {
		t.Fatal("nil defaults")
	}
	o := &driver.PushOptions{Urgency: driver.UrgencyCritical, Kind: "withdraw"}
	if o.EffectiveUrgency() != "critical" || o.EffectiveKind() != "withdraw" {
		t.Fatal("set values")
	}
	for _, u := range []driver.Urgency{"passive", "active", "time_sensitive", "critical"} {
		if !u.Valid() {
			t.Errorf("%q invalid", u)
		}
	}
	for _, u := range []driver.Urgency{"", "loud", "time-sensitive"} {
		if u.Valid() {
			t.Errorf("%q valid", u)
		}
	}
}
