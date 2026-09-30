package email

import "github.com/xraph/herald/driver"

var (
	_ driver.Describer = (*SMTPDriver)(nil)
	_ driver.Describer = (*ResendDriver)(nil)
)

// Fields describes what the SMTP driver reads.
func (d *SMTPDriver) Fields() []driver.Field {
	return append([]driver.Field{
		{Key: "host", Label: "Host", Required: true, Placement: driver.PlacementSetting},
		{Key: "port", Label: "Port", Required: true, Help: "Usually 587, or 465 with implicit TLS.", Placement: driver.PlacementSetting},
		{Key: "use_tls", Label: "Implicit TLS", Help: `"true" to connect over TLS from the start (port 465).`, Placement: driver.PlacementSetting},
		{Key: "username", Label: "Username", Placement: driver.PlacementCredential},
		{Key: "password", Label: "Password", Secret: true, Placement: driver.PlacementCredential},
	}, driver.SenderFields()...)
}

// Fields describes what the Resend driver reads.
func (d *ResendDriver) Fields() []driver.Field {
	return append([]driver.Field{
		{Key: "api_key", Label: "API key", Required: true, Secret: true, Placement: driver.PlacementCredential},
		{Key: "base_url", Label: "API base URL", Help: "Leave empty for Resend's own API.", Placement: driver.PlacementSetting},
	}, driver.SenderFields()...)
}
