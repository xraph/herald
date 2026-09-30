package messagebird

import "github.com/xraph/herald/driver"

var _ driver.Describer = (*Driver)(nil)

// Fields describes what the MessageBird driver reads.
func (d *Driver) Fields() []driver.Field {
	return []driver.Field{
		{Key: "access_key", Label: "Access key", Required: true, Secret: true, Placement: driver.PlacementCredential},
		{Key: "originator", Label: "Originator", Required: true, Help: "Sender number or name. A routing rule's from phone overrides it.", Placement: driver.PlacementSetting},
		{Key: "base_url", Label: "API base URL", Placement: driver.PlacementSetting},
	}
}
