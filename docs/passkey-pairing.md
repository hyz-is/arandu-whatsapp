# Passkey pairing on WhatsApp

This document describes the API's support for pairing WhatsApp accounts that require a Passkey during the `whatsmeow` connection flow.

The Go worker remains the owner of the WhatsApp client. The browser extension is configured separately and is not part of this API flow.

## Purpose

Some WhatsApp accounts require a WebAuthn assertion before they allow a new device to be linked. Because the worker is headless, it fetches the challenge through `whatsmeow`, hands that challenge to the panel, receives the assertion produced in the account owner's browser, and sends the response back through the same `*whatsmeow.Client`.

The challenge and the assertion are ephemeral:

- they are not persisted in the database;
- they are not sent to external webhooks;
- they must not appear in logs;
- an assertion accepted for processing consumes the challenge.

## Prerequisites

- The instance has to exist and be active.
- The request has to carry a valid Arandu session in the configured tenant.
- The session's role has to be authorized for `ActionConnectionPair` in
  `Config.Policy.Roles`.
- There has to be an active QR pairing session for the instance.
- The WhatsApp client has to be connected and not logged in yet.
- The same process has to receive both the challenge and the assertion, because the internal Passkey cache lives inside the `*whatsmeow.Client` object.

## Endpoints

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/whatsapp/instances/{instance}/connection/passkey/challenge` | Returns or creates a WebAuthn challenge for the active pairing session. |
| `POST` | `/whatsapp/instances/{instance}/connection/passkey/assertion` | Receives the WebAuthn assertion and sends it to WhatsApp. |

## Headers

| Header | Required | Value |
| --- | --- | --- |
| `Cookie` | Yes | The `arandu_session` issued by the host application. |
| `Content-Type` | Yes, for the assertion | `application/json` |

The identity comes from the Arandu session, and every operation requires the Grant
the policy issues for the matching instance and tenant.

## Requesting a challenge

```http
POST /whatsapp/instances/beplus/connection/passkey/challenge
Cookie: arandu_session=<host-session>
```

Response `200 OK`:

```json
{
  "data": {
    "requestId": "7bbaf109-e0cc-44de-a434-8d48dfd5cb7b",
    "state": "AWAITING_ASSERTION",
    "expiresAt": "2026-07-06T18:30:00Z",
    "publicKey": {
      "challenge": "base64url-unpadded",
      "timeout": 300000,
      "rpId": "whatsapp.com",
      "allowCredentials": [
        {
          "id": "base64url-unpadded",
          "type": "public-key",
          "transports": ["internal", "hybrid"]
        }
      ],
      "userVerification": "required",
      "extensions": {}
    }
  }
}
```

If a valid, unconsumed challenge already exists, the endpoint returns the same `requestId` and the same `publicKey`. That keeps a double click from producing several challenges.

## Submitting the assertion

```http
POST /whatsapp/instances/beplus/connection/passkey/assertion
Cookie: arandu_session=<host-session>
Content-Type: application/json
```

Body:

```json
{
  "requestId": "7bbaf109-e0cc-44de-a434-8d48dfd5cb7b",
  "assertion": {
    "id": "credential-id",
    "rawId": "base64url-unpadded",
    "type": "public-key",
    "response": {
      "clientDataJSON": "base64url-unpadded",
      "authenticatorData": "base64url-unpadded",
      "signature": "base64url-unpadded",
      "userHandle": null
    }
  }
}
```

Response `202 Accepted`:

```json
{
  "data": {
    "state": "AWAITING_CONFIRMATION",
    "message": "The assertion was sent to WhatsApp."
  }
}
```

The final result still arrives through `whatsmeow`'s QR channel: Passkey confirmation, `PairSuccess` and the connection going online.

## States

| State | Meaning |
| --- | --- |
| `IDLE` | No Passkey flow is active. |
| `FETCHING_CHALLENGE` | The worker is fetching the challenge from WhatsApp. |
| `AWAITING_ASSERTION` | The challenge is available, waiting for the assertion from the browser. |
| `SUBMITTING_ASSERTION` | The assertion was validated locally and is being sent to WhatsApp. |
| `AWAITING_CONFIRMATION` | WhatsApp received the assertion and may require approval on the phone. |
| `CONFIRMATION_SENT` | The worker sent `SendPasskeyConfirmation`. |
| `COMPLETED` | Pairing finished. |
| `FAILED` | Passkey pairing failed. |
| `EXPIRED` | The challenge expired before it was used. |

## Errors

The envelope follows the API's current shape:

```json
{
  "data": {
    "statusCode": 409,
    "error": "conflict",
    "messages": ["invalid pairing state"],
    "code": "INVALID_PAIRING_STATE"
  }
}
```

| HTTP | Code |
| --- | --- |
| `403` | Session, tenant or role without `ActionConnectionPair`. |
| `404` | `PAIRING_SESSION_NOT_FOUND` |
| `409` | `PAIRING_SESSION_NOT_ACTIVE` |
| `409` | `INVALID_PAIRING_STATE` |
| `409` | `PASSKEY_REQUEST_MISMATCH` |
| `409` | `PASSKEY_CHALLENGE_ALREADY_USED` |
| `409` | `INSTANCE_ALREADY_CONNECTED` |
| `410` | `PASSKEY_CHALLENGE_EXPIRED` |
| `422` | `INVALID_PASSKEY_ASSERTION` |
| `422` | `PASSKEY_NOT_AVAILABLE` |
| `503` | `WHATSAPP_CLIENT_NOT_CONNECTED` |
| `503` | `PASSKEY_SERVICE_UNAVAILABLE` |

## Flow sequence

1. The panel starts the existing QR code flow.
2. The worker creates the `*whatsmeow.Client`, calls `GetQRChannel` and connects.
3. When WhatsApp requires a Passkey, the QR channel may emit `passkey-request`; if it does not, the panel calls the challenge endpoint.
4. The challenge endpoint uses the same `ManagedWhatsAppClient` and calls `DangerousInternals().GetPasskeyRequestOptions`.
5. The panel hands `publicKey` to the browser extension.
6. The extension runs WebAuthn on `web.whatsapp.com` and returns the assertion to the panel.
7. The panel sends the assertion to the canonical
   `/connection/passkey/assertion` route.
8. The worker validates `requestId`, the state, the expiry and single use, marks the challenge as consumed and calls `SendPasskeyResponse`.
9. The QR channel receives `passkey-confirmation`. If `SkipHandoffUX` is `false`, the worker calls `SendPasskeyConfirmation`; if it is `true`, `whatsmeow`'s own QR channel has already confirmed.
10. WhatsApp emits success and the existing flow publishes the instance as online.

## Base64url

The `challenge`, `allowCredentials[].id`, `rawId`, `clientDataJSON`, `authenticatorData`, `signature` and `userHandle` fields use base64url without padding.

Do not convert to standard base64, do not add `=`, do not swap `-` for `+`, do not swap `_` for `/`, and do not decode and re-encode in the panel. The API deserializes the assertion straight into `go.mau.fi/whatsmeow/types.WebAuthnResponse`.

## The same client

The challenge, the assertion and the confirmation have to use the same `*whatsmeow.Client` that started the QR channel. `whatsmeow` keeps an ephemeral cache inside that object during pairing.

There is no manual confirmation endpoint. Confirmation belongs to the worker that owns the client.

## Multiple replicas

This flow is correct for a single-node deployment, or for environments where an instance is always routed to the node that owns its `ManagedWhatsAppClient`.

If the challenge is created on node A and the assertion is sent to node B, pairing fails, because node B does not hold the internal cache of the `*whatsmeow.Client`. Redis or a database do not solve this on their own, and the cache must not be serialized.

In environments with several replicas, use per-instance affinity or ownership to make sure both Passkey endpoints reach the same process.

## The extension

The browser extension is neither installed nor configured by this API. All it has to do is receive the `publicKey`, run `navigator.credentials.get` in the right context, and return the assertion without transforming the base64url fields.
