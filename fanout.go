package herald

import (
	"bytes"
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/xraph/herald/device"
	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/internal/telemetry"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/template"
)

// fanoutJob is what send has already worked out when a push goes to a user's
// devices.
type fanoutJob struct {
	req      *SendRequest
	channel  string
	span     trace.Span
	prov     *provider.Provider
	drv      driver.Driver
	outbound *driver.OutboundMessage
	rendered *template.RenderedContent
	dataErr  error
}

// sendToDevices delivers one push to each of the user's devices that belong
// to the resolved provider's driver. Devices are logged by ID, never by
// address.
func (h *Herald) sendToDevices(ctx context.Context, j *fanoutJob) (*SendResult, error) {
	if h.devices == nil {
		return nil, ErrNoDeviceStore
	}
	req := j.req

	all, err := h.devices.ListDevices(ctx, req.AppID, req.UserID)
	if err != nil {
		return nil, err
	}
	var devices []*device.Device
	for _, d := range all {
		if d.Channel == pushChannel && d.Provider == j.prov.Driver {
			devices = append(devices, d)
		}
	}
	j.span.SetAttributes(telemetry.KeyRecipientCount.Int(len(devices)))

	now := h.now().UTC()
	var (
		results                  = make([]DeviceResult, 0, len(devices))
		first                    *SendResult
		firstErr                 string
		sent, failed, suppressed int
	)
	for _, d := range devices {
		var dr DeviceResult
		switch {
		case req.DeviceFilter != nil && !req.DeviceFilter(d):
			dr = DeviceResult{DeviceID: d.ID, Status: message.StatusFiltered}
			h.logSkipped(ctx, j, d, dr.Status, "filtered by the caller")
		case inQuietHours(d, req.Push, now):
			dr = DeviceResult{DeviceID: d.ID, Status: message.StatusSuppressed}
			h.logSkipped(ctx, j, d, dr.Status, "quiet hours")
			h.metrics.suppress(j.channel)
			suppressed++
		default:
			r := h.deliverToDevice(ctx, j, d)
			dr = DeviceResult{DeviceID: d.ID, Status: r.Status, Error: r.Error}
			if first == nil {
				first = r
			}
			if r.Status == message.StatusSent {
				sent++
			} else {
				failed++
				if firstErr == "" {
					firstErr = r.Error
				}
			}
		}
		results = append(results, dr)
	}

	j.span.SetAttributes(telemetry.KeySent.Int(sent), telemetry.KeyFailed.Int(failed))

	res := &SendResult{ProviderID: j.prov.ID.String(), Devices: results}
	if first != nil {
		res.MessageID = first.MessageID
		res.ProviderMessageID = first.ProviderMessageID
		res.Logged = first.Logged
	}
	switch {
	case sent > 0:
		res.Status = message.StatusSent
	case failed > 0:
		res.Status = message.StatusFailed
		res.Error = firstErr
	case suppressed > 0:
		res.Status = message.StatusSuppressed
	default:
		res.Status = message.StatusFiltered
	}
	if len(devices) == 0 {
		res.Error = "no devices"
	}
	if failed > 0 {
		telemetry.FailErr(j.span, classDeliveryFailed, errors.New(firstErr))
	}
	return res, nil
}

// inQuietHours reports whether d is in its quiet hours at now. Critical
// pushes and withdrawals go through regardless. An empty or unknown time
// zone means UTC.
func inQuietHours(d *device.Device, opts *driver.PushOptions, now time.Time) bool {
	if d.QuietHours == nil ||
		opts.EffectiveUrgency() == driver.UrgencyCritical ||
		opts.EffectiveKind() == driver.PushKindWithdraw {
		return false
	}
	loc := time.UTC
	if d.TimeZone != "" {
		if l, err := time.LoadLocation(d.TimeZone); err == nil {
			loc = l
		}
	}
	return d.QuietHours.Contains(now, loc)
}

// logSkipped records a device that was not delivered to and why.
func (h *Herald) logSkipped(ctx context.Context, j *fanoutJob, d *device.Device, status message.Status, reason string) {
	msg := h.deviceMessage(j, d)
	msg.ProviderID = ""
	msg.Status = status
	msg.Error = reason
	h.logMessage(ctx, msg)
}

// deviceMessage is the message row for one device. Recipient is the device
// ID, never its address.
func (h *Herald) deviceMessage(j *fanoutJob, d *device.Device) *message.Message {
	req := j.req
	return &message.Message{
		ID:         id.NewMessageID(),
		AppID:      req.AppID,
		EnvID:      req.EnvID,
		TemplateID: req.Template,
		ProviderID: j.prov.ID.String(),
		Channel:    req.Channel,
		Recipient:  d.ID,
		Subject:    j.rendered.Subject,
		Body:       truncate(j.rendered.Text, h.config.TruncateBodyAt),
		Status:     message.StatusSending,
		Metadata:   req.Metadata,
		Async:      req.Async,
		Attempts:   1,
		CreatedAt:  h.now().UTC(),
	}
}

// deleteGoneDevice removes a device whose address the driver reported dead.
// A phone that re-registered after the list keeps its row ID (SaveDevice
// upserts on the public key), so the row is re-read first and deleted only
// while it still holds the address and key that failed.
func (h *Herald) deleteGoneDevice(ctx context.Context, d *device.Device) {
	cur, err := h.devices.GetDevice(ctx, d.AppID, d.ID)
	switch {
	case errors.Is(err, device.ErrNotFound):
		return
	case err != nil:
		h.logger.Warn("herald: could not re-read a gone device", "device_id", d.ID, "error", err)
		return
	case cur.Address != d.Address || !bytes.Equal(cur.PublicKey, d.PublicKey):
		return
	}
	if delErr := h.devices.DeleteDevice(ctx, d.AppID, d.ID); delErr != nil && !errors.Is(delErr, device.ErrNotFound) {
		h.logger.Warn("herald: could not delete a gone device", "device_id", d.ID, "error", delErr)
	}
}

// deliverToDevice sends to one device and records the outcome. A driver that
// reports the address gone gets the device deleted.
func (h *Herald) deliverToDevice(ctx context.Context, j *fanoutJob, d *device.Device) *SendResult {
	msg := h.deviceMessage(j, d)
	logged := h.logMessage(ctx, msg)

	var del message.Delivery
	if j.dataErr != nil {
		del = message.Delivery{Status: message.StatusFailed, Error: j.dataErr.Error()}
		h.metrics.delivery(j.channel, j.prov.Driver, j.dataErr, 0, false)
	} else {
		out := *j.outbound
		out.To = d.Address
		out.PublicKey = d.PublicKey
		start := time.Now()
		result, err := j.drv.Send(ctx, &out)
		h.metrics.delivery(j.channel, j.prov.Driver, err, time.Since(start), true)
		switch {
		case err != nil:
			del = message.Delivery{Status: message.StatusFailed, Error: err.Error()}
			if errors.Is(err, driver.ErrAddressGone) {
				h.deleteGoneDevice(ctx, d)
			}
		default:
			sentAt := time.Now().UTC()
			del = message.Delivery{Status: message.StatusSent, SentAt: &sentAt}
			if result != nil {
				del.ProviderMessageID = result.ProviderMessageID
			}
		}
	}
	if logged {
		if err := h.store.RecordDelivery(ctx, msg.ID, del); err != nil {
			h.metrics.logWriteFailed(logOpRecordDelivery)
			h.logger.Warn("herald: failed to record delivery", "message_id", msg.ID.String(), "error", err)
		}
	}
	return &SendResult{
		MessageID:         msg.ID,
		Status:            del.Status,
		ProviderID:        j.prov.ID.String(),
		ProviderMessageID: del.ProviderMessageID,
		Error:             del.Error,
		Logged:            logged,
	}
}
