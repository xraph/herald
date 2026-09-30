package slack

import "github.com/xraph/herald/driver"

var _ driver.Describer = (*Driver)(nil)

// Fields describes what the Slack driver reads. It posts either through an
// incoming webhook or through the API with a bot token and a channel; the
// schema can't express "one of", so nothing is required here and Validate
// enforces the pairing.
func (d *Driver) Fields() []driver.Field {
	return []driver.Field{
		{Key: "webhook_url", Label: "Incoming webhook URL", Secret: true, Help: "Use this, or a bot token and a channel.", Placement: driver.PlacementCredential},
		{Key: "bot_token", Label: "Bot token", Secret: true, Help: "Needs a channel too.", Placement: driver.PlacementCredential},
		{Key: "channel", Label: "Channel", Help: "Channel ID or name, used with a bot token.", Placement: driver.PlacementSetting},
		{Key: "base_url", Label: "API base URL", Placement: driver.PlacementSetting},
	}
}
