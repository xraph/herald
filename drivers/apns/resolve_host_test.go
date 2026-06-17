package apns

import "testing"

func TestResolveHost(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		sandbox bool
		want    string
	}{
		{"default prod", "", false, "https://api.push.apple.com"},
		{"default sandbox", "", true, "https://api.sandbox.push.apple.com"},
		{"override wins over prod", "http://127.0.0.1:9", false, "http://127.0.0.1:9"},
		{"override wins over sandbox", "http://127.0.0.1:9", true, "http://127.0.0.1:9"},
	}
	for _, tc := range cases {
		if got := resolveHost(tc.baseURL, tc.sandbox); got != tc.want {
			t.Errorf("%s: resolveHost(%q,%v) = %q, want %q", tc.name, tc.baseURL, tc.sandbox, got, tc.want)
		}
	}
}
