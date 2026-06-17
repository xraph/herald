package drivertest_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/xraph/herald/driver/drivertest"
)

func TestServerCapturesRequest(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"ok":true}`)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/path?x=1", strings.NewReader(`{"a":"b"}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer k")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if srv.Captured.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", srv.Captured.Method)
	}
	if srv.Captured.Path != "/path" {
		t.Errorf("path = %q, want /path", srv.Captured.Path)
	}
	if srv.Captured.Query != "x=1" {
		t.Errorf("query = %q, want x=1", srv.Captured.Query)
	}
	if got := srv.Captured.Header.Get("Authorization"); got != "Bearer k" {
		t.Errorf("auth = %q, want Bearer k", got)
	}
	var body struct{ A string }
	srv.Captured.DecodeJSON(t, &body)
	if body.A != "b" {
		t.Errorf("body.A = %q, want b", body.A)
	}
}
