package mailgun

import "github.com/xraph/herald/driver"

var _ driver.Describer = (*Driver)(nil)

// Fields describes what the Mailgun driver reads.
func (d *Driver) Fields() []driver.Field {
	return append([]driver.Field{
		{Key: "api_key", Label: "API key", Required: true, Secret: true, Placement: driver.PlacementCredential},
		{Key: "domain", Label: "Sending domain", Required: true, Placement: driver.PlacementSetting},
		{Key: "base_url", Label: "API base URL", Help: "Set to the EU endpoint for EU domains.", Placement: driver.PlacementSetting},
	}, driver.SenderFields()...)
}
