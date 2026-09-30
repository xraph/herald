package push

import "github.com/xraph/herald/driver"

var _ driver.Describer = (*FCMDriver)(nil)

// Fields describes what the FCM driver reads. It needs one of access_token or
// server_key; the schema can't express "one of", so neither is required here
// and Validate enforces it.
func (d *FCMDriver) Fields() []driver.Field {
	return []driver.Field{
		{Key: "project_id", Label: "Project ID", Required: true, Placement: driver.PlacementSetting},
		{Key: "access_token", Label: "OAuth access token", Help: "Set this or a server key. Tokens are short-lived and Herald does not refresh them.", Secret: true, Placement: driver.PlacementCredential},
		{Key: "server_key", Label: "Server key", Help: "Legacy key. Set this or an access token.", Secret: true, Placement: driver.PlacementCredential},
		{Key: "base_url", Label: "API base URL", Placement: driver.PlacementSetting},
	}
}
