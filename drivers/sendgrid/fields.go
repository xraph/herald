package sendgrid

import "github.com/xraph/herald/driver"

var _ driver.Describer = (*Driver)(nil)

// Fields describes what the SendGrid driver reads.
func (d *Driver) Fields() []driver.Field {
	return append([]driver.Field{
		{Key: "api_key", Label: "API key", Required: true, Secret: true, Placement: driver.PlacementCredential},
		{Key: "base_url", Label: "API base URL", Placement: driver.PlacementSetting},
	}, driver.SenderFields()...)
}
