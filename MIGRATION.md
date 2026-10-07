# Dashboard migration: templ to React shell

Herald's dashboard used to render server-side with templ and ForgeUI, from
`herald/dashboard/`. It now lives in the Forge dashboard's React shell as
`@forge-go/dashboard-plugin-herald`, reading the `herald` contract contributor
in `extension/contract`. The templ package is gone.

This file is the record of that move. We wrote it by walking every templ file
before deleting it: 27 `.templ` files (13 pages and a helpers file, 8
components, 4 widgets and 1 settings panel), plus `bridge.go`, `contributor.go`,
`data.go` and `manifest.go`. Once the directory is gone there's nothing left to
check against, so anything not written down here is a feature that went missing
by accident. Every page, column, badge, filter, empty state, action, widget and
bridge function is listed, and each one says whether it moved, changed, was
dropped, or is blocked, and why.

## What you need to do

If you ran the templ dashboard through forge's dashboard extension, there's
nothing to change in Herald. The extension stopped registering a templ
`DashboardContributor` in 29c6271, when forge removed its contributor package
and `DashboardAware`. It registers the contract contributor instead, and the
shell finds it.

If your own code imports `github.com/xraph/herald/dashboard`, that import has
to go, because the package no longer exists. That covers `dashboard.New`,
`dashboard.NewManifest` and `dashboard.RegisterBridge`. Herald never called
`RegisterBridge` itself, so the 24 ForgeUI bridge functions (`herald.getOverview`
and the rest) only ran if your application registered them on a bridge of its
own. Nothing replaces them as bridge functions. Their work moved into contract
intents, listed under "Bridge functions" below, and the REST API is still there
for anything else.

Add the plugin to your shell:

```tsx
import heraldPlugin from "@forge-go/dashboard-plugin-herald"

const plugins = [corePlugin, heraldPlugin]
```

The plugin's pages use Tailwind classes of their own, so the shell's stylesheet
has to scan the package. If your shell declares its sources with `@source`, add
`@source "<path to>/packages/plugin-herald/src";` next to the others.

The dashboard works on one app per session, and you choose which. It takes the
`app_id` claim from the session when there is one, then `dashboard_app_id` from
the extension config, and the `""` app when neither is set, which is what an
install with one app and no `app_id` claims gets. A session whose `app_id` claim is
present but empty, blank or not a string is refused, never moved to another
app. The templ dashboard showed the `""` app unless the URL carried an
`app_id` query parameter, and it believed whatever that parameter said (see
"Bugs found on the way").

Herald needs forge v1.12.0. It's the first forge release whose dashboard
packages import neither templ nor forgeui, so neither is in Herald's module
graph any more. If a workspace `replace` or another module holds forge back,
lift it.

Providers the templ dashboard created before v1.7.0 have plaintext credentials,
because the form wrote them before Herald could encrypt anything. Set
`credentials_key` and run `EncryptStoredCredentials` once per app, or press
"Encrypt stored credentials" on the overview, which does the same thing. The
CHANGELOG's v1.7.0 notes cover the key and its rotation.

## Bugs found on the way

Building the React pages meant reading the templ ones closely, and that turned
up bugs they'd been hiding. The ones below are what a templ dashboard user
could have run into. All of them are fixed, either in the engine for v1.7.0 or
because the React page works differently. The CHANGELOG's v1.7.0 notes list
the engine and store fixes in full, so we don't repeat them here.

- Any viewer could read another app. Every page took `app_id` from the query
  string with no check at all, so anyone who could open the dashboard could
  edit the URL and see another app's providers, templates, messages and
  inboxes, and the pages that write would write there too. The widgets and the bridge ignored the parameter and
  always read the `""` app. The contract takes the app from the session and
  refuses a claim it can't use.
- The provider page showed the last four characters of every credential.
  `maskCredential` printed eight dots and then the real tail of the value. The
  bridge did worse: `herald.getProvider` and `herald.getProviders` returned the
  whole provider row, credential values included, to the browser. The REST API
  did the same until v1.7.0. Now no response carries a credential value, the
  React pages show only each key's protection (encrypted or plaintext), and
  secret inputs are password fields that are cleared once a save succeeds.
- Credential inputs on the create form were plain text fields, so a key was on
  screen as you typed it, and stayed there for anyone behind you until the page
  changed.
- Enable, Disable and Delete on a provider or a template wrote straight to the
  store and threw the result away. An enable that failed reloaded the page as
  if it had worked. The bridge's create, update and toggle functions also went
  straight to the store, so they skipped validation and encryption. Every
  write now goes through the engine and reports what happened.
- Deleting a provider said nothing about the routing rules that named it, and
  left them pointing at nothing. The React confirm lists those rules first.
- Templates were saved without being parsed. A template with a broken action
  was stored as typed and only failed when something tried to send it. The
  template workspace runs Herald's renderer as you edit and puts each problem
  on its line and column before you save.
- Every new version was created live, so adding a locale changed what the next
  send in that locale used before anyone had read the content, and a typo in a
  new `fr` version went out on the next send in `fr`. A new locale now starts
  inactive.
- Once a version existed, nothing in the templ dashboard could change its
  content or remove it. The template page listed versions and that was all.
- The overview's numbers were wrong in several ways at once. "Messages, Total
  sent" counted every status, not sent ones, and stopped at 1,000. "Failed"
  and its "pending" line came from the newest 1,000 messages only, and
  "pending" lumped queued, sending and suppressed together. The Notification
  Stats widget's message count could only ever be 0 or 1, because it counted
  the result of a one-row query. Any store error read as zero. The React
  overview counts by status and channel over a window you pick, with no cap,
  and a failed count says it failed.
- The channel breakdown came out in a different order on every load. It
  ranged over a Go map.
- On a message, the Template link was always broken. `Message.TemplateID`
  holds the template's slug, and the link passed it where a template ID
  belongs. The React page resolves the slug and links to the template when it
  still exists.
- The message page never showed the body. The React page shows the logged text
  body and says that HTML isn't logged and that bodies are cut at
  `truncateBodyAt`.
- Send test said "Test notification sent successfully!" whenever `Send`
  returned no error, whatever status the result carried, so a send that was
  suppressed read as a success. It also sent with no confirmation. The React
  page confirms first, naming the provider, its driver, the channel and the
  recipient, and the result says what actually happened.
- Send test accepted a template slug and a raw body together and used the
  template. `send.test` refuses a request that gives both.
- The inbox handled "mark read" and "delete" for single notifications on the
  server, but no button ever sent them. The React rows have both.
- The bridge's `herald.updatePreferences` replaced a user's whole override map.
  Writing a map without an opt-out quietly opted the user back in, which an
  operator should never be able to do. The dashboard can only opt a user out.
- The messages and inbox lists showed the newest 50 rows with no way to see
  more, and the messages badge counted the rows on screen. Both now page with
  a cursor.
- The messages filter offered Queued, Delivered and Bounced, which Herald never
  writes, and had no Suppressed, which it does. The channel filters on
  templates and messages had no webhook or chat.

## Deliberately dropped

- The four widgets: `herald-stats`, `herald-recent-messages`,
  `herald-delivery-status` and `herald-channel-breakdown`. The React host has
  no generic widget slot, and the overview page replaces them, with counts
  that aren't capped at 1,000 rows. (The stats widget's message count was
  never right anyway, as above.)
- Retry on a failed message. It re-sent the stored subject and body as a raw
  message, and the stored body is the text part only, truncated, with no
  template data, so a "retry" sent something different from the original and
  showed nothing when the provider failed again. The message page has "Send a
  test to this recipient" instead, which opens Send test prefilled with the
  channel, recipient and template.
- The manifest's `searchable` capability. It was never implemented.
- The topbar: its title, bell logo, `#f59e0b` accent and search box. The shell
  themes every plugin the same way and has its own page search.
- The channel filter on the providers list. An app has a handful of providers,
  the list shows each one's channel, and the spec for the React page didn't
  carry the filter over. `providers.list` already takes a channel, so bringing
  it back is a change to the page alone.
- Max batch size, which the settings panel showed. `engine.info` still returns
  it as `maxBatchSize`, but no page displays it, because nothing on the
  dashboard depends on it.
- The overview's "Recent Messages" card and its quick action buttons. The
  messages list is newest first and one click away, and "New provider" and
  "New template" live on the lists they belong to.

## Blocked

Nothing is blocked. Everything the templ dashboard did either has a React page
or is dropped above for a reason.

## Page by page

Status is one of **migrated** (same thing, same place), **changed** (different
on purpose, with the reason), **dropped** (gone, with the reason) or
**blocked** (wanted, and waiting on work outside Herald).

A few things hold on every React page, so the tables don't repeat them. Every
page header names the app in view, from `engine.info`: "Default app" or the
app ID. Identifiers (IDs, slugs, driver names, locales, notification types,
vendor message IDs) are in mono. A timestamp is a date and time in your
browser's zone, and an absent value says "none". The templ pages printed dates
with no zone and a dash for nothing. Paged lists show the server's count in
the caption, never the length of the page.

### Route map

| templ route | React route | status |
|---|---|---|
| `/` | `/` (Overview) | migrated |
| `/providers` | `/providers` | migrated |
| `/providers/create` | `/new-provider` | changed: a create page never sits under the list, so no ID can collide with it |
| `/providers/detail` | `/providers/:id`, with edit at `/providers/:id/edit` | changed: the templ dashboard had no edit page |
| `/templates` | `/templates`, plus `/templates-without-fallback` | changed: the second list is new, linked from the overview |
| `/templates/create` | `/new-template` | changed: same reason as providers |
| `/templates/detail` | `/templates/:id`, the template workspace | changed: see Template detail |
| `/templates/versions/create` | the workspace's locale rail | changed: see Version create |
| `/messages` | `/messages` | migrated |
| `/messages/detail` | `/messages/:id` | migrated |
| `/inbox` | `/inbox` | migrated |
| `/preferences` | `/preferences` | changed: operators can opt a user out now |
| `/send-test` | `/send-test`, plus `/providers/:id/send-test` and `/messages/:id/send-test`, prefilled | changed |
| none | `/routing` | new: routing rules (scoped configs) and a "Who sends?" tester |

The templ links carried IDs as query parameters (`detail?id=`). The React
routes put them in the path, encoded.

### Overview

`pages/overview.templ`, `components/stat_card.templ`.

| templ | React | status |
|---|---|---|
| Title "Notifications Overview", "Monitor your notification delivery system at a glance." | "Notifications" | changed |
| "Send Test" button in the header | "Send test" in the nav | changed |
| Stat card Providers, "N active" | the Posture panel's Providers section ("Nothing can send until you add one", or until you enable one), from `overview.stats` | changed: the panel says what's wrong instead of printing a count |
| Stat card Templates, "Notification templates" | the Posture panel's Fallback coverage section: how many templates have no `""` version, linking to `/templates-without-fallback` | changed |
| Stat card Messages, "Total sent", capped at 1,000 | the "Accepted by providers" table: a row per channel, a column per status that has rows, over 24 hours, 7 days or 30 days, from `overview.stats` | changed: the old figure counted every status and stopped at 1,000 |
| Stat card Failed, "N pending" | the same table's Failed column | changed: "pending" mixed three statuses |
| Quick action "Add Provider" | "New provider" on the providers list | changed |
| Quick action "Create Template" | "New template" on the templates list | changed |
| Section "Channel Breakdown", a stat card per channel in random order | the table's rows, one per channel, sorted | changed |
| Card "Recent Messages", "The last 5 notification deliveries." | none | dropped: the messages list is newest first |
| Empty state "No messages yet." | "No messages in this window." | changed |
| none | Posture: whether credentials are encrypted and how many values are plaintext, with "Encrypt stored credentials" (`providers.encryptStored`) behind a confirm | new |
| none | Posture: whether the REST API has an auth middleware | new |

### Providers

`pages/providers.templ`, `components/provider_table.templ`.

| templ | React | status |
|---|---|---|
| Title "Providers", "Manage notification delivery providers." | "Providers" | migrated |
| Badge "N providers" | the table caption | changed |
| "Add Provider" button | "New provider" | migrated |
| Channel filter buttons: All, Email, SMS, Push, In-App, Webhook, Chat | none | dropped: see Deliberately dropped |
| Column Name | Name, linking to the provider | migrated |
| Column Channel, a coloured badge | Channel, plain text | changed: a channel is a category, not a signal |
| Column Driver | Driver, mono | migrated |
| Column Status, `EnabledBadge` | Status, enabled (outline) or disabled (secondary) | migrated |
| none | Priority and Credentials ("3 encrypted", "2 plaintext, 1 encrypted") | new |
| none | a callout above the table when no credential key is configured | new |
| Empty state "No providers found", "Create a provider to start sending notifications." | "No providers yet. Add one so Herald has somewhere to send." | changed |
| Data from `ListAllProviders` or `ListProviders` for the `""` app | `providers.list` for the session's app | changed |

### Provider create

`pages/provider_create.templ`, the `create_provider` handler in
`contributor.go`.

| templ | React | status |
|---|---|---|
| Title "Create Provider", "Configure a new notification delivery provider." | "New provider" | changed |
| Back button and Cancel | Cancel | changed |
| Field Name, required | Name | migrated |
| Field Channel, every channel | Channel, from `engine.info` | migrated |
| Field Driver, every registered driver whatever the channel | Driver, filtered to the chosen channel | changed |
| Field Priority, "Lower values = higher priority." | Priority | migrated |
| none | Enabled | new: the templ form always created an enabled provider |
| Credentials: three key and value text rows (the handler read up to ten) | a field per credential from the driver's schema, secret ones as password inputs; a driver without a schema gets key and value rows with a "secret" box per row | changed |
| Settings: three key and value rows | a field per setting from the driver's schema, or key and value rows | changed |
| Error banner with the engine's error | an alert titled "Could not create the provider", and the form keeps what you typed | changed |
| On success, the providers list | the new provider's page | changed |
| `h.CreateProvider` (since v1.7.0) | `providers.create`, the same engine call | migrated |

### Provider detail

`pages/provider_detail.templ`, the `enable`, `disable` and `delete` actions in
`contributor.go`.

| templ | React | status |
|---|---|---|
| Title: the provider's name, "Provider details and configuration." | the provider's name | migrated |
| "Disable", with "Disable this provider? It will stop handling notifications." | the Enabled switch on the edit page, `providers.update` | changed: the store write ignored its result |
| "Enable" | the same switch | changed |
| "Delete", "Delete this provider? This action cannot be undone." | Delete, behind a confirm that lists the routing rules that will dangle, `providers.delete` | changed |
| Card "Provider Information": ID, Driver, Priority, Status, Channel, Created, Updated | the summary list, from `providers.detail` | migrated |
| Card "Settings", every key and value | the Settings table | migrated |
| Card "Credentials", "Credential values are masked for security.", eight dots and the last four characters | the Credentials table: key, protection, key ID, never a value | changed: the tail was a leak |
| Quick link "View Messages", the messages list filtered to the provider's channel | none | dropped: a channel filter shows other providers' messages too. "Send a test through this provider" is there instead |
| none | "Used by": the routing rules that name this provider | new |
| none | "Edit", to `/providers/:id/edit`, where a secret can be replaced or removed but never shown | new |
| Error banner "Provider not found." | the page's not-found state | changed |

### Templates

`pages/templates.templ`, `components/template_table.templ`.

| templ | React | status |
|---|---|---|
| Title "Templates", "Manage notification templates and versions." | "Templates" | migrated |
| Badge "N templates" | the table caption | changed |
| "Create Template" button | "New template" | migrated |
| Channel filter: All, Email, SMS, Push, In-App | a Channel filter with every channel `engine.info` lists | changed: webhook and chat were missing |
| Category filter: All Categories, Auth, Transactional, Marketing, System | a Category filter with the same four | migrated |
| none | a Fallback filter, "Without a fallback version" | new |
| Column Name | Name, linking to the workspace | migrated |
| Column Slug, mono | Slug, mono | migrated |
| Column Channel, badge | Channel, plain text | changed |
| Column Category, `CategoryBadge` or a dash | Category | changed: plain text |
| Column Status, `EnabledBadge` | Status | migrated |
| none | Locales (inactive ones marked) and Origin (system or custom) | new |
| none | "Reset system templates" (`templates.resetDefaults`) behind a confirm | new |
| Empty state "No templates found", "Create a template to define notification content." | an empty table with "New template" | changed |
| Category filtering in the page handler, after the store read | `templates.list` with a `category` parameter | changed |

### Template create

`pages/template_create.templ`, the `create_template` handler.

| templ | React | status |
|---|---|---|
| Title "Create Template", "Define a new notification template with initial content." | "New template", "Its content, variables and other locales are edited in the template workspace once it exists." | changed |
| Section "Template Details": Name, Slug ("Unique identifier used in API calls."), Channel, Category | Name, Slug, Channel, Category, with Herald's own slug pattern checked before the round trip | migrated |
| Section "Initial Content Version": Locale (default `en`, "BCP 47 language tag."), Subject, Title, HTML Body, Text Body | "First version's locale" only; the content is written in the workspace | changed: the workspace checks the content as you type, and the form couldn't |
| A version written only when some content was filled in, and written live | `templates.create` writes the template and its first version, live, since it's the only one | changed |
| Error "Template created but initial version failed: ...", leaving the template behind | one command: if the version can't be written, the template is removed again | changed |
| Write straight to `CreateTemplate` in the store, unparsed | `templates.create` through the contract | changed |
| On success, the templates list | the new template's workspace | changed |

### Template detail

`pages/template_detail.templ`, the `enable`, `disable` and `delete` actions.

The React page for a template is the template workspace: a locale rail on the
left, an editor in the middle and a live preview on the right, with Variables
and Settings tabs and a "Review changes" diff before you save.

| templ | React | status |
|---|---|---|
| Title: the template's name, "Template details and versions." | the template's name | migrated |
| "Disable", "Disable this template?" | the Enabled switch on the Settings tab, saved with `templates.update` | changed |
| "Enable" | the same switch | changed |
| "Delete", "Delete this template? This action cannot be undone." | Delete on the Settings tab, behind a confirm, `templates.delete` | changed |
| Card "Template Information": ID, Slug, Channel, Category, Status, Created, Updated | the header and the Settings tab | changed |
| Card "Variables", a badge per variable name | the Variables tab: name, required, default and description, editable | changed: the templ page couldn't edit variables at all |
| Card "Versions", "Template content versions by locale." | the locale rail, from `templates.detail` | changed |
| Versions table: Locale, Subject, Active (Active or Inactive), Updated | the rail: each locale with live or inactive, plus a "Test a locale" tester that traces which version answers (`templates.resolve`) | changed |
| "Add Version" button | "Add locale" in the rail | changed: see Version create |
| Empty state "No versions yet. Add a version to define notification content." | the rail's empty state | changed |
| none | edit each version's subject, title, HTML and text in CodeMirror, with problems from `templates.render` on their line and column, and a preview with sample data | new: the templ dashboard couldn't edit a version |
| none | put a version live or take it offline, and delete it (`versions.update`, `versions.delete`), with a confirm that says which locales fall back where | new |
| Error banner "Template not found." | the page's not-found state | changed |

### Version create

`pages/version_create.templ`, the `create_version` handler.

| templ | React | status |
|---|---|---|
| Title "Add Version", "Create a new locale-specific content version." | the "Add locale" dialog in the workspace's locale rail | changed |
| Field Locale, required, default `en` | Locale, checked against Herald's locale pattern and the locales already there | changed |
| Fields Subject, Title, HTML Body, Text Body | none in the dialog: the new locale starts as a copy of the version you have open, unsaved edits included, and you edit it in the workspace | changed |
| Version written live | `versions.create`, and the new locale starts inactive | changed: a new locale shouldn't answer sends before anyone has read it |
| Error banner "Invalid template ID" | none: the dialog only exists inside a loaded template | dropped |

### Messages

`pages/messages.templ`, `components/message_table.templ`.

| templ | React | status |
|---|---|---|
| Title "Messages", "Notification delivery log." | "Messages" | migrated |
| Badge "N messages", the rows on screen | the caption | changed |
| Status filter: All, Queued, Sending, Sent, Delivered, Failed, Bounced | a Status filter with the four Herald writes (sending, sent, failed, suppressed), and a line saying delivered and bounced are never recorded | changed |
| Channel filter: All Channels, Email, SMS, Push, In-App | a Channel filter with every channel | changed: webhook and chat were missing |
| The newest 50 rows | 25 per page with a cursor pager, `messages.list` | changed |
| Column Recipient | Recipient | migrated |
| Column Channel, badge | Channel, plain text | changed |
| Column Status, `MessageStatusBadge` | Status: sent outline ("Accepted by provider"), sending and suppressed secondary, failed destructive | changed |
| Column Template, mono slug or a dash | Template, mono slug, or a "none" cell | migrated |
| Column Sent, relative time ("5m ago") from `CreatedAt` | Created, a timestamp | changed: the column was named Sent and showed the creation time |
| none | ID and Provider | new |
| A whole row clickable | the ID links to the message | changed |
| Empty state "No messages found", "Messages will appear here after notifications are sent." | three: "Nothing has been sent in this app yet.", "No messages match these filters." and "Nothing further." | changed |

### Message detail

`pages/message_detail.templ`, the `retry` action.

| templ | React | status |
|---|---|---|
| Title "Message Detail", "Delivery details for this notification." | "Message" | migrated |
| "Retry" on a failed message, "Retry sending this message?" | "Send a test to this recipient" | dropped: see Deliberately dropped |
| Fields ID, Recipient, Channel, Status, Subject, Created, Sent At | the same, from `messages.detail` | migrated |
| Field Provider, linking to `providers/detail?id=` | Provider, linking to the provider when it still exists | migrated |
| Field Template, linking to `templates/detail?id=<slug>`, which never resolved | Template, resolved by slug, linking when it still exists | changed: the link was broken |
| Card "Error" in a code block | the error in a `<pre>` | migrated |
| Card "Metadata" | Metadata | migrated |
| none | the text body, with the note that HTML isn't logged and bodies are cut at `truncateBodyAt` | new |
| none | the vendor's message ID and the environment | new |
| Error banner "Retry failed: ..." | none | dropped: with Retry |
| Error banner "Message not found." | the page's not-found state | changed |

### Inbox

`pages/inbox.templ`, `components/inbox_table.templ`, the `mark_all_read`,
`mark_read` and `delete` actions.

| templ | React | status |
|---|---|---|
| Title "Inbox", "Manage in-app notifications for users." | "Inbox" | migrated |
| User ID search with a 500 ms delay | User ID with a short debounce | migrated |
| Badge "N unread" | the unread count beside "Mark all read" | migrated |
| "Mark All Read", "Mark all notifications as read?" | "Mark all read" behind a confirm, `inbox.markAllRead` | migrated |
| `mark_read` and `delete` handled on the server with no button anywhere | "Mark read" and "Delete" on each row, `inbox.markRead` and `inbox.delete` | changed: they were unreachable |
| The newest 50 notifications | 25 per page with a cursor pager, `inbox.list` | changed |
| Column Title, cut at 40 bytes | Title, wrapping | changed: cutting bytes could split a character |
| Column Type | Type, mono | migrated |
| Column Status, `ReadBadge` | Read: when it was read, or "Unread" | changed |
| Column Created, relative time | Created, a timestamp | changed |
| none | Expires | new |
| Empty state "Enter a User ID", "Search for a user to view their in-app notifications." | the same prompt | migrated |
| Empty state "No notifications", "This user has no in-app notifications." | "No notifications for <user> in this app.", or "Nothing further." past the last page | changed |
| Error banner with the store error | an alert per failed command | changed |

### Preferences

`pages/preferences.templ`.

| templ | React | status |
|---|---|---|
| Title "Preferences", "View user notification preferences and opt-outs." | "Preferences" | migrated |
| User ID search with a 500 ms delay | User ID with a short debounce | migrated |
| Card "Preference Details": User ID, App ID, Created, Updated | the header names the app; the user is the one you searched | changed |
| Card "Channel Overrides", "Per-notification-type channel preferences." | the overrides matrix, from `preferences.get` | migrated |
| A row per type in the user's overrides, in random order | a row per type in the overrides plus the app's template slugs (`knownTypes`), sorted | changed |
| Columns Email, SMS, Push, In-App, each "Default", "Active" or "Opted Out" | the same four channels, each "Default", "On" or "Opted out" | changed: "Active" read like a status |
| Read only | "Opt out" on any cell that isn't opted out, behind a confirm naming the user, the type and the channel, `preferences.optOut` | changed |
| none | a line saying there's no way back in, and why | new |
| Empty state "Enter a User ID", "Search for a user to view their notification preferences." | the same prompt | migrated |
| Empty state "No preferences found", "This user has no custom notification preferences." | the matrix, with every cell "Default" for a user who has no record; only when the app has no templates and the user no opt-outs does it say there's nothing to show | changed: a user with no record can still be opted out |

### Send test

`pages/send_test.templ`, the `send_test` handler.

| templ | React | status |
|---|---|---|
| Title "Send Test Notification", "Send a test notification to verify your setup." | "Send test" | migrated |
| Field Channel | Channel | migrated |
| none | Provider, optional; left empty, the page shows which provider the resolver picks and why (`send.resolve`) | new |
| Field Recipient, "e.g., user@example.com or +1234567890" | Recipient | migrated |
| Field "Template Slug (optional)", a free text slug | a template picker, a locale, and a form built from the template's declared variables, with the rendered preview inline | changed |
| Fields Subject and Body | Subject and Body when no template is picked | migrated |
| "Send Test" submits straight away | "Send test" opens a confirm naming the provider, its driver, the channel and the recipient, and says so when the provider is disabled | changed |
| Banner "Test notification sent successfully!" on any send without an error | a result for each outcome: "Accepted by" the provider with the vendor's ID and the note that delivery isn't confirmed, the provider's error in a `<pre>` for failed, the opt-out explained for suppressed | changed: the old banner called a suppressed send a success |
| Card "Send Result": Message ID, Status, Provider, Error | the result card, plus a line when the message log couldn't be written, and when the provider that sent differs from the one you confirmed | changed |
| `h.Send` with both a template and a body accepted | `send.test`, which refuses both | changed |

### Settings

`settings/config.templ`, the `herald-config` panel ("Notification Settings",
"Configure notification engine behavior"). `engine.info` replaces it and feeds
every page header.

| templ | React | status |
|---|---|---|
| Card "General Configuration", "Current notification engine settings" | none as a page | changed: each value shows where it matters |
| Default Locale | `defaultLocale`, used by the workspace's locale tester and Send test | changed |
| Max Batch Size | `maxBatchSize` in `engine.info`, not displayed | dropped: see Deliberately dropped |
| Truncate Body At, "N chars" | the note on a message's body, in bytes | changed: the limit is bytes |
| Card "Registered Drivers": Driver and Channel | the driver list on the provider form, filtered by channel, with each driver's fields from its schema | changed |
| A bare list of driver names when no channel was known | none | dropped: every driver reports its channel |

### Widgets

`widgets/stats.templ`, `widgets/recent_messages.templ`,
`widgets/delivery_status.templ`, `widgets/channel_breakdown.templ`. All four
are dropped, for the reason under "Deliberately dropped".

| templ | React | status |
|---|---|---|
| `herald-stats`, "Notification Stats", "Provider, template, and message counts", size md, every 60 seconds: Providers ("N active"), Templates, Messages ("Total sent", 0 or 1) | the overview | dropped |
| `herald-recent-messages`, "Recent Messages", "Latest deliveries", size md, every 30 seconds: the last 5 messages, "No recent messages." | the messages list | dropped |
| `herald-delivery-status`, "Delivery Status", "Success and failure breakdown", size lg, every 60 seconds: Sent ("N% success rate"), Failed ("Delivery failed"), Pending ("In progress") | the overview's table | dropped |
| `herald-channel-breakdown`, "Channel Breakdown", "Messages per channel type", size md, every 60 seconds: a stat card per channel, "No channel data yet." | the overview's table | dropped |

### Navigation and manifest

`manifest.go`.

| templ | React | status |
|---|---|---|
| Nav item Overview, group Notifications, icon layout-dashboard | Overview, group Notifications | migrated |
| Nav item Providers, group Notifications, icon server | Providers, group Notifications | migrated |
| Nav item Templates, group Notifications, icon file-text | Templates, group Notifications | migrated |
| Nav item Messages, group Notifications, icon send | Messages, group Notifications | migrated |
| Nav item Inbox, group Delivery, icon inbox | Inbox, group Notifications | changed: one group for the whole plugin |
| Nav item Preferences, group Configuration, icon users | Preferences, group Notifications | changed |
| none | Routing and Send test, group Notifications | new |
| Topbar action "Send Test", linking to `/send-test` | the "Send test" nav item | changed |
| Display name "Herald", icon bell, version "1.0.0", layout "extension", sidebar shown | plugin label "Herald" | changed: the shell decides layout and sidebar |
| Topbar title "Herald", logo bell, accent `#f59e0b`, search shown | none | dropped: see Deliberately dropped |
| Settings descriptor `herald-config` | `engine.info` | changed: see Settings |
| Widget descriptors | none | dropped: see Widgets |
| Capability `searchable` | none | dropped: never implemented |

### Bridge functions

`bridge.go`. Each function read or wrote the `""` app (or an `app_id` the
caller sent), straight through the store unless noted.

| templ bridge function | contract intent | status |
|---|---|---|
| `herald.getOverview` | `overview.stats` | changed: counts by status and channel, no cap |
| `herald.getMessageCounts` | `overview.stats` | changed |
| `herald.getProviders` | `providers.list` | changed: no credential values |
| `herald.getProvider` | `providers.detail` | changed: no credential values |
| `herald.createProvider` | `providers.create` | changed: validated and encrypted by the engine |
| `herald.updateProvider` | `providers.update` | changed: a partial update; secrets merge, and a new `base_url` or `host` needs them again |
| `herald.deleteProvider` | `providers.delete` | migrated |
| `herald.toggleProvider` | `providers.update` with `enabled` | changed |
| `herald.getTemplates` | `templates.list` | migrated |
| `herald.getTemplate` | `templates.detail` | changed: adds which version answers each locale |
| `herald.createTemplate` | `templates.create` | changed: with its first version |
| `herald.updateTemplate` | `templates.update` | changed: a partial update |
| `herald.deleteTemplate` | `templates.delete` | migrated |
| `herald.createVersion` | `versions.create` | changed: starts inactive |
| `herald.getMessages` | `messages.list` | changed: cursor paging |
| `herald.getMessage` | `messages.detail` | changed: resolves the template slug and the provider |
| `herald.getInbox` | `inbox.list` | changed: cursor paging |
| `herald.markRead` | `inbox.markRead` | migrated |
| `herald.markAllRead` | `inbox.markAllRead` | migrated |
| `herald.deleteNotification` | `inbox.delete` | migrated |
| `herald.getPreferences` | `preferences.get` | changed: adds the known notification types |
| `herald.updatePreferences` | `preferences.optOut` | changed: one opt-out at a time, never back in |
| `herald.sendTest` | `send.test` | changed: refuses a template and a body together, and can pin a provider |
| `herald.getConfig` | `engine.info` | changed: adds the app in view, drivers with their schemas, encryption and API protection |

### Shared components and helpers

`components/channel_badge.templ`, `components/empty_state.templ`,
`components/inbox_table.templ`, `components/message_table.templ`,
`components/provider_table.templ`, `components/stat_card.templ`,
`components/status_badge.templ`, `components/template_table.templ`,
`pages/helpers.templ`. The four tables are covered on the pages that rendered
them: `InboxTable` on Inbox, `MessageTable` on Messages and the overview,
`ProviderTable` on Providers, `TemplateTable` on Templates.

| templ | React | status |
|---|---|---|
| `ChannelBadge`: a different badge variant per channel | plain text | changed: as on every list, see Providers |
| `MessageStatusBadge`: sent and delivered default, failed and bounced destructive, the rest secondary | `MessageStatusBadge` in `badges.tsx`, with the reasons written down | changed |
| `EnabledBadge`: "Active" or "Disabled" | `EnabledBadge`: enabled (outline) or disabled (secondary) | changed |
| `ReadBadge`: "Read" or "Unread" | the Read column's timestamp | changed |
| `CategoryBadge` | plain text | changed |
| `EmptyState`: a 48 pixel icon, a title, a description | the kit's empty states | changed |
| `StatCard` and `resolveIcon` | none | dropped: the overview has no stat cards |
| `fieldRow` | the kit's `DescriptionList` | migrated |
| `codeBlock` | a `<pre>` | migrated |
| `sectionHeader` | plain headings | migrated |
| `errorBanner` | the kit's alerts, titled with what failed | changed |
| `successBanner`, green | a status line | changed: the dashboard uses no green |
| `maskCredential` | protection labels, never a value | changed: see Bugs found on the way |
| `formatDate` and `formatDateTime`, with a dash for an empty time | the kit's `Timestamp` and "none" cells | changed |
| `formatTimeAgo`, `msgTimeAgo` and `inboxTimeAgo` | the kit's `Timestamp` | changed |
| `truncateString` and `truncateStr`, which cut bytes | wrapping text | changed |
| `formatJSON` | none | dropped: never used |
| A whole table row clickable through `hx-get` | a link in the row | changed |
| Back buttons on every create and detail page | Cancel on create pages; the sidebar elsewhere | changed |
| Browser `hx-confirm` dialogs | `ConfirmDialog`, which shows errors inside and can't close while a command runs | changed |
| htmx swaps of `#content` with `hx-push-url` | the shell's router | changed |

## Still open

These are known gaps. Most were never in the templ pages either, but the React
pages show them plainly where the templ ones didn't.

- Nothing learns about delivery or bounces after a send. There are no receipt
  webhooks, so `delivered` and `bounced` never appear, and the dashboard says
  "Accepted by provider" rather than "delivered".
- Only the text body is logged, cut at `truncateBodyAt`. HTML bodies aren't.
- `Message.TemplateID` holds the template's slug, not its ID. The dashboard
  resolves the slug. Changing the field would break every existing row.
- `Async` is stored and ignored. Delivery is always synchronous, and a crash in
  the middle of a send leaves a row at `sending`.
- A scoped config's `default_locale` is stored and never used by `Send`. The
  routing page labels it that way.
- Send test confirms the provider the resolver picks, but doesn't pin it. If
  the routing changes between the confirm and the send, the result card says
  the sending provider differs. We report the drift after the fact instead of
  preventing it, because pinning a provider changes the sender, bypasses
  Enabled and audits the send as "chosen".
- An empty loop in a template can still burn CPU, and the workspace's preview
  runs the real renderer, so `{{range 100000000000}}{{end}}` in the editor
  keeps a server core busy until it finishes. Memory is bounded per field.
- Two people editing one template at once: the workspace rebases your draft on
  each new answer from the server and only writes the fields you changed, but
  two saves of the same field still mean the last one wins.
- `a-h/templ` and `forgeui` are out of Herald's module graph on forge v1.12.0.
  If a later forge release pulls either back in through its own dashboard
  packages, that's forge's dependency, not Herald's.
