package discord

import "github.com/xraph/herald/driver"

var _ driver.Describer = (*Driver)(nil)

// Fields describes what the Discord driver reads. The webhook URL embeds its
// own token, so it is a secret.
func (d *Driver) Fields() []driver.Field {
	return []driver.Field{
		{Key: "webhook_url", Label: "Webhook URL", Required: true, Secret: true, Help: "Contains Discord's token, so treat it like a password.", Placement: driver.PlacementCredential},
	}
}
