<p align="center">
  <img src="assets/readme-banner.svg" alt="Herald, a Go notification library for email, SMS, push and in-app messages with templates and i18n" width="100%">
</p>

# Herald: multi-channel notifications for Go

Herald is a Go library for sending notifications over email, SMS, push, in-app, chat and webhooks from one API. You store providers (SMTP, Resend, SendGrid, Twilio, FCM, APNs, Slack and more), write templates with a version per locale, and Herald picks the provider through app, org and user routing rules, skips users who opted out, logs every message, and keeps an in-app inbox per user. It runs inside your process on PostgreSQL, SQLite, MongoDB or memory, and it ships an extension for [Forge](https://github.com/xraph/forge) with a REST API.

[![Go 1.26+](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/dl/) [![CI](https://github.com/xraph/herald/actions/workflows/ci.yml/badge.svg)](https://github.com/xraph/herald/actions/workflows/ci.yml) [![Go Reference](https://pkg.go.dev/badge/github.com/xraph/herald.svg)](https://pkg.go.dev/github.com/xraph/herald) [![GitHub Stars](https://img.shields.io/github/stars/xraph/herald?style=flat)](https://github.com/xraph/herald/stargazers)

[Documentation](https://www.xraph.com/docs/herald) · [Quick start](#quick-start) · [Features](#why-herald) · [Drivers](#channels-and-drivers) · [Templates](#templates-and-locales) · [Forge](#use-it-with-forge) · [Configuration](#configuration) · [FAQ](#faq)

## Quick start

You need [Go 1.26 or later](https://go.dev/dl/).

```bash
go get github.com/xraph/herald@latest
```

This program needs no mail server or API key. It uses the in-memory store and the in-app driver, creates a template in English and French, and sends it to one user's inbox:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/xraph/herald"
	"github.com/xraph/herald/driver/inapp"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/store/memory"
	"github.com/xraph/herald/template"
)

func main() {
	ctx := context.Background()

	h, err := herald.New(
		herald.WithStore(memory.New()),
		herald.WithDriver(&inapp.Driver{}),
	)
	if err != nil {
		log.Fatal(err)
	}

	// The in-app driver needs no credentials, so Herald can create its provider.
	if err := h.SeedDefaultProviders(ctx, "my-app"); err != nil {
		log.Fatal(err)
	}

	// One template, two locales.
	tmpl := &template.Template{
		ID:      id.NewTemplateID(),
		AppID:   "my-app",
		Slug:    "order.shipped",
		Name:    "Order shipped",
		Channel: "inapp",
		Enabled: true,
	}
	if err := h.Store().CreateTemplate(ctx, tmpl); err != nil {
		log.Fatal(err)
	}
	for _, v := range []template.Version{
		{Locale: "en", Title: "Order {{.order}} shipped", Text: "It arrives {{.eta}}."},
		{Locale: "fr", Title: "Commande {{.order}} expédiée", Text: "Livraison prévue {{.eta}}."},
	} {
		v.ID = id.NewTemplateVersionID()
		v.TemplateID = tmpl.ID
		v.Active = true
		if err := h.Store().CreateVersion(ctx, &v); err != nil {
			log.Fatal(err)
		}
	}

	res, err := h.Send(ctx, &herald.SendRequest{
		AppID:    "my-app",
		UserID:   "user-42",
		Channel:  "inapp",
		Template: "order.shipped",
		Locale:   "fr-CA", // falls back to "fr"
		To:       []string{"user-42"},
		Data:     map[string]any{"order": "#1001", "eta": "jeudi"},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("status:", res.Status)

	inbox, err := h.Store().ListNotifications(ctx, "my-app", "user-42", 10, 0)
	if err != nil {
		log.Fatal(err)
	}
	for _, n := range inbox {
		fmt.Printf("inbox: %s / %s\n", n.Title, n.Body)
	}
}
```

Run it with `go run .`. After a log line about the seeded provider, you'll see:

```text
status: sent
inbox: Commande #1001 expédiée / Livraison prévue jeudi.
```

The request asked for `fr-CA`. There's no version for that, so Herald used the `fr` one. Templates belong to a channel, so for email you'd create the same template with `Channel: "email"`, register an email driver, add a provider for it, and send the same request with `Channel: "email"` and an address in `To`.

## Why Herald?

You probably started with one email provider called straight from a handler. Then someone asks for SMS for one customer, push on mobile, a bell icon in the web app, French copy and a way for users to turn off marketing mail, and before long your notification code is spread across five services that each know a little about your vendors. Herald puts that in one place. One request type, every channel.

| You need | Herald gives you |
| --- | --- |
| One API for every channel | `Send` for one channel and `Notify` to send the same template on several, across `email`, `sms`, `push`, `inapp`, `chat` and `webhook` |
| Swappable providers | Providers stored per app with a driver, credentials, settings and a priority, managed with `CreateProvider`, `UpdateProvider` and `DeleteProvider` |
| Per-customer routing | Scoped configs at app, org and user level that pick the provider and the sender address; the resolver tries user, then org, then app, then the enabled provider with the lowest priority number |
| Templates with i18n | Go templates with one version per locale, required variables with defaults, and fallback from `fr-CA` to `fr` to the default version |
| Ready-made auth emails | `SeedDefaultTemplates` adds welcome, verification, password reset, sign-in alert, MFA and organization emails |
| User opt-outs | Per-user preferences by notification type and channel; an opted-out send is logged as `suppressed` and nothing goes out |
| An in-app inbox | Notifications stored per user, with unread counts and mark-as-read |
| A delivery log | One message row per recipient, with status, error and the vendor's message ID |
| Encrypted credentials | AES-256-GCM for provider credentials, with key rotation through previous keys |
| An audit trail | Audit events for sends and suppressions, through [Chronicle](https://github.com/xraph/chronicle) or slog |

## Channels and drivers

Each provider you store names a driver, and you register that driver on the engine with `herald.WithDriver`. Five come in the root module:

| Driver | Channel | Package | Credentials and settings |
| --- | --- | --- | --- |
| `smtp` | email | `github.com/xraph/herald/driver/email` (`&email.SMTPDriver{}`) | `username`, `password`; settings `host`, `port`, `use_tls` |
| `resend` | email | `github.com/xraph/herald/driver/email` (`&email.ResendDriver{}`) | `api_key` |
| `twilio` | sms | `github.com/xraph/herald/driver/sms` (`&sms.TwilioDriver{}`) | `account_sid`, `auth_token`; setting `from_number` |
| `fcm` | push | `github.com/xraph/herald/driver/push` (`&push.FCMDriver{}`) | `access_token` or `server_key`; setting `project_id` |
| `inapp` | inapp | `github.com/xraph/herald/driver/inapp` (`&inapp.Driver{}`) | none |

The rest are separate Go modules, so you only download the vendors you actually use:

| Driver | Channel | Module |
| --- | --- | --- |
| `sendgrid`, `mailgun`, `postmark`, `ses`, `cloudflare` | email | `github.com/xraph/herald/drivers/<name>` |
| `vonage`, `messagebird` | sms | `github.com/xraph/herald/drivers/<name>` |
| `apns` | push | `github.com/xraph/herald/drivers/apns` |
| `slack`, `discord` | chat | `github.com/xraph/herald/drivers/<name>` |
| `webhook` | webhook | `github.com/xraph/herald/drivers/webhook`, which can deliver through [Relay](https://github.com/xraph/relay) for retries and signing |

```bash
go get github.com/xraph/herald/drivers/slack@latest
```

Each driver exports a `Driver` type (`&slack.Driver{}`), except webhook, which you build with `webhook.New(r)` and a `*relay.Relay` or nil. A driver's `Fields()` says which keys are credentials and which are settings, and Herald refuses a provider that puts a setting such as `host` or `base_url` in its credentials.

Adding an SMTP provider looks like this:

```go
err := h.CreateProvider(ctx, &provider.Provider{
	AppID:    "my-app",
	Name:     "Mailpit",
	Channel:  "email",
	Driver:   "smtp",
	Settings: map[string]string{"host": "localhost", "port": "1025", "from": "hello@example.com"},
	Enabled:  true,
})
```

Watch this one. A failed delivery doesn't come back as an error from `Send`: it comes back in the result, with `Status` set to `failed` and the vendor's reason in `Error`, and the message log keeps the same record so you can look it up later. `Send` returns an error when it can't get as far as a driver: no provider for the channel, a missing or disabled template, a driver that isn't registered.

## Templates and locales

A template belongs to an app and a channel, has a slug, and carries one `Version` per locale with any of `Subject`, `HTML`, `Text` and `Title`. You write them as Go templates. Subject, text and title render with `text/template`, HTML renders with `html/template`, and you can call `upper`, `lower`, `title`, `truncate`, `default`, `now` and `formatDate` from either. If you declare `Variables` on the template, you can mark some as required and give the others a default.

When you send with `Locale: "fr-CA"`, Herald looks for an active `fr-CA` version, then `fr`, then the version whose locale is `""`. An empty `Locale` on the request means the engine's default, `en` unless you set `herald.WithDefaultLocale`.

You don't have to use a template. Leave `Template` empty, or set `Body`, and `Send` uses your `Subject` and `Body` as they are.

## Preferences and the inbox

A `preference.Preference` holds, per user, overrides keyed by notification type (the template slug) with a flag per channel. Store one with `h.Store().SetPreference(ctx, p)`. From then on, whenever a send carries both a `UserID` and a `Template` and that user has turned the channel off for that template, Herald writes the message to the log as `suppressed`, records an audit event, returns the suppressed result to you and never calls a driver, so the opt-out holds even when the code doing the sending has no idea it exists.

An `inapp` send with a `UserID` adds a notification to that user's inbox. Read it back with `ListNotifications`, `UnreadCount`, `MarkRead` and `MarkAllRead` on the store, or through the REST API.

## Pick a store

`herald.New` needs a store and returns `ErrNoStore` without one.

| Store | Package |
| --- | --- |
| In-memory | `github.com/xraph/herald/store/memory` |
| PostgreSQL | `github.com/xraph/herald/store/postgres`, on a `*grove.DB` with Grove's `pgdriver` |
| SQLite | `github.com/xraph/herald/store/sqlite`, on a `*grove.DB` with Grove's `sqlitedriver` |
| MongoDB | `github.com/xraph/herald/store/mongo`, on a `*grove.DB` with Grove's `mongodriver` |

Call `Migrate` on a database store before you send, unless you're on the Forge extension, which does it for you. Every store passes the same conformance suite in `store/storetest`, and you can run it against a store of your own.

## Use it with Forge

The `extension` package mounts Herald in a Forge app:

```go
import (
	"github.com/xraph/forge"
	heraldext "github.com/xraph/herald/extension"
	"github.com/xraph/herald/store/memory"
)

app := forge.NewApp(forge.AppConfig{
	Name: "notifications",
	Extensions: []forge.Extension{
		heraldext.New(
			heraldext.WithStore(memory.New()),
			heraldext.WithAPIMiddleware(requireAdmin), // your auth middleware
		),
	},
})
```

During `Register` the extension:

1. Picks a store: `WithStore`, a named `*grove.DB` from `WithGroveDatabase(name)` or `grove_database`, or the default `*grove.DB` in the container.
2. Uses a `chronicle.Emitter` from the container for audit events if there is one, and slog if not.
3. Registers the five built-in drivers. Add others with `heraldext.WithDriver`.
4. Runs migrations, seeds the in-app provider and any `providers` from your config, and mounts the REST API under `/herald`.
5. Provides `*herald.Herald` to the container, so you can resolve it with `forge.Inject[*herald.Herald](app.Container())`.

The REST API lives under `/herald/v1`: `providers`, `templates` (with `versions`), `send`, `notify`, `messages`, `inbox`, `preferences` and `config` for routing rules. With no middleware, the extension logs a warning, because anyone who can reach the API can read and change notification data. Pass your auth middleware with `WithAPIMiddleware`.

The extension also registers a contract contributor for the [Forge dashboard](https://github.com/xraph/forge-dashboard). [MIGRATION.md](MIGRATION.md) explains what replaced the old templ dashboard.

## Configuration

The extension reads `extensions.herald` and falls back to a top-level `herald` key:

```yaml
extensions:
  herald:
    base_path: /herald
    disable_routes: false
    disable_migrate: false
    grove_database: ""
    credentials_key: "<base64 of 32 random bytes>"
    credentials_key_id: "2026-10"
    previous_credentials_keys:
      - id: "2026-01"
        key: "<base64>"
    providers:
      - name: Resend
        app_id: my-app
        channel: email
        driver: resend
        credentials:
          api_key: re_xxx
        settings:
          from: hello@example.com
        priority: 0
```

Providers listed under `providers` are created once, if no provider with that name exists for the app yet, so edits you make later through the API or dashboard stick. With `credentials_key` set, new credentials are stored encrypted, and `previous_credentials_keys` lets Herald read rows written under an older key. In code, the same key goes in with `herald.WithCredentialKey` and `herald.WithPreviousCredentialKey`.

The extension options are `WithStore`, `WithGroveDatabase`, `WithDriver`, `WithBasePath`, `WithDisableRoutes`, `WithDisableMigrate`, `WithAPIMiddleware`, `WithConfig`, `WithRequireConfig` and `WithHeraldOption` for any engine option. The engine settings that come from the embedded `herald.Config` (`DefaultLocale`, `MaxBatchSize`) carry no YAML tags today and don't bind from a config file, so set them in code with `WithHeraldOption(herald.WithDefaultLocale("fr"))`.

## Documentation

| If you want to | Read |
| --- | --- |
| Send a first notification | [Getting started](https://www.xraph.com/docs/herald/getting-started), [full example](https://www.xraph.com/docs/herald/guides/full-example) |
| Set up vendors | [Providers](https://www.xraph.com/docs/herald/subsystems/providers), [drivers](https://www.xraph.com/docs/herald/subsystems/drivers) |
| Write and translate messages | [Templates](https://www.xraph.com/docs/herald/subsystems/templates) |
| Route per customer | [Scoped config](https://www.xraph.com/docs/herald/subsystems/scoped-config), [multi-tenant patterns](https://www.xraph.com/docs/herald/guides/multi-tenant-patterns) |
| Respect opt-outs and build a bell icon | [Preferences](https://www.xraph.com/docs/herald/subsystems/preferences), [inbox](https://www.xraph.com/docs/herald/subsystems/inbox) |
| Run it on a database | [PostgreSQL](https://www.xraph.com/docs/herald/stores/postgres), [SQLite](https://www.xraph.com/docs/herald/stores/sqlite), [MongoDB](https://www.xraph.com/docs/herald/stores/mongo) |
| Mount it in Forge | [Forge extension guide](https://www.xraph.com/docs/herald/guides/forge-extension), [HTTP API](https://www.xraph.com/docs/herald/api-reference/http-api) |

The Go API is on [pkg.go.dev](https://pkg.go.dev/github.com/xraph/herald). [CHANGELOG.md](CHANGELOG.md) lists what changed in each release, breaking changes first.

## FAQ

### How do I send email and SMS from Go with one API?

Register an email driver and an SMS driver, create a provider for each, and call `h.Send` with `Channel: "email"` or `Channel: "sms"`. To send the same template on both at once, call `h.Notify` with `Channels: []string{"email", "sms"}`. You get one result per channel.

### How does Herald choose a provider?

It tries a routing rule for the user, then the org, then the app. If none names a provider for the channel, it takes the enabled provider with the lowest `Priority`. `h.ResolveProvider` tells you which one would send and why, and `PreviewSend` shows the provider and sender without sending anything. You can skip the resolver by setting `ProviderID` on the request.

### Can I translate notification templates?

Yes. Add one template version per locale. A request for `pt-BR` uses the `pt-BR` version if there is one, then `pt`, then the version with an empty locale.

### Can I use Herald without Forge?

Yes. Everything in the [quick start](#quick-start) is plain Go. Only the `extension`, `extension/contract` and `api` packages need Forge.

### Are provider API keys stored in plain text?

Only if you don't configure a key. Set `credentials_key` (or `herald.WithCredentialKey`) and Herald encrypts credentials with AES-256-GCM before it stores them. `EncryptStoredCredentials` encrypts rows written before you had a key, and the REST API never returns credential values.

## Contributing

Issues and focused pull requests are welcome on [GitHub](https://github.com/xraph/herald/issues). Run Go commands with `GOWORK=off`:

```bash
GOWORK=off go build ./...
GOWORK=off go test -race ./...                   # root module only
make check                                       # fmt, vet and lint
cd drivers/slack && GOWORK=off go test -race ./...   # each driver is its own module
```

`./...` stops at every nested `go.mod`, so the root commands never touch `drivers/`, and CI doesn't test them either. If you change root API a driver uses, test that driver yourself. The PostgreSQL and MongoDB store suites skip unless you set `HERALD_TEST_POSTGRES_DSN` and `HERALD_TEST_MONGO_URI`. [AGENTS.md](AGENTS.md) has the details, the lint rules and how releases tag the root and every driver. Skip `make build`, `make run` and `make all`, which point at a `cmd/herald` that doesn't exist. Commits follow [Conventional Commits](https://www.conventionalcommits.org/), and user-visible changes go in [CHANGELOG.md](CHANGELOG.md).

## License

See the [Herald repository](https://github.com/xraph/herald) for license terms.

Herald is part of the [forgery stack](https://github.com/xraph/forge#the-forgery-stack) built on [Forge](https://github.com/xraph/forge), by [Rex Raphael](https://github.com/juicycleff) and [contributors](https://github.com/xraph/herald/graphs/contributors).
