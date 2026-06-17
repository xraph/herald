# Cloudflare Email driver, full driver test backfill, and config.yaml providers

**Date:** 2026-06-17
**Status:** Approved (design)

## Background

Herald has three related concepts that this work touches:

- **`driver/`** (singular) — the core driver framework (`Driver` interface, `Registry`,
  `OutboundMessage`, `DeliveryResult`) plus the batteries-included drivers compiled into the
  main module (`driver/email`: smtp, resend; `driver/sms`: twilio; `driver/push`: fcm;
  `driver/inapp`). These are auto-registered by the Forge extension.
- **`drivers/`** (plural) — opt-in add-on drivers, each its own Go module
  (`drivers/mailgun`, `sendgrid`, `ses`, `postmark`, `slack`, `discord`, `messagebird`,
  `vonage`, `apns`, `webhook`). Each has its own `go.mod` with `replace github.com/xraph/herald => ../../`.
  The core module does not import them; consumers opt in via `go get` + `herald.WithDriver`.
  The split exists for dependency isolation and independent versioning.
- **`provider/`** — the configured, app-scoped entity (`Provider`) persisted in the store. A
  driver is the *code*; a provider is a *configured instance* of a driver with credentials.

This document specifies three pieces of work:

- **Part A** — a new Cloudflare Email Service driver (`drivers/cloudflare`), built test-first.
- **Part B** — backfill unit tests for all ~16 drivers, phased by channel, with a small
  uniformity refactor so every HTTP driver's endpoint is injectable.
- **Part C** — support declaring providers in the extension's `config.yaml`, seeded into the
  store on startup.

## Goals

- Add an outbound Cloudflare Email Service driver that fits the existing `Driver` interface.
- Close the driver test gap: every driver has unit tests.
- Let operators declare providers (driver + credentials + settings) in `config.yaml`.

## Non-goals

- Cloudflare Email Routing (inbound) — does not fit the outbound `Driver.Send` interface.
- The Workers `send_email` binding — not callable from a Go backend.
- Reconcile/source-of-truth config semantics — explicitly **not** chosen (see Part C).
- Importing every `drivers/*` module into the extension — would reintroduce the dependency
  bloat the two-folder split prevents.

---

## Part A — Cloudflare Email Service driver

### Placement

`drivers/cloudflare` as a standalone opt-in module, mirroring `drivers/sendgrid`:

```
drivers/cloudflare/
  go.mod            # module github.com/xraph/herald/drivers/cloudflare
                    # require github.com/xraph/herald v0.0.0
                    # replace github.com/xraph/herald => ../../
  go.sum
  cloudflare.go     # package cloudflare; type Driver struct{}; var _ driver.Driver = (*Driver)(nil)
  cloudflare_test.go
```

### API

Cloudflare Email Service (public beta, 2025/2026) exposes a REST API suited to a Go HTTP driver:

- Endpoint: `POST {base_url}/accounts/{account_id}/email/sending/send`
- Default `base_url`: `https://api.cloudflare.com/client/v4`
- Auth: `Authorization: Bearer <api_token>`
- Request body (flat — **not** SendGrid/MailChannels `personalizations`/`content` arrays):
  ```json
  { "to": "...", "from": "...", "subject": "...", "html": "...", "text": "..." }
  ```
- Success response is recipient-grouped, not a single id:
  ```json
  { "success": true, "errors": [], "messages": [],
    "result": { "delivered": ["..."], "permanent_bounces": [], "queued": [] } }
  ```

### Behavior

- `Name() → "cloudflare"`, `Channel() → "email"`.
- `Validate(credentials, _)` requires `api_token` and `account_id`.
- `Send`:
  - Reads `msg.Data["api_token"]`, `msg.Data["account_id"]`, optional `msg.Data["base_url"]`
    (default above). `account_id` is part of the URL path, hence mandatory.
  - `from` = `msg.From`, or `"FromName <from@addr>"` when `FromName` is set (matches the resend
    driver; the flat `from` string accepts the RFC 5322 display-name form).
  - Marshals the flat body; `POST` with bearer auth, `Content-Type: application/json`,
    10s client timeout.
  - `status >= 400` → error with a 1KB body snippet (same pattern as the other drivers).
  - Parses the response; `success == true` with no `permanent_bounces` → `DeliveryResult{Status:
    message.StatusSent}`. `ProviderMessageID` is left empty (REST API returns no single id,
    consistent with the SMTP driver).

### Tests (written first)

1. Happy path: `httptest` server asserts `POST`, path `/accounts/{acct}/email/sending/send`,
   `Authorization: Bearer <token>`, and body fields (`to`/`from`/`subject`/`html`); returns
   `{success:true, result:{delivered:[to]}}`; assert `StatusSent`.
2. `FromName` set → `from` is `"Name <email>"`.
3. `4xx` response → error containing the body snippet.
4. Response with a non-empty `permanent_bounces` → error / non-sent.
5. `Validate` with missing `api_token` or `account_id` → error.

### Open item (resolved during implementation, does not change design)

Whether a *named* Cloudflare sender must use an object form (`{address|email, name}`) instead of
the `"Name <email>"` string. Default to the string; switch only if the named form requires the
object. This affects only `from` serialization.

---

## Part B — Driver test backfill (phased by channel)

### Shared harness

New **exported** helper package `driver/drivertest` in the core module (stdlib-only). Provides
`httptest`-based request-capture and assertion helpers (captured method, path, headers, decoded
JSON/form body) plus small response builders. Both built-in driver tests and opt-in module tests
import it (opt-in modules already `require github.com/xraph/herald` with a local `replace`), so
there is no duplicated boilerplate and no break in module isolation.

### Uniformity refactor

Every HTTP driver honors `msg.Data["base_url"]`, defaulting to its current hardcoded URL. This is
the enabler for pointing tests at an `httptest` server, and is good consistency on its own.

- Already injectable: resend, mailgun, postmark, webhook (URL from creds), cloudflare (Part A).
- Needs `base_url` added: **twilio, fcm, sendgrid, ses, slack, messagebird, vonage, apns**.

### Per-driver test pattern

- HTTP drivers: set `base_url` to the test server; assert request shape (method/path/auth
  header/body) and map a canned response → `DeliveryResult`; plus a `4xx` error case and
  `Validate` cases. Table-driven where natural.
- SES: additionally assert a well-formed SigV4 `Authorization` header is present.
- SMTP: a tiny in-process SMTP listener asserts the `MAIL FROM` / `RCPT TO` / `DATA` dialog and
  `StatusSent`.
- inapp: constructed against an in-memory/fake store; assert it persists the expected record;
  `Validate` cases.

### Phases (each phase green before the next)

- **B1 — email:** resend, smtp, mailgun, sendgrid, ses, postmark *(+ cloudflare from Part A)*.
  Adds `base_url` to sendgrid and ses.
- **B2 — sms:** twilio, messagebird, vonage. Adds `base_url` to all three.
- **B3 — push:** fcm, apns. Adds `base_url` to both.
- **B4 — chat/other:** slack, discord, webhook. Adds `base_url` to slack.
- **B5 — inapp:** store-backed.

### Open items (resolved during implementation)

- APNs uses HTTP/2; the test server may need explicit HTTP/2 enablement.
- Depth of the SES signature assertion (presence/format vs. full signature verification).

---

## Part C — config.yaml provider support

### Schema

Add to the extension `Config` (`extension/config.go`):

```go
// Providers declares notification providers seeded into the store on startup.
Providers []ProviderConfig `json:"providers" yaml:"providers" mapstructure:"providers"`
```

```go
type ProviderConfig struct {
    Name        string            `yaml:"name" json:"name" mapstructure:"name"`
    Channel     string            `yaml:"channel" json:"channel" mapstructure:"channel"`
    Driver      string            `yaml:"driver" json:"driver" mapstructure:"driver"`
    AppID       string            `yaml:"app_id" json:"app_id" mapstructure:"app_id"`       // default ""
    Credentials map[string]string `yaml:"credentials" json:"credentials" mapstructure:"credentials"`
    Settings    map[string]string `yaml:"settings" json:"settings" mapstructure:"settings"`
    Priority    int               `yaml:"priority" json:"priority" mapstructure:"priority"`
    Enabled     *bool             `yaml:"enabled" json:"enabled" mapstructure:"enabled"`    // nil → true
}
```

Example:

```yaml
extensions:
  herald:
    providers:
      - name: cloudflare-email
        channel: email
        driver: cloudflare
        credentials:
          api_token: ${CLOUDFLARE_EMAIL_TOKEN}   # env interpolation via the config layer
          account_id: ${CLOUDFLARE_ACCOUNT_ID}
        settings: {}
        priority: 10
        enabled: true
```

### Seeding semantics — seed-if-absent (bootstrap only)

New core method `Herald.SeedConfiguredProviders(ctx, appID string, providers []provider.Provider) error`,
mirroring `SeedDefaultProviders`:

- For each declared provider, look up existing providers for the app
  (`ListAllProviders`); if one with the same `Name` exists, **skip** it (the stored record,
  including dashboard edits, wins). Otherwise `CreateProvider`.
- Log seeded vs skipped. Warn (do not fail) if the named driver is not registered, or if
  `driver.Validate(credentials, settings)` fails.
- Consequence (documented): changing a credential in `config.yaml` after first boot has no
  effect on an already-seeded provider — matches existing `SeedDefaultProviders` behavior.

### Wiring

- A pure mapper `ProviderConfig → provider.Provider` (sets `ID`, timestamps, `Enabled` default
  true when nil) — unit-tested.
- In `extension.Init`, immediately after `SeedDefaultProviders`, the extension maps
  `e.config.Providers` ([]ProviderConfig) through the mapper and passes the resulting
  `[]provider.Provider` to `SeedConfiguredProviders`. The method itself resolves each provider's
  driver from the registry (`h.drivers.Get`) and runs `drv.Validate(p.Credentials, p.Settings)`,
  warning (not failing) on a missing driver or invalid credentials.
- **Driver registration stays in code.** YAML supplies credentials/config; an opt-in driver
  like `cloudflare` is still registered via `extension.WithDriver(&cloudflare.Driver{})`.
  Built-ins remain auto-registered. The extension does not import `drivers/*` modules.

### Tests

- `ProviderConfig → provider.Provider` mapper: enabled default, field mapping.
- `SeedConfiguredProviders` against an in-memory store: creates on first run, skips on second
  run when a provider of that name already exists; warns on unregistered driver / failed validate.

---

## Build order

A → C → B1 → B2 → B3 → B4 → B5.

Cloudflare (A) establishes the `httptest` pattern and `driver/drivertest` helper used by the
rest. C is small and independent. B is the large, mechanical backfill, batched by channel so each
phase ships green.

## Risks / notes

- Cloudflare Email Service is in public beta; endpoint paths could shift. The `base_url` override
  mitigates this without code changes.
- Credentials in `config.yaml` rely on the config layer's env interpolation; secrets should be
  supplied via `${ENV}` references, not committed literals.
