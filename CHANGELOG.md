# Changelog

## v1.7.0

This release hardens providers, stores, templates and the REST API. Some of it breaks things, so read the first section before you upgrade.

### Breaking changes

- The templ dashboard is gone. `github.com/xraph/herald/dashboard`, with its pages, widgets, settings panel and `RegisterBridge`, no longer exists. The React plugin in forge-dashboard replaces it, and `MIGRATION.md` lists where each page, action, widget and bridge function went. Herald needs forge v1.12.0 and no longer depends on templ or forgeui.
- `message.Store` no longer has `UpdateMessageStatus`. It has `RecordDelivery(ctx, messageID, message.Delivery{Status, Error, ProviderMessageID, SentAt})`, which writes the whole outcome of a send in one call, and a new `CountMessages(ctx, appID, since)` that counts messages by status and channel. A store that lives outside this repo stops compiling until it implements both.
- `SendResult.ProviderID` is now always Herald's own provider ID. The ID the vendor handed back moved to `SendResult.ProviderMessageID`. If you stored the old value to look a message up at the vendor, read the new field.
- A send to someone who opted out returns the status `suppressed` and a message ID. It used to return `sent`, which was wrong, because nothing was sent.
- `Send` returns a resolver or store error as it is. It used to wrap every one of them as `ErrNoProviderConfigured`. That error now means one thing again: no provider handles the channel. If you matched on it to catch a database failure, match on the real error.
- FCM and webhook payloads carry only settings whose key starts with `data.`. If you relied on unprefixed custom keys reaching the payload, rename them in the provider's settings (`campaign` becomes `data.campaign`).
- Provider responses from the REST API no longer contain credential values, ever. They carry `credentials: [{key, protection, key_id}]` so you can see which keys are set and whether each is encrypted.
- `PUT /v1/providers/:id` takes a partial body. Leave a field out and it stays as it was, so a request without `enabled` no longer switches a provider off. Credentials merge into what's stored, and `remove_credentials` lists the keys to delete. A provider's channel and driver can't change after it's created.
- Changing a provider's `base_url` or `host` to a new value now needs its secret credentials in the same update. Without that, anyone who can edit a provider could point it at a server they run and collect the API key or password on the next send. The API never shows you credential values, and editing a provider shouldn't become a way to read them. Send the secrets again in `credentials` next to the new target. If you don't, `UpdateProvider` refuses with `ErrInvalidProvider` (400 over REST), names the setting and the keys to enter again, and writes nothing. A secret is whatever the driver's schema marks secret. For a driver with no schema, that's every credential. You don't need to send anything again to remove the setting (which puts the vendor's default back) or to set it to the value it already has. The check looks at the value the driver actually reads, which is the setting when there is one and the credential otherwise. So you can't get around it by putting `base_url` in `credentials`, or by removing the setting to let a credential copy through. `base_url` and `host` belong in settings, and Herald refuses them in credentials.
- Herald refuses a provider that keeps a setting in `credentials`: `base_url` and `host` for every driver, plus any field the driver's schema places in settings (SMTP's `host`, `port` and `use_tls`, for example). Create, update and seeding all check it. Seeding only logs a warning, as it does for every validation failure, and stores the row as you wrote it. Older docs got this wrong for more than SMTP. Their driver tables also listed Twilio's `from_number`, FCM's `project_id` and Resend's `base_url` as credentials. The docs match the schemas now, but a provider you built from the old ones may still carry those keys, so check your providers. A stored row like that still sends, but every update to it fails until you move the keys. One update does it: list them in `remove_credentials` and send the same values in `settings`. Keeping the same target needs no secrets.
- By-ID routes (providers, templates, versions and so on) take an optional `app_id`. A row that belongs to another app answers 404, the same as a row that doesn't exist. Leave `app_id` out and you mean the `""` app. List routes still require `app_id` and answer 400 without it. Their filters (`channel`, `status`) and paging (`offset`, `limit`) are optional, so `GET /v1/providers?app_id=app_a` lists every provider of the app.
- The REST API answers 400, 404 and 409 where it used to answer 500 for bad input, missing rows and duplicates.
- Template `category` and a version's `subject`, `html`, `text` and `title` are no longer required fields.
- A routing rule (scoped config) can only name a provider of its own app, on the channel of the field it sits in. `PUT /v1/config/app`, `/org/:orgId` and `/user/:userId` answer 400 for anything else and store nothing. Rules already stored that break this are skipped at send time with a warning in the log, and the resolver falls through to the next scope.
- The resolver returns a store failure instead of treating it as "no rule here". If the database is down while `Send` looks up a routing rule, you get that error back, not a send through the fallback provider.
- `GetPreference` and `GetScopedConfig` on the SQLite, Postgres and Mongo stores return `ErrPreferenceNotFound` and `ErrScopedConfigNotFound` for a missing row. They used to return `(nil, nil)`, so if your code only checked for a nil result, check the error now.
- The memory store's update and delete of a row that doesn't exist return the not-found sentinel. They used to succeed and do nothing.
- `Send` no longer reports every template load failure as `ErrTemplateNotFound`. A template that isn't there still is one. Anything else, a database error say, comes back as itself, wrapped with the template slug.

### Stores

Every backend (memory, SQLite, Postgres, Mongo) now returns the same not-found and duplicate sentinels, so `errors.Is` works the same wherever your data lives. `ErrDuplicateSlug` and `ErrDuplicateLocale` are real errors now. Before, they existed and nothing returned them.

An empty app ID matches only rows stored with `app_id = ''`, on every backend. It never means "every app".

`sent_at` and the vendor's message ID persist on every backend. Template lists load their versions, and saving a template again keeps the row's identity. The memory store copies on read and write and sorts like the SQL stores, so a test against it behaves like production.

The same conformance suite now runs against all four backends. It runs the Postgres and Mongo cases when you set `HERALD_TEST_POSTGRES_DSN` and `HERALD_TEST_MONGO_URI`.

Message and in-app notification lists now break `created_at` ties by ID on SQLite, Postgres and Mongo (newest first, then ID descending), as the memory store already did, so paging by offset no longer repeats or skips rows that share a timestamp. Every recipient of one multi-recipient send shares a timestamp, so this was common.

### Provider credentials

- Herald can encrypt credentials at rest. Set `credentials_key` (32 bytes, standard base64) and optionally `credentials_key_id`. Each value is encrypted on its own and carries its key ID, so `previous_credentials_keys` keeps old values readable while you rotate.
- At startup the extension tells you whether credentials are encrypted. With `credentials_key` set it logs an Info line naming the key ID. Without it, it warns that provider credentials are stored in plaintext. Key material never reaches the log.
- A stored value that has the encrypted prefix but can't be read (`credential.ErrMalformed`) answers 400 over REST, not 500.
- Credentials are decrypted at send time and nowhere else. If a provider's credentials were encrypted under a key that's no longer configured, `UpdateProvider` refuses with `ErrCredentialKeyUnavailable` and leaves the stored row alone.
- Turning the key on doesn't touch what's already stored. Run `EncryptStoredCredentials(ctx, appID)` (or `POST /v1/providers/encrypt`) once for each app to encrypt its existing rows. A second run changes nothing.
- Providers the templ dashboard created before this release are still plaintext. The one `EncryptStoredCredentials` run after you upgrade picks them up.
- Herald calls `Validate` when a provider is created or updated through the engine or the REST API. Providers seeded from `config.yaml` are validated too, but a failure there is logged and the provider is still created.
- `SendRequest.ProviderID` sends through a chosen provider. One that belongs to another app fails with `ErrProviderNotFound` and nothing goes out.
- Drivers can describe their fields: which ones they read, which are required, which are secret, and whether each one lives in credentials or settings. Every optional driver does.

### Templates

- `Variable.Default` is applied when a send leaves the variable out. The MFA SMS template no longer prints `<no value>` for a missing variable.
- `template.Resolve` picks the version for a locale, `Explain` lists the steps it took to get there, and `Renderer.RenderVersion` renders that version. `Renderer.Preview` renders any field and reports problems with a line and column, counted in characters, so a line with non-ASCII text before the error still points at the right place.

### Extension

`extension.WithAPIMiddleware` puts your middleware in front of Herald's routes. It applies them with `group.Use`. We don't use forge's `WithGroupMiddleware` or `WithGroupAuth`: neither guards routes in a sub-group, and `WithGroupAuth` only writes OpenAPI metadata, so a route could look protected and not be. That's also why there's no `api_auth_providers` setting.

Tenant isolation on the REST API rests on that middleware. Herald takes `app_id` from the request as given, so your middleware has to bind each caller to the app they're allowed to use. If you turn Herald's routes off and mount them yourself through `Extension.RegisterRoutes`, none of this runs and you need to put your own middleware on that router.

### Drivers

- APNs caches its JWT per signing key: team ID, key ID and a SHA-256 fingerprint of the public key. One driver serves every APNs provider, and before this they shared a single token. The IDs alone weren't enough, because they're settings anyone editing a provider can type, and a corrected `.p8` under the same key ID would have kept the old token.
- SMTP dials with the send's context and puts a 30 second deadline on the whole conversation (or the context's own deadline, if that comes sooner), so a server that stops answering can't hang a send.
- Discord keeps the query string already on your webhook URL when it adds `wait=true`.
- Discord, Slack and webhook errors no longer contain the webhook URL, which carries its token.
- `drivers/sendgrid` and `drivers/ses` still report one unused-code lint issue each (`sgResponse` and `sesMessage`). They were there before this release and we left them alone.

### Dashboard contract

The extension registers a `herald` contributor with forge's dashboard contract, so the React dashboard can manage Herald: providers, templates and their versions, the delivery log, a real test send, in-app inboxes, user preferences and routing rules. It needs forge v1.12.0 or later.

The dashboard works on one app per session. It takes the `app_id` claim from the session when there is one, then `dashboard_app_id` from the extension config, then the `""` app. A session whose `app_id` claim is present but empty, blank or not a string is refused, never moved to another app.

Operators can opt a user out of a notification type on a channel. They can't opt a user back in, and the dashboard never deletes a preference record, because a missing record means the user gets everything.

`api.ForgeAPI` no longer has its own routing check. `(*Herald).CheckRouting` is the same check, and the REST API and the dashboard both call it. `(*Herald).PreviewSend` tells you which provider and sender a send would use without sending anything. It also refuses a provider whose driver isn't registered on this server, with the same error `Send` returns. `send.test`, the dashboard's test send, refuses a request that gives both a template and a body.

With no rule from phone, an SMS send takes its sender from the driver's own setting: `from_number` for twilio and vonage, `originator` for messagebird. `PreviewSend` used to look only at a `from` setting, so it told you a twilio provider had no sender when it did. The send itself was always right, because each driver falls back to its own key.

### Still open

- Mongo stores template variables and preference overrides as BSON binary (`json.RawMessage`), where Postgres uses JSONB. Changing it needs a data migration for existing documents.
- `Send` returns only the first recipient's result on a multi-recipient send, and audits only that one.
- No driver learns about delivery or bounces afterwards. There are no receipt webhooks, no Twilio status callback and no Vonage DLR. `delivered` and `bounced` exist as statuses and nothing writes them.
- Only the text body is logged. HTML isn't.
- `Message.TemplateID` holds the template slug, not its ID. Changing it would break every existing row, so the dashboard resolves the slug instead.
- `Async` is stored and ignored. Delivery is always synchronous, and a crash mid-send leaves a row at `sending`.
- A scoped config's `default_locale` is stored and never used by `Send`.
- SES has no session-token support, and FCM's `access_token` is a static token that's never refreshed.
- SES builds its API host from the `region` setting when `base_url` is empty, so a crafted region can redirect a signed request. The secret key itself isn't sent.
- The webhook driver's `url` is a credential, so changing it alone isn't treated as a move, and the `signing_secret` Herald keeps signs payloads sent to the new URL.
- Connection targets are a fixed list of keys (`base_url` and `host`). A third-party driver whose target has another name isn't protected, because `driver.Field` has no flag that marks a connection target.
- An empty loop in a template can still burn CPU in a preview or a send. Memory is bounded per field: you get at most 1 MiB of output, and the string-building functions (`print`, `printf`, `println`, `html`, `js`, `urlquery` and the string helpers) stop at 1 MiB per result and 4 MiB per field. `{{range 100000000000}}{{end}}` builds nothing, though, so it runs until it's done. `printf` checks a worst case before it formats, so a call close to the limit can be refused even when its real result would fit.
