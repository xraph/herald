package cloudflare

import "github.com/xraph/herald/driver"

var _ driver.Describer = (*Driver)(nil)

// Fields describes what the Cloudflare email driver reads.
func (d *Driver) Fields() []driver.Field {
	return append([]driver.Field{
		{Key: "api_token", Label: "API token", Required: true, Secret: true, Placement: driver.PlacementCredential},
		{Key: "account_id", Label: "Account ID", Required: true, Placement: driver.PlacementSetting},
		{Key: "base_url", Label: "API base URL", Placement: driver.PlacementSetting},
	}, driver.SenderFields()...)
}
