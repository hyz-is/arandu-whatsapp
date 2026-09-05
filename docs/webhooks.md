# Webhooks

Technical documentation of the webhooks implemented in the executable code as it stands.

## Contents
- [Overview](#overview)
- [Configuration](#configuration)
- [Event map](#event-map)
- [HTTP headers](#http-headers)
- [Standard envelope](#standard-envelope)
- [Instance structure](#instance-structure)
- [Delivery and error handling](#delivery-and-error-handling)
- [Events](#events)
- [Unsupported or ignored events](#unsupported-or-ignored-events)

## Overview

Webhooks are asynchronous HTTP `POST` requests the application sends to a URL the consumer configures. Every delivery carries a common envelope holding the external event name, the instance the event came from, the event-specific `data` and the `timestamp` the webhook was created at.

There are two possible destinations. With the default `Config.Prefix`, the instance webhook is configured through `PUT /whatsapp/instances/{instance}/webhook`; the dispatcher reads that configuration straight from the database and creates deliveries only for events whose flags are enabled in `events`. The global webhook is configured through `Config.Webhooks.GlobalURL` and `Config.Webhooks.GlobalEnabled`; when enabled it receives every supported event, without applying the instance flags.

For each enabled destination, the module saves an immutable snapshot of the URL, body and headers in `whatsapp_webhook_deliveries` and, in the same transaction, inserts a job carrying only the `deliveryId`. If the instance has no webhook enabled and the global webhook is disabled, the event is dropped without an error.

Deliveries use Hesape's native `DatabaseQueue`. The application registers the handler with `Module.RegisterJobHandlers`, includes `whatsapp.WebhookQueueName` among the queues `jobs.NewModule` watches, and runs `aru queue:work --queue=whatsapp-webhooks --workers=N`. HTTP `2xx` responses are a success; network errors, timeouts and non-`2xx` responses return an error so the job retries with the same `X-Arandu-Delivery-ID`. The relative order between events is not guaranteed.

- Document version: `1.2.0`.
- Official events documented: `27`.
- Constants package: `internal/database/types/webhook.go`.
- Dispatcher: `internal/webhook/manager.go`.
- Audited whatsmeow version: `v0.0.0-20260904121843-28bfe537ea6a`.

Examples of events with static fields are verified with `hesape/jsonschema` v0.12. For events marked as carrying dynamic fields, the presence and root type of `data` are still verified, but the object is neither closed nor validated field by field: that builder cannot express open objects, and closing these would reject legitimate properties coming from whatsmeow.

## Configuration

### Typed configuration

The module reads no environment variables. The host application provides:

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `Config.Webhooks.GlobalURL` | URL | empty | HTTP or HTTPS URL of the global webhook. |
| `Config.Webhooks.GlobalEnabled` | boolean | `false` | Enables the global webhook; requires `GlobalURL`. |
| `Config.Webhooks.SigningSecret` | string | empty | HMAC secret of at least 32 bytes; required before enabling any webhook. |
| `Config.Webhooks.Retention` | duration | `720h` (30 days) | Longest a snapshot is retained, failures included; zero uses the default. |
| `Config.Webhooks.Workers` | integer | ignored | Obsolete; set `--workers=N` on `aru queue:work` instead. |
| `Config.Webhooks.QueueSize` | integer | ignored | Obsolete; the native queue is durable in the database. |

The host application also has to apply the `DatabaseQueue` migrations, register the module's handler on the worker and consume the `whatsapp-webhooks` queue.

### Instance webhook

To configure or update it:

```http
PUT /whatsapp/instances/beplus/webhook HTTP/1.1
Cookie: arandu_session=<host-session>
Content-Type: application/json
```

To read it:

```http
GET /whatsapp/instances/beplus/webhook HTTP/1.1
Cookie: arandu_session=<host-session>
```

Both routes authenticate through the Arandu session and require Grants issued by the policy for `ActionWebhookSet` and `ActionWebhookView` respectively. Configuration responses use the Arandu envelope:

```json
{
  "data": {
    "enabled": true,
    "events": {
      "callUpsert": true,
      "chatsDeleted": true,
      "chatsUpdated": true,
      "connectionUpdated": true,
      "contactsUpdated": true,
      "contactsUpsert": true,
      "groupsParticipantsUpdated": true,
      "groupsUpdated": true,
      "groupsUpsert": true,
      "historySync": true,
      "identityUpdated": true,
      "labelsAssociation": true,
      "labelsEdit": true,
      "mediaRetry": true,
      "messagesDeleted": true,
      "messagesStarred": true,
      "messagesUndecryptable": true,
      "messagesUpdated": true,
      "messagesUpsert": true,
      "newsLetter": true,
      "presenceUpdated": true,
      "profilePictureUpdated": true,
      "qrcodeUpdated": true,
      "sendMessage": true,
      "settingsUpdated": true,
      "statusInstance": true,
      "userAboutUpdated": true
    },
    "url": "https://example.com/webhooks/beplus"
  }
}
```

`url` has to use `http` or `https` and be at most 500 characters long. An absent `enabled` is taken as `true` on creation and update. When `events` is omitted the existing flags are preserved; when `events` is `{}` the flags are removed. Unknown fields in `events` are rejected.

## Event map

| Flag | External event | Description |
| --- | --- | --- |
| `callUpsert` | `call.upsert` | A voice or video call was updated. |
| `chatsDeleted` | `chats.delete` | A chat was deleted or cleared. |
| `chatsUpdated` | `chats.updated` | Chat properties were updated. |
| `connectionUpdated` | `connection.update` | The instance connection state changed. |
| `contactsUpdated` | `contacts.update` | An existing contact was partially updated. |
| `contactsUpsert` | `contacts.upsert` | A contact was created or updated in the local records. |
| `groupsParticipantsUpdated` | `groups.participants.update` | Group participants changed. |
| `groupsUpdated` | `groups.update` | Group metadata was partially updated. |
| `groupsUpsert` | `groups.upsert` | A group was created, discovered or synchronized. |
| `historySync` | `history.sync` | A history synchronization was received from WhatsApp. |
| `identityUpdated` | `identity.update` | A contact's cryptographic identity changed. |
| `labelsAssociation` | `labels.association` | A label was associated with or removed from a chat or message. |
| `labelsEdit` | `labels.edit` | A label was created, changed or removed. |
| `mediaRetry` | `media.retry` | Result or error of a media retry attempt. |
| `messagesDeleted` | `messages.delete` | A message was removed locally by the DeleteForMe event. |
| `messagesStarred` | `messages.star` | A message was starred or unstarred. |
| `messagesUndecryptable` | `messages.undecryptable` | A message received that could not be decrypted. |
| `messagesUpdated` | `messages.update` | The receipt or status of an already known message was updated. |
| `messagesUpsert` | `messages.upsert` | A message received and persisted by the application. |
| `newsLetter` | `news.letter` | Events related to newsletters and channels. |
| `presenceUpdated` | `presence.updated` | User presence or chat presence was updated. |
| `profilePictureUpdated` | `profile.picture.update` | The profile picture of the instance itself or of another JID was updated. |
| `qrcodeUpdated` | `qrcode.updated` | A new QR code is available for pairing the instance. |
| `sendMessage` | `send.message` | A message sent through the API, after a successful send and persistence. |
| `settingsUpdated` | `settings.update` | User or instance settings were updated. |
| `statusInstance` | `status.instance` | Operational state events or warnings from the instance. |
| `userAboutUpdated` | `user.about.update` | A user's about text was updated. |

## HTTP headers

| Header | Example | Description |
| --- | --- | --- |
| `Content-Type` | `application/json` | Payload format. |
| `User-Agent` | `Arandu-WhatsApp/1.0` | Identifies the sender of the webhook. |
| `x-request-id` | `UUID or request id from the context` | Tracing id for the delivery; it is not a guaranteed idempotency key. |
| `x-owner-jid` | `Owner JID or an empty string` | Owner of the instance when available. |
| `x-instance-name` | `Instance name` | Public name of the instance. |
| `x-instance-id` | `1` | Internal numeric identifier of the instance. |
| `x-webhook-event` | `External event name` | The same value as the event field in the envelope. |
| `X-Arandu-Delivery-ID` | `Delivery UUID` | Stable key across retries; use it for idempotent deduplication. |
| `X-Arandu-Timestamp` | `Unix timestamp` | Signed instant of the HTTP attempt; it is renewed between retries. |
| `X-Arandu-Signature` | `sha256=<hex>` | HMAC-SHA256 of timestamp.deliveryId.body using Config.Webhooks.SigningSecret. |

An example of the request the consumer receives:

```http
POST /webhooks/beplus HTTP/1.1
Host: example.com
Content-Type: application/json
User-Agent: Arandu-WhatsApp/1.0
x-request-id: 019f0000-0000-7000-8000-000000000000
x-owner-jid: 5531999999999@s.whatsapp.net
x-instance-name: beplus
x-instance-id: 1
x-webhook-event: messages.upsert
X-Arandu-Delivery-ID: 019f0000-0000-7000-8000-000000000001
X-Arandu-Timestamp: 1785876000
X-Arandu-Signature: sha256=<hex>
```

`x-owner-jid` can be an empty string when the instance is not connected yet, or when the owner is not stored in the snapshot the dispatcher used.

## Standard envelope


```json
{
  "data": {},
  "event": "event.name",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5531999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

`event` is the external event name. `instance` holds the minimal snapshot of the instance responsible for the event. `data` holds the event-specific payload and can be an object or an array. `timestamp` is generated when the envelope is built, in RFC3339 UTC.

## Instance structure


```json
{
  "connectionStatus": "online",
  "externalAttributes": {},
  "id": 1,
  "name": "beplus",
  "ownerJid": "5531999999999@s.whatsapp.net"
}
```

`id` is the internal numeric identifier of the instance. `name` is the name used in the routes. `connectionStatus` carries the persisted connection values, such as `offline`, `connecting`, `qr_code`, `pairing_code`, `pairing`, `online`, `reconnecting`, `disconnected`, `connection_timeout`, `logged_out`, `session_missing`, `stream_replaced`, `keepalive_timeout`, `client_outdated`, `temporary_ban` and `connection_error`. `ownerJid` is a `string` or `null` in the body; in the `x-owner-jid` header, a null value becomes an empty string. `externalAttributes` is always a JSON object; absent, `null` or invalid values are serialized as `{}`.

## Delivery and error handling


```json
{
  "delivery": {
    "allowedSchemes": [
      "http",
      "https"
    ],
    "backoff": [
      "5s",
      "30s",
      "2m",
      "10m"
    ],
    "contentType": "application/json",
    "idempotencyHeader": "X-Arandu-Delivery-ID",
    "job": "whatsapp.webhook.deliver",
    "maxTries": 5,
    "method": "POST",
    "ordering": "not_guaranteed",
    "queue": "whatsapp-webhooks",
    "successStatus": "200-299",
    "timeoutSeconds": 15
  }
}
```

- Only HTTP 2xx responses count as a success.
- Network failures, timeouts and non-2xx responses persist a failed status and return an error to the native queue.
- The job allows five attempts with backoff; after that the native queue parks it for inspection.
- A delivery already marked as delivered is completed without a new POST when the same job reappears.
- A job whose snapshot expired or was removed with the instance finishes idempotently, without a pointless retry.
- The delivery queue is durable and processed by the host application's native workers.
- The relative order between events is guaranteed neither across instances nor across different events of the same instance.
- Some events depend on prior persistence. When persistence fails, the event may not be emitted.
- Every enabled delivery requires a Config.Webhooks.SigningSecret of at least 32 bytes and carries an HMAC-SHA256 the destination can verify.
- The signature covers the exact bytes of X-Arandu-Timestamp + dot + X-Arandu-Delivery-ID + dot + body; compare it in constant time.
- Use HTTPS and reject timestamps outside the accepted window before processing the event.
- x-request-id is for tracing and correlation; X-Arandu-Delivery-ID is the stable key for deduplication across retries.
- Creating the snapshot and inserting the job share a `data.Transaction`: either both commit or both roll back.
- A completed delivery keeps only the tombstone idempotency needs; the URL, body, headers and response are erased immediately.
- Snapshots in any state expire after `Config.Webhooks.Retention`; orphaned jobs end as a no-op, so a manual redrive only exists inside that window.
- `Start` and `Close` neither create nor stop webhook workers; the queue's lifecycle belongs to the host application.

## Events

### `call.upsert`

A voice or video call was updated.

**Flag:** `callUpsert`

**Internal events:** `*events.CallOffer`, `*events.CallAccept`, `*events.CallOfferNotice`, `*events.CallPreAccept`, `*events.CallTransport`, `*events.CallTerminate`, `*events.CallReject`, `*events.CallRelayLatency`, `*events.UnknownCallEvent`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `CallUpsertWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_extended_events.go`, `internal/webhook/payload.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: call.upsert
```

#### Body

```json
{
  "data": {
    "callerPn": "5511999999999",
    "chatId": "5511999999999@s.whatsapp.net",
    "date": "2026-07-04T19:05:00Z",
    "from": "5511999999999@s.whatsapp.net",
    "groupJid": null,
    "id": "3EB0C4D0A1",
    "isGroup": false,
    "isVideo": false,
    "latencyMs": null,
    "offline": false,
    "status": "offer"
  },
  "event": "call.upsert",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `chatId`: `string`, required, does not accept `null`. Chat JID of the call.
- `from`: `string`, required, does not accept `null`. Source JID.
- `callerPn`: `string | null`, required, accepts `null`. Phone number of the caller when available.
- `isGroup`: `boolean | null`, required, accepts `null`. Whether it is a group call, when the normalizer can infer it.
- `groupJid`: `string | null`, required, accepts `null`. Group JID when available.
- `id`: `string`, required, does not accept `null`. Call id.
- `date`: `string`, required, does not accept `null`. RFC3339 timestamp of the call or of the processing.
- `isVideo`: `boolean | null`, required, accepts `null`. Whether it is a video call, when the normalizer can infer it.
- `status`: `string`, required, does not accept `null`. Normalized call status. Possible values: `offer`, `ringing`, `preaccept`, `transport`, `relaylatency`, `timeout`, `reject`, `accept`, `terminate`, `unknown`.
- `offline`: `boolean`, required, does not accept `null`. Whether the event arrived as offline.
- `latencyMs`: `number | null`, required, accepts `null`. Latency in milliseconds when reported.

#### Possible values

- `status`: `offer`, `ringing`, `preaccept`, `transport`, `relaylatency`, `timeout`, `reject`, `accept`, `terminate`, `unknown`

#### Notes

- No additional notes.

### `chats.delete`

A chat was deleted or cleared.

**Flag:** `chatsDeleted`

**Internal events:** `*events.DeleteChat`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `ChatDeletedWebhookData`

**Dynamic fields:** yes

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_events.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: chats.delete
```

#### Body

```json
{
  "data": {
    "chatJid": "5511999999999@s.whatsapp.net",
    "dateTime": "2026-07-04T18:00:00Z",
    "deleteMedia": false
  },
  "event": "chats.delete",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `chatJid`: `string`, required, does not accept `null`. Chat JID.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 timestamp of the processing.
- `deleteMedia`: `boolean`, optional, does not accept `null`. Whether local media was removed, when present.
- `additionalProperties`: `object`, optional, does not accept `null`. Flattened fields of the original action.

#### Notes

- No additional notes.

### `chats.updated`

Chat properties were updated.

**Flag:** `chatsUpdated`

**Internal events:** `*events.Blocklist`, `*events.BlocklistChange`, `*events.Archive`, `*events.UnarchiveChatsSetting`, `*events.ClearChat`, `*events.Pin`, `*events.Mute`, `*events.MarkChatAsRead`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `ChatUpdatedWebhookData`

**Dynamic fields:** yes

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_events.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: chats.updated
```

#### Body

```json
{
  "data": {
    "archived": true,
    "chatJid": "5511999999999@s.whatsapp.net",
    "dateTime": "2026-07-04T18:00:00Z",
    "type": "archive"
  },
  "event": "chats.updated",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `type`: `string`, required, does not accept `null`. Subtype of the chat update. Possible values: `blocklist`, `blocklist.change`, `archive`, `unarchive.setting`, `clear`, `pin`, `mute`, `mark.read`.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 UTC timestamp of the event or of the processing.
- `chatJid`: `string`, optional, does not accept `null`. Chat JID when the subtype has a specific chat.
- `fromFullSync`: `boolean`, optional, does not accept `null`. Whether it came from a full synchronization, when available.
- `additionalProperties`: `object`, optional, does not accept `null`. Flattened fields of the original whatsmeow event.

#### Possible values

- `type`: `blocklist`, `blocklist.change`, `archive`, `unarchive.setting`, `clear`, `pin`, `mute`, `mark.read`

#### Notes

- UserStatusMute events are documented under settings.update, because the current registration routes them to that external event.

### `connection.update`

The instance connection state changed.

**Flag:** `connectionUpdated`

**Internal events:** `*events.PairSuccess`, `*events.PairError`, `*events.Connected`, `*events.Disconnected`, `*events.LoggedOut`, `*events.StreamReplaced`, `*events.KeepAliveTimeout`, `*events.KeepAliveRestored`, `*events.ConnectFailure`, `*events.ManualLoginReconnect`, `*events.StreamError`, `*events.CATRefreshError`

**Persistence:** The instance status is updated by the connection flows before or alongside the delivery, depending on the subtype.

**Type of `data`:** `object`

**DTO/normalizer:** `ConnectionUpdateWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/webhook/payload.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: connection.update
```

#### Body

```json
{
  "data": {
    "connection": "open",
    "lastConnection": "2026-07-04T18:50:00Z",
    "type": "connected"
  },
  "event": "connection.update",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `type`: `string`, required, does not accept `null`. Normalized connection subtype. Possible values: `pair.success`, `connected`, `disconnected`, `logged.out`, `stream.replaced`, `keepalive.timeout`, `keepalive.restored`, `connect.failure`, `manual.reconnect`, `pair.error`, `stream.error`, `cat.refresh.error`.
- `connection`: `string`, required, does not accept `null`. External connection state. Possible values: `connecting`, `open`, `close`, `replaced`, `timeout`.
- `statusReason`: `number`, optional, does not accept `null`. Numeric reason code when non-zero; omitted when zero.
- `lastConnection`: `string`, optional, does not accept `null`. RFC3339 UTC timestamp when reported; omitted when absent.
- `message`: `string`, optional, does not accept `null`. Technical message when reported; omitted when empty.

#### Possible values

- `type`: `pair.success`, `connected`, `disconnected`, `logged.out`, `stream.replaced`, `keepalive.timeout`, `keepalive.restored`, `connect.failure`, `manual.reconnect`, `pair.error`, `stream.error`, `cat.refresh.error`
- `connection`: `connecting`, `open`, `close`, `replaced`, `timeout`

#### Notes

- `statusReason`, `lastConnection` and `message` use `omitempty`; when they are zero or empty they do not appear in the JSON.

### `contacts.update`

An existing contact was partially updated.

**Flag:** `contactsUpdated`

**Internal events:** `*events.PushName`, `*events.BusinessName`

**Persistence:** Requires Config.Persistence.Contacts=true. The contact is updated before delivery where applicable.

**Persistence flag:** `Config.Persistence.Contacts`

**Type of `data`:** `array`

**DTO/normalizer:** `ContactUpdateWebhookData[]`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/event_persistence.go`, `internal/whatsapp/webhook_extended_events.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: contacts.update
```

#### Body

```json
{
  "data": [
    {
      "action": "updated",
      "businessName": null,
      "id": 41,
      "lid": null,
      "pushName": "Cliente Atualizado",
      "remoteJid": "5511999999999@s.whatsapp.net",
      "source": "pushName"
    }
  ],
  "event": "contacts.update",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `id`: `number`, required, does not accept `null`. Internal id of the persisted contact.
- `remoteJid`: `string`, required, does not accept `null`. Remote JID of the contact.
- `lid`: `string | null`, required, accepts `null`. LID of the contact when known.
- `pushName`: `string | null`, optional, accepts `null`. Updated push name when present.
- `businessName`: `string | null`, optional, accepts `null`. Updated business name when present.
- `action`: `string`, required, does not accept `null`. Acao executada. Possible values: `updated`.
- `source`: `string`, required, does not accept `null`. Source of the change. Possible values: `pushName`, `businessName`.

#### Possible values

- `action`: `updated`
- `source`: `pushName`, `businessName`

#### Notes

- The payload is an array; the current handler normally sends one item per delivery.

### `contacts.upsert`

A contact was created or updated in the local records.

**Flag:** `contactsUpsert`

**Internal events:** `*events.Contact`

**Persistence:** Requires Config.Persistence.Contacts=true. The contact is stored before delivery.

**Persistence flag:** `Config.Persistence.Contacts`

**Type of `data`:** `object`

**DTO/normalizer:** `ContactUpsertWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/event_persistence.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: contacts.upsert
```

#### Body

```json
{
  "data": {
    "action": "upserted",
    "id": 41,
    "lid": "279847268053216@lid",
    "profilePicUrl": null,
    "pushName": "Cliente",
    "remoteJid": "5511999999999@s.whatsapp.net"
  },
  "event": "contacts.upsert",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `id`: `number`, required, does not accept `null`. Internal id of the persisted contact.
- `remoteJid`: `string`, required, does not accept `null`. Remote JID of the contact.
- `lid`: `string | null`, required, accepts `null`. LID of the contact when known.
- `pushName`: `string | null`, required, accepts `null`. Push name stored for the contact.
- `profilePicUrl`: `string | null`, required, accepts `null`. Profile picture URL when known.
- `action`: `string`, required, does not accept `null`. Acao executada. Possible values: `upserted`.

#### Possible values

- `action`: `upserted`

#### Notes

- No additional notes.

### `groups.participants.update`

Group participants changed.

**Flag:** `groupsParticipantsUpdated`

**Internal events:** `*events.GroupInfo`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `GroupParticipantsUpdatedWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_extended_events.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: groups.participants.update
```

#### Body

```json
{
  "data": {
    "action": "add",
    "author": "5531999999999@s.whatsapp.net",
    "authorPn": "5531999999999",
    "id": "120363000000000000@g.us",
    "participants": [
      {
        "admin": null,
        "id": "5511999999999@s.whatsapp.net",
        "isAdmin": false,
        "isSuperAdmin": false
      }
    ]
  },
  "event": "groups.participants.update",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `id`: `string`, required, does not accept `null`. Group JID.
- `author`: `string`, required, does not accept `null`. JID of the author of the change; empty string when absent.
- `authorPn`: `string`, optional, does not accept `null`. Phone number of the author when available; omitted when absent.
- `participants`: `GroupParticipantWebhookData[]`, required, does not accept `null`. Participantes afetados.
- `action`: `string`, required, does not accept `null`. Acao aplicada. Possible values: `add`, `remove`, `promote`, `demote`.

#### Possible values

- `action`: `add`, `remove`, `promote`, `demote`

#### Notes

- No additional notes.

### `groups.update`

Group metadata was partially updated.

**Flag:** `groupsUpdated`

**Internal events:** `*events.GroupInfo`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `array`

**DTO/normalizer:** `GroupUpdateWebhookData[]`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_extended_events.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: groups.update
```

#### Body

```json
{
  "data": [
    {
      "partial": {
        "announce": true,
        "id": "120363000000000000@g.us",
        "subject": "New group name",
        "subjectTime": 1783188000
      }
    }
  ],
  "event": "groups.update",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `partial`: `GroupPartialWebhookData`, required, does not accept `null`. Partial metadata changed on the group.
- `partial.notify`: `string`, optional, does not accept `null`. Notification name of the group when reported.
- `partial.addressingMode`: `string`, optional, does not accept `null`. Addressing mode of the group when reported.
- `partial.owner`: `string`, optional, does not accept `null`. Owner JID when reported.
- `partial.ownerPn`: `string`, optional, does not accept `null`. Phone number of the owner when reported.
- `partial.ownerUsername`: `string`, optional, does not accept `null`. Username of the owner when reported.
- `partial.ownerCountryCode`: `string`, optional, does not accept `null`. Country code of the owner when reported.
- `partial.subjectOwner`: `string`, optional, does not accept `null`. JID of whoever set the subject, when reported.
- `partial.subjectOwnerPn`: `string`, optional, does not accept `null`. Phone number of whoever set the subject, when reported.
- `partial.subjectOwnerUsername`: `string`, optional, does not accept `null`. Username of whoever set the subject, when reported.
- `partial.subjectTime`: `number`, optional, does not accept `null`. Unix timestamp of the subject when reported.
- `partial.creation`: `number`, optional, does not accept `null`. Unix timestamp of the creation when reported.
- `partial.desc`: `string`, optional, does not accept `null`. Group description when reported.
- `partial.descOwner`: `string`, optional, does not accept `null`. JID of whoever set the description, when reported.
- `partial.descOwnerPn`: `string`, optional, does not accept `null`. Phone number of whoever set the description, when reported.
- `partial.descOwnerUsername`: `string`, optional, does not accept `null`. Username of whoever set the description, when reported.
- `partial.descId`: `string`, optional, does not accept `null`. Description id when reported.
- `partial.descTime`: `number`, optional, does not accept `null`. Unix timestamp of the description when reported.
- `partial.linkedParent`: `string`, optional, does not accept `null`. Parent group or community when reported.
- `partial.restrict`: `boolean`, optional, does not accept `null`. Edit restriction when reported.
- `partial.announce`: `boolean`, optional, does not accept `null`. Announcement mode when reported.
- `partial.memberAddMode`: `boolean`, optional, does not accept `null`. Member-add mode when reported.
- `partial.joinApprovalMode`: `boolean`, optional, does not accept `null`. Join-approval mode when reported.
- `partial.isCommunity`: `boolean`, optional, does not accept `null`. Whether it is a community, when reported.
- `partial.isCommunityAnnounce`: `boolean`, optional, does not accept `null`. Whether it is the community announcement group, when reported.
- `partial.size`: `number`, optional, does not accept `null`. Group size when reported.
- `partial.ephemeralDuration`: `number`, optional, does not accept `null`. Disappearing-message duration in seconds when reported.
- `partial.inviteCode`: `string`, optional, does not accept `null`. Invite code when reported.
- `partial.author`: `string`, optional, does not accept `null`. Author of the change when reported.
- `partial.authorPn`: `string`, optional, does not accept `null`. Phone number of the author when reported.
- `partial.authorUsername`: `string`, optional, does not accept `null`. Username of the author when reported.

#### Notes

- The current handler sends an array holding one item that carries partial.

### `groups.upsert`

A group was created, discovered or synchronized.

**Flag:** `groupsUpsert`

**Internal events:** `*events.JoinedGroup`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `array`

**DTO/normalizer:** `GroupUpsertWebhookData[]`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_extended_events.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: groups.upsert
```

#### Body

```json
{
  "data": [
    {
      "addressingMode": "pn",
      "creation": 1783187000,
      "id": "120363000000000000@g.us",
      "isCommunity": false,
      "owner": "5531999999999@s.whatsapp.net",
      "participants": [
        {
          "admin": "admin",
          "id": "5511999999999@s.whatsapp.net",
          "isAdmin": true,
          "isSuperAdmin": false,
          "lid": "279847268053216@lid"
        }
      ],
      "subject": "Group"
    }
  ],
  "event": "groups.upsert",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `id`: `string`, required, does not accept `null`. Group JID.
- `subject`: `string`, required, does not accept `null`. Group name.
- `participants`: `GroupParticipantWebhookData[]`, required, does not accept `null`. Known participants of the group.
- `notify`: `string`, optional, does not accept `null`. Notification name of the group when reported.
- `addressingMode`: `string`, optional, does not accept `null`. Addressing mode of the group when reported.
- `owner`: `string`, optional, does not accept `null`. Owner JID when reported.
- `ownerPn`: `string`, optional, does not accept `null`. Phone number of the owner when reported.
- `ownerUsername`: `string`, optional, does not accept `null`. Username of the owner when reported.
- `ownerCountryCode`: `string`, optional, does not accept `null`. Country code of the owner when reported.
- `subjectOwner`: `string`, optional, does not accept `null`. JID of whoever set the subject, when reported.
- `subjectOwnerPn`: `string`, optional, does not accept `null`. Phone number of whoever set the subject, when reported.
- `subjectOwnerUsername`: `string`, optional, does not accept `null`. Username of whoever set the subject, when reported.
- `subjectTime`: `number`, optional, does not accept `null`. Unix timestamp of the subject when reported.
- `creation`: `number`, optional, does not accept `null`. Unix timestamp of the creation when reported.
- `desc`: `string`, optional, does not accept `null`. Group description when reported.
- `descOwner`: `string`, optional, does not accept `null`. JID of whoever set the description, when reported.
- `descOwnerPn`: `string`, optional, does not accept `null`. Phone number of whoever set the description, when reported.
- `descOwnerUsername`: `string`, optional, does not accept `null`. Username of whoever set the description, when reported.
- `descId`: `string`, optional, does not accept `null`. Description id when reported.
- `descTime`: `number`, optional, does not accept `null`. Unix timestamp of the description when reported.
- `linkedParent`: `string`, optional, does not accept `null`. Parent group or community when reported.
- `restrict`: `boolean`, optional, does not accept `null`. Edit restriction when reported.
- `announce`: `boolean`, optional, does not accept `null`. Announcement mode when reported.
- `memberAddMode`: `boolean`, optional, does not accept `null`. Member-add mode when reported.
- `joinApprovalMode`: `boolean`, optional, does not accept `null`. Join-approval mode when reported.
- `isCommunity`: `boolean`, optional, does not accept `null`. Whether it is a community, when reported.
- `isCommunityAnnounce`: `boolean`, optional, does not accept `null`. Whether it is the community announcement group, when reported.
- `size`: `number`, optional, does not accept `null`. Group size when reported.
- `ephemeralDuration`: `number`, optional, does not accept `null`. Disappearing-message duration in seconds when reported.
- `inviteCode`: `string`, optional, does not accept `null`. Invite code when reported.
- `author`: `string`, optional, does not accept `null`. Author of the change when reported.
- `authorPn`: `string`, optional, does not accept `null`. Phone number of the author when reported.
- `authorUsername`: `string`, optional, does not accept `null`. Username of the author when reported.

#### Notes

- The payload is an array for compatibility with list contracts, even when a delivery carries a single group.

### `history.sync`

A history synchronization was received from WhatsApp.

**Flag:** `historySync`

**Internal events:** `*events.HistorySync`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `HistorySyncWebhookData`

**Dynamic fields:** yes

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_extended_events.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: history.sync
```

#### Body

```json
{
  "data": {
    "data": {
      "syncType": "INITIAL_BOOTSTRAP"
    },
    "dateTime": "2026-07-04T18:00:00Z",
    "type": "history.sync"
  },
  "event": "history.sync",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `type`: `string`, required, does not accept `null`. Fixed payload type. Possible values: `history.sync`.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 UTC timestamp of the event or of the processing.
- `data`: `object`, optional, does not accept `null`. Normalized content of the history event when available.

#### Possible values

- `type`: `history.sync`

#### Notes

- Dynamic payload, because the content comes from whatsmeow's history proto.

### `identity.update`

A contact's cryptographic identity changed.

**Flag:** `identityUpdated`

**Internal events:** `*events.IdentityChange`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `IdentityUpdatedWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/webhook/payload.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: identity.update
```

#### Body

```json
{
  "data": {
    "dateTime": "2026-07-04T18:00:00Z",
    "implicit": true,
    "jid": "5511999999999@s.whatsapp.net"
  },
  "event": "identity.update",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `jid`: `string`, required, does not accept `null`. JID cuja identidade mudou.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 UTC timestamp of the event or of the processing.
- `implicit`: `boolean`, required, does not accept `null`. Whether whatsmeow reported the change as implicit.

#### Notes

- No additional notes.

### `labels.association`

A label was associated with or removed from a chat or message.

**Flag:** `labelsAssociation`

**Internal events:** `*events.LabelAssociationChat`, `*events.LabelAssociationMessage`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `LabelsAssociationWebhookData`

**Dynamic fields:** yes

**Implemented in:** `internal/whatsapp/service.go`, `internal/webhook/payload.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: labels.association
```

#### Body

```json
{
  "data": {
    "action": "add",
    "chatJid": "5511999999999@s.whatsapp.net",
    "dateTime": "2026-07-04T18:00:00Z",
    "labelId": "7",
    "type": "chat"
  },
  "event": "labels.association",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `type`: `string`, required, does not accept `null`. Association type. Possible values: `chat`, `message`.
- `chatJid`: `string`, required, does not accept `null`. Chat JID.
- `messageId`: `string`, optional, does not accept `null`. Message id when type=message.
- `labelId`: `string`, required, does not accept `null`. Label id.
- `action`: `string`, optional, does not accept `null`. Action inferred when labeled is present. Possible values: `add`, `remove`.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 timestamp of the processing.
- `additionalProperties`: `object`, optional, does not accept `null`. Flattened fields of the original event.

#### Possible values

- `type`: `chat`, `message`
- `action`: `add`, `remove`

#### Notes

- No additional notes.

### `labels.edit`

A label was created, changed or removed.

**Flag:** `labelsEdit`

**Internal events:** `*events.LabelEdit`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `LabelsEditWebhookData`

**Dynamic fields:** yes

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_extended_events.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: labels.edit
```

#### Body

```json
{
  "data": {
    "color": 3,
    "deleted": false,
    "id": "12",
    "name": "Cliente"
  },
  "event": "labels.edit",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `id`: `string`, required, does not accept `null`. Label id, derived from labelId.
- `name`: `string`, optional, does not accept `null`. Label name when reported.
- `color`: `number`, optional, does not accept `null`. Label color when reported.
- `deleted`: `boolean`, optional, does not accept `null`. Whether the label was removed, when reported.
- `additionalProperties`: `object`, optional, does not accept `null`. Flattened fields of the original event.

#### Notes

- The normalizer adds neither a `type` nor a `dateTime` field for this event.

### `media.retry`

Result or error of a media retry attempt.

**Flag:** `mediaRetry`

**Internal events:** `*events.MediaRetry`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `MediaRetryWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_events.go`, `internal/webhook/payload.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: media.retry
```

#### Body

```json
{
  "data": {
    "chatJid": "5511999999999@s.whatsapp.net",
    "dateTime": "2026-07-04T18:00:00Z",
    "errorCode": 404,
    "hasCiphertext": true,
    "keyFromMe": false,
    "keyId": "ABC123",
    "senderJid": "5511988888888@s.whatsapp.net"
  },
  "event": "media.retry",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `keyId`: `string`, required, does not accept `null`. Message id.
- `chatJid`: `string`, required, does not accept `null`. Chat JID.
- `senderJid`: `string`, optional, does not accept `null`. Sender JID when available; omitted when absent.
- `keyFromMe`: `boolean`, required, does not accept `null`. Whether the message belongs to the instance itself.
- `hasCiphertext`: `boolean`, required, does not accept `null`. Whether the event carried ciphertext.
- `errorCode`: `number`, optional, does not accept `null`. Error code when reported; omitted when absent.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 UTC timestamp of the event or of the processing.

#### Notes

- The ciphertext and IV whatsmeow received are not exposed in the webhook.

### `messages.delete`

A message was removed locally by the DeleteForMe event.

**Flag:** `messagesDeleted`

**Internal events:** `*events.DeleteForMe`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `MessageDeletedWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_events.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: messages.delete
```

#### Body

```json
{
  "data": {
    "chatJid": "5511999999999@s.whatsapp.net",
    "dateTime": "2026-07-04T18:00:00Z",
    "deleteMedia": true,
    "fromFullSync": false,
    "id": 1024,
    "keyFromMe": false,
    "keyId": "ABC123",
    "messageTime": "2026-07-04T17:59:00Z",
    "senderJid": "5511988888888@s.whatsapp.net"
  },
  "event": "messages.delete",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `id`: `number`, optional, does not accept `null`. Internal id of the persisted message when found by keyId.
- `chatJid`: `string`, required, does not accept `null`. Chat JID.
- `senderJid`: `string`, optional, does not accept `null`. Sender JID when available; omitted when absent.
- `keyFromMe`: `boolean`, required, does not accept `null`. Whether the message belonged to the instance itself.
- `keyId`: `string`, required, does not accept `null`. Id of the deleted message.
- `deleteMedia`: `boolean`, required, does not accept `null`. Whether the local media should be removed.
- `fromFullSync`: `boolean`, required, does not accept `null`. Whether it came from a full synchronization.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 UTC timestamp of the event or of the processing.
- `messageTime`: `string`, optional, does not accept `null`. Original RFC3339 UTC timestamp of the message when reported; omitted when absent.

#### Notes

- No additional notes.

### `messages.star`

A message was starred or unstarred.

**Flag:** `messagesStarred`

**Internal events:** `*events.Star`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `MessageStarredWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_events.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: messages.star
```

#### Body

```json
{
  "data": {
    "chatJid": "5511999999999@s.whatsapp.net",
    "dateTime": "2026-07-04T18:00:00Z",
    "fromFullSync": false,
    "keyFromMe": false,
    "keyId": "ABC123",
    "senderJid": "5511988888888@s.whatsapp.net",
    "starred": true
  },
  "event": "messages.star",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `chatJid`: `string`, required, does not accept `null`. Chat JID.
- `senderJid`: `string`, optional, does not accept `null`. Sender JID when available; omitted when absent.
- `keyFromMe`: `boolean`, required, does not accept `null`. Whether the message belongs to the instance itself.
- `keyId`: `string`, required, does not accept `null`. Message id.
- `starred`: `boolean`, required, does not accept `null`. true when the message is starred.
- `fromFullSync`: `boolean`, required, does not accept `null`. Whether it came from a full synchronization.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 UTC timestamp of the event or of the processing.

#### Notes

- No additional notes.

### `messages.undecryptable`

A message received that could not be decrypted.

**Flag:** `messagesUndecryptable`

**Internal events:** `*events.UndecryptableMessage`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `MessageUndecryptableWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_events.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: messages.undecryptable
```

#### Body

```json
{
  "data": {
    "chatJid": "5511999999999@s.whatsapp.net",
    "dateTime": "2026-07-04T18:00:00Z",
    "decryptFailMode": "hide",
    "isUnavailable": true,
    "keyFromMe": false,
    "keyId": "ABC123",
    "senderJid": "5511988888888@s.whatsapp.net",
    "unavailableType": "view_once"
  },
  "event": "messages.undecryptable",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `keyId`: `string`, required, does not accept `null`. Id of the key or message that failed.
- `chatJid`: `string`, required, does not accept `null`. Chat JID.
- `senderJid`: `string`, optional, does not accept `null`. Sender JID when available; omitted when absent.
- `keyFromMe`: `boolean`, required, does not accept `null`. Whether the message belongs to the instance itself.
- `isUnavailable`: `boolean`, required, does not accept `null`. Whether the content was marked as unavailable.
- `unavailableType`: `string`, optional, does not accept `null`. Unavailability type. Possible values: `view_once`.
- `decryptFailMode`: `string`, optional, does not accept `null`. Reported failure mode. Possible values: `hide`.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 UTC timestamp of the event or of the processing.

#### Possible values

- `unavailableType`: `view_once`
- `decryptFailMode`: `hide`

#### Notes

- Empty fields are dropped by omitempty.

### `messages.update`

The receipt or status of an already known message was updated.

**Flag:** `messagesUpdated`

**Internal events:** `*events.Receipt`

**Persistence:** The webhook is delivered even when Config.Persistence.MessageUpdates=false. When true, the message is located and the update persisted before delivery.

**Type of `data`:** `object`

**DTO/normalizer:** `MessageUpdateWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/event_persistence.go`, `internal/webhook/payload.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: messages.update
```

#### Body

```json
{
  "data": {
    "dateTime": "2026-07-04T18:05:00Z",
    "id": 1024,
    "keyId": "ABC123",
    "status": "read"
  },
  "event": "messages.update",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `id`: `number`, required, does not accept `null`. Internal id of the persisted message; 0 when update persistence is disabled.
- `keyId`: `string`, required, does not accept `null`. External id or key of the message on WhatsApp.
- `status`: `string`, required, does not accept `null`. Normalized receipt status. Possible values: `delivered`, `sent`, `read`, `played`, `server_error`, `retry`, `unknown`.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 UTC timestamp of the receipt; it uses the event timestamp or the processing time.

#### Possible values

- `status`: `delivered`, `sent`, `read`, `played`, `server_error`, `retry`, `unknown`

#### Notes

- With Config.Persistence.MessageUpdates=true the message has to exist; if it is not found after the configured attempts, the event is dropped.
- With persistence disabled, id is 0 and keyId still identifies the message the receipt is about.

### `messages.upsert`

A message received and persisted by the application.

**Flag:** `messagesUpsert`

**Internal events:** `*events.Message`, `*events.FBMessage`

**Persistence:** Requires Config.Persistence.Messages=true. The message is persisted with CreateOrIgnore and read back before delivery.

**Persistence flag:** `Config.Persistence.Messages`

**Type of `data`:** `object`

**DTO/normalizer:** `MessageWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/event_persistence.go`, `internal/webhook/payload.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: messages.upsert
```

#### Body

```json
{
  "data": {
    "content": {
      "text": "Ola"
    },
    "device": "ios",
    "id": 1024,
    "isGroup": false,
    "keyFromMe": false,
    "keyId": "ABC123",
    "keyLid": null,
    "keyParticipant": null,
    "keyParticipantLid": null,
    "keyRemoteJid": "5511999999999@s.whatsapp.net",
    "messageTimestamp": 1783188000,
    "messageType": "conversation",
    "metadata": null,
    "pushName": "Cliente"
  },
  "event": "messages.upsert",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `id`: `number`, required, does not accept `null`. Internal message id.
- `keyId`: `string`, required, does not accept `null`. External id or key of the message on WhatsApp.
- `keyRemoteJid`: `string | null`, required, accepts `null`. Remote JID of the message.
- `keyLid`: `string | null`, required, accepts `null`. Remote LID of the message.
- `keyFromMe`: `boolean`, required, does not accept `null`. Whether the message was sent by the instance itself.
- `keyParticipant`: `string | null`, required, accepts `null`. Participant in group messages.
- `keyParticipantLid`: `string | null`, required, accepts `null`. LID of the participant in group messages.
- `pushName`: `string | null`, required, accepts `null`. Display name of the sender when known.
- `messageType`: `string`, required, does not accept `null`. Normalized message type.
- `content`: `object`, required, does not accept `null`. Normalized message content.
- `messageTimestamp`: `number`, required, does not accept `null`. Unix timestamp in seconds.
- `device`: `string | null`, required, accepts `null`. Device or source inferred for the message.
- `isGroup`: `boolean`, required, does not accept `null`. Whether the message belongs to a group.
- `metadata`: `object | null`, required, accepts `null`. Metadados adicionais normalizados.

#### Notes

- If persisting or reading the message back fails, the webhook is not emitted.

### `news.letter`

Events related to newsletters and channels.

**Flag:** `newsLetter`

**Internal events:** `*events.NewsletterJoin`, `*events.NewsletterLeave`, `*events.NewsletterLiveUpdate`, `*events.NewsletterMessageMeta`, `*events.NewsletterMuteChange`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `NewsLetterWebhookData`

**Dynamic fields:** yes

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_extended_events.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: news.letter
```

#### Body

```json
{
  "data": {
    "dateTime": "2026-07-04T18:00:00Z",
    "muted": true,
    "newsletterJid": "120363000000000000@newsletter",
    "type": "mute.change"
  },
  "event": "news.letter",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `type`: `string`, required, does not accept `null`. Subtype of the newsletter event. Possible values: `join`, `leave`, `live.update`, `message.meta`, `mute.change`.
- `newsletterJid`: `string`, optional, does not accept `null`. Newsletter JID when the source carries an id or jid.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 timestamp of the processing.
- `additionalProperties`: `object`, optional, does not accept `null`. Flattened fields of the original event.

#### Possible values

- `type`: `join`, `leave`, `live.update`, `message.meta`, `mute.change`

#### Notes

- No additional notes.

### `presence.updated`

User presence or chat presence was updated.

**Flag:** `presenceUpdated`

**Internal events:** `*events.ChatPresence`, `*events.Presence`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `PresenceUpdatedWebhookData`

**Dynamic fields:** yes

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_events.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: presence.updated
```

#### Body

```json
{
  "data": {
    "chatJid": "5511999999999@s.whatsapp.net",
    "dateTime": "2026-07-04T18:00:00Z",
    "media": "text",
    "senderJid": "5511999999999@s.whatsapp.net",
    "state": "composing"
  },
  "event": "presence.updated",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `type`: `string`, optional, does not accept `null`. Fixed presence type in the payload coming from *events.Presence. Possible values: `presence`.
- `chatJid`: `string`, optional, does not accept `null`. Chat JID in the ChatPresence payload.
- `senderJid`: `string`, optional, does not accept `null`. Sender JID in the ChatPresence payload.
- `state`: `string`, optional, does not accept `null`. Presence state in the ChatPresence payload.
- `media`: `string`, optional, does not accept `null`. Media type when the presence relates to media.
- `jid`: `string`, optional, does not accept `null`. JID in the Presence payload.
- `unavailable`: `boolean`, optional, does not accept `null`. Whether the user is unavailable, in the Presence payload.
- `lastSeen`: `string`, optional, does not accept `null`. Last seen when reported.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 timestamp of the processing.

#### Possible values

- `type`: `presence`

#### Notes

- The shape differs between ChatPresence and Presence; use the fields present in the payload you receive.

### `profile.picture.update`

The profile picture of the instance itself or of another JID was updated.

**Flag:** `profilePictureUpdated`

**Internal events:** `*events.Picture`

**Persistence:** When the JID is the instance itself, the instance profilePicUrl is updated before delivery. For other JIDs there is no specific persistence.

**Type of `data`:** `object`

**DTO/normalizer:** `ProfilePictureUpdatedWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/webhook/payload.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: profile.picture.update
```

#### Body

```json
{
  "data": {
    "author": "5531999999999@s.whatsapp.net",
    "dateTime": "2026-07-04T18:00:00Z",
    "isGroup": true,
    "jid": "120363000000000000@g.us",
    "pictureId": "pic-123",
    "remove": false
  },
  "event": "profile.picture.update",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `jid`: `string`, required, does not accept `null`. JID whose picture changed.
- `author`: `string`, optional, does not accept `null`. Author JID when reported; omitted when empty.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 UTC timestamp of the event or of the processing.
- `remove`: `boolean`, required, does not accept `null`. Whether the picture was removed.
- `pictureId`: `string`, optional, does not accept `null`. Picture id when reported; omitted when empty.
- `isGroup`: `boolean`, required, does not accept `null`. Whether the JID belongs to a group.

#### Notes

- No additional notes.

### `qrcode.updated`

A new QR code is available for pairing the instance.

**Flag:** `qrcodeUpdated`

**Internal events:** `QR channel`

**Persistence:** Sets the instance status to qr_code before delivery.

**Type of `data`:** `object`

**DTO/normalizer:** `QRCodeUpdatedWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/webhook/manager.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: qrcode.updated
```

#### Body

```json
{
  "data": {
    "base64": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA",
    "code": "2@abc",
    "count": 1,
    "expiresAt": "2026-07-04T18:01:00Z",
    "expiresInSeconds": 60
  },
  "event": "qrcode.updated",
  "instance": {
    "connectionStatus": "qr_code",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": null
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `count`: `number`, required, does not accept `null`. Number of QR codes emitted in this attempt.
- `code`: `string`, required, does not accept `null`. Raw QR code payload.
- `base64`: `string`, required, does not accept `null`. QR code image as a base64 data URL.
- `expiresInSeconds`: `number`, required, does not accept `null`. Time left as reported by the QR channel, in seconds.
- `expiresAt`: `string`, required, does not accept `null`. RFC3339 UTC timestamp computed for the QR code expiry.

#### Notes

- Emitted by the QR flow rather than directly by a whatsmeow event struct.

### `send.message`

A message sent through the API, after a successful send and persistence.

**Flag:** `sendMessage`

**Internal events:** `message service send result`

**Persistence:** Persisted before delivery by the API message-sending flow.

**Type of `data`:** `object`

**DTO/normalizer:** `MessageWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/message/service.go`, `internal/message/audio.go`, `internal/webhook/payload.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: send.message
```

#### Body

```json
{
  "data": {
    "content": {
      "text": "Sent message"
    },
    "device": "web",
    "id": 2048,
    "isGroup": false,
    "keyFromMe": true,
    "keyId": "ABC123",
    "keyLid": null,
    "keyParticipant": null,
    "keyParticipantLid": null,
    "keyRemoteJid": "5511999999999@s.whatsapp.net",
    "messageTimestamp": 1783188000,
    "messageType": "conversation",
    "metadata": null,
    "pushName": null
  },
  "event": "send.message",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `id`: `number`, required, does not accept `null`. Internal message id.
- `keyId`: `string`, required, does not accept `null`. External id or key of the message on WhatsApp.
- `keyRemoteJid`: `string | null`, required, accepts `null`. Remote JID of the message.
- `keyLid`: `string | null`, required, accepts `null`. Remote LID of the message.
- `keyFromMe`: `boolean`, required, does not accept `null`. Whether the message was sent by the instance itself.
- `keyParticipant`: `string | null`, required, accepts `null`. Participant in group messages.
- `keyParticipantLid`: `string | null`, required, accepts `null`. LID of the participant in group messages.
- `pushName`: `string | null`, required, accepts `null`. Display name of the sender when known.
- `messageType`: `string`, required, does not accept `null`. Normalized message type.
- `content`: `object`, required, does not accept `null`. Normalized message content.
- `messageTimestamp`: `number`, required, does not accept `null`. Unix timestamp in seconds.
- `device`: `string | null`, required, accepts `null`. Device or source inferred for the message.
- `isGroup`: `boolean`, required, does not accept `null`. Whether the message belongs to a group.
- `metadata`: `object | null`, required, accepts `null`. Metadados adicionais normalizados.

#### Notes

- Uses the same DTO as messages.upsert, but the source is a send through the API itself.
- When a message is accepted with options.mentionAll=true, the same send.message event also delivers the final result of the asynchronous processing. In that case data carries processId, status, mentionAll, externalAttributes and, on success, data.messageId, data.remoteJid, data.participantCount and data.timestamp. On failure it carries error.code and error.message.

### `settings.update`

User or instance settings were updated.

**Flag:** `settingsUpdated`

**Internal events:** `*events.PushNameSetting`, `*events.UserStatusMute`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `SettingsUpdatedWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/whatsapp/webhook_events.go`, `internal/webhook/payload.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: settings.update
```

#### Body

```json
{
  "data": {
    "dateTime": "2026-07-04T18:00:00Z",
    "fromFullSync": false,
    "name": "My instance",
    "type": "push.name"
  },
  "event": "settings.update",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `type`: `string`, required, does not accept `null`. Settings subtype. Possible values: `push.name`, `user.status.mute`.
- `jid`: `string`, optional, does not accept `null`. Affected JID when the subtype reports one.
- `name`: `string`, optional, does not accept `null`. Name set in the push.name subtype.
- `muted`: `boolean`, optional, does not accept `null`. Mute state in the user.status.mute subtype.
- `fromFullSync`: `boolean`, required, does not accept `null`. Whether it came from a full synchronization.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 UTC timestamp of the event or of the processing.

#### Possible values

- `type`: `push.name`, `user.status.mute`

#### Notes

- No additional notes.

### `status.instance`

Operational state events or warnings from the instance.

**Flag:** `statusInstance`

**Internal events:** `*events.ClientOutdated`, `*events.TemporaryBan`, `*events.OfflineSyncPreview`, `*events.OfflineSyncCompleted`, `*events.PrivacySettings`, `*events.AppState`, `*events.AppStateSyncComplete`, `*events.AppStateSyncError`, `*events.AccountReachoutTimelock`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `InstanceStatusWebhookData`

**Dynamic fields:** yes

**Implemented in:** `internal/whatsapp/service.go`, `internal/webhook/payload.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: status.instance
```

#### Body

```json
{
  "data": {
    "data": {
      "count": 185
    },
    "status": "completed",
    "type": "offline.sync.completed"
  },
  "event": "status.instance",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `type`: `string`, required, does not accept `null`. Subtype of the instance status.
- `status`: `string`, optional, does not accept `null`. Textual status of the subtype; omitted when empty.
- `message`: `string`, optional, does not accept `null`. Technical or human-readable message; omitted when empty.
- `data`: `object`, optional, does not accept `null`. Additional data for the subtype; omitted when absent.

#### Possible values

- `type`: `client.outdated`, `temporary.ban`, `offline.sync.preview`, `offline.sync.completed`, `privacy.settings`, `app.state`, `app.state.sync.completed`, `app.state.sync.error`, `account.reachout.timelock`

#### Notes

- No additional notes.

### `user.about.update`

A user's about text was updated.

**Flag:** `userAboutUpdated`

**Internal events:** `*events.UserAbout`

**Persistence:** Persists no specific data before delivering the webhook.

**Type of `data`:** `object`

**DTO/normalizer:** `UserAboutUpdatedWebhookData`

**Dynamic fields:** no

**Implemented in:** `internal/whatsapp/service.go`, `internal/webhook/payload.go`

#### Request

```http
POST /webhooks/beplus HTTP/1.1
Content-Type: application/json
x-webhook-event: user.about.update
```

#### Body

```json
{
  "data": {
    "dateTime": "2026-07-04T18:00:00Z",
    "jid": "5511999999999@s.whatsapp.net",
    "status": "Disponivel"
  },
  "event": "user.about.update",
  "instance": {
    "connectionStatus": "online",
    "externalAttributes": {},
    "id": 1,
    "name": "beplus",
    "ownerJid": "5511999999999@s.whatsapp.net"
  },
  "timestamp": "2026-07-04T18:00:00Z"
}
```

#### Fields of `data`

- `jid`: `string`, required, does not accept `null`. User JID.
- `status`: `string`, optional, does not accept `null`. About text when reported.
- `dateTime`: `string`, required, does not accept `null`. RFC3339 timestamp of the processing.

#### Notes

- No additional notes.

## Unsupported or ignored events

| Internal event | Status | Reason |
| --- | --- | --- |
| `PairPasskeyConfirmation` | `intentionally_ignored` | Interactive pairing event carrying a code; it is not part of the webhook contract. |
| `PairPasskeyError` | `handled_without_webhook` | Handled as pairing state and log; there is no stable public payload. |
| `PairPasskeyRequest` | `intentionally_ignored` | Carries the pairing challenge and public key, and is not serialized into a webhook. |
| `QRScannedWithoutMultidevice` | `handled_without_webhook` | The QR channel turns this case into a pairing failure; future direct emissions fall through to the fallback log. |
| `MediaRetryError` | `internal_only` | Support error struct used inside the media.retry payload. |
| `MexNotificationData` | `internal_only` | Support struct for MEX notifications with no dedicated public event. |
| `NewsletterMessageMeta` | `internal_only` | Support struct used inside news.letter. |
