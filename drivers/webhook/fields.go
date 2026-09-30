package webhook

import "github.com/xraph/herald/driver"

var _ driver.Describer = (*Driver)(nil)

// Fields describes what the webhook driver reads. Settings whose key starts
// with "data." are forwarded in the payload's data object, with the prefix
// removed.
func (d *Driver) Fields() []driver.Field {
	return []driver.Field{
		{Key: "url", Label: "Endpoint URL", Required: true, Secret: true, Help: "Often carries a token, so it's kept with the credentials.", Placement: driver.PlacementCredential},
		{Key: "signing_secret", Label: "Signing secret", Secret: true, Help: "Signs each request with HMAC-SHA256 in X-Webhook-Signature.", Placement: driver.PlacementCredential},
		{Key: "event_type", Label: "Event type", Help: `Defaults to "notification".`, Placement: driver.PlacementSetting},
	}
}
