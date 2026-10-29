package pushgateway

import (
	"context"
	"maps"
	"time"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/push"
)

// driverData returns a fresh map for one send: the platform's credentials,
// then its settings (settings win), with the apns sandbox flag set from the
// handle. The platform's own maps are never modified.
func driverData(p *Platform, c claims) map[string]string {
	data := make(map[string]string, len(p.Credentials)+len(p.Settings)+1)
	maps.Copy(data, p.Credentials)
	maps.Copy(data, p.Settings)
	if c.P == platformIOS {
		switch c.E {
		case envSandbox:
			data["sandbox"] = "true"
		case envProduction:
			delete(data, "sandbox")
		}
	}
	return data
}

// deliver maps one validated send onto the platform driver (Contract C).
func (g *gateway) deliver(ctx context.Context, p *Platform, c claims, b *push.SendBody, urgency driver.Urgency, kind string) (*driver.DeliveryResult, error) {
	msg := &driver.OutboundMessage{To: c.T, Data: driverData(p, c)}
	ttl := time.Duration(b.TTLSeconds) * time.Second
	withdraw := kind == driver.PushKindWithdraw

	switch {
	case c.P == platformIOS && withdraw:
		msg.Push = &driver.PushOptions{
			ContentAvailable: true,
			Urgency:          driver.UrgencyPassive,
			TTL:              ttl,
			Data:             map[string]string{"withdraw": b.CollapseID},
		}
	case c.P == platformIOS:
		msg.Title = g.placeholderTitle
		msg.Text = g.placeholderBody
		msg.Push = &driver.PushOptions{
			MutableContent: true,
			Urgency:        urgency,
			CollapseID:     b.CollapseID,
			TTL:            ttl,
			Data:           map[string]string{"sealed": b.Sealed, "kid": b.KeyID, "collapse_id": b.CollapseID},
		}
	default:
		data := map[string]string{"kid": b.KeyID, "kind": kind, "collapse_id": b.CollapseID}
		if !withdraw {
			data["sealed"] = b.Sealed
		}
		msg.Push = &driver.PushOptions{
			DataOnly:   true,
			Urgency:    urgency,
			CollapseID: b.CollapseID,
			TTL:        ttl,
			Data:       data,
		}
	}
	return p.Driver.Send(ctx, msg)
}
