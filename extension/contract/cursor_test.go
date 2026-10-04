package contract

import "testing"

func TestCursorRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 25, 10000} {
		got, err := decodeCursor(encodeCursor(n))
		if err != nil || got != n {
			t.Errorf("round trip %d: got %d, %v", n, got, err)
		}
	}
	if got, err := decodeCursor(""); err != nil || got != 0 {
		t.Errorf("empty cursor: got %d, %v; want 0", got, err)
	}
	for _, bad := range []string{"not-base64!", encodeRaw("x:5"), encodeRaw("o:-1"), encodeRaw("o:abc")} {
		if _, err := decodeCursor(bad); codeOf(err) != "BAD_REQUEST" {
			t.Errorf("decodeCursor(%q) = %v, want BAD_REQUEST", bad, err)
		}
	}
}

func TestPageLimit(t *testing.T) {
	cases := []struct{ in, want int }{{0, 25}, {-3, 25}, {10, 10}, {500, 100}}
	for _, c := range cases {
		if got := pageLimit(c.in, 25, 100); got != c.want {
			t.Errorf("pageLimit(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestNextCursor(t *testing.T) {
	if got := nextCursor(0, 25, 25); got != "" {
		t.Errorf("a full page with nothing beyond it: got %q, want none", got)
	}
	got := nextCursor(25, 25, 26)
	off, err := decodeCursor(got)
	if err != nil || off != 50 {
		t.Errorf("probe row present: cursor %q decodes to %d, %v; want 50", got, off, err)
	}
}
