package vonage

import "github.com/xraph/herald/driver"

var _ driver.Describer = (*Driver)(nil)

// Fields describes what the Vonage driver reads.
func (d *Driver) Fields() []driver.Field {
	return []driver.Field{
		{Key: "api_key", Label: "API key", Required: true, Placement: driver.PlacementCredential},
		{Key: "api_secret", Label: "API secret", Required: true, Secret: true, Placement: driver.PlacementCredential},
		{Key: "from_number", Label: "From number", Required: true, Help: "A routing rule's from phone overrides it.", Placement: driver.PlacementSetting},
		{Key: "base_url", Label: "API base URL", Placement: driver.PlacementSetting},
	}
}
