# Cloudflare Email, Driver Test Backfill, and config.yaml Providers — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an outbound Cloudflare Email Service driver, backfill unit tests for every Herald driver, and let operators declare providers in the extension's `config.yaml`.

**Architecture:** A shared stdlib-only `driver/drivertest` helper (in the core module, exported) gives every driver test an `httptest`-based mock server. Each HTTP driver is made endpoint-injectable via `msg.Data["base_url"]` so tests can point it at the mock. Cloudflare ships as a new opt-in module `drivers/cloudflare`. The extension gains a `Providers` config section seeded into the store (seed-if-absent).

**Tech Stack:** Go 1.25.7, standard library (`net/http`, `net/http/httptest`, `net/smtp`, `crypto/*`), Herald core packages (`driver`, `message`, `provider`, `id`, `store/memory`).

**Design spec:** [docs/superpowers/specs/2026-06-17-cloudflare-email-and-driver-tests-design.md](../specs/2026-06-17-cloudflare-email-and-driver-tests-design.md)

## Global Constraints

- Go version floor: **1.25.7** (every `go.mod`).
- Opt-in drivers live in `drivers/<name>/` as their **own module** with `replace github.com/xraph/herald => ../../`. The core module must not import them.
- Drivers are **stdlib-only** except `drivers/webhook` (which uses `github.com/xraph/relay`).
- Error strings are prefixed with the driver name: `fmt.Errorf("ses: ...")`. Keep the existing `//nolint:errcheck` comments on best-effort body reads/decodes.
- A driver leaves `ProviderMessageID` empty when its API returns no single id (matches the SMTP driver).
- Commit messages use Conventional Commits (`feat:`, `fix:`, `test:`, `refactor:`, `docs:`). **Never** add a `Co-Authored-By` trailer (user rule).
- `base_url` overrides must preserve each driver's current production URL as the default — behavior is unchanged when `base_url` is unset.
- Run a driver module's tests from inside its own directory (`cd drivers/<name> && go test ./...`); run core tests from the repo root (`go test ./...`).

---

## Task 1: `driver/drivertest` mock-server helper

**Files:**
- Create: `driver/drivertest/drivertest.go`
- Test: `driver/drivertest/drivertest_test.go`

**Interfaces:**
- Produces:
  - `type CapturedRequest struct { Method, Path, Query string; Header http.Header; Body []byte }`
  - `func (c *CapturedRequest) DecodeJSON(t *testing.T, v any)`
  - `func (c *CapturedRequest) FormValue(t *testing.T, key string) string`
  - `type Server struct { *httptest.Server; Captured *CapturedRequest }`
  - `func NewServer(t *testing.T, status int, respBody string) *Server`

- [ ] **Step 1: Write the failing test**

Create `driver/drivertest/drivertest_test.go`:

```go
package drivertest_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/xraph/herald/driver/drivertest"
)

func TestServerCapturesRequest(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"ok":true}`)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/path?x=1", strings.NewReader(`{"a":"b"}`))
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./driver/drivertest/ -run TestServerCapturesRequest -v`
Expected: FAIL — build error, package `drivertest` has no `NewServer`.

- [ ] **Step 3: Write the helper**

Create `driver/drivertest/drivertest.go`:

```go
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
```

Note: some drivers (sendgrid, apns) return their message id in a **response header**, not the body. Those tests start the server directly with `httptest.NewServer` and set the header in the handler; `NewServer` covers the common body case. Add a header-capable variant only if a second task needs it — do not pre-build it (YAGNI).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./driver/drivertest/ -run TestServerCapturesRequest -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add driver/drivertest/
git commit -m "test: add shared driver/drivertest mock-server helper"
```

---

## Task 2: `drivers/cloudflare` — Cloudflare Email Service driver (test-first)

**Files:**
- Create: `drivers/cloudflare/go.mod`, `drivers/cloudflare/cloudflare.go`, `drivers/cloudflare/cloudflare_test.go`
- Generated: `drivers/cloudflare/go.sum` (via `go mod tidy`)

**Interfaces:**
- Consumes: `driver.Driver`, `driver.OutboundMessage`, `driver.DeliveryResult`, `message.StatusSent`, `drivertest.NewServer`.
- Produces: `type Driver struct{}` implementing `driver.Driver`; `Name()=="cloudflare"`, `Channel()=="email"`. Credentials: `api_token`, `account_id`, optional `base_url`.

- [ ] **Step 1: Create the module skeleton**

Create `drivers/cloudflare/go.mod`:

```
module github.com/xraph/herald/drivers/cloudflare

go 1.25.7

require github.com/xraph/herald v0.0.0

replace github.com/xraph/herald => ../../
```

Create a minimal `drivers/cloudflare/cloudflare.go` so the module compiles:

```go
// Package cloudflare provides a Herald driver for the Cloudflare Email Service.
package cloudflare

import (
	"context"
	"fmt"

	"github.com/xraph/herald/driver"
)

// Driver delivers email via the Cloudflare Email Service REST API.
type Driver struct{}

var _ driver.Driver = (*Driver)(nil)

func (d *Driver) Name() string    { return "cloudflare" }
func (d *Driver) Channel() string { return "email" }

func (d *Driver) Validate(credentials, _ map[string]string) error {
	if credentials["api_token"] == "" {
		return fmt.Errorf("cloudflare: missing required credential 'api_token'")
	}
	if credentials["account_id"] == "" {
		return fmt.Errorf("cloudflare: missing required credential 'account_id'")
	}
	return nil
}

func (d *Driver) Send(_ context.Context, _ *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	return nil, fmt.Errorf("cloudflare: not implemented")
}
```

Run: `cd drivers/cloudflare && go mod tidy && go build ./... && cd ../..`
Expected: builds; `go.sum` is created.

- [ ] **Step 2: Write the failing tests**

Create `drivers/cloudflare/cloudflare_test.go`:

```go
package cloudflare_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/cloudflare"
	"github.com/xraph/herald/message"
)

func TestSendHappyPath(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK,
		`{"success":true,"errors":[],"result":{"delivered":["to@example.com"],"permanent_bounces":[],"queued":[]}}`)

	d := &cloudflare.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To:      "to@example.com",
		From:    "from@example.com",
		Subject: "Hi",
		HTML:    "<b>hi</b>",
		Text:    "hi",
		Data: map[string]string{
			"api_token":  "tok",
			"account_id": "acct123",
			"base_url":   srv.URL,
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent {
		t.Errorf("status = %q, want sent", res.Status)
	}
	if srv.Captured.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", srv.Captured.Method)
	}
	if srv.Captured.Path != "/accounts/acct123/email/sending/send" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	if got := srv.Captured.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("auth = %q, want Bearer tok", got)
	}
	var body map[string]string
	srv.Captured.DecodeJSON(t, &body)
	if body["to"] != "to@example.com" || body["from"] != "from@example.com" || body["subject"] != "Hi" {
		t.Errorf("body = %+v", body)
	}
	if body["html"] != "<b>hi</b>" {
		t.Errorf("html = %q", body["html"])
	}
}

func TestSendFromNameFormatted(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"success":true,"result":{"delivered":["to@example.com"]}}`)
	d := &cloudflare.Driver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@example.com", From: "from@example.com", FromName: "Acme",
		Data: map[string]string{"api_token": "t", "account_id": "a", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	var body map[string]string
	srv.Captured.DecodeJSON(t, &body)
	if body["from"] != "Acme <from@example.com>" {
		t.Errorf("from = %q, want \"Acme <from@example.com>\"", body["from"])
	}
}

func TestSendAPIError(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusBadRequest, `{"success":false,"errors":[{"message":"bad domain"}]}`)
	d := &cloudflare.Driver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@example.com", From: "from@example.com",
		Data: map[string]string{"api_token": "t", "account_id": "a", "base_url": srv.URL},
	})
	if err == nil {
		t.Fatal("expected error on 400, got nil")
	}
}

func TestSendPermanentBounce(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK,
		`{"success":true,"result":{"delivered":[],"permanent_bounces":["to@example.com"],"queued":[]}}`)
	d := &cloudflare.Driver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@example.com", From: "from@example.com",
		Data: map[string]string{"api_token": "t", "account_id": "a", "base_url": srv.URL},
	})
	if err == nil {
		t.Fatal("expected error on permanent bounce, got nil")
	}
}

func TestValidate(t *testing.T) {
	d := &cloudflare.Driver{}
	if err := d.Validate(map[string]string{"account_id": "a"}, nil); err == nil {
		t.Error("expected error for missing api_token")
	}
	if err := d.Validate(map[string]string{"api_token": "t"}, nil); err == nil {
		t.Error("expected error for missing account_id")
	}
	if err := d.Validate(map[string]string{"api_token": "t", "account_id": "a"}, nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd drivers/cloudflare && go test ./... ; cd ../..`
Expected: FAIL — `Send` returns "not implemented".

- [ ] **Step 4: Implement `Send`**

Replace the stub `Send` in `drivers/cloudflare/cloudflare.go` and add imports:

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/message"
)
```

```go
type cfRequest struct {
	To      string `json:"to"`
	From    string `json:"from"`
	Subject string `json:"subject"`
	HTML    string `json:"html,omitempty"`
	Text    string `json:"text,omitempty"`
}

type cfResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Delivered        []string `json:"delivered"`
		PermanentBounces []string `json:"permanent_bounces"`
		Queued           []string `json:"queued"`
	} `json:"result"`
}

func (d *Driver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	apiToken := msg.Data["api_token"]
	accountID := msg.Data["account_id"]
	baseURL := msg.Data["base_url"]
	if baseURL == "" {
		baseURL = "https://api.cloudflare.com/client/v4"
	}

	from := msg.From
	if msg.FromName != "" {
		from = msg.FromName + " <" + msg.From + ">"
	}

	body := cfRequest{
		To:      msg.To,
		From:    from,
		Subject: msg.Subject,
		HTML:    msg.HTML,
		Text:    msg.Text,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/accounts/%s/email/sending/send", baseURL, accountID)
	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("cloudflare: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("cloudflare: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result cfResponse
	_ = json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck // best-effort response parse

	if len(result.Result.PermanentBounces) > 0 {
		return nil, fmt.Errorf("cloudflare: permanent bounce for %v", result.Result.PermanentBounces)
	}

	return &driver.DeliveryResult{
		Status: message.StatusSent,
	}, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd drivers/cloudflare && go test ./... -v ; cd ../..`
Expected: PASS (all five tests).

- [ ] **Step 6: Commit**

```bash
git add drivers/cloudflare/
git commit -m "feat(drivers): add Cloudflare Email Service driver"
```

---

## Task 3: `ProviderConfig` type and mapper (config.yaml support, part 1)

**Files:**
- Create: `extension/providers.go`
- Test: `extension/providers_test.go`

**Interfaces:**
- Consumes: `provider.Provider`, `id.NewProviderID`.
- Produces:
  - `type ProviderConfig struct { Name, Channel, Driver, AppID string; Credentials, Settings map[string]string; Priority int; Enabled *bool }`
  - `func (pc ProviderConfig) toProvider(now time.Time) provider.Provider`
  - `func providersFromConfig(pcs []ProviderConfig, now time.Time) []provider.Provider`

- [ ] **Step 1: Write the failing test**

Create `extension/providers_test.go`:

```go
package extension

import (
	"testing"
	"time"
)

func boolPtr(b bool) *bool { return &b }

func TestToProviderDefaults(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	pc := ProviderConfig{
		Name:        "cf-email",
		Channel:     "email",
		Driver:      "cloudflare",
		Credentials: map[string]string{"api_token": "t"},
		Priority:    5,
		// Enabled left nil -> should default to true
	}
	p := pc.toProvider(now)

	if p.Name != "cf-email" || p.Channel != "email" || p.Driver != "cloudflare" {
		t.Errorf("core fields wrong: %+v", p)
	}
	if p.Priority != 5 {
		t.Errorf("priority = %d, want 5", p.Priority)
	}
	if !p.Enabled {
		t.Error("Enabled should default to true when nil")
	}
	if p.Credentials["api_token"] != "t" {
		t.Errorf("credentials not mapped: %+v", p.Credentials)
	}
	if p.ID.String() == "" {
		t.Error("ID should be generated")
	}
	if !p.CreatedAt.Equal(now) || !p.UpdatedAt.Equal(now) {
		t.Error("timestamps should be set to now")
	}
}

func TestToProviderExplicitDisabled(t *testing.T) {
	p := ProviderConfig{Name: "x", Enabled: boolPtr(false)}.toProvider(time.Unix(0, 0))
	if p.Enabled {
		t.Error("Enabled should be false when explicitly set")
	}
}

func TestProvidersFromConfig(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	out := providersFromConfig([]ProviderConfig{{Name: "a"}, {Name: "b"}}, now)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].Name != "a" || out[1].Name != "b" {
		t.Errorf("names = %q,%q", out[0].Name, out[1].Name)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./extension/ -run TestToProvider -v`
Expected: FAIL — `ProviderConfig` / `toProvider` undefined.

- [ ] **Step 3: Implement the type and mapper**

Create `extension/providers.go`:

```go
package extension

import (
	"time"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
)

// ProviderConfig declares a notification provider in config.yaml. It is
// seeded into the store on startup (see SeedConfiguredProviders).
type ProviderConfig struct {
	Name        string            `json:"name" yaml:"name" mapstructure:"name"`
	Channel     string            `json:"channel" yaml:"channel" mapstructure:"channel"`
	Driver      string            `json:"driver" yaml:"driver" mapstructure:"driver"`
	AppID       string            `json:"app_id" yaml:"app_id" mapstructure:"app_id"`
	Credentials map[string]string `json:"credentials" yaml:"credentials" mapstructure:"credentials"`
	Settings    map[string]string `json:"settings" yaml:"settings" mapstructure:"settings"`
	Priority    int               `json:"priority" yaml:"priority" mapstructure:"priority"`
	Enabled     *bool             `json:"enabled" yaml:"enabled" mapstructure:"enabled"`
}

// toProvider maps the config entry to a provider.Provider. Enabled defaults to
// true when unset; ID and timestamps are assigned here.
func (pc ProviderConfig) toProvider(now time.Time) provider.Provider {
	enabled := true
	if pc.Enabled != nil {
		enabled = *pc.Enabled
	}
	return provider.Provider{
		ID:          id.NewProviderID(),
		AppID:       pc.AppID,
		Name:        pc.Name,
		Channel:     pc.Channel,
		Driver:      pc.Driver,
		Credentials: pc.Credentials,
		Settings:    pc.Settings,
		Priority:    pc.Priority,
		Enabled:     enabled,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func providersFromConfig(pcs []ProviderConfig, now time.Time) []provider.Provider {
	out := make([]provider.Provider, 0, len(pcs))
	for _, pc := range pcs {
		out = append(out, pc.toProvider(now))
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./extension/ -run "TestToProvider|TestProvidersFromConfig" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add extension/providers.go extension/providers_test.go
git commit -m "feat(extension): add ProviderConfig type and provider mapper"
```

---

## Task 4: `Herald.SeedConfiguredProviders` (config.yaml support, part 2)

**Files:**
- Modify: `herald.go` (add method near `SeedDefaultProviders`, ~line 339)
- Test: `seed_providers_test.go` (new, package `herald`, repo root)

**Interfaces:**
- Consumes: `provider.Provider`, the registry (`h.drivers`), `h.store` (`provider.Store` methods `ListAllProviders`, `CreateProvider`), `memory.New` (test only).
- Produces: `func (h *Herald) SeedConfiguredProviders(ctx context.Context, providers []provider.Provider) error`

- [ ] **Step 1: Write the failing test**

Create `seed_providers_test.go` at the repo root:

```go
package herald

import (
	"context"
	"testing"

	"github.com/xraph/herald/driver/email"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/store/memory"
)

func newTestHerald(t *testing.T) (*Herald, *memory.Store) {
	t.Helper()
	st := memory.New()
	h, err := New(WithStore(st), WithDriver(&email.ResendDriver{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h, st
}

func TestSeedConfiguredProvidersCreates(t *testing.T) {
	h, st := newTestHerald(t)
	p := provider.Provider{ID: id.NewProviderID(), Name: "resend-main", Channel: "email", Driver: "resend", Enabled: true}

	if err := h.SeedConfiguredProviders(context.Background(), []provider.Provider{p}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, _ := st.ListAllProviders(context.Background(), "")
	if len(got) != 1 {
		t.Fatalf("provider count = %d, want 1", len(got))
	}
	if got[0].Name != "resend-main" {
		t.Errorf("name = %q", got[0].Name)
	}
}

func TestSeedConfiguredProvidersSkipsExisting(t *testing.T) {
	h, st := newTestHerald(t)
	p := provider.Provider{ID: id.NewProviderID(), Name: "resend-main", Channel: "email", Driver: "resend", Enabled: true}

	// First seed creates it.
	_ = h.SeedConfiguredProviders(context.Background(), []provider.Provider{p})
	// Second seed (different ID, same name) must be skipped.
	p2 := provider.Provider{ID: id.NewProviderID(), Name: "resend-main", Channel: "email", Driver: "resend", Enabled: true}
	if err := h.SeedConfiguredProviders(context.Background(), []provider.Provider{p2}); err != nil {
		t.Fatalf("seed 2: %v", err)
	}
	got, _ := st.ListAllProviders(context.Background(), "")
	if len(got) != 1 {
		t.Fatalf("provider count = %d, want 1 (no duplicate)", len(got))
	}
}

func TestSeedConfiguredProvidersUnregisteredDriverDoesNotFail(t *testing.T) {
	h, st := newTestHerald(t)
	p := provider.Provider{ID: id.NewProviderID(), Name: "ghost", Channel: "email", Driver: "no-such-driver", Enabled: true}
	// Unknown driver: warn, still create the record, never error.
	if err := h.SeedConfiguredProviders(context.Background(), []provider.Provider{p}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, _ := st.ListAllProviders(context.Background(), "")
	if len(got) != 1 {
		t.Errorf("provider count = %d, want 1", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestSeedConfiguredProviders -v`
Expected: FAIL — `SeedConfiguredProviders` undefined.

- [ ] **Step 3: Implement the method**

In `herald.go`, immediately after the `SeedDefaultProviders` method, add:

```go
// SeedConfiguredProviders persists providers declared in configuration into
// the store, seed-if-absent: a provider is created only when no provider of
// the same name exists for its app. Existing records (including dashboard
// edits) are left untouched. A missing driver or failed Validate is logged as
// a warning, never an error.
func (h *Herald) SeedConfiguredProviders(ctx context.Context, providers []provider.Provider) error {
	for i := range providers {
		p := providers[i]

		existing, _ := h.store.ListAllProviders(ctx, p.AppID) //nolint:errcheck // skip lookup failures
		duplicate := false
		for _, e := range existing {
			if e.Name == p.Name {
				duplicate = true
				break
			}
		}
		if duplicate {
			h.logger.Info("herald: configured provider already exists, skipping",
				"name", p.Name, "app_id", p.AppID)
			continue
		}

		if drv, err := h.drivers.Get(p.Driver); err != nil {
			h.logger.Warn("herald: configured provider references unregistered driver",
				"name", p.Name, "driver", p.Driver)
		} else if vErr := drv.Validate(p.Credentials, p.Settings); vErr != nil {
			h.logger.Warn("herald: configured provider failed driver validation",
				"name", p.Name, "driver", p.Driver, "error", vErr)
		}

		if err := h.store.CreateProvider(ctx, &p); err != nil {
			h.logger.Warn("herald: failed to seed configured provider",
				"name", p.Name, "error", err)
			continue
		}
		h.logger.Info("herald: seeded configured provider",
			"name", p.Name, "channel", p.Channel, "driver", p.Driver, "provider_id", p.ID.String())
	}
	return nil
}
```

Verify `provider` and `context` are already imported in `herald.go` (they are — `SeedDefaultProviders` uses both).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run TestSeedConfiguredProviders -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add herald.go seed_providers_test.go
git commit -m "feat: add Herald.SeedConfiguredProviders (seed-if-absent)"
```

---

## Task 5: Wire `Providers` config into the extension (config.yaml support, part 3)

**Files:**
- Modify: `extension/config.go` (add `Providers` field)
- Modify: `extension/extension.go` (call seeding after `SeedDefaultProviders`, ~line 162-165)

**Interfaces:**
- Consumes: `e.config.Providers` ([]ProviderConfig), `providersFromConfig`, `Herald.SeedConfiguredProviders`, `time.Now`.

- [ ] **Step 1: Add the `Providers` field**

In `extension/config.go`, add to the `Config` struct (after `GroveDatabase`):

```go
	// Providers declares notification providers seeded into the store on
	// startup (seed-if-absent). Credentials may use ${ENV} interpolation.
	Providers []ProviderConfig `json:"providers" yaml:"providers" mapstructure:"providers"`
```

- [ ] **Step 2: Call seeding from Init**

In `extension/extension.go`, locate the block (around line 162):

```go
	// Seed default providers for built-in drivers (e.g. inapp).
	if err := e.h.SeedDefaultProviders(context.Background(), ""); err != nil {
		e.Logger().Warn("herald: failed to seed default providers", forge.Error(err))
	}
```

Immediately after it, add:

```go
	// Seed providers declared in config.yaml (seed-if-absent).
	if len(e.config.Providers) > 0 {
		seeded := providersFromConfig(e.config.Providers, time.Now())
		if err := e.h.SeedConfiguredProviders(context.Background(), seeded); err != nil {
			e.Logger().Warn("herald: failed to seed configured providers", forge.Error(err))
		}
	}
```

Add `"time"` to the imports in `extension/extension.go`.

- [ ] **Step 3: Verify it builds and existing tests pass**

Run: `go build ./... && go test ./extension/ -v`
Expected: builds; extension tests PASS.

- [ ] **Step 4: Add a usage example to the spec/docs comment**

In `extension/providers.go`, confirm the `ProviderConfig` doc comment references config.yaml (already added in Task 3). No code change needed if present.

- [ ] **Step 5: Commit**

```bash
git add extension/config.go extension/extension.go
git commit -m "feat(extension): seed config.yaml providers on startup"
```

---

## Phase B1 — email driver tests

### Task 6: `resend` test (already endpoint-injectable)

**Files:** Test: `driver/email/resend_test.go`

- [ ] **Step 1: Write the test**

```go
package email_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/driver/email"
	"github.com/xraph/herald/message"
)

func TestResendSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"id":"re_123"}`)
	d := &email.ResendDriver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", From: "from@x.com", Subject: "Hi", HTML: "<b>x</b>",
		Data: map[string]string{"api_key": "k", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "re_123" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/emails" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	if got := srv.Captured.Header.Get("Authorization"); got != "Bearer k" {
		t.Errorf("auth = %q", got)
	}
	var body struct {
		To []string `json:"to"`
	}
	srv.Captured.DecodeJSON(t, &body)
	if len(body.To) != 1 || body.To[0] != "to@x.com" {
		t.Errorf("to = %v", body.To)
	}
}

func TestResendValidate(t *testing.T) {
	d := &email.ResendDriver{}
	if err := d.Validate(map[string]string{}, nil); err == nil {
		t.Error("expected error for missing api_key")
	}
	if err := d.Validate(map[string]string{"api_key": "k"}, nil); err != nil {
		t.Errorf("unexpected: %v", err)
	}
}

func TestResendAPIError(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusUnauthorized, `{"message":"bad key"}`)
	d := &email.ResendDriver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", From: "from@x.com",
		Data: map[string]string{"api_key": "k", "base_url": srv.URL},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Run — expect PASS** (resend already honors `base_url`)

Run: `go test ./driver/email/ -run TestResend -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add driver/email/resend_test.go
git commit -m "test(driver): cover resend email driver"
```

### Task 7: `smtp` test (mock SMTP listener)

**Files:** Test: `driver/email/smtp_test.go`

- [ ] **Step 1: Write the test with an in-process SMTP server**

```go
package email_test

import (
	"bufio"
	"context"
	"net"
	"net/textproto"
	"strings"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/email"
	"github.com/xraph/herald/message"
)

// startMockSMTP starts a minimal SMTP server on 127.0.0.1 that accepts one
// message and returns the raw DATA payload via the channel.
func startMockSMTP(t *testing.T) (host, port string, dataCh <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ch := make(chan string, 1)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		tc := textproto.NewConn(conn)
		_ = tc.PrintfLine("220 mock ESMTP")
		var data strings.Builder
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				_ = tc.PrintfLine("250 mock")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				_ = tc.PrintfLine("250 OK")
			case strings.HasPrefix(cmd, "DATA"):
				_ = tc.PrintfLine("354 End data with <CR><LF>.<CR><LF>")
				for {
					dl, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					if strings.TrimRight(dl, "\r\n") == "." {
						break
					}
					data.WriteString(dl)
				}
				_ = tc.PrintfLine("250 OK queued")
			case strings.HasPrefix(cmd, "QUIT"):
				_ = tc.PrintfLine("221 Bye")
				ch <- data.String()
				return
			}
		}
	}()

	h, p, _ := net.SplitHostPort(ln.Addr().String())
	return h, p, ch
}

func TestSMTPSend(t *testing.T) {
	host, port, dataCh := startMockSMTP(t)
	d := &email.SMTPDriver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", From: "from@x.com", Subject: "Hello", Text: "body text",
		Data: map[string]string{"host": host, "port": port}, // no username -> no auth
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent {
		t.Errorf("status = %q", res.Status)
	}
	payload := <-dataCh
	if !strings.Contains(payload, "Subject: Hello") {
		t.Errorf("missing subject in payload:\n%s", payload)
	}
	if !strings.Contains(payload, "To: to@x.com") {
		t.Errorf("missing To in payload:\n%s", payload)
	}
	if !strings.Contains(payload, "body text") {
		t.Errorf("missing body in payload:\n%s", payload)
	}
}

func TestSMTPValidate(t *testing.T) {
	d := &email.SMTPDriver{}
	if err := d.Validate(map[string]string{"host": "h"}, nil); err == nil {
		t.Error("expected error for missing port")
	}
	if err := d.Validate(map[string]string{"host": "h", "port": "25"}, nil); err != nil {
		t.Errorf("unexpected: %v", err)
	}
}
```

- [ ] **Step 2: Run — expect PASS**

Run: `go test ./driver/email/ -run TestSMTP -v`
Expected: PASS. If it hangs, the mock server's command handling is wrong — verify EHLO returns a single `250` line with no advertised AUTH/STARTTLS.

- [ ] **Step 3: Commit**

```bash
git add driver/email/smtp_test.go
git commit -m "test(driver): cover smtp email driver with mock SMTP server"
```

### Task 8: `mailgun` test (already injectable)

**Files:** Test: `drivers/mailgun/mailgun_test.go`

- [ ] **Step 1: Write the test**

```go
package mailgun_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/mailgun"
	"github.com/xraph/herald/message"
)

func TestMailgunSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"id":"<msg@mg>"}`)
	d := &mailgun.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", From: "from@x.com", Subject: "Hi", Text: "t",
		Data: map[string]string{"api_key": "key", "domain": "mg.example.com", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "<msg@mg>" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/v3/mg.example.com/messages" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	user, pass, ok := parseBasicAuth(srv.Captured.Header.Get("Authorization"))
	if !ok || user != "api" || pass != "key" {
		t.Errorf("basic auth = %q/%q ok=%v", user, pass, ok)
	}
	if got := srv.Captured.FormValue(t, "to"); got != "to@x.com" {
		t.Errorf("form to = %q", got)
	}
}

func TestMailgunValidate(t *testing.T) {
	d := &mailgun.Driver{}
	if err := d.Validate(map[string]string{"api_key": "k"}, nil); err == nil {
		t.Error("expected error for missing domain")
	}
	if err := d.Validate(map[string]string{"api_key": "k", "domain": "d"}, nil); err != nil {
		t.Errorf("unexpected: %v", err)
	}
}
```

Add this shared helper at the bottom of the same test file (basic-auth decode without importing internal packages):

```go
func parseBasicAuth(header string) (user, pass string, ok bool) {
	const prefix = "Basic "
	if len(header) < len(prefix) {
		return "", "", false
	}
	r, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", header)
	return r.BasicAuth()
}
```

- [ ] **Step 2: Run — expect PASS**

Run: `cd drivers/mailgun && go test ./... -v ; cd ../..`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add drivers/mailgun/mailgun_test.go
git commit -m "test(drivers): cover mailgun email driver"
```

### Task 9: `sendgrid` — add `base_url`, then test

**Files:** Modify `drivers/sendgrid/sendgrid.go`; Test `drivers/sendgrid/sendgrid_test.go`

- [ ] **Step 1: Write the failing test**

```go
package sendgrid_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/drivers/sendgrid"
	"github.com/xraph/herald/message"
)

func TestSendgridSend(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = readAll(r)
		w.Header().Set("X-Message-Id", "sg-1")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	d := &sendgrid.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", From: "from@x.com", FromName: "Acme", Subject: "Hi", HTML: "<b>x</b>",
		Data: map[string]string{"api_key": "K", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "sg-1" {
		t.Errorf("res = %+v", res)
	}
	if gotPath != "/v3/mail/send" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer K" {
		t.Errorf("auth = %q", gotAuth)
	}
	if len(gotBody) == 0 {
		t.Error("empty body")
	}
}
```

Add a tiny `readAll` helper in the same file:

```go
import "io"

func readAll(r *http.Request) ([]byte, error) { return io.ReadAll(r.Body) }
```

- [ ] **Step 2: Run — expect FAIL** (driver ignores `base_url`, hits real SendGrid)

Run: `cd drivers/sendgrid && go test ./... -run TestSendgridSend ; cd ../..`
Expected: FAIL (network error or non-202).

- [ ] **Step 3: Add `base_url` override**

In `drivers/sendgrid/sendgrid.go`, inside `Send`, replace the hardcoded URL. Change:

```go
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.sendgrid.com/v3/mail/send", bytes.NewReader(body))
```

to:

```go
	baseURL := msg.Data["base_url"]
	if baseURL == "" {
		baseURL = "https://api.sendgrid.com"
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v3/mail/send", bytes.NewReader(body))
```

- [ ] **Step 4: Run — expect PASS**

Run: `cd drivers/sendgrid && go test ./... -v ; cd ../..`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add drivers/sendgrid/
git commit -m "test(drivers): cover sendgrid driver; add base_url override"
```

### Task 10: `ses` — add `base_url`, then test (assert SigV4 header)

**Files:** Modify `drivers/ses/ses.go`; Test `drivers/ses/ses_test.go`

- [ ] **Step 1: Write the failing test**

```go
package ses_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/ses"
	"github.com/xraph/herald/message"
)

func TestSESSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"MessageId":"ses-1"}`)
	d := &ses.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", From: "from@x.com", Subject: "Hi", Text: "t",
		Data: map[string]string{
			"access_key_id": "AKIDEXAMPLE", "secret_access_key": "secret",
			"region": "us-east-1", "base_url": srv.URL,
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "ses-1" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/v2/email/outbound-emails" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	auth := srv.Captured.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/") {
		t.Errorf("auth = %q, want SigV4 with credential", auth)
	}
	if srv.Captured.Header.Get("X-Amz-Date") == "" {
		t.Error("missing X-Amz-Date")
	}
	var body struct {
		FromEmailAddress string
		Destination      struct{ ToAddresses []string }
	}
	srv.Captured.DecodeJSON(t, &body)
	if body.FromEmailAddress != "from@x.com" {
		t.Errorf("from = %q", body.FromEmailAddress)
	}
	if len(body.Destination.ToAddresses) != 1 || body.Destination.ToAddresses[0] != "to@x.com" {
		t.Errorf("to = %v", body.Destination.ToAddresses)
	}
}

func TestSESValidate(t *testing.T) {
	d := &ses.Driver{}
	if err := d.Validate(map[string]string{"access_key_id": "a", "secret_access_key": "s"}, nil); err == nil {
		t.Error("expected error for missing region")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (driver builds endpoint from region, ignores `base_url`)

Run: `cd drivers/ses && go test ./... -run TestSESSend ; cd ../..`
Expected: FAIL.

- [ ] **Step 3: Add `base_url` override**

In `drivers/ses/ses.go`, inside `Send`, change:

```go
	endpoint := fmt.Sprintf("https://email.%s.amazonaws.com/v2/email/outbound-emails", region)
```

to:

```go
	baseURL := msg.Data["base_url"]
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://email.%s.amazonaws.com", region)
	}
	endpoint := baseURL + "/v2/email/outbound-emails"
```

(The SigV4 signer signs whatever request it's given, so pointing at the mock works; the `host` in the canonical request becomes the mock's host, which is fine for the assertion.)

- [ ] **Step 4: Run — expect PASS**

Run: `cd drivers/ses && go test ./... -v ; cd ../..`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add drivers/ses/
git commit -m "test(drivers): cover ses driver; add base_url override"
```

### Task 11: `postmark` test (already injectable)

**Files:** Test: `drivers/postmark/postmark_test.go`

- [ ] **Step 1: Write the test**

```go
package postmark_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/postmark"
	"github.com/xraph/herald/message"
)

func TestPostmarkSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"MessageID":"pm-1"}`)
	d := &postmark.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", From: "from@x.com", Subject: "Hi", HTML: "<b>x</b>",
		Data: map[string]string{"server_token": "tok", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "pm-1" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/email" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	if got := srv.Captured.Header.Get("X-Postmark-Server-Token"); got != "tok" {
		t.Errorf("token header = %q", got)
	}
	var body struct{ From, To, Subject, HtmlBody string }
	srv.Captured.DecodeJSON(t, &body)
	if body.To != "to@x.com" || body.Subject != "Hi" || body.HtmlBody != "<b>x</b>" {
		t.Errorf("body = %+v", body)
	}
}

func TestPostmarkValidate(t *testing.T) {
	d := &postmark.Driver{}
	if err := d.Validate(map[string]string{}, nil); err == nil {
		t.Error("expected error for missing server_token")
	}
}
```

- [ ] **Step 2: Run — expect PASS**

Run: `cd drivers/postmark && go test ./... -v ; cd ../..`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add drivers/postmark/postmark_test.go
git commit -m "test(drivers): cover postmark email driver"
```

---

## Phase B2 — SMS driver tests

### Task 12: `twilio` — add `base_url`, then test

**Files:** Modify `driver/sms/twilio.go`; Test `driver/sms/twilio_test.go`

- [ ] **Step 1: Write the failing test**

```go
package sms_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/driver/sms"
	"github.com/xraph/herald/message"
)

func TestTwilioSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusCreated, `{"sid":"SM123"}`)
	d := &sms.TwilioDriver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "+15005550006", Text: "hello",
		Data: map[string]string{
			"account_sid": "AC1", "auth_token": "tok", "from_number": "+15005550001",
			"base_url": srv.URL,
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "SM123" {
		t.Errorf("res = %+v", res)
	}
	if !strings.HasSuffix(srv.Captured.Path, "/Accounts/AC1/Messages.json") {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	if srv.Captured.FormValue(t, "To") != "+15005550006" {
		t.Errorf("To = %q", srv.Captured.FormValue(t, "To"))
	}
	if srv.Captured.FormValue(t, "Body") != "hello" {
		t.Errorf("Body = %q", srv.Captured.FormValue(t, "Body"))
	}
}

func TestTwilioValidate(t *testing.T) {
	d := &sms.TwilioDriver{}
	if err := d.Validate(map[string]string{"account_sid": "a", "auth_token": "t"}, nil); err == nil {
		t.Error("expected error for missing from_number")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./driver/sms/ -run TestTwilioSend`
Expected: FAIL.

- [ ] **Step 3: Add `base_url`**

In `driver/sms/twilio.go`, change:

```go
	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", accountSID)
```

to:

```go
	baseURL := msg.Data["base_url"]
	if baseURL == "" {
		baseURL = "https://api.twilio.com"
	}
	apiURL := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", baseURL, accountSID)
```

- [ ] **Step 4: Run — expect PASS**

Run: `go test ./driver/sms/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add driver/sms/twilio.go driver/sms/twilio_test.go
git commit -m "test(driver): cover twilio sms driver; add base_url override"
```

### Task 13: `messagebird` — add `base_url`, then test

**Files:** Modify `drivers/messagebird/messagebird.go`; Test `drivers/messagebird/messagebird_test.go`

- [ ] **Step 1: Write the failing test**

```go
package messagebird_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/messagebird"
	"github.com/xraph/herald/message"
)

func TestMessageBirdSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusCreated, `{"id":"mb-1"}`)
	d := &messagebird.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "+3120", Text: "hi",
		Data: map[string]string{"access_key": "ak", "originator": "Acme", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "mb-1" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/messages" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	if got := srv.Captured.Header.Get("Authorization"); got != "AccessKey ak" {
		t.Errorf("auth = %q", got)
	}
	var body struct {
		Originator string   `json:"originator"`
		Recipients []string `json:"recipients"`
		Body       string   `json:"body"`
	}
	srv.Captured.DecodeJSON(t, &body)
	if body.Originator != "Acme" || len(body.Recipients) != 1 || body.Recipients[0] != "+3120" {
		t.Errorf("body = %+v", body)
	}
}

func TestMessageBirdValidate(t *testing.T) {
	d := &messagebird.Driver{}
	if err := d.Validate(map[string]string{"access_key": "k"}, nil); err == nil {
		t.Error("expected error for missing originator")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd drivers/messagebird && go test ./... -run TestMessageBirdSend ; cd ../..`
Expected: FAIL.

- [ ] **Step 3: Add `base_url`**

In `drivers/messagebird/messagebird.go`, change:

```go
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://rest.messagebird.com/messages", bytes.NewReader(jsonBody))
```

to:

```go
	baseURL := msg.Data["base_url"]
	if baseURL == "" {
		baseURL = "https://rest.messagebird.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/messages", bytes.NewReader(jsonBody))
```

- [ ] **Step 4: Run — expect PASS**

Run: `cd drivers/messagebird && go test ./... -v ; cd ../..`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add drivers/messagebird/
git commit -m "test(drivers): cover messagebird sms driver; add base_url override"
```

### Task 14: `vonage` — add `base_url`, then test (status-code mapping)

**Files:** Modify `drivers/vonage/vonage.go`; Test `drivers/vonage/vonage_test.go`

- [ ] **Step 1: Write the failing test**

```go
package vonage_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/vonage"
	"github.com/xraph/herald/message"
)

func TestVonageSendSuccess(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK,
		`{"messages":[{"message-id":"v-1","status":"0"}]}`)
	d := &vonage.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "+4477", Text: "hi",
		Data: map[string]string{"api_key": "k", "api_secret": "s", "from_number": "Acme", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "v-1" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/sms/json" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
}

func TestVonageSendFailureStatus(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK,
		`{"messages":[{"status":"2","error-text":"Missing api_key"}]}`)
	d := &vonage.Driver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "+4477", Text: "hi",
		Data: map[string]string{"api_key": "k", "api_secret": "s", "from_number": "Acme", "base_url": srv.URL},
	})
	if err == nil {
		t.Fatal("expected error when status != 0")
	}
}

func TestVonageValidate(t *testing.T) {
	d := &vonage.Driver{}
	if err := d.Validate(map[string]string{"api_key": "k", "api_secret": "s"}, nil); err == nil {
		t.Error("expected error for missing from_number")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd drivers/vonage && go test ./... -run TestVonageSendSuccess ; cd ../..`
Expected: FAIL.

- [ ] **Step 3: Add `base_url`**

In `drivers/vonage/vonage.go`, change:

```go
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://rest.nexmo.com/sms/json", bytes.NewReader(jsonBody))
```

to:

```go
	baseURL := msg.Data["base_url"]
	if baseURL == "" {
		baseURL = "https://rest.nexmo.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/sms/json", bytes.NewReader(jsonBody))
```

- [ ] **Step 4: Run — expect PASS**

Run: `cd drivers/vonage && go test ./... -v ; cd ../..`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add drivers/vonage/
git commit -m "test(drivers): cover vonage sms driver; add base_url override"
```

---

## Phase B3 — push driver tests

### Task 15: `fcm` — add `base_url`, then test

**Files:** Modify `driver/push/fcm.go`; Test `driver/push/fcm_test.go`

> **Note (out of scope):** `fcm.go` copies the whole `msg.Data` map (which includes credentials like `access_token`/`project_id`) into the FCM `message.data` payload. The test below does not assert on `message.data` to avoid coupling to that behavior. See the plan's "Discovered issues" section — this is flagged as a follow-up, not fixed here.

- [ ] **Step 1: Write the failing test**

```go
package push_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/driver/push"
	"github.com/xraph/herald/message"
)

func TestFCMSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"name":"projects/p/messages/fcm-1"}`)
	d := &push.FCMDriver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "device-token", Title: "Hi", Text: "body",
		Data: map[string]string{"project_id": "p", "access_token": "tok", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "projects/p/messages/fcm-1" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/v1/projects/p/messages:send" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	if got := srv.Captured.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("auth = %q", got)
	}
	var body struct {
		Message struct {
			Token        string `json:"token"`
			Notification struct {
				Title string `json:"title"`
				Body  string `json:"body"`
			} `json:"notification"`
		} `json:"message"`
	}
	srv.Captured.DecodeJSON(t, &body)
	if body.Message.Token != "device-token" {
		t.Errorf("token = %q", body.Message.Token)
	}
	if body.Message.Notification.Title != "Hi" || body.Message.Notification.Body != "body" {
		t.Errorf("notification = %+v", body.Message.Notification)
	}
}

func TestFCMValidate(t *testing.T) {
	d := &push.FCMDriver{}
	if err := d.Validate(map[string]string{}, nil); err == nil {
		t.Error("expected error for missing server_key/access_token")
	}
	if err := d.Validate(map[string]string{"access_token": "t"}, nil); err != nil {
		t.Errorf("unexpected: %v", err)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./driver/push/ -run TestFCMSend`
Expected: FAIL.

- [ ] **Step 3: Add `base_url`**

In `driver/push/fcm.go`, change:

```go
	apiURL := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", projectID)
```

to:

```go
	baseURL := msg.Data["base_url"]
	if baseURL == "" {
		baseURL = "https://fcm.googleapis.com"
	}
	apiURL := fmt.Sprintf("%s/v1/projects/%s/messages:send", baseURL, projectID)
```

- [ ] **Step 4: Run — expect PASS**

Run: `go test ./driver/push/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add driver/push/fcm.go driver/push/fcm_test.go
git commit -m "test(driver): cover fcm push driver; add base_url override"
```

### Task 16: `apns` — add `base_url`, then test (generate ECDSA key)

**Files:** Modify `drivers/apns/apns.go`; Test `drivers/apns/apns_test.go`

- [ ] **Step 1: Write the failing test**

```go
package apns_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/drivers/apns"
	"github.com/xraph/herald/message"
)

func testKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestAPNSSend(t *testing.T) {
	var gotPath, gotAuth, gotTopic string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotTopic = r.Header.Get("apns-topic")
		w.Header().Set("apns-id", "apns-1")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := &apns.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "devicetoken", Title: "Hi", Text: "body",
		Data: map[string]string{
			"key_id": "K1", "team_id": "T1", "bundle_id": "com.example.app",
			"private_key": testKeyPEM(t), "base_url": srv.URL,
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "apns-1" {
		t.Errorf("res = %+v", res)
	}
	if gotPath != "/3/device/devicetoken" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "bearer ") {
		t.Errorf("auth = %q, want bearer <jwt>", gotAuth)
	}
	if gotTopic != "com.example.app" {
		t.Errorf("apns-topic = %q", gotTopic)
	}
}

func TestAPNSValidateBadKey(t *testing.T) {
	d := &apns.Driver{}
	err := d.Validate(map[string]string{
		"key_id": "K", "team_id": "T", "bundle_id": "b", "private_key": "not-a-key",
	}, nil)
	if err == nil {
		t.Error("expected error for invalid private_key")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (driver hardcodes the Apple host)

Run: `cd drivers/apns && go test ./... -run TestAPNSSend ; cd ../..`
Expected: FAIL.

- [ ] **Step 3: Add `base_url`**

In `drivers/apns/apns.go`, change:

```go
	host := "https://api.push.apple.com"
	if sandbox {
		host = "https://api.sandbox.push.apple.com"
	}
```

to:

```go
	host := msg.Data["base_url"]
	if host == "" {
		host = "https://api.push.apple.com"
		if sandbox {
			host = "https://api.sandbox.push.apple.com"
		}
	}
```

- [ ] **Step 4: Run — expect PASS**

Run: `cd drivers/apns && go test ./... -v ; cd ../..`
Expected: PASS. (The test server is plain HTTP; the driver's default client speaks HTTP/1.1 to it, which is fine for asserting request shape.)

- [ ] **Step 5: Commit**

```bash
git add drivers/apns/
git commit -m "test(drivers): cover apns push driver; add base_url override"
```

---

## Phase B4 — chat / webhook driver tests

### Task 17: `slack` — add `base_url` for API mode, then test both modes

**Files:** Modify `drivers/slack/slack.go`; Test `drivers/slack/slack_test.go`

- [ ] **Step 1: Write the failing test**

```go
package slack_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/slack"
	"github.com/xraph/herald/message"
)

func TestSlackWebhookMode(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `ok`)
	d := &slack.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		Text: "hello",
		Data: map[string]string{"webhook_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent {
		t.Errorf("status = %q", res.Status)
	}
	var body struct{ Text string }
	srv.Captured.DecodeJSON(t, &body)
	if body.Text != "hello" {
		t.Errorf("text = %q", body.Text)
	}
}

func TestSlackAPIMode(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"ok":true,"ts":"123.456"}`)
	d := &slack.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "#general", Text: "hello",
		Data: map[string]string{"bot_token": "xoxb", "channel": "#default", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "123.456" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/api/chat.postMessage" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	if got := srv.Captured.Header.Get("Authorization"); got != "Bearer xoxb" {
		t.Errorf("auth = %q", got)
	}
	var body struct{ Channel, Text string }
	srv.Captured.DecodeJSON(t, &body)
	if body.Channel != "#general" { // msg.To overrides channel
		t.Errorf("channel = %q", body.Channel)
	}
}

func TestSlackAPIError(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"ok":false,"error":"channel_not_found"}`)
	d := &slack.Driver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "#x", Text: "hi",
		Data: map[string]string{"bot_token": "xoxb", "channel": "#x", "base_url": srv.URL},
	})
	if err == nil {
		t.Fatal("expected error when ok=false")
	}
}

func TestSlackValidate(t *testing.T) {
	d := &slack.Driver{}
	if err := d.Validate(map[string]string{}, nil); err == nil {
		t.Error("expected error with neither webhook_url nor bot_token+channel")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (webhook test passes, but API test hits real slack.com)

Run: `cd drivers/slack && go test ./... -run TestSlackAPIMode ; cd ../..`
Expected: FAIL.

- [ ] **Step 3: Add `base_url` to the API path**

In `drivers/slack/slack.go`, in `sendAPI`, change:

```go
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(body))
```

to:

```go
	baseURL := msg.Data["base_url"]
	if baseURL == "" {
		baseURL = "https://slack.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/chat.postMessage", bytes.NewReader(body))
```

- [ ] **Step 4: Run — expect PASS**

Run: `cd drivers/slack && go test ./... -v ; cd ../..`
Expected: PASS (all four).

- [ ] **Step 5: Commit**

```bash
git add drivers/slack/
git commit -m "test(drivers): cover slack driver; add base_url for API mode"
```

### Task 18: `discord` test (webhook URL is injectable)

**Files:** Test: `drivers/discord/discord_test.go`

- [ ] **Step 1: Write the test**

```go
package discord_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/discord"
	"github.com/xraph/herald/message"
)

func TestDiscordSendContent(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"id":"d-1"}`)
	d := &discord.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		Text: "plain message",
		Data: map[string]string{"webhook_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "d-1" {
		t.Errorf("res = %+v", res)
	}
	if !strings.Contains(srv.Captured.Query, "wait=true") {
		t.Errorf("query = %q, want wait=true", srv.Captured.Query)
	}
	var body struct{ Content string }
	srv.Captured.DecodeJSON(t, &body)
	if body.Content != "plain message" {
		t.Errorf("content = %q", body.Content)
	}
}

func TestDiscordSendEmbed(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"id":"d-2"}`)
	d := &discord.Driver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		Title: "Alert", Text: "details",
		Data: map[string]string{"webhook_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	var body struct {
		Embeds []struct{ Title, Description string }
	}
	srv.Captured.DecodeJSON(t, &body)
	if len(body.Embeds) != 1 || body.Embeds[0].Title != "Alert" || body.Embeds[0].Description != "details" {
		t.Errorf("embeds = %+v", body.Embeds)
	}
}

func TestDiscordValidate(t *testing.T) {
	d := &discord.Driver{}
	if err := d.Validate(map[string]string{}, nil); err == nil {
		t.Error("expected error for missing webhook_url")
	}
}
```

- [ ] **Step 2: Run — expect PASS**

Run: `cd drivers/discord && go test ./... -v ; cd ../..`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add drivers/discord/discord_test.go
git commit -m "test(drivers): cover discord chat driver"
```

### Task 19: `webhook` test (direct mode, HMAC signing)

**Files:** Test: `drivers/webhook/webhook_test.go`

- [ ] **Step 1: Write the test**

```go
package webhook_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/webhook"
	"github.com/xraph/herald/message"
)

func TestWebhookDirectSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, ``)
	d := webhook.New(nil) // no relay -> direct HTTP POST
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", Subject: "Hi", Text: "body",
		Data: map[string]string{"url": srv.URL, "signing_secret": "shh", "event_type": "test.event"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent {
		t.Errorf("status = %q", res.Status)
	}
	sig := srv.Captured.Header.Get("X-Webhook-Signature")
	if !strings.HasPrefix(sig, "sha256=") {
		t.Errorf("signature = %q, want sha256= prefix", sig)
	}
	var body struct {
		Event   string `json:"event"`
		To      string `json:"to"`
		Subject string `json:"subject"`
	}
	srv.Captured.DecodeJSON(t, &body)
	if body.Event != "test.event" || body.To != "to@x.com" || body.Subject != "Hi" {
		t.Errorf("body = %+v", body)
	}
}

func TestWebhookValidate(t *testing.T) {
	d := webhook.New(nil)
	if err := d.Validate(map[string]string{}, nil); err == nil {
		t.Error("expected error for missing url")
	}
}
```

- [ ] **Step 2: Run — expect PASS**

Run: `cd drivers/webhook && go test ./... -v ; cd ../..`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add drivers/webhook/webhook_test.go
git commit -m "test(drivers): cover webhook driver direct-send mode"
```

---

## Phase B5 — inapp driver test

### Task 20: `inapp` test (no-op driver)

**Files:** Test: `driver/inapp/inapp_test.go`

- [ ] **Step 1: Write the test**

```go
package inapp_test

import (
	"context"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/inapp"
	"github.com/xraph/herald/message"
)

func TestInappSendReturnsDelivered(t *testing.T) {
	d := &inapp.Driver{}
	if d.Name() != "inapp" || d.Channel() != "inapp" {
		t.Errorf("name/channel = %q/%q", d.Name(), d.Channel())
	}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{To: "user-1", Text: "hi"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusDelivered {
		t.Errorf("status = %q, want delivered", res.Status)
	}
}

func TestInappValidate(t *testing.T) {
	d := &inapp.Driver{}
	if err := d.Validate(nil, nil); err != nil {
		t.Errorf("Validate should always pass, got %v", err)
	}
}
```

- [ ] **Step 2: Run — expect PASS**

Run: `go test ./driver/inapp/ -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add driver/inapp/inapp_test.go
git commit -m "test(driver): cover inapp no-op driver"
```

---

## Final verification

- [ ] **Step 1: Core module tests**

Run: `go test ./...`
Expected: PASS (core + built-in drivers + extension).

- [ ] **Step 2: Each opt-in driver module**

Run:
```bash
for m in cloudflare mailgun sendgrid ses postmark messagebird vonage apns slack discord webhook; do
  echo "== $m =="; (cd "drivers/$m" && go test ./...) || exit 1
done
```
Expected: every module PASS.

- [ ] **Step 3: Lint**

Run: `golangci-lint run ./...` (and per-module if the project lints modules separately — check `Makefile`).
Expected: no new findings. Fix any (most likely unchecked-error or import-ordering).

- [ ] **Step 4: Confirm no behavior change for default URLs**

Re-read each `base_url` edit: with `base_url` unset, the constructed URL must equal the original hardcoded production URL. Verify by inspection.

---

## Discovered issues (out of scope — flag, do not fix here)

- **FCM credential leak:** `driver/push/fcm.go` assigns the entire `msg.Data` map (which carries `project_id`, `access_token`, `server_key`) into the FCM `message.data` field, sending credentials to Google as message data. Should filter to non-credential keys (cf. `drivers/webhook`'s `filterData`). Recommend a follow-up fix + test.

## Self-review notes

- Spec coverage: Part A → Task 2; Part B (drivertest helper) → Task 1; Part B per-driver → Tasks 6–20 (email B1: 6–11 + Task 2 cloudflare; sms B2: 12–14; push B3: 15–16; chat/other B4: 17–19; inapp B5: 20); Part C → Tasks 3–5. All covered.
- The `inapp` driver is a no-op; the spec's "store-backed" phrasing was corrected to a trivial no-op test (Task 20).
- Type consistency: `drivertest.NewServer`, `CapturedRequest.DecodeJSON`, `CapturedRequest.FormValue`, `SeedConfiguredProviders([]provider.Provider)`, `providersFromConfig([]ProviderConfig, time.Time)`, `ProviderConfig.toProvider(time.Time)` are used identically wherever referenced.
- sendgrid and apns return their id via response header, so their tests use a hand-rolled `httptest.NewServer` (header settable) rather than `drivertest.NewServer`; both noted in Task 1.
