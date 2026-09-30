package ses

import "github.com/xraph/herald/driver"

var _ driver.Describer = (*Driver)(nil)

// Fields describes what the SES driver reads. Session tokens aren't
// supported, so temporary credentials won't work.
func (d *Driver) Fields() []driver.Field {
	return append([]driver.Field{
		{Key: "access_key_id", Label: "Access key ID", Required: true, Placement: driver.PlacementCredential},
		{Key: "secret_access_key", Label: "Secret access key", Required: true, Secret: true, Placement: driver.PlacementCredential},
		{Key: "region", Label: "Region", Required: true, Help: "e.g. us-east-1", Placement: driver.PlacementSetting},
		{Key: "base_url", Label: "API base URL", Placement: driver.PlacementSetting},
	}, driver.SenderFields()...)
}
