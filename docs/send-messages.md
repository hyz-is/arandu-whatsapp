# Sending messages

This document describes the sending endpoints under
`/whatsapp/instances/{instance}/messages` and the `options.mentionAll` feature.

Every route requires an Arandu session in the configured tenant and a role
authorized for `ActionMessageSend`:

```http
Cookie: arandu_session=<host-session>
```

Available routes:

```text
POST /whatsapp/instances/{instance}/messages/text
POST /whatsapp/instances/{instance}/messages/link
POST /whatsapp/instances/{instance}/messages/media
POST /whatsapp/instances/{instance}/messages/media/file
POST /whatsapp/instances/{instance}/messages/audio
POST /whatsapp/instances/{instance}/messages/audio/file
POST /whatsapp/instances/{instance}/messages/contact
POST /whatsapp/instances/{instance}/messages/location
POST /whatsapp/instances/{instance}/messages/reaction
```

The identity comes from the Arandu session and the Grant the policy issues. JSON responses are
Arandu resources and live under the `data` key.

## MessageOptions

`MessageOptions` is optional. When `mentionAll` is absent or `false`, the send stays synchronous and returns the persisted message with `200 OK`.

```json
{
  "delay": 1000,
  "presence": "composing",
  "quotedMessageId": 123,
  "quotedMessage": {
    "keyId": "A5FDD9082F21LGHLKJLGB6C3FF6BFA6F",
    "keyRemoteJid": "120363000000000000@g.us",
    "keyFromMe": false,
    "messageType": "extendedTextMessage",
    "content": {}
  },
  "externalAttributes": {
    "requestId": "request-456"
  },
  "mentionAll": true
}
```

`delay`: optional integer in milliseconds. General message sends accept up to `120000`. WhatsApp audio sends accept up to `300000`.

`presence`: optional string. Text, link, media, contact and location accept `composing`. Audio and PTV accept `recording`. WhatsApp audio also accepts `paused`.

`quotedMessageId`: optional internal id of the message to quote. The message has to belong to the same instance.

`quotedMessage`: optional snapshot of the quoted message, carrying `keyId`, `keyRemoteJid`, `messageType` and `content`.

`externalAttributes`: optional object copied into the persisted message's metadata and into the asynchronous `mentionAll` result webhooks.

`mentionAll`: optional boolean. When `true`, the recipient has to be a group JID, and the message is accepted for asynchronous processing when the WhatsApp protobuf message type supports `ContextInfo`.

## Endpoint bodies

### sendText

```http
POST /whatsapp/instances/beplus/messages/text
Content-Type: application/json
```

```json
{
  "number": "120363000000000000@g.us",
  "options": {
    "mentionAll": true,
    "presence": "composing",
    "delay": 1000
  },
  "textMessage": {
    "text": "An important notice for everyone."
  }
}
```

Supports `mentionAll`.

### sendLink

```http
POST /whatsapp/instances/beplus/messages/link
Content-Type: application/json
```

```json
{
  "number": "5531999999999",
  "options": {
    "presence": "composing"
  },
  "linkMessage": {
    "link": "https://example.com",
    "thumbnailUrl": "https://example.com/thumb.jpg",
    "title": "Example",
    "description": "Example link"
  }
}
```

Supports `mentionAll`.

### sendMedia

```http
POST /whatsapp/instances/beplus/messages/media
Content-Type: application/json
```

```json
{
  "number": "5531999999999",
  "options": {
    "presence": "composing"
  },
  "mediaMessage": {
    "mediatype": "image",
    "fileName": "image.jpg",
    "caption": "Caption",
    "media": "https://example.com/image.jpg"
  }
}
```

`mediatype` accepts `image`, `document`, `video`, `audio` and `ptv`. Supports `mentionAll`.

### sendMediaFile

```http
POST /whatsapp/instances/beplus/messages/media/file
Content-Type: multipart/form-data
```

Multipart fields:

```json
{
  "number": "5531999999999",
  "mediaType": "image",
  "caption": "Caption",
  "presence": "composing",
  "delay": "1200",
  "quotedMessageId": "123",
  "quotedMessage": "{\"keyId\":\"abc\",\"keyRemoteJid\":\"5531999999999@s.whatsapp.net\",\"messageType\":\"extendedTextMessage\",\"content\":{\"text\":\"quoted\"}}",
  "mentionAll": "false",
  "attachment": "<binary file>"
}
```

`attachment` is the file field. `mediaType` accepts `image`, `document`, `video`, `audio` and `ptv`. Supports `mentionAll`.

### sendWhatsAppAudio

```http
POST /whatsapp/instances/beplus/messages/audio
Content-Type: application/json
```

```json
{
  "number": "5531999999999",
  "options": {
    "presence": "recording"
  },
  "audioMessage": {
    "audio": "https://example.com/audio.mp3"
  }
}
```

Downloads the audio, converts and prepares it as WhatsApp PTT audio, and sends an `audioMessage`. Supports `mentionAll`.

### sendWhatsAppAudioFile

```http
POST /whatsapp/instances/beplus/messages/audio/file
Content-Type: multipart/form-data
```

Multipart fields:

```json
{
  "number": "5531999999999",
  "presence": "recording",
  "delay": "1200",
  "quotedMessageId": "123",
  "quotedMessage": "{\"keyId\":\"abc\",\"keyRemoteJid\":\"5531999999999@s.whatsapp.net\",\"messageType\":\"extendedTextMessage\",\"content\":{\"text\":\"quoted\"}}",
  "mentionAll": "false",
  "attachment": "<binary audio file>"
}
```

`attachment` is the audio file field. Supports `mentionAll`.

### sendContact

```http
POST /whatsapp/instances/beplus/messages/contact
Content-Type: application/json
```

```json
{
  "number": "5531999999999",
  "options": {
    "quotedMessageId": 123,
    "presence": "composing"
  },
  "contactMessage": [
    {
      "fullName": "BePlus",
      "wuid": "5531999999999@s.whatsapp.net",
      "phoneNumber": "+55 31 99999-9999",
      "organization": "BePlus",
      "vcard": "BEGIN:VCARD\nVERSION:3.0\nFN:BePlus\nTEL;type=CELL;waid=5531999999999:+55 31 99999-9999\nEND:VCARD"
    }
  ]
}
```

`contactMessage` accepts one or more contacts. If `vcard` is omitted, the service builds one from `fullName`, `wuid`, `phoneNumber` and `organization`. Supports `mentionAll`.

### sendLocation

```http
POST /whatsapp/instances/beplus/messages/location
Content-Type: application/json
```

```json
{
  "number": "5531999999999",
  "options": {
    "presence": "composing"
  },
  "locationMessage": {
    "name": "Belo Horizonte",
    "address": "Minas Gerais",
    "url": "https://example.com/place",
    "latitude": -19.9212,
    "longitude": -43.9378
  }
}
```

Supports `mentionAll`.

### sendReaction

```http
POST /whatsapp/instances/beplus/messages/reaction
Content-Type: application/json
```

```json
{
  "reactionMessage": {
    "key": {
      "remoteJid": "5531999999999@s.whatsapp.net",
      "fromMe": true,
      "id": "3EB0FDD9082F21A9AC3D"
    },
    "reaction": "ok"
  }
}
```

Does not support `mentionAll`, because `ReactionMessage` points at an existing message and has no valid `ContextInfo.MentionedJID` field. If `options.mentionAll=true`, the API answers `400 Bad Request` with the code `MENTION_ALL_NOT_SUPPORTED_FOR_MESSAGE_TYPE`.

## Success responses

Synchronous sends:

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

The body is `{"data": <persisted message>}`. The `send.message` webhook fires
after persistence.

Asynchronous sends with `mentionAll`:

```http
HTTP/1.1 202 Accepted
Content-Type: application/json
```

```json
{
  "data": {
    "statusCode": 202,
    "status": "processing",
    "message": "The message was accepted and is being processed.",
    "processId": "019f4ec1-f9b1-7c33-a4ef-d47715cb29e4",
    "instanceName": "beplus"
  }
}
```

`202 Accepted` means the snapshot, the processing job and the retention job were
committed in the same database transaction. The final result is delivered by the
existing `send.message` webhook and correlated through `processId`.

## Invisible mentions

`mentionAll=true` mentions every current participant of the group by filling WhatsApp's `ContextInfo.MentionedJID`. The server adds no visible `@phone` markers to the text, captions, contact cards, locations or media bodies.

Supported endpoints:

```text
sendText
sendLink
sendMedia
sendMediaFile
sendWhatsAppAudio
sendWhatsAppAudioFile
sendContact
sendLocation
```

Unsupported endpoints:

```text
sendReaction
```

The participant list is fetched when the worker processes the job. Participants who join after that fetch are not mentioned. Participants who leave during processing may still be present in the fetched list.

## The result arrives by webhook

The existing webhook event is reused:

```text
send.message
```

A success example:

```json
{
  "event": "send.message",
  "instance": {
    "id": 1,
    "name": "beplus",
    "connectionStatus": "online",
    "ownerJid": "5511999999999@s.whatsapp.net",
    "externalAttributes": {}
  },
  "data": {
    "processId": "019f4ec1-f9b1-7c33-a4ef-d47715cb29e4",
    "status": "sent",
    "mentionAll": true,
    "data": {
      "messageId": "3EB0FDD9082F21A9AC3D",
      "remoteJid": "120363000000000000@g.us",
      "participantCount": 84,
      "timestamp": "2026-07-07T15:00:00Z"
    },
    "externalAttributes": {
      "requestId": "request-456"
    }
  },
  "timestamp": "2026-07-07T15:00:01Z"
}
```

A failure example:

```json
{
  "event": "send.message",
  "instance": {
    "id": 1,
    "name": "beplus",
    "connectionStatus": "online",
    "ownerJid": "5511999999999@s.whatsapp.net",
    "externalAttributes": {}
  },
  "data": {
    "processId": "019f4ec1-f9b1-7c33-a4ef-d47715cb29e4",
    "status": "failed",
    "mentionAll": true,
    "error": {
      "code": "GROUP_MENTION_PROCESSING_FAILED",
      "message": "The group message could not be completed."
    },
    "externalAttributes": {
      "requestId": "request-456"
    }
  },
  "timestamp": "2026-07-07T15:00:01Z"
}
```

Error codes implemented for the asynchronous webhooks:

```text
INSTANCE_NOT_CONNECTED
GROUP_INFO_FETCH_FAILED
GROUP_HAS_NO_PARTICIPANTS
MESSAGE_SEND_FAILED
GROUP_MENTION_PROCESSING_FAILED
```

## HTTP errors

The recipient is not a group:

```json
{
  "data": {
    "statusCode": 400,
    "error": "bad-request",
    "code": "MENTION_ALL_REQUIRES_GROUP",
    "messages": [
      "mention all requires group"
    ]
  }
}
```

The message type does not support `mentionAll`:

```json
{
  "data": {
    "statusCode": 400,
    "error": "bad-request",
    "code": "MENTION_ALL_NOT_SUPPORTED_FOR_MESSAGE_TYPE",
    "messages": [
      "mention all is not supported for message type"
    ]
  }
}
```

Other errors of validation, authentication, instance, media, upload, persistence and the WhatsApp connection keep the API's standard error envelope.

## Queue behaviour

Asynchronous sends use Hesape's native `DatabaseQueue`. The package persists the
prepared protobuf and its metadata in `whatsapp_message_jobs`; the queue payload
carries only `processId`, so messages larger than the queue's 32 KiB limit stay
safe in the SQL snapshot. Creating the snapshot, the processing job and the
retention job happens in a single transaction: either all of them exist or none
is accepted.

The application registers the handler explicitly with
`Module.RegisterJobHandlers` and runs the dedicated queue:

```bash
aru queue:work --queue=whatsapp-messages --workers=N
```

The worker reissues the Grant with the same tenant and the same
`whatsapp.message.send` action Hesape recorded. Each job allows five attempts
with backoff. The `messageId` is stable across attempts, and the snapshot is
removed only after the persisted message, the result webhook and the completion
are committed; a missing snapshot is treated as an idempotent completion. On a
terminal failure, the package removes the snapshot only after committing the
durable job for the error webhook. If that commit fails, the dead letter keeps
the snapshot available to `queue:retry` up to the retention limit. On expiry,
the native cleanup job removes both the snapshot and the main job, which is what
stops message content being retained indefinitely.

## Arandu configuration

The module reads no environment variables. The application provides
`Config.Processing` (`ProcessingTimeout`, `GroupInfoTimeout`, `SendTimeout` and
`Retention`) and
`Config.Media` (limits, timeout, temporary directory and the paths to
`ffmpeg` and `ffprobe`). `Processing.Workers` and `Processing.QueueSize` are kept
for compatibility only and are ignored; concurrency belongs to
`aru queue:work --workers=N`. The defaults are 60s for the full processing, 30s
for lookup and send, and 30 days for recoverable retention.

## Processing flow

```text
1. The client sends a message compatible with options.mentionAll=true.
2. The API validates the authentication, the instance, the payload and the recipient.
3. The API confirms the recipient is a group JID.
4. The API prepares the protobuf and creates a `processId` with `data.NewID`.
5. The API persists the snapshot and the DatabaseQueue processing and retention jobs in the same transaction.
6. The API returns HTTP 202 Accepted.
7. A worker reloads the instance and resolves the WhatsApp session connected at that moment.
8. The worker fetches the group's current participants.
9. The participant JIDs are deduplicated and added to ContextInfo.MentionedJID.
10. The original visible body of the message is preserved.
11. The message is sent through whatsmeow.
12. Persistence, the durable result webhook, and removal of the snapshot and the retention job are committed atomically.
```

## Known limitations

`mentionAll` works only for group JIDs on the `g.us` server.

The server adds no visible `@phone` markers.

`sendReaction` rejects `mentionAll`, because reactions carry no valid message-level `ContextInfo`.

Very large groups can increase the processing time.

`202 Accepted` confirms only that the snapshot and the durable jobs were written;
it does not confirm that WhatsApp has sent the message yet.

The webhook is the source of the final result.

WhatsApp clients may display or notify invisible mentions in different ways.
