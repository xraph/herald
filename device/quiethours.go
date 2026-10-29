package device

import (
	"fmt"
	"time"
)

const minutesPerHour = 60

// QuietHours is a daily window of local wall-clock time, "HH:MM" on a 24-hour clock.
type QuietHours struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Contains reports whether t falls in [Start, End) on local wall-clock minutes
// in loc (UTC when nil). An End before Start wraps midnight. Start equal to End,
// or a value that does not parse, means never.
func (q QuietHours) Contains(t time.Time, loc *time.Location) bool {
	start, err := minutesOfDay(q.Start)
	if err != nil {
		return false
	}

	end, err := minutesOfDay(q.End)
	if err != nil || start == end {
		return false
	}

	if loc == nil {
		loc = time.UTC
	}

	local := t.In(loc)
	now := local.Hour()*minutesPerHour + local.Minute()

	if start < end {
		return now >= start && now < end
	}

	return now >= start || now < end
}

// Validate checks that both ends are "HH:MM" between 00:00 and 23:59.
func (q QuietHours) Validate() error {
	if _, err := minutesOfDay(q.Start); err != nil {
		return fmt.Errorf("%w: quiet hours start %q is not HH:MM", ErrInvalidDevice, q.Start)
	}

	if _, err := minutesOfDay(q.End); err != nil {
		return fmt.Errorf("%w: quiet hours end %q is not HH:MM", ErrInvalidDevice, q.End)
	}

	return nil
}

func minutesOfDay(s string) (int, error) {
	if len(s) != len("15:04") {
		return 0, ErrInvalidDevice
	}

	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, err //nolint:wrapcheck // callers only test for failure
	}

	return t.Hour()*minutesPerHour + t.Minute(), nil
}
