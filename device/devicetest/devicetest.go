// Package devicetest is the conformance suite every device.Store backend runs.
package devicetest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xraph/herald/device"
)

// Run executes every conformance check against the backend open returns. open
// is called once per subtest and must return a fresh, empty store.
func Run(t *testing.T, open func(t *testing.T) device.Store) {
	t.Helper()

	cases := []struct {
		name string
		fn   func(t *testing.T, s device.Store)
	}{
		{"SaveAssignsIDAndCreatedAt", saveAssignsIDAndCreatedAt},
		{"UpsertByPublicKey", upsertByPublicKey},
		{"UpsertByAddressWithoutKey", upsertByAddressWithoutKey},
		{"UsersAreSeparate", usersAreSeparate},
		{"RoundTripsEveryField", roundTripsEveryField},
		{"GetWrongAppIsNotFound", getWrongAppIsNotFound},
		{"DeleteMissingIsNotFound", deleteMissingIsNotFound},
		{"ListScopedAndOrdered", listScopedAndOrdered},
		{"DeleteUserDevices", deleteUserDevices},
		{"ConcurrentSaveIsOneRow", concurrentSaveIsOneRow},
		{"ReadsAreCopies", readsAreCopies},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.fn(t, open(t)) })
	}
}

func key32(n byte) []byte { return bytes.Repeat([]byte{n}, 32) }

func sealed(user string, key []byte, address string) *device.Device {
	return &device.Device{
		AppID: "app_a", UserID: user, Channel: "push", Provider: "sealed",
		Address: address, PublicKey: key, Platform: "android",
	}
}

func mustSave(t *testing.T, s device.Store, d *device.Device) {
	t.Helper()

	if err := s.SaveDevice(context.Background(), d); err != nil {
		t.Fatalf("SaveDevice: %v", err)
	}
}

func mustList(t *testing.T, s device.Store, app, user string) []*device.Device {
	t.Helper()

	got, err := s.ListDevices(context.Background(), app, user)
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}

	return got
}

func saveAssignsIDAndCreatedAt(t *testing.T, s device.Store) {
	d := sealed("u1", key32(1), "h1.a")
	mustSave(t, s, d)

	if !strings.HasPrefix(d.ID, "hdev_") {
		t.Errorf("ID = %q, want hdev_ prefix", d.ID)
	}

	if d.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
}

func upsertByPublicKey(t *testing.T, s device.Store) {
	first := sealed("u1", key32(1), "h1.first")
	mustSave(t, s, first)

	second := sealed("u1", key32(1), "h1.second")
	second.Name = "Pixel 9"
	mustSave(t, s, second)

	got := mustList(t, s, "app_a", "u1")
	if len(got) != 1 {
		t.Fatalf("rows = %d, want 1", len(got))
	}

	if got[0].ID != first.ID || second.ID != first.ID {
		t.Errorf("ID changed: first %q second %q stored %q", first.ID, second.ID, got[0].ID)
	}

	if !got[0].CreatedAt.Equal(first.CreatedAt) || !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("CreatedAt changed: first %v second %v stored %v", first.CreatedAt, second.CreatedAt, got[0].CreatedAt)
	}

	if got[0].Address != "h1.second" || got[0].Name != "Pixel 9" {
		t.Errorf("fields not overwritten: %+v", got[0])
	}
}

func upsertByAddressWithoutKey(t *testing.T, s device.Store) {
	mustSave(t, s, sealed("u1", nil, "tok-1"))
	mustSave(t, s, sealed("u1", nil, "tok-1"))

	if got := mustList(t, s, "app_a", "u1"); len(got) != 1 {
		t.Fatalf("rows = %d, want 1", len(got))
	}
}

func usersAreSeparate(t *testing.T, s device.Store) {
	mustSave(t, s, sealed("u1", key32(1), "h1.a"))
	mustSave(t, s, sealed("u2", key32(1), "h1.a"))

	if n := len(mustList(t, s, "app_a", "u1")); n != 1 {
		t.Errorf("u1 rows = %d, want 1", n)
	}

	if n := len(mustList(t, s, "app_a", "u2")); n != 1 {
		t.Errorf("u2 rows = %d, want 1", n)
	}
}

func roundTripsEveryField(t *testing.T, s device.Store) {
	d := &device.Device{
		AppID: "app_a", UserID: "u1", Channel: "push", Provider: "sealed",
		Address: "h1.abc", PublicKey: key32(7), Platform: "ios", Name: "Rex's iPhone",
		Locale: "en-GB", TimeZone: "Europe/London",
		QuietHours: &device.QuietHours{Start: "22:00", End: "07:00"},
		Settings:   map[string]string{"level.production": "critical", "push.live": "false"},
		LastSeenAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	mustSave(t, s, d)

	got, err := s.GetDevice(context.Background(), "app_a", d.ID)
	if err != nil {
		t.Fatalf("GetDevice: %v", err)
	}

	// Backends store timestamps at microsecond precision, so compare those
	// fields by instant and the rest structurally.
	for _, p := range []struct {
		name      string
		got, want time.Time
	}{
		{"LastSeenAt", got.LastSeenAt, d.LastSeenAt},
		{"CreatedAt", got.CreatedAt, d.CreatedAt},
		{"UpdatedAt", got.UpdatedAt, d.UpdatedAt},
	} {
		if !p.got.Equal(p.want) {
			t.Errorf("%s = %v, want %v", p.name, p.got, p.want)
		}
	}

	got.LastSeenAt, got.CreatedAt, got.UpdatedAt = d.LastSeenAt, d.CreatedAt, d.UpdatedAt
	if !reflect.DeepEqual(got, d) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, d)
	}
}

func getWrongAppIsNotFound(t *testing.T, s device.Store) {
	d := sealed("u1", key32(1), "h1.a")
	mustSave(t, s, d)

	if _, err := s.GetDevice(context.Background(), "app_b", d.ID); !errors.Is(err, device.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func deleteMissingIsNotFound(t *testing.T, s device.Store) {
	if err := s.DeleteDevice(context.Background(), "app_a", "hdev_missing"); !errors.Is(err, device.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func listScopedAndOrdered(t *testing.T, s device.Store) {
	var want []string

	for i := byte(1); i <= 3; i++ {
		d := sealed("u1", key32(i), fmt.Sprintf("h1.%d", i))
		mustSave(t, s, d)
		want = append(want, d.ID)
	}

	mustSave(t, s, sealed("u2", key32(9), "h1.other"))

	got := mustList(t, s, "app_a", "u1")
	if len(got) != len(want) {
		t.Fatalf("rows = %d, want %d", len(got), len(want))
	}

	for i, d := range got {
		if d.ID != want[i] {
			t.Errorf("position %d = %s, want %s", i, d.ID, want[i])
		}
	}
}

func deleteUserDevices(t *testing.T, s device.Store) {
	ctx := context.Background()

	mustSave(t, s, sealed("u1", key32(1), "h1.a"))
	mustSave(t, s, sealed("u1", key32(2), "h1.b"))
	mustSave(t, s, sealed("u2", key32(3), "h1.c"))

	if err := s.DeleteUserDevices(ctx, "app_a", "u1"); err != nil {
		t.Fatalf("DeleteUserDevices: %v", err)
	}

	if n := len(mustList(t, s, "app_a", "u1")); n != 0 {
		t.Errorf("u1 rows = %d, want 0", n)
	}

	if n := len(mustList(t, s, "app_a", "u2")); n != 1 {
		t.Errorf("u2 rows = %d, want 1", n)
	}

	if err := s.DeleteUserDevices(ctx, "app_a", "u1"); err != nil {
		t.Errorf("second DeleteUserDevices: %v", err)
	}
}

func concurrentSaveIsOneRow(t *testing.T, s device.Store) {
	const workers = 10

	var wg sync.WaitGroup

	errs := make(chan error, workers)

	for i := range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			errs <- s.SaveDevice(context.Background(), sealed("u1", key32(1), fmt.Sprintf("h1.%d", i)))
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("SaveDevice: %v", err)
		}
	}

	if n := len(mustList(t, s, "app_a", "u1")); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
}

func readsAreCopies(t *testing.T, s device.Store) {
	d := sealed("u1", key32(1), "h1.a")
	d.Settings = map[string]string{"k": "v"}
	d.QuietHours = &device.QuietHours{Start: "22:00", End: "07:00"}
	mustSave(t, s, d)

	first := mustList(t, s, "app_a", "u1")[0]
	first.Settings["k"] = "changed"
	first.PublicKey[0] = 0xff
	first.QuietHours.Start = "01:00"

	second := mustList(t, s, "app_a", "u1")[0]
	if second.Settings["k"] != "v" {
		t.Errorf("Settings leaked a mutation: %v", second.Settings)
	}

	if !bytes.Equal(second.PublicKey, key32(1)) {
		t.Error("PublicKey leaked a mutation")
	}

	if second.QuietHours.Start != "22:00" {
		t.Error("QuietHours leaked a mutation")
	}
}
