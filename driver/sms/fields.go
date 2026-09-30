package sms

import "github.com/xraph/herald/driver"

var _ driver.Describer = (*TwilioDriver)(nil)

// Fields describes what the Twilio driver reads.
func (d *TwilioDriver) Fields() []driver.Field {
	return []driver.Field{
		{Key: "account_sid", Label: "Account SID", Required: true, Placement: driver.PlacementCredential},
		{Key: "auth_token", Label: "Auth token", Required: true, Secret: true, Placement: driver.PlacementCredential},
		{Key: "from_number", Label: "From number", Required: true, Help: "E.164, e.g. +15550100. A routing rule's from phone overrides it.", Placement: driver.PlacementSetting},
		{Key: "base_url", Label: "API base URL", Help: "Leave empty for Twilio's own API.", Placement: driver.PlacementSetting},
	}
}
