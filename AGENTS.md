# AGENTS.md

Herald is a multi-channel notification engine for Go. It sends email, SMS, push, in-app, webhook and chat messages through pluggable provider drivers, renders templates per locale, skips users who opted out, and resolves routing rules from app to org to user. You can use it as a plain library (`herald.New(...)`) or mount it in a Forge app through `extension/`.

## Modules

There are twelve Go modules in this repo, and the release workflow tags all of them.

| Module | Directory | Tag |
|---|---|---|
| `github.com/xraph/herald` | `.` | `vX.Y.Z` |
| `github.com/xraph/herald/drivers/<name>` | `drivers/<name>/` | `drivers/<name>/vX.Y.Z` |

The driver modules are apns, cloudflare, discord, mailgun, messagebird, postmark, sendgrid, ses, slack, vonage and webhook. Each one requires the root at a published version and carries `replace github.com/xraph/herald => ../../`, so a local build compiles against your working tree. That replace is intentional. Leave it in.

SMTP, Resend, Twilio, FCM, the sealed push driver and in-app are different. They live inside the root module under `driver/`, and the Forge extension registers them for you.

## Commands

Run go commands with `GOWORK=off` so you build exactly what CI builds. Make passes the variable through, so `GOWORK=off make test` works too.

```bash
GOWORK=off go build ./...
GOWORK=off go test -race ./...   # root module only
make test        # go test -v ./...  (CI's test job runs this target)
make vet         # go vet ./...
make lint        # golangci-lint run ./...
make lint-fix    # golangci-lint run --fix ./...
make fmt         # gofmt -s -w . then goimports -local github.com/xraph/herald
make check       # fmt, vet, lint
make sec         # gosec and govulncheck
make coverage    # coverage.out plus a per-function summary
```

`./...` stops at every nested `go.mod`, so none of the commands above touch `drivers/`. To work on a driver, run its tests from its own directory:

```bash
cd drivers/slack && GOWORK=off go test -race ./...
```

Herald is a library with no binary. `make build`, `make run`, `make install`, `make dev` and `make all` all point at `./cmd/herald`, which doesn't exist, so they fail. Don't use them.

### Tests and what they need

Most tests are plain unit tests and need nothing. Driver tests use `driver/drivertest`, an httptest mock server that is stdlib-only so the driver modules can import it without pulling in anything else.

Every store backend runs the same conformance suite, `store/storetest`. Memory and SQLite always run (SQLite opens a file under `t.TempDir()`). Postgres and Mongo need a server you start yourself, because there are no testcontainers in this repo.

| Env var | Package | Example value |
|---|---|---|
| `HERALD_TEST_POSTGRES_DSN` | `store/postgres` | `postgres://user:pass@localhost:55432/herald_conformance?sslmode=disable` |
| `HERALD_TEST_MONGO_URI` | `store/mongo` | `mongodb://localhost:57017` |

Without the variable, each suite calls `t.Skip` and the run still passes. CI sets neither one. That means CI has never run the Postgres or Mongo store, so if you change either, run its suite locally before you push. The Postgres suite truncates every `herald_*` table at the start of each subtest: give it a database you don't care about. Mongo creates a uniquely named database per subtest and drops it afterwards.

### What CI runs

`.github/workflows/ci.yml` calls the shared `xraph/workflows/.github/workflows/go-ci.yml@v1` on ubuntu, with the Go version `ci.yml` sets. Against the root module that means `make test`, a coverage run with `go test -race -covermode=atomic`, golangci-lint (the shared workflow pins its version), `gofmt -l .`, `make vet`, a `go mod tidy` diff on the root `go.mod` and `go.sum`, gosec and govulncheck. gosec runs standalone, so it ignores the excludes in `.golangci.yml` (it skips only G115), and any finding fails the job. A nested-modules job checks that no `drivers/*/go.mod` requires the root at `v0.0.0` or a non-semver version.

Two jobs live in `ci.yml` itself. Format runs `goimports -l -local github.com/xraph/herald .` over the whole tree, drivers included. Docs runs `pnpm types:check`, `pnpm lint` and `pnpm build` in `docs/`.

Nothing in CI compiles or tests a driver module. The release workflow is the first thing that does. If you changed root API that a driver uses, test that driver yourself.

## Layout

| Path | What lives there |
|---|---|
| `herald.go`, `options.go`, `config.go` | The `Herald` engine: `Send`, `Notify`, provider resolution, the functional options |
| `telemetry.go` | Span and metric names, the label values, and the reasons a send is rejected |
| `errors.go` | Root sentinels, many re-exported from `store` and `template` |
| `providers.go`, `routing.go`, `sendpreview.go` | Provider CRUD with validation, routing rule checks, `PreviewSend` |
| `id/` | TypeID identifiers (`hpvd_`, `htpl_`, `htpv_`, `hmsg_`, `hinb_`, `hprf_`, `hscf_`, `hdev_`) |
| `provider/`, `template/`, `message/`, `inbox/`, `preference/`, `scope/` | One entity each, plus its own `Store` interface |
| `template/` | Also the renderer, locale resolution and `guard.go`, which caps what a template may build |
| `scope/` | Scoped config and the app, org, user resolver |
| `credential/` | AES-256-GCM encryption of provider credentials (`enc:v1:<keyID>:...`) |
| `device/` | A user's push devices: `Device`, `QuietHours`, the `Store` interface, and `devicetest`, the conformance suite a device store runs. Memory and Postgres implement it; SQLite and Mongo don't |
| `pushgateway/` | An HTTP handler that registers device tokens as sealed handles and relays signed, sealed sends to FCM and APNs. It imports no driver module |
| `store/` | The aggregate `Store` and the shared not-found and duplicate sentinels |
| `store/memory`, `store/sqlite`, `store/postgres`, `store/mongo` | Backends (the last three take a `*grove.DB`) |
| `store/storetest/` | The conformance suite every backend runs |
| `internal/storeerr/` | Sentinels `scope` needs without importing `store` (which imports `scope`) |
| `internal/telemetry/` | The tracer, attribute keys and error classes every span shares. `metricsvec/` caches labelled series on a go-utils sink, and `telemetrytest/` has the span recorder and fake sink the tests use |
| `driver/` | The `Driver` interface, registry, field schemas and the built-in drivers |
| `drivers/` | Opt-in driver modules, one `go.mod` each |
| `bridge/` | Herald's own slim `Chronicle` audit interface, a slog fallback, and `chronicleadapter` for the real chronicle |
| `api/` | REST handlers on a `forge.Router` |
| `extension/` | The Forge extension: config, store discovery, route mounting |
| `extension/contract/` | The `herald` contributor for the Forge React dashboard, driven by `manifest.yaml` |
| `docs/` | Fumadocs site (Next.js, pnpm) |

## Conventions

Everything is built with functional options. Core options are `type Option func(*Herald) error`, and `New` returns the first error an option gives. The extension has its own `ExtOption func(*Extension)` in `extension/options.go`.

Errors are package-level sentinels made with `errors.New("herald: ...")`, and callers match them with `errors.Is`. If you write or change a backend, it has to return the same sentinels from `store/errors.go` for a missing row (`ErrProviderNotFound`, `ErrTemplateNotFound` and the rest) and for a unique violation (`ErrDuplicateSlug`, `ErrDuplicateLocale`). Returning `(nil, nil)` for a missing row is a bug. Wrap other errors with `%w` and context, and never put a credential value or a URL carrying a token into an error message: the `url_leak_test.go` files in the drivers exist to catch exactly that.

IDs come from `id/`. Each entity has a prefix constant, a type alias (`id.MessageID = id.ID`), `NewXID()` and `ParseXID()`. If you add an entity, add all of them.

Adding a field to an entity is more work than it looks. You'll change the struct, then the model and a new migration in each of `store/sqlite`, `store/postgres` and `store/mongo`, `store/memory/clone.go` if the field is a map, slice or pointer, and `store/storetest`, which has to populate the field with a real value, because a suite that only round-trips empty structs is how a serialization bug survives on one backend and not the others. Migrations are grove `migrate.Group`s named `herald`, versioned by timestamp strings such as `20240201000001`. Mongo migrations assert a `*mongomigrate.Executor`.

A driver implements `driver.Driver` (`Name`, `Channel`, `Send`, `Validate`) and usually `driver.Describer`, whose `Fields()` says which keys are credentials and which are settings. Get the placement right. Herald refuses a provider that keeps a setting in its credentials, and you'll get `ErrInvalidProvider` from create and update (seeding only logs a warning).

The core logs with `*slog.Logger` (`WithLogger`, defaulting to `slog.Default()`). `api/` and `extension/` log through `forge.Logger`. The extension doesn't hand its logger to the core.

### Telemetry

`Send` and `Notify` record spans on the global tracer provider and metrics on the sink from `WithMetrics`, which the extension fills with `app.Metrics()`. The observability docs page lists every span, attribute, metric and label value; keep it in step when you change one.

A driver that calls an HTTP API builds its client with `driver.HTTPClient(timeout, driver.Endpoint{...})`, never `&http.Client{}`. Give it a URL template with placeholders for anything variable in the path, and set `HideHost` when the user supplies the whole URL. The client injects no trace headers, since vendors are third parties.

Keep spans and labels narrow. A recipient, template data, subject, body, metadata, credential, user ID or URL must never reach either, and neither may an error's message, because driver errors quote vendor responses. A failed span gets a class and `exception.type` through `telemetry.Fail` or `telemetry.FailErr`. Labels take only the documented values, so clamp anything a caller supplies (`channelLabel` does this for the channel) and use a driver name only after the registry found it. Template slugs and provider IDs may go on spans, never on labels.

Telemetry tests install a span recorder as the global provider (`telemetrytest.Spans`), so they can't run in parallel. Each one feeds a value like `alice@example.com` through and checks `telemetrytest.Leaks` finds it nowhere.

The Forge extension embeds `*forge.BaseExtension`, loads config from `extensions.herald` or `herald`, and finds a `*grove.DB` and a `chronicle.Emitter` in the vessel container when they're registered. It provides `*herald.Herald` back into the container. `ExtensionVersion` in `extension/extension.go` is maintained by hand, and the release workflow doesn't touch it.

### Lint rules that bite

- `govet` runs every analyzer except `fieldalignment`, so `shadow` is on. Redeclaring `err` in an inner scope fails.
- `errcheck` has `check-blank: true`. If you drop an error on purpose, write `//nolint:errcheck // reason`. The codebase has dozens of these.
- `revive`'s `exported` rule wants a doc comment on every exported name, private receivers included.
- `nolintlint` is on, so a stale or reason-less `//nolint` gets flagged too.
- gosec, errcheck and gocritic are off in `_test.go` files.
- Imports go in three groups: stdlib, third party, then `github.com/xraph/herald/...`. Run `make fmt` and you're done.

Write commit subjects with conventional prefixes (`feat(contract):`, `fix(store):`, `chore(deps):`, `ci(release):`). If your change is visible to users, add it to `CHANGELOG.md`, which has a section per version with breaking changes listed first.

## Dependencies

Herald's root requires chronicle, grove (with its mongodriver, pgdriver and sqlitedriver), forge, go-utils and vessel from the xraph set, plus `go.jetify.com/typeid/v2`, the Mongo driver and the OpenTelemetry API (and the SDK, which only `internal/telemetry/telemetrytest` imports). `drivers/webhook` also requires relay. The other drivers need nothing beyond the root, which brings the OTel API with it through `driver`. Authsome consumes the root module. Versions live in each `go.mod`, so read them there.

To bump an xraph dependency:

```bash
GOWORK=off go get github.com/xraph/<mod>@vX.Y.Z   # once per module, in the module that requires it
./updatemod.sh                                    # go mod tidy in every directory with a go.mod
```

Keep grove and its three driver modules on the same version, the way `go.mod` has them now. For relay, run `go get` inside `drivers/webhook`.

Never commit a `go.work`, and never commit a `replace` that points at a sibling repo (`../grove`, `../../forge`). The `../../` replaces inside `drivers/*/go.mod` point back into this repo and are meant to be there.

## Releasing

You cut a release by dispatching the workflow. Don't push a tag by hand.

```bash
gh workflow run release.yml --ref main -f tag=vX.Y.Z
```

Before you run it, main's CI has to be green and every upstream xraph module herald needs (chronicle, grove, forge, vessel, and relay for the webhook driver) has to be released and pinned here. Authsome re-pins to the new herald afterwards.

The workflow then:

1. Creates `vX.Y.Z` on the dispatched commit and pushes it.
2. In the root, runs `go mod download`, `go mod tidy` with a diff check on `go.mod` and `go.sum`, `go build ./...` and `go test -race -count=1 ./...`.
3. Finds every `drivers/*/go.mod` and runs the same checks in each.
4. Tags each driver `drivers/<name>/vX.Y.Z` at the same commit and pushes those tags. A driver tag that already exists at that commit is skipped. One at a different commit stops the run.
5. Publishes a GitHub release with the commit log and a `go get` line per driver.

Discovery means a new driver ships on the next release without anyone editing a list.

The root tag goes out in step 1, before any verification. If a driver fails to tidy or test, `vX.Y.Z` is already on the remote with no driver tags, and re-running the dispatch fails at `git tag` because the tag exists. So before you dispatch, run `./updatemod.sh` and commit whatever it changes (the release fails on any tidy diff), then run every driver's tests. If it happens anyway, fix main and release the next patch.

The release doesn't touch the drivers' `require github.com/xraph/herald` line. It stays at whatever published version it names, and the replace hides that locally. So if a driver starts using root API that isn't in any published release, the driver tag from that release still names the old root, and someone who runs `go get` on the driver alone gets a root that lacks the API. Raise that require to the version you're about to release, in the same change, and cut the release at exactly that version. The replace covers it until the tag exists, and the workflow pushes the root tag before it checks any driver. The drivers require v1.8.0 for this reason, since they all call `driver.HTTPClient`.

## Branch rules

The ruleset on `main` blocks deletion and force-pushes. It requires no checks and no reviews, so a direct push to `main` goes through.
