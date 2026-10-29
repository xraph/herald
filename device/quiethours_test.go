package device_test

import (
	"errors"
	"testing"
	"time"

	"github.com/xraph/herald/device"
)

func TestQuietHoursContains(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}

	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}

	at := func(hhmm string) time.Time {
		v, perr := time.Parse("2006-01-02 15:04", "2026-10-28 "+hhmm)
		if perr != nil {
			t.Fatal(perr)
		}

		return v
	}
	utc := func(s string) time.Time {
		v, perr := time.Parse(time.RFC3339, s)
		if perr != nil {
			t.Fatal(perr)
		}

		return v
	}
	night := device.QuietHours{Start: "22:00", End: "07:00"}

	tests := []struct {
		name string
		q    device.QuietHours
		at   time.Time
		loc  *time.Location
		want bool
	}{
		{"wrap late evening", night, at("23:30"), time.UTC, true},
		{"wrap early morning", night, at("06:59"), time.UTC, true},
		{"end is exclusive", night, at("07:00"), time.UTC, false},
		{"just before start", night, at("21:59"), time.UTC, false},
		{"start is inclusive", night, at("22:00"), time.UTC, true},
		{"same-day inside", device.QuietHours{Start: "13:00", End: "15:00"}, at("14:00"), time.UTC, true},
		{"same-day end exclusive", device.QuietHours{Start: "13:00", End: "15:00"}, at("15:00"), time.UTC, false},
		{"start equals end", device.QuietHours{Start: "09:00", End: "09:00"}, at("09:00"), time.UTC, false},
		{"unparsable", device.QuietHours{Start: "25:00", End: "07:00"}, at("23:30"), time.UTC, false},
		{"tokyo local 23:30", night, utc("2026-10-28T14:30:00Z"), tokyo, true},
		{"new york local 10:30", night, utc("2026-10-28T14:30:00Z"), ny, false},
		{"new york 01:30 EST before spring forward", night, utc("2026-03-08T06:30:00Z"), ny, true},
		{"new york 07:30 EDT after spring forward", night, utc("2026-03-08T11:30:00Z"), ny, false},
		{"nil location is UTC", night, at("23:30"), nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.q.Contains(tc.at, tc.loc); got != tc.want {
				t.Fatalf("Contains = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestQuietHoursValidate(t *testing.T) {
	if err := (device.QuietHours{Start: "22:00", End: "07:00"}).Validate(); err != nil {
		t.Fatalf("valid hours: %v", err)
	}

	for _, q := range []device.QuietHours{
		{Start: "7:00", End: "22:00"},
		{Start: "24:00", End: "07:00"},
		{Start: "22:60", End: "07:00"},
		{Start: "", End: "07:00"},
	} {
		if err := q.Validate(); !errors.Is(err, device.ErrInvalidDevice) {
			t.Errorf("%+v: err = %v, want ErrInvalidDevice", q, err)
		}
	}
}
