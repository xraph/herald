package herald

import (
	"bytes"
	"errors"
	"testing"

	"github.com/xraph/herald/device"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/store/memory"
)

func testDevice() *device.Device {
	return &device.Device{
		AppID: "app1", UserID: "u1", Provider: "sealed", Address: "h1.abc.def",
		PublicKey: bytes.Repeat([]byte{7}, 32), Platform: "ios",
	}
}

func TestDeviceMethodsUseTheMainStore(t *testing.T) {
	h := newHerald(t, memory.New())
	if err := h.SaveDevice(bg, testDevice()); err != nil {
		t.Fatalf("SaveDevice: %v", err)
	}
	got, err := h.ListDevices(bg, "app1", "u1")
	if err != nil || len(got) != 1 {
		t.Fatalf("ListDevices = %d, %v", len(got), err)
	}
	if got[0].Channel != "push" {
		t.Errorf("Channel = %q, want push", got[0].Channel)
	}
	if got[0].LastSeenAt.IsZero() {
		t.Error("LastSeenAt is zero")
	}
}

func TestDeviceStoreOptionWins(t *testing.T) {
	wrapped := struct{ store.Store }{memory.New()}

	h := newHerald(t, wrapped, WithDeviceStore(memory.New()))
	if err := h.SaveDevice(bg, testDevice()); err != nil {
		t.Fatalf("SaveDevice with option: %v", err)
	}

	h = newHerald(t, wrapped)
	if err := h.SaveDevice(bg, testDevice()); !errors.Is(err, ErrNoDeviceStore) {
		t.Fatalf("SaveDevice without option = %v, want ErrNoDeviceStore", err)
	}
}

func TestSaveDeviceValidates(t *testing.T) {
	tests := map[string]func(d *device.Device){
		"missing app id":  func(d *device.Device) { d.AppID = "" },
		"missing address": func(d *device.Device) { d.Address = "" },
		"bad channel":     func(d *device.Device) { d.Channel = "email" },
		"short key":       func(d *device.Device) { d.PublicKey = make([]byte, 31) },
		"sealed no key":   func(d *device.Device) { d.Provider, d.PublicKey = "sealed", nil },
		"sealed 0 key":    func(d *device.Device) { d.Provider, d.PublicKey = "sealed", []byte{} },
		"local time zone": func(d *device.Device) { d.TimeZone = "Local" },
		"bad time zone":   func(d *device.Device) { d.TimeZone = "Mars/Olympus" },
		"bad quiet hours": func(d *device.Device) { d.QuietHours = &device.QuietHours{Start: "22:00", End: "7"} },
	}
	h := newHerald(t, memory.New())
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			d := testDevice()
			mutate(d)
			if err := h.SaveDevice(bg, d); !errors.Is(err, device.ErrInvalidDevice) {
				t.Fatalf("err = %v, want ErrInvalidDevice", err)
			}
		})
	}
}

func TestSaveDeviceAcceptsSealedWithKey(t *testing.T) {
	h := newHerald(t, memory.New())
	d := testDevice()
	d.Provider, d.PublicKey = "sealed", make([]byte, 32)
	if err := h.SaveDevice(bg, d); err != nil {
		t.Fatalf("SaveDevice: %v", err)
	}
}

func TestSaveDeviceKeepsIDOnUpsert(t *testing.T) {
	h := newHerald(t, memory.New())
	first := testDevice()
	if err := h.SaveDevice(bg, first); err != nil {
		t.Fatal(err)
	}
	second := testDevice()
	second.Address = "h1.abc.new"
	if err := h.SaveDevice(bg, second); err != nil {
		t.Fatal(err)
	}
	if second.ID == "" || second.ID != first.ID {
		t.Errorf("second ID = %q, want %q", second.ID, first.ID)
	}
	got, err := h.ListDevices(bg, "app1", "u1")
	if err != nil || len(got) != 1 {
		t.Fatalf("ListDevices = %d, %v", len(got), err)
	}
	if got[0].Address != "h1.abc.new" {
		t.Errorf("Address = %q", got[0].Address)
	}
}

func TestGetDeviceOfAnotherApp(t *testing.T) {
	h := newHerald(t, memory.New())
	d := testDevice()
	if err := h.SaveDevice(bg, d); err != nil {
		t.Fatal(err)
	}
	if _, err := h.GetDevice(bg, "other", d.ID); !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("err = %v, want ErrDeviceNotFound", err)
	}
}
