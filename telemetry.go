package herald

import (
	"errors"
	"time"

	gometrics "github.com/xraph/go-utils/metrics"

	"github.com/xraph/herald/internal/telemetry"
	"github.com/xraph/herald/internal/telemetry/metricsvec"
)

// Span names herald records.
const (
	spanSend    = "send notification"
	spanNotify  = "notify channels"
	spanRender  = "render template"
	spanResolve = "resolve provider"
)

// Metric names as herald registers them. A Forge app exports each one with
// its namespace prefix, so herald_deliveries_attempted_total scrapes as
// forge_herald_deliveries_attempted_total.
const (
	metricDeliveriesAttempted = "herald_deliveries_attempted_total"
	metricDeliveryDuration    = "herald_delivery_duration_seconds"
	metricMessagesSuppressed  = "herald_messages_suppressed_total"
	metricSendsRejected       = "herald_sends_rejected_total"
	metricLogWritesFailed     = "herald_message_log_writes_failed_total"
)

// Values of the result label.
const (
	resultOK    = "ok"
	resultError = "error"
)

// Values of the op label on metricLogWritesFailed.
const (
	logOpCreate         = "create"
	logOpRecordDelivery = "record_delivery"
)

// channelOther is the channel label for anything that isn't one of the six
// channel constants. Callers name the channel, so it is never used raw.
const channelOther = "other"

// deliveryBuckets are the bounds, in seconds, for metricDeliveryDuration. A
// delivery is one vendor call: the HTTP drivers give up at 10s and SMTP at
// 30s.
var deliveryBuckets = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

// Reasons a Send is rejected before any delivery, the values of the reason
// label on metricSendsRejected and the status description of a rejected
// send's span.
const (
	reasonTemplateNotFound = "template_not_found"
	reasonTemplateDisabled = "template_disabled"
	reasonNoLocale         = "no_locale"
	reasonMissingVariable  = "missing_variable"
	reasonRenderFailed     = "render_failed"
	reasonNoProvider       = "no_provider"
	reasonProviderNotFound = "provider_not_found"
	reasonInvalidChannel   = "invalid_channel"
	reasonDriverNotFound   = "driver_not_found"
	reasonNoRecipients     = "no_recipients"
	reasonOther            = "other"
)

// Status descriptions for a send that ran but didn't deliver everything, and
// a notify where some channel failed.
const (
	classDeliveryFailed = "delivery_failed"
	classChannelFailed  = "channel_failed"
)

// rejection pairs a sentinel with its reason and the name a span reports as
// exception.type, since a sentinel's Go type (*errors.errorString) says
// nothing.
var rejections = []struct {
	err    error
	reason string
	name   string
}{
	{ErrTemplateNotFound, reasonTemplateNotFound, "herald.ErrTemplateNotFound"},
	{ErrTemplateDisabled, reasonTemplateDisabled, "herald.ErrTemplateDisabled"},
	{ErrNoVersionForLocale, reasonNoLocale, "herald.ErrNoVersionForLocale"},
	{ErrMissingRequiredVariable, reasonMissingVariable, "herald.ErrMissingRequiredVariable"},
	{ErrTemplateRenderFailed, reasonRenderFailed, "herald.ErrTemplateRenderFailed"},
	{ErrNoProviderConfigured, reasonNoProvider, "herald.ErrNoProviderConfigured"},
	{ErrProviderNotFound, reasonProviderNotFound, "herald.ErrProviderNotFound"},
	{ErrInvalidChannel, reasonInvalidChannel, "herald.ErrInvalidChannel"},
	{ErrDriverNotFound, reasonDriverNotFound, "herald.ErrDriverNotFound"},
}

// classify returns the reason a Send failed with err, and the type its span
// reports. Anything that isn't a herald sentinel (a store failure, say) is
// reasonOther with the innermost error's Go type.
func classify(err error) (reason, errType string) {
	for _, r := range rejections {
		if errors.Is(err, r.err) {
			return r.reason, r.name
		}
	}
	return reasonOther, telemetry.TypeOf(err)
}

// channelLabel clamps a caller's channel to the six constants, so neither a
// label nor a span ever carries a string the caller made up.
func channelLabel(channel string) string {
	if c := ChannelType(channel); c.IsValid() {
		return string(c)
	}
	return channelOther
}

// sendMetrics is what Send records on. Every method is a no-op on a nil
// *sendMetrics, which is what you get without WithMetrics.
type sendMetrics struct {
	deliveries *metricsvec.CounterVec
	duration   *metricsvec.HistogramVec
	suppressed *metricsvec.CounterVec
	rejected   *metricsvec.CounterVec
	logFailed  *metricsvec.CounterVec
}

func newSendMetrics(m gometrics.Metrics) *sendMetrics {
	if m == nil {
		return nil
	}
	return &sendMetrics{
		deliveries: metricsvec.NewCounterVec(m, metricDeliveriesAttempted,
			"Deliveries herald attempted, one per recipient, by channel, driver and result.",
			"channel", "driver", "result"),
		duration: metricsvec.NewHistogramVec(m, metricDeliveryDuration,
			"Time one driver took to deliver to one recipient.",
			deliveryBuckets, "channel", "driver", "result"),
		suppressed: metricsvec.NewCounterVec(m, metricMessagesSuppressed,
			"Recipients skipped because the user opted out, by channel.",
			"channel"),
		rejected: metricsvec.NewCounterVec(m, metricSendsRejected,
			"Sends that failed before any delivery, by channel and reason.",
			"channel", "reason"),
		logFailed: metricsvec.NewCounterVec(m, metricLogWritesFailed,
			"Message log writes that failed. The send went ahead without the row.",
			"op"),
	}
}

// delivery records one recipient's delivery. called is false when the driver
// never ran (the provider's credentials couldn't be opened): that counts as a
// failed delivery but has no duration.
func (m *sendMetrics) delivery(channel, driverName string, err error, elapsed time.Duration, called bool) {
	if m == nil {
		return
	}
	result := resultOK
	if err != nil {
		result = resultError
	}
	m.deliveries.Inc(channel, driverName, result)
	if called {
		m.duration.Observe(elapsed.Seconds(), channel, driverName, result)
	}
}

func (m *sendMetrics) suppress(channel string) {
	if m == nil {
		return
	}
	m.suppressed.Inc(channel)
}

func (m *sendMetrics) reject(channel, reason string) {
	if m == nil {
		return
	}
	m.rejected.Inc(channel, reason)
}

func (m *sendMetrics) logWriteFailed(op string) {
	if m == nil {
		return
	}
	m.logFailed.Inc(op)
}
