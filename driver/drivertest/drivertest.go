// Package drivertest provides shared HTTP mock-server helpers for driver
// unit tests. It is intentionally stdlib-only so opt-in driver modules can
// import it without adding dependencies.
package drivertest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// CapturedRequest holds the details of the last request the mock server saw.
type CapturedRequest struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   []byte
}

// DecodeJSON unmarshals the captured body into v, failing the test on error.
func (c *CapturedRequest) DecodeJSON(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(c.Body, v); err != nil {
		t.Fatalf("drivertest: decode JSON body: %v (body=%q)", err, string(c.Body))
	}
}

// FormValue parses the captured body as a URL-encoded form and returns key.
func (c *CapturedRequest) FormValue(t *testing.T, key string) string {
	t.Helper()
	vals, err := url.ParseQuery(string(c.Body))
	if err != nil {
		t.Fatalf("drivertest: parse form body: %v", err)
	}
	return vals.Get(key)
}

// Server wraps an httptest.Server that records the request it received and
// replies with a fixed status code and body.
type Server struct {
	*httptest.Server
	Captured *CapturedRequest
}

// NewServer starts a mock HTTP server. Every request is recorded into
// Captured; the server replies with status and respBody. extraHeaders set on
// the response can be added by the caller via the returned server if needed.
func NewServer(t *testing.T, status int, respBody string) *Server {
	t.Helper()
	captured := &CapturedRequest{}
	s := &Server{Captured: captured}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body) //nolint:errcheck // test body read
		captured.Method = r.Method
		captured.Path = r.URL.Path
		captured.Query = r.URL.RawQuery
		captured.Header = r.Header.Clone()
		captured.Body = body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody) //nolint:errcheck // test response write
	}))
	t.Cleanup(s.Close)
	return s
}
