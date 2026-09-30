package apns

import "github.com/xraph/herald/driver"

var _ driver.Describer = (*Driver)(nil)

// Fields describes what the APNs driver reads.
func (d *Driver) Fields() []driver.Field {
	return []driver.Field{
		{Key: "key_id", Label: "Key ID", Required: true, Placement: driver.PlacementSetting},
		{Key: "team_id", Label: "Team ID", Required: true, Placement: driver.PlacementSetting},
		{Key: "bundle_id", Label: "Bundle ID", Required: true, Help: "Sent as the apns-topic header.", Placement: driver.PlacementSetting},
		{Key: "private_key", Label: "Private key (.p8)", Required: true, Secret: true, Help: "The PKCS#8 PEM from Apple.", Placement: driver.PlacementCredential},
		{Key: "sandbox", Label: "Sandbox", Help: `"true" to use Apple's development servers.`, Placement: driver.PlacementSetting},
		{Key: "base_url", Label: "API base URL", Placement: driver.PlacementSetting},
	}
}
