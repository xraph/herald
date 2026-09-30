# Changelog

## v1.7.0

This release hardens providers, stores, templates and the REST API. Some of it breaks things, so read the first section before you upgrade.

### Breaking changes

- `message.Store` no longer has `UpdateMessageStatus`. It has `RecordDelivery(ctx, messageID, message.Delivery{Status, Error, ProviderMessageID, SentAt})`, which writes the whole outcome of a send in one call, and a new `CountMessages(ctx, appID, since)` that counts messages by status and channel. A store that lives outside this repo stops compiling until it implements both.
- `SendResult.ProviderID` is now always Herald's own provider ID. The ID the vendor handed back moved to `SendResult.ProviderMessageID`. If you stored the old value to look a message up at the vendor, read the new field.
- A send to someone who opted out returns the status `suppressed` and a message ID. It used to return `sent`, which was wrong, because nothing was sent.
- `Send` returns a resolver or store error as it is. It used to wrap every one of them as `ErrNoProviderConfigured`. That error now means one thing again: no provider handles the channel. If you matched on it to catch a database failure, match on the real error.
- FCM and webhook payloads carry only settings whose key starts with `data.`. If you relied on unprefixed custom keys reaching the payload, rename them in the provider's settings (`campaign` becomes `data.campaign`).
- Provider responses from the REST API no longer contain credential values, ever. They carry `credentials: [{key, protection, key_id}]` so you can see which keys are set and whether each is encrypted.
- `PUT /v1/providers/:id` takes a partial body. Leave a field out and it stays as it was, so a request without `enabled` no longer switches a provider off. Credentials merge into what's stored, and `remove_credentials` lists the keys to delete. A provider's channel and driver can't change after it's created.
- Changing a provider's `base_url` or `host` to a new value now needs its secret credentials in the same update. Without that, anyone who can edit a provider could point it at a server they run and collect the API key or password on the next send. The API never shows you credential values, and editing a provider shouldn't become a way to read them. Send the secrets again in `credentials` next to the new target. If you don't, `UpdateProvider` refuses with `ErrInvalidProvider` (400 over REST), names the setting and the keys to enter again, and writes nothing. A secret is whatever the driver's schema marks secret. For a driver with no schema, that's every credential. You don't need to send anything again to remove the setting (which puts the vendor's default back) or to set it to the value it already has.
- By-ID routes (providers, templates, versions and so on) take an optional `app_id`. A row that belongs to another app answers 404, the same as a row that doesn't exist. Leave `app_id` out and you mean the `""` app. List routes still require `app_id` and answer 400 without it. Their filters (`channel`, `status`) and paging (`offset`, `limit`) are optional, so `GET /v1/providers?app_id=app_a` lists every provider of the app.
- The REST API answers 400, 404 and 409 where it used to answer 500 for bad input, missing rows and duplicates.
- Template `category` and a version's `subject`, `html`, `text` and `title` are no longer required fields.

### Stores

Every backend (memory, SQLite, Postgres, Mongo) now returns the same not-found and duplicate sentinels, so `errors.Is` works the same wherever your data lives. `ErrDuplicateSlug` and `ErrDuplicateLocale` are real errors now. Before, they existed and nothing returned them.

An empty app ID matches only rows stored with `app_id = ''`, on every backend. It never means "every app".

`sent_at` and the vendor's message ID persist on every backend. Template lists load their versions, and saving a template again keeps the row's identity. The memory store copies on read and write and sorts like the SQL stores, so a test against it behaves like production.

The same conformance suite now runs against all four backends. It runs the Postgres and Mongo cases when you set `HERALD_TEST_POSTGRES_DSN` and `HERALD_TEST_MONGO_URI`.

### Provider credentials

- Herald can encrypt credentials at rest. Set `credentials_key` (32 bytes, standard base64) and optionally `credentials_key_id`. Each value is encrypted on its own and carries its key ID, so `previous_credentials_keys` keeps old values readable while you rotate.
- Credentials are decrypted at send time and nowhere else. If a provider's credentials were encrypted under a key that's no longer configured, `UpdateProvider` refuses with `ErrCredentialKeyUnavailable` and leaves the stored row alone.
- Turning the key on doesn't touch what's already stored. Run `EncryptStoredCredentials(ctx, appID)` (or `POST /v1/providers/encrypt`) once for each app to encrypt its existing rows. A second run changes nothing.
- The templ dashboard creates providers through the engine too, so they're validated and encrypted like any other. Rows it wrote before this release are still plaintext, and the one `EncryptStoredCredentials` run after you upgrade picks them up.
- Herald calls `Validate` when a provider is created or updated through the engine or the REST API. Providers seeded from `config.yaml` are validated too, but a failure there is logged and the provider is still created.
- `SendRequest.ProviderID` sends through a chosen provider. One that belongs to another app fails with `ErrProviderNotFound` and nothing goes out.
- Drivers can describe their fields: which ones they read, which are required, which are secret, and whether each one lives in credentials or settings. Every optional driver does.

### Templates

- `Variable.Default` is applied when a send leaves the variable out. The MFA SMS template no longer prints `<no value>` for a missing variable.
- `template.Resolve` picks the version for a locale, `Explain` lists the steps it took to get there, and `Renderer.RenderVersion` renders that version. `Renderer.Preview` renders any field and reports problems with a line and column, counted in characters, so a line with non-ASCII text before the error still points at the right place.

### Extension

`extension.WithAPIMiddleware` puts your middleware in front of Herald's routes. It applies them with `group.Use`. We don't use forge's `WithGroupMiddleware` or `WithGroupAuth`: neither guards routes in a sub-group, and `WithGroupAuth` only writes OpenAPI metadata, so a route could look protected and not be. That's also why there's no `api_auth_providers` setting.

### Drivers

- APNs caches its JWT per team ID and key ID. One driver serves every APNs provider, and before this they shared a single token.
- SMTP dials with the send's context and puts a 30 second deadline on the whole conversation (or the context's own deadline, if that comes sooner), so a server that stops answering can't hang a send.
- Discord keeps the query string already on your webhook URL when it adds `wait=true`.
- Discord, Slack and webhook errors no longer contain the webhook URL, which carries its token.
- `drivers/sendgrid` and `drivers/ses` still report one unused-code lint issue each (`sgResponse` and `sesMessage`). They were there before this release and we left them alone.

### Still open

- Mongo stores template variables and preference overrides as BSON binary (`json.RawMessage`), where Postgres uses JSONB. Changing it needs a data migration for existing documents.
- `Send` returns only the first recipient's result on a multi-recipient send, and audits only that one.
- No driver learns about delivery or bounces afterwards. There are no receipt webhooks, no Twilio status callback and no Vonage DLR. `delivered` and `bounced` exist as statuses and nothing writes them.
- Only the text body is logged. HTML isn't.
- `Message.TemplateID` holds the template slug, not its ID. Changing it would break every existing row, so the dashboard resolves the slug instead.
- `Async` is stored and ignored. Delivery is always synchronous, and a crash mid-send leaves a row at `sending`.
- A scoped config's `default_locale` is stored and never used by `Send`.
- SES has no session-token support, and FCM's `access_token` is a static token that's never refreshed.
