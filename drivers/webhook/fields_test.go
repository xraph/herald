package webhook

import (
	"testing"

	"github.com/xraph/herald/driver"
)

func TestFieldsCoverValidate(t *testing.T) {
	d := &Driver{}
	merged := map[string]string{}
	seen := map[string]bool{}
	for _, f := range d.Fields() {
		if seen[f.Key] {
			t.Errorf("field %q declared twice", f.Key)
		}
		seen[f.Key] = true
		if f.Secret && f.Placement != driver.PlacementCredential {
			t.Errorf("secret field %q must be a credential", f.Key)
		}
		if f.Required {
			merged[f.Key] = "x"
		}
	}
	for k, v := range extra(t) {
		merged[k] = v
	}
	if err := d.Validate(merged, nil); err != nil {
		t.Fatalf("required fields filled, Validate still says: %v", err)
	}
}

func extra(*testing.T) map[string]string { return nil }
