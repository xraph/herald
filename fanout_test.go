package herald

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/xraph/herald/device"
	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/internal/telemetry/telemetrytest"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/store/memory"
)

// seedPushProvider adds an enabled push provider backed by the "rec" driver.
func seedPushProvider(t *testing.T, h *Herald) {
	t.Helper()
	p := &provider.Provider{
		ID: id.NewProviderID(), AppID: "app_a", Name: "push-rec", Channel: "push",
		Driver: "rec", Enabled: true,
	}
	if err := h.Store().CreateProvider(bg, p); err != nil {
		t.Fatalf("create provider: %v", err)
	}
}

func saveFanoutDevice(t *testing.T, h *Herald, user, prov, addr string, key byte) *device.Device {
	t.Helper()
	d := &device.Device{
		AppID: "app_a", UserID: user, Provider: prov, Address: addr,
		PublicKey: bytes.Repeat([]byte{key}, 32), Platform: "android",
	}
	if err := h.SaveDevice(bg, d); err != nil {
		t.Fatalf("SaveDevice: %v", err)
	}
	return d
}

// fanoutHerald is a Herald with a "rec" push provider, a device store and a
// clock fixed at 14:30 UTC.
func fanoutHerald(t *testing.T) (*Herald, *recordingDriver, *memory.Store) {
	t.Helper()
	st := memory.New()
	rec := &recordingDriver{name: "rec", channel: "push", vendorID: "pm_1"}
	h := newHerald(t, st, WithDriver(rec))
	h.now = func() time.Time { return time.Date(2026, 10, 28, 14, 30, 0, 0, time.UTC) }
	seedPushProvider(t, h)
	return h, rec, st
}

func pushReq(user string, opts *driver.PushOptions) *SendRequest {
	return &SendRequest{
		AppID: "app_a", Channel: "push", UserID: user,
		Subject: "Deploy failed", Body: "api in production", Push: opts,
	}
}

func TestPushFanOutSendsToEveryDeviceOfTheUser(t *testing.T) {
	h, rec, _ := fanoutHerald(t)
	a := saveFanoutDevice(t, h, "u1", "rec", "tok-a", 1)
	b := saveFanoutDevice(t, h, "u1", "rec", "tok-b", 2)
	saveFanoutDevice(t, h, "u1", "fcm", "tok-c", 3)
	saveFanoutDevice(t, h, "u2", "rec", "tok-d", 4)

	res, err := h.Send(bg, pushReq("u1", &driver.PushOptions{Urgency: "active"}))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(rec.sent) != 2 {
		t.Fatalf("driver calls = %d, want 2", len(rec.sent))
	}
	for i, want := range []*device.Device{a, b} {
		got := rec.sent[i]
		if got.To != want.Address || !bytes.Equal(got.PublicKey, want.PublicKey) {
			t.Errorf("call %d to %q key %x, want %q key %x", i, got.To, got.PublicKey, want.Address, want.PublicKey)
		}
		if got.Title != "Deploy failed" || got.Text != "api in production" {
			t.Errorf("call %d title %q text %q", i, got.Title, got.Text)
		}
		if got.Push == nil || got.Push.Urgency != "active" {
			t.Errorf("call %d push = %+v", i, got.Push)
		}
	}
	if res.Status != message.StatusSent || len(res.Devices) != 2 {
		t.Fatalf("res = %+v", res)
	}
	for i, want := range []*device.Device{a, b} {
		if res.Devices[i].DeviceID != want.ID || res.Devices[i].Status != message.StatusSent {
			t.Errorf("device result %d = %+v", i, res.Devices[i])
		}
	}
}

func TestPushFanOutDeviceFilter(t *testing.T) {
	h, rec, _ := fanoutHerald(t)
	saveFanoutDevice(t, h, "u1", "rec", "tok-a", 1)
	b := saveFanoutDevice(t, h, "u1", "rec", "tok-b", 2)
	b.Settings = map[string]string{"push.live": "false"}
	if err := h.SaveDevice(bg, b); err != nil {
		t.Fatal(err)
	}

	req := pushReq("u1", nil)
	req.DeviceFilter = func(d *device.Device) bool { return d.Settings["push.live"] != "false" }
	res, err := h.Send(bg, req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(rec.sent) != 1 {
		t.Fatalf("driver calls = %d, want 1", len(rec.sent))
	}
	if res.Status != message.StatusSent {
		t.Errorf("status = %q", res.Status)
	}
	for _, dr := range res.Devices {
		if dr.DeviceID == b.ID && dr.Status != message.StatusFiltered {
			t.Errorf("B status = %q, want filtered", dr.Status)
		}
	}
}

func TestPushFanOutQuietHoursSuppressOnlyNonCriticalShows(t *testing.T) {
	h, rec, _ := fanoutHerald(t)
	d := saveFanoutDevice(t, h, "u1", "rec", "tok-a", 1)
	d.TimeZone = "Asia/Tokyo" // 23:30 there
	d.QuietHours = &device.QuietHours{Start: "22:00", End: "07:00"}
	if err := h.SaveDevice(bg, d); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		opts *driver.PushOptions
		want message.Status
		call int
	}{
		{"active", &driver.PushOptions{Urgency: "active"}, message.StatusSuppressed, 0},
		{"critical", &driver.PushOptions{Urgency: driver.UrgencyCritical}, message.StatusSent, 1},
		{"withdraw", &driver.PushOptions{Urgency: "active", Kind: driver.PushKindWithdraw}, message.StatusSent, 2},
	}
	for _, c := range cases {
		res, err := h.Send(bg, pushReq("u1", c.opts))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if res.Status != c.want || len(rec.sent) != c.call {
			t.Errorf("%s: status %q, calls %d; want %q, %d", c.name, res.Status, len(rec.sent), c.want, c.call)
		}
	}
}

func TestPushFanOutQuietHoursFallBackToUTC(t *testing.T) {
	h, rec, st := fanoutHerald(t)
	quiet := &device.QuietHours{Start: "14:00", End: "15:00"}
	d := saveFanoutDevice(t, h, "u1", "rec", "tok-a", 1)
	d.QuietHours = quiet
	if err := h.SaveDevice(bg, d); err != nil {
		t.Fatal(err)
	}
	res, err := h.Send(bg, pushReq("u1", nil))
	if err != nil || res.Status != message.StatusSuppressed {
		t.Fatalf("empty zone: %+v, %v", res, err)
	}

	// SaveDevice would refuse this zone, so write it straight in.
	mars := &device.Device{
		AppID: "app_a", UserID: "u2", Channel: "push", Provider: "rec", Address: "tok-m",
		PublicKey: bytes.Repeat([]byte{9}, 32), Platform: "android",
		TimeZone: "Mars/Olympus", QuietHours: quiet,
	}
	if err = st.SaveDevice(bg, mars); err != nil {
		t.Fatal(err)
	}
	res, err = h.Send(bg, pushReq("u2", nil))
	if err != nil || res.Status != message.StatusSuppressed {
		t.Fatalf("unknown zone: %+v, %v", res, err)
	}
	if len(rec.sent) != 0 {
		t.Errorf("driver calls = %d, want 0", len(rec.sent))
	}
}

func TestPushFanOutDeletesGoneDevices(t *testing.T) {
	h, rec, _ := fanoutHerald(t)
	a := saveFanoutDevice(t, h, "u1", "rec", "tok-a", 1)
	b := saveFanoutDevice(t, h, "u1", "rec", "tok-b", 2)
	rec.errByAddr = map[string]error{"tok-a": fmt.Errorf("x: %w", driver.ErrAddressGone)}

	res, err := h.Send(bg, pushReq("u1", nil))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent {
		t.Errorf("status = %q, want sent", res.Status)
	}
	if res.Devices[0].DeviceID != a.ID || res.Devices[0].Status != message.StatusFailed ||
		!bytes.Contains([]byte(res.Devices[0].Error), []byte("address gone")) {
		t.Errorf("A result = %+v", res.Devices[0])
	}
	if _, err := h.GetDevice(bg, "app_a", a.ID); !errors.Is(err, ErrDeviceNotFound) {
		t.Errorf("GetDevice(A) = %v, want ErrDeviceNotFound", err)
	}
	left, _ := h.ListDevices(bg, "app_a", "u1")
	if len(left) != 1 || left[0].ID != b.ID || res.Devices[1].Status != message.StatusSent {
		t.Errorf("left = %d, B result = %+v", len(left), res.Devices[1])
	}
}

func TestPushFanOutNoDevices(t *testing.T) {
	h, rec, _ := fanoutHerald(t)
	res, err := h.Send(bg, pushReq("nobody", nil))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusFiltered || res.Error != "no devices" || len(res.Devices) != 0 || len(rec.sent) != 0 {
		t.Errorf("res = %+v, calls %d", res, len(rec.sent))
	}
}

func TestPushFanOutLogsDeviceIDNotAddress(t *testing.T) {
	h, _, _ := fanoutHerald(t)
	a := saveFanoutDevice(t, h, "u1", "rec", "tok-secret-a", 1)
	b := saveFanoutDevice(t, h, "u1", "rec", "tok-secret-b", 2)
	if _, err := h.Send(bg, pushReq("u1", nil)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	rows, err := h.Store().ListMessages(bg, "app_a", message.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range rows {
		got[m.Recipient] = true
		if bytes.Contains([]byte(fmt.Sprintf("%+v", m)), []byte("tok-secret")) {
			t.Errorf("row leaks an address: %+v", m)
		}
	}
	if len(rows) != 2 || !got[a.ID] || !got[b.ID] {
		t.Errorf("recipients = %v, want %s and %s", got, a.ID, b.ID)
	}
}

func TestPushFanOutNeedsADeviceStore(t *testing.T) {
	st := struct{ store.Store }{memory.New()}
	h := newHerald(t, st, WithDriver(&recordingDriver{name: "rec", channel: "push"}))
	seedPushProvider(t, h)
	_, err := h.Send(bg, pushReq("u1", nil))
	if !errors.Is(err, ErrNoDeviceStore) {
		t.Fatalf("err = %v, want ErrNoDeviceStore", err)
	}
}

func TestPushFanOutTelemetryNeverCarriesAddress(t *testing.T) {
	rec := telemetrytest.Spans(t)
	h, _, _ := fanoutHerald(t)
	const addr = "h1.secret-handle"
	key := bytes.Repeat([]byte{5}, 32)
	d := &device.Device{AppID: "app_a", UserID: "u1", Provider: "rec", Address: addr, PublicKey: key, Platform: "ios"}
	if err := h.SaveDevice(bg, d); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Send(bg, pushReq("u1", nil)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	for _, v := range []string{addr, base64.StdEncoding.EncodeToString(key)} {
		if leaks := telemetrytest.Leaks(rec, v); len(leaks) > 0 {
			t.Errorf("%q leaked into %v", v, leaks)
		}
	}
}

func TestPushRawSendUsesSubjectAsTitle(t *testing.T) {
	st := memory.New()
	pushRec := &recordingDriver{name: "rec", channel: "push"}
	mailRec := &recordingDriver{name: "mail", channel: "email"}
	h := newHerald(t, st, WithDriver(pushRec), WithDriver(mailRec))
	seedPushProvider(t, h)
	seedProvider(t, h, "app_a", "mail", "mail", 0, true)

	if _, err := h.Send(bg, &SendRequest{AppID: "app_a", Channel: "push", To: []string{"tok-1"}, Subject: "Checkout is failing"}); err != nil {
		t.Fatal(err)
	}
	if got := pushRec.sent[0].Title; got != "Checkout is failing" {
		t.Errorf("push title = %q", got)
	}
	if _, err := h.Send(bg, &SendRequest{AppID: "app_a", Channel: "email", To: []string{"a@example.com"}, Subject: "Checkout is failing"}); err != nil {
		t.Fatal(err)
	}
	if got := mailRec.sent[0].Title; got != "" {
		t.Errorf("email title = %q, want empty", got)
	}
}

func TestPushWithRecipientsSkipsFanOut(t *testing.T) {
	h, rec, _ := fanoutHerald(t)
	saveFanoutDevice(t, h, "u1", "rec", "tok-a", 1)
	req := pushReq("u1", &driver.PushOptions{CollapseID: "c1"})
	req.To = []string{"tok-1"}
	res, err := h.Send(bg, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.sent) != 1 || rec.sent[0].To != "tok-1" || rec.sent[0].Push == nil || rec.sent[0].Push.CollapseID != "c1" {
		t.Fatalf("sent = %+v", rec.sent)
	}
	if res.Devices != nil {
		t.Errorf("Devices = %+v, want nil", res.Devices)
	}
}

// reregisterDriver changes the stored device while the send is in flight, the
// way a phone that re-registers between the list and the send does.
type reregisterDriver struct {
	recordingDriver
	onSend func()
}

func (d *reregisterDriver) Send(ctx context.Context, m *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	if d.onSend != nil {
		d.onSend()
	}
	return d.recordingDriver.Send(ctx, m)
}

func TestPushFanOutKeepsDeviceThatReRegisteredMidSend(t *testing.T) {
	st := memory.New()
	rd := &reregisterDriver{recordingDriver: recordingDriver{name: "rec", channel: "push", vendorID: "pm_1"}}
	h := newHerald(t, st, WithDriver(rd))
	seedPushProvider(t, h)
	a := saveFanoutDevice(t, h, "u1", "rec", "tok-old", 1)
	rd.errByAddr = map[string]error{"tok-old": fmt.Errorf("x: %w", driver.ErrAddressGone)}
	rd.onSend = func() {
		// Same public key, new address: SaveDevice keeps the row ID.
		saveFanoutDevice(t, h, "u1", "rec", "tok-new", 1)
	}

	res, err := h.Send(bg, pushReq("u1", nil))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Devices[0].Status != message.StatusFailed {
		t.Errorf("result = %+v, want failed", res.Devices[0])
	}
	got, err := h.GetDevice(bg, "app_a", a.ID)
	if err != nil {
		t.Fatalf("GetDevice: %v, want the re-registered device kept", err)
	}
	if got.Address != "tok-new" {
		t.Errorf("address = %q, want tok-new", got.Address)
	}
}
