package docs

import (
	"sort"

	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

const (
	documentVersion  = "1.2.0"
	whatsmeowVersion = "v0.0.0-20260904121843-28bfe537ea6a"
)

// Document is the structured source used to generate the public webhook docs.
type Document struct {
	Version         string            `json:"version"`
	GeneratedFrom   GeneratedFrom     `json:"generatedFrom"`
	Configuration   Configuration     `json:"configuration"`
	Delivery        Delivery          `json:"delivery"`
	Headers         []Header          `json:"headers"`
	Envelope        EnvelopeDoc       `json:"envelope"`
	CommonTypes     []CommonTypeDoc   `json:"commonTypes"`
	Events          []EventDoc        `json:"events"`
	IgnoredEvents   []IgnoredEventDoc `json:"ignoredEvents"`
	Compatibility   []string          `json:"compatibility"`
	Security        []string          `json:"security"`
	ErrorHandling   []string          `json:"errorHandling"`
	Ordering        []string          `json:"orderingAndConsistency"`
	GlobalNotes     []string          `json:"globalNotes"`
	SupportedEvents []string          `json:"supportedEvents"`
}

type GeneratedFrom struct {
	ConstantsPackage string   `json:"constantsPackage"`
	Dispatcher       string   `json:"dispatcher"`
	Normalizers      []string `json:"normalizers"`
	WhatsmeowVersion string   `json:"whatsmeowVersion"`
}

type Configuration struct {
	InstanceEndpoint   string   `json:"instanceEndpoint"`
	FindEndpoint       string   `json:"findEndpoint"`
	GlobalConfigFields []string `json:"globalConfigFields"`
	InstanceFields     []Field  `json:"instanceFields"`
	Notes              []string `json:"notes"`
}

type Delivery struct {
	Method                string   `json:"method"`
	Queue                 string   `json:"queue"`
	Job                   string   `json:"job"`
	MaxTries              int      `json:"maxTries"`
	Backoff               []string `json:"backoff"`
	HTTPTimeout           string   `json:"httpTimeout"`
	SuccessStatus         string   `json:"successStatus"`
	Retry                 string   `json:"retry"`
	InstanceFiltering     string   `json:"instanceFiltering"`
	GlobalFiltering       string   `json:"globalFiltering"`
	Persistence           string   `json:"persistence"`
	IdempotencyHeader     string   `json:"idempotencyHeader"`
	ExternalAttributes    string   `json:"externalAttributes"`
	AllowedWebhookSchemes []string `json:"allowedWebhookSchemes"`
}

type Header struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

type EnvelopeDoc struct {
	DataType string   `json:"dataType"`
	Fields   []Field  `json:"fields"`
	Notes    []string `json:"notes"`
}

type CommonTypeDoc struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Fields      []Field `json:"fields"`
}

type EventDoc struct {
	Name                    string         `json:"name"`
	Flag                    string         `json:"flag"`
	Description             string         `json:"description"`
	InternalEvents          []string       `json:"internalEvents"`
	Persistence             string         `json:"persistence"`
	DataType                string         `json:"dataType"`
	DataSchema              string         `json:"dataSchema"`
	DynamicFields           bool           `json:"dynamicFields"`
	Fields                  []Field        `json:"fields"`
	PossibleValues          []PossibleEnum `json:"possibleValues,omitempty"`
	Example                 map[string]any `json:"example"`
	Notes                   []string       `json:"notes,omitempty"`
	ImplementedIn           []string       `json:"implementedIn"`
	RequiresPersistenceFlag string         `json:"requiresPersistenceFlag,omitempty"`
}

type Field struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Nullable    bool     `json:"nullable"`
	Description string   `json:"description"`
	Values      []string `json:"values,omitempty"`
}

type PossibleEnum struct {
	Field  string   `json:"field"`
	Values []string `json:"values"`
}

type IgnoredEventDoc struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Description string `json:"description"`
}

func Build() Document {
	events := []EventDoc{
		callUpsertDoc(),
		chatsDeleteDoc(),
		chatsUpdatedDoc(),
		connectionUpdateDoc(),
		contactsUpdateDoc(),
		contactsUpsertDoc(),
		groupsParticipantsUpdateDoc(),
		groupsUpdateDoc(),
		groupsUpsertDoc(),
		historySyncDoc(),
		identityUpdateDoc(),
		labelsAssociationDoc(),
		labelsEditDoc(),
		mediaRetryDoc(),
		messagesDeleteDoc(),
		messagesStarDoc(),
		messagesUndecryptableDoc(),
		messagesUpdateDoc(),
		messagesUpsertDoc(),
		newsLetterDoc(),
		presenceUpdatedDoc(),
		profilePictureUpdateDoc(),
		qrcodeUpdatedDoc(),
		sendMessageDoc(),
		settingsUpdateDoc(),
		statusInstanceDoc(),
		userAboutUpdateDoc(),
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Name < events[j].Name
	})

	return Document{
		Version: documentVersion,
		GeneratedFrom: GeneratedFrom{
			ConstantsPackage: "internal/database/types/webhook.go",
			Dispatcher:       "internal/webhook/manager.go",
			Normalizers: []string{
				"internal/webhook/payload.go",
				"internal/webhook/normalizer.go",
				"internal/whatsapp/service.go",
				"internal/whatsapp/event_persistence.go",
				"internal/whatsapp/webhook_events.go",
				"internal/whatsapp/webhook_extended_events.go",
				"internal/message/service.go",
				"internal/message/audio.go",
			},
			WhatsmeowVersion: whatsmeowVersion,
		},
		Configuration: Configuration{
			InstanceEndpoint: "PUT /whatsapp/instances/{instance}/webhook",
			FindEndpoint:     "GET /whatsapp/instances/{instance}/webhook",
			GlobalConfigFields: []string{
				"Config.Webhooks.GlobalURL",
				"Config.Webhooks.GlobalEnabled",
				"Config.Webhooks.SigningSecret",
				"Config.Webhooks.Retention",
				"Config.Webhooks.Workers",
				"Config.Webhooks.QueueSize",
			},
			InstanceFields: []Field{
				field("url", "string", true, false, "HTTP or HTTPS URL used for the instance webhook."),
				field("enabled", "boolean", true, false, "Enables or disables the instance webhook."),
				field("events", "object", false, false, "Map of per-event flags. An absent field preserves the current configuration; an empty object removes the flags."),
			},
			Notes: []string{
				"Unknown fields in events are rejected by the DTO and repository validation.",
				"The per-instance configuration filters events using the flags described in this document.",
				"The global webhook, when enabled, receives every supported event without consulting the instance flags.",
				"Accepted URLs use only the http and https schemes.",
			},
		},
		Delivery: Delivery{
			Method:                "POST",
			Queue:                 "whatsapp-webhooks",
			Job:                   "whatsapp.webhook.deliver",
			MaxTries:              5,
			Backoff:               []string{"5s", "30s", "2m", "10m"},
			HTTPTimeout:           "15s",
			SuccessStatus:         "HTTP 2xx",
			Retry:                 "The native queue retries network failures, timeouts and non-2xx responses.",
			InstanceFiltering:     "The instance webhook delivers only events enabled in Webhook.events.",
			GlobalFiltering:       "The global webhook delivers every supported event when Config.Webhooks.GlobalEnabled=true.",
			Persistence:           "The durable snapshot lives in whatsapp_webhook_deliveries and the job carries only deliveryId; after a success the sensitive fields are compacted, and every snapshot expires under the configured retention.",
			IdempotencyHeader:     "X-Arandu-Delivery-ID",
			ExternalAttributes:    "instance.externalAttributes is always serialized as an object; absent, null or invalid values become {}.",
			AllowedWebhookSchemes: []string{"http", "https"},
		},
		Headers: []Header{
			{Name: "Content-Type", Value: "application/json", Description: "Payload format."},
			{Name: "User-Agent", Value: "Arandu-WhatsApp/1.0", Description: "Identifies the sender of the webhook."},
			{Name: "x-request-id", Value: "UUID or request id from the context", Description: "Tracing id for the delivery; it is not a guaranteed idempotency key."},
			{Name: "x-owner-jid", Value: "Owner JID or an empty string", Description: "Owner of the instance when available."},
			{Name: "x-instance-name", Value: "Instance name", Description: "Public name of the instance."},
			{Name: "x-instance-id", Value: "1", Description: "Internal numeric identifier of the instance."},
			{Name: "x-webhook-event", Value: "External event name", Description: "The same value as the event field in the envelope."},
			{Name: "X-Arandu-Delivery-ID", Value: "Delivery UUID", Description: "Stable key across retries; use it for idempotent deduplication."},
			{Name: "X-Arandu-Timestamp", Value: "Unix timestamp", Description: "Signed instant of the HTTP attempt; it is renewed between retries."},
			{Name: "X-Arandu-Signature", Value: "sha256=<hex>", Description: "HMAC-SHA256 of timestamp.deliveryId.body using Config.Webhooks.SigningSecret."},
		},
		Envelope: EnvelopeDoc{
			DataType: "object",
			Fields: []Field{
				field("event", "string", true, false, "External event name."),
				field("instance", "WebhookInstance", true, false, "Summary of the instance the event came from."),
				field("data", "object | array", true, false, "Event-specific payload."),
				field("timestamp", "string", true, false, "RFC3339 timestamp generated when the envelope is assembled."),
			},
			Notes: []string{
				"Every example below shows the complete envelope sent to the endpoint.",
				"The data field changes per event, but the envelope fields stay stable.",
			},
		},
		CommonTypes: []CommonTypeDoc{
			{
				Name:        "WebhookInstance",
				Description: "Summary of the instance used in every envelope.",
				Fields: []Field{
					field("id", "number", true, false, "Internal numeric identifier of the instance."),
					field("name", "string", true, false, "Instance name."),
					field("connectionStatus", "string", true, false, "Current connection status stored on the instance, normally in lower case."),
					field("ownerJid", "string", true, true, "JID of the instance owner; null when there is none."),
					field("externalAttributes", "object", true, false, "External attributes of the instance; always an object."),
				},
			},
			{
				Name:        "MessageWebhookData",
				Description: "DTO used by messages.upsert and send.message.",
				Fields:      messageFields(),
			},
			{
				Name:        "ContactUpsertWebhookData",
				Description: "DTO for the initial creation or update of a contact.",
				Fields:      contactUpsertFields(),
			},
			{
				Name:        "ContactUpdateWebhookData",
				Description: "DTO for a partial contact update.",
				Fields:      contactUpdateFields(),
			},
			{
				Name:        "GroupParticipantWebhookData",
				Description: "Participant used in the group events.",
				Fields:      groupParticipantFields(),
			},
		},
		Events: events,
		IgnoredEvents: []IgnoredEventDoc{
			{Name: "PairPasskeyConfirmation", Status: "intentionally_ignored", Description: "Interactive pairing event carrying a code; it is not part of the webhook contract."},
			{Name: "PairPasskeyError", Status: "handled_without_webhook", Description: "Handled as pairing state and log; there is no stable public payload."},
			{Name: "PairPasskeyRequest", Status: "intentionally_ignored", Description: "Carries the pairing challenge and public key, and is not serialized into a webhook."},
			{Name: "QRScannedWithoutMultidevice", Status: "handled_without_webhook", Description: "The QR channel turns this case into a pairing failure; future direct emissions fall through to the fallback log."},
			{Name: "MediaRetryError", Status: "internal_only", Description: "Support error struct used inside the media.retry payload."},
			{Name: "MexNotificationData", Status: "internal_only", Description: "Support struct for MEX notifications with no dedicated public event."},
			{Name: "NewsletterMessageMeta", Status: "internal_only", Description: "Support struct used inside news.letter."},
		},
		Compatibility: []string{
			"The official event names are the values listed in events[].name.",
			"Boolean fields of the per-instance configuration use the names listed in events[].flag.",
			"New whatsmeow events have to be added to the official types in internal/database/types/webhook.go first, and to this contract afterwards.",
		},
		Security: []string{
			"Every enabled delivery requires a Config.Webhooks.SigningSecret of at least 32 bytes and carries an HMAC-SHA256 the destination can verify.",
			"The signature covers the exact bytes of X-Arandu-Timestamp + dot + X-Arandu-Delivery-ID + dot + body; compare it in constant time.",
			"Use HTTPS and reject timestamps outside the accepted window before processing the event.",
			"x-request-id is for tracing and correlation; X-Arandu-Delivery-ID is the stable key for deduplication across retries.",
		},
		ErrorHandling: []string{
			"Only HTTP 2xx responses count as a success.",
			"Network failures, timeouts and non-2xx responses persist a failed status and return an error to the native queue.",
			"The job allows five attempts with backoff; after that the native queue parks it for inspection.",
			"A delivery already marked as delivered is completed without a new POST when the same job reappears.",
			"A job whose snapshot expired or was removed with the instance finishes idempotently, without a pointless retry.",
		},
		Ordering: []string{
			"The delivery queue is durable and processed by the host application's native workers.",
			"The relative order between events is guaranteed neither across instances nor across different events of the same instance.",
			"Some events depend on prior persistence. When persistence fails, the event may not be emitted.",
		},
		GlobalNotes: []string{
			"This document describes only events actually implemented in the current code.",
			"The examples use illustrative values while keeping the real shape of the envelope and the DTOs.",
			"Fields marked as dynamic can carry additional properties, depending on the originating whatsmeow event.",
		},
		SupportedEvents: supportedEventNames(),
	}
}

func qrcodeUpdatedDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventQRCodeUpdated),
		Flag:           "qrcodeUpdated",
		Description:    "A new QR code is available for pairing the instance.",
		InternalEvents: []string{"QR channel"},
		Persistence:    "Sets the instance status to qr_code before delivery.",
		DataType:       "object",
		DataSchema:     "QRCodeUpdatedWebhookData",
		Fields: []Field{
			field("count", "number", true, false, "Number of QR codes emitted in this attempt."),
			field("code", "string", true, false, "Raw QR code payload."),
			field("base64", "string", true, false, "QR code image as a base64 data URL."),
			field("expiresInSeconds", "number", true, false, "Time left as reported by the QR channel, in seconds."),
			field("expiresAt", "string", true, false, "RFC3339 UTC timestamp computed for the QR code expiry."),
		},
		Example: envelopeWithInstance(dbtypes.WebhookEventQRCodeUpdated, map[string]any{
			"id":                 1,
			"name":               "beplus",
			"connectionStatus":   "qr_code",
			"ownerJid":           nil,
			"externalAttributes": map[string]any{},
		}, map[string]any{
			"count":            1,
			"code":             "2@abc",
			"base64":           "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA",
			"expiresInSeconds": 60,
			"expiresAt":        "2026-07-04T18:01:00Z",
		}),
		Notes: []string{"Emitted by the QR flow rather than directly by a whatsmeow event struct."},
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/webhook/manager.go",
		},
	}
}

func historySyncDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventHistorySync),
		Flag:           "historySync",
		Description:    "A history synchronization was received from WhatsApp.",
		InternalEvents: []string{"*events.HistorySync"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "HistorySyncWebhookData",
		DynamicFields:  true,
		Fields: []Field{
			field("type", "string", true, false, "Fixed payload type.", "history.sync"),
			field("dateTime", "string", true, false, "RFC3339 UTC timestamp of the event or of the processing."),
			field("data", "object", false, false, "Normalized content of the history event when available."),
		},
		PossibleValues: []PossibleEnum{{Field: "type", Values: []string{"history.sync"}}},
		Example: envelope(dbtypes.WebhookEventHistorySync, map[string]any{
			"type":     "history.sync",
			"dateTime": "2026-07-04T18:00:00Z",
			"data": map[string]any{
				"syncType": "INITIAL_BOOTSTRAP",
			},
		}),
		Notes: []string{"Dynamic payload, because the content comes from whatsmeow's history proto."},
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_extended_events.go",
		},
	}
}

func messagesUpsertDoc() EventDoc {
	return EventDoc{
		Name:                    string(dbtypes.WebhookEventMessagesUpsert),
		Flag:                    "messagesUpsert",
		Description:             "A message received and persisted by the application.",
		InternalEvents:          []string{"*events.Message", "*events.FBMessage"},
		Persistence:             "Requires Config.Persistence.Messages=true. The message is persisted with CreateOrIgnore and read back before delivery.",
		RequiresPersistenceFlag: "Config.Persistence.Messages",
		DataType:                "object",
		DataSchema:              "MessageWebhookData",
		Fields:                  messageFields(),
		Example: envelope(dbtypes.WebhookEventMessagesUpsert, map[string]any{
			"id":                1024,
			"keyId":             "ABC123",
			"keyRemoteJid":      "5511999999999@s.whatsapp.net",
			"keyLid":            nil,
			"keyFromMe":         false,
			"keyParticipant":    nil,
			"keyParticipantLid": nil,
			"pushName":          "Cliente",
			"messageType":       "conversation",
			"content":           map[string]any{"text": "Ola"},
			"messageTimestamp":  1783188000,
			"device":            "ios",
			"isGroup":           false,
			"metadata":          nil,
		}),
		Notes: []string{"If persisting or reading the message back fails, the webhook is not emitted."},
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/event_persistence.go",
			"internal/webhook/payload.go",
		},
	}
}

func messagesUpdateDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventMessagesUpdated),
		Flag:           "messagesUpdated",
		Description:    "The receipt or status of an already known message was updated.",
		InternalEvents: []string{"*events.Receipt"},
		Persistence:    "The webhook is delivered even when Config.Persistence.MessageUpdates=false. When true, the message is located and the update persisted before delivery.",
		DataType:       "object",
		DataSchema:     "MessageUpdateWebhookData",
		Fields: []Field{
			field("id", "number", true, false, "Internal id of the persisted message; 0 when update persistence is disabled."),
			field("keyId", "string", true, false, "External id or key of the message on WhatsApp."),
			field("status", "string", true, false, "Normalized receipt status.", "delivered", "sent", "read", "played", "server_error", "retry", "unknown"),
			field("dateTime", "string", true, false, "RFC3339 UTC timestamp of the receipt; it uses the event timestamp or the processing time."),
		},
		PossibleValues: []PossibleEnum{{Field: "status", Values: []string{"delivered", "sent", "read", "played", "server_error", "retry", "unknown"}}},
		Example: envelope(dbtypes.WebhookEventMessagesUpdated, map[string]any{
			"id":       1024,
			"keyId":    "ABC123",
			"status":   "read",
			"dateTime": "2026-07-04T18:05:00Z",
		}),
		Notes: []string{
			"With Config.Persistence.MessageUpdates=true the message has to exist; if it is not found after the configured attempts, the event is dropped.",
			"With persistence disabled, id is 0 and keyId still identifies the message the receipt is about.",
		},
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/event_persistence.go",
			"internal/webhook/payload.go",
		},
	}
}

func messagesDeleteDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventMessagesDeleted),
		Flag:           "messagesDeleted",
		Description:    "A message was removed locally by the DeleteForMe event.",
		InternalEvents: []string{"*events.DeleteForMe"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "MessageDeletedWebhookData",
		Fields: []Field{
			field("id", "number", false, false, "Internal id of the persisted message when found by keyId."),
			field("chatJid", "string", true, false, "Chat JID."),
			field("senderJid", "string", false, false, "Sender JID when available; omitted when absent."),
			field("keyFromMe", "boolean", true, false, "Whether the message belonged to the instance itself."),
			field("keyId", "string", true, false, "Id of the deleted message."),
			field("deleteMedia", "boolean", true, false, "Whether the local media should be removed."),
			field("fromFullSync", "boolean", true, false, "Whether it came from a full synchronization."),
			field("dateTime", "string", true, false, "RFC3339 UTC timestamp of the event or of the processing."),
			field("messageTime", "string", false, false, "Original RFC3339 UTC timestamp of the message when reported; omitted when absent."),
		},
		Example: envelope(dbtypes.WebhookEventMessagesDeleted, map[string]any{
			"id":           1024,
			"chatJid":      "5511999999999@s.whatsapp.net",
			"senderJid":    "5511988888888@s.whatsapp.net",
			"keyFromMe":    false,
			"keyId":        "ABC123",
			"deleteMedia":  true,
			"fromFullSync": false,
			"dateTime":     "2026-07-04T18:00:00Z",
			"messageTime":  "2026-07-04T17:59:00Z",
		}),
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_events.go",
		},
	}
}

func messagesStarDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventMessagesStarred),
		Flag:           "messagesStarred",
		Description:    "A message was starred or unstarred.",
		InternalEvents: []string{"*events.Star"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "MessageStarredWebhookData",
		Fields: []Field{
			field("chatJid", "string", true, false, "Chat JID."),
			field("senderJid", "string", false, false, "Sender JID when available; omitted when absent."),
			field("keyFromMe", "boolean", true, false, "Whether the message belongs to the instance itself."),
			field("keyId", "string", true, false, "Message id."),
			field("starred", "boolean", true, false, "true when the message is starred."),
			field("fromFullSync", "boolean", true, false, "Whether it came from a full synchronization."),
			field("dateTime", "string", true, false, "RFC3339 UTC timestamp of the event or of the processing."),
		},
		Example: envelope(dbtypes.WebhookEventMessagesStarred, map[string]any{
			"chatJid":      "5511999999999@s.whatsapp.net",
			"senderJid":    "5511988888888@s.whatsapp.net",
			"keyFromMe":    false,
			"keyId":        "ABC123",
			"starred":      true,
			"fromFullSync": false,
			"dateTime":     "2026-07-04T18:00:00Z",
		}),
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_events.go",
		},
	}
}

func messagesUndecryptableDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventMessagesUndecryptable),
		Flag:           "messagesUndecryptable",
		Description:    "A message received that could not be decrypted.",
		InternalEvents: []string{"*events.UndecryptableMessage"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "MessageUndecryptableWebhookData",
		Fields: []Field{
			field("keyId", "string", true, false, "Id of the key or message that failed."),
			field("chatJid", "string", true, false, "Chat JID."),
			field("senderJid", "string", false, false, "Sender JID when available; omitted when absent."),
			field("keyFromMe", "boolean", true, false, "Whether the message belongs to the instance itself."),
			field("isUnavailable", "boolean", true, false, "Whether the content was marked as unavailable."),
			field("unavailableType", "string", false, false, "Unavailability type.", "view_once"),
			field("decryptFailMode", "string", false, false, "Reported failure mode.", "hide"),
			field("dateTime", "string", true, false, "RFC3339 UTC timestamp of the event or of the processing."),
		},
		PossibleValues: []PossibleEnum{
			{Field: "unavailableType", Values: []string{"view_once"}},
			{Field: "decryptFailMode", Values: []string{"hide"}},
		},
		Example: envelope(dbtypes.WebhookEventMessagesUndecryptable, map[string]any{
			"keyId":           "ABC123",
			"chatJid":         "5511999999999@s.whatsapp.net",
			"senderJid":       "5511988888888@s.whatsapp.net",
			"keyFromMe":       false,
			"isUnavailable":   true,
			"unavailableType": "view_once",
			"decryptFailMode": "hide",
			"dateTime":        "2026-07-04T18:00:00Z",
		}),
		Notes: []string{"Empty fields are dropped by omitempty."},
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_events.go",
		},
	}
}

func sendMessageDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventSendMessage),
		Flag:           "sendMessage",
		Description:    "A message sent through the API, after a successful send and persistence.",
		InternalEvents: []string{"message service send result"},
		Persistence:    "Persisted before delivery by the API message-sending flow.",
		DataType:       "object",
		DataSchema:     "MessageWebhookData",
		Fields:         messageFields(),
		Example: envelope(dbtypes.WebhookEventSendMessage, map[string]any{
			"id":                2048,
			"keyId":             "ABC123",
			"keyRemoteJid":      "5511999999999@s.whatsapp.net",
			"keyLid":            nil,
			"keyFromMe":         true,
			"keyParticipant":    nil,
			"keyParticipantLid": nil,
			"pushName":          nil,
			"messageType":       "conversation",
			"content":           map[string]any{"text": "Sent message"},
			"messageTimestamp":  1783188000,
			"device":            "web",
			"isGroup":           false,
			"metadata":          nil,
		}),
		Notes: []string{
			"Uses the same DTO as messages.upsert, but the source is a send through the API itself.",
			"When a message is accepted with options.mentionAll=true, the same send.message event also delivers the final result of the asynchronous processing. In that case data carries processId, status, mentionAll, externalAttributes and, on success, data.messageId, data.remoteJid, data.participantCount and data.timestamp. On failure it carries error.code and error.message.",
		},
		ImplementedIn: []string{
			"internal/message/service.go",
			"internal/message/audio.go",
			"internal/webhook/payload.go",
		},
	}
}

func contactsUpsertDoc() EventDoc {
	return EventDoc{
		Name:                    string(dbtypes.WebhookEventContactsUpsert),
		Flag:                    "contactsUpsert",
		Description:             "A contact was created or updated in the local records.",
		InternalEvents:          []string{"*events.Contact"},
		Persistence:             "Requires Config.Persistence.Contacts=true. The contact is stored before delivery.",
		RequiresPersistenceFlag: "Config.Persistence.Contacts",
		DataType:                "object",
		DataSchema:              "ContactUpsertWebhookData",
		Fields:                  contactUpsertFields(),
		PossibleValues:          []PossibleEnum{{Field: "action", Values: []string{"upserted"}}},
		Example: envelope(dbtypes.WebhookEventContactsUpsert, map[string]any{
			"id":            41,
			"remoteJid":     "5511999999999@s.whatsapp.net",
			"lid":           "279847268053216@lid",
			"pushName":      "Cliente",
			"profilePicUrl": nil,
			"action":        "upserted",
		}),
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/event_persistence.go",
		},
	}
}

func contactsUpdateDoc() EventDoc {
	return EventDoc{
		Name:                    string(dbtypes.WebhookEventContactsUpdated),
		Flag:                    "contactsUpdated",
		Description:             "An existing contact was partially updated.",
		InternalEvents:          []string{"*events.PushName", "*events.BusinessName"},
		Persistence:             "Requires Config.Persistence.Contacts=true. The contact is updated before delivery where applicable.",
		RequiresPersistenceFlag: "Config.Persistence.Contacts",
		DataType:                "array",
		DataSchema:              "ContactUpdateWebhookData[]",
		Fields:                  contactUpdateFields(),
		PossibleValues: []PossibleEnum{
			{Field: "action", Values: []string{"updated"}},
			{Field: "source", Values: []string{"pushName", "businessName"}},
		},
		Example: envelope(dbtypes.WebhookEventContactsUpdated, []any{
			map[string]any{
				"id":           41,
				"remoteJid":    "5511999999999@s.whatsapp.net",
				"lid":          nil,
				"pushName":     "Cliente Atualizado",
				"businessName": nil,
				"action":       "updated",
				"source":       "pushName",
			},
		}),
		Notes: []string{"The payload is an array; the current handler normally sends one item per delivery."},
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/event_persistence.go",
			"internal/whatsapp/webhook_extended_events.go",
		},
	}
}

func chatsUpdatedDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventChatsUpdated),
		Flag:           "chatsUpdated",
		Description:    "Chat properties were updated.",
		InternalEvents: []string{"*events.Blocklist", "*events.BlocklistChange", "*events.Archive", "*events.UnarchiveChatsSetting", "*events.ClearChat", "*events.Pin", "*events.Mute", "*events.MarkChatAsRead"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "ChatUpdatedWebhookData",
		DynamicFields:  true,
		Fields: []Field{
			field("type", "string", true, false, "Subtype of the chat update.", "blocklist", "blocklist.change", "archive", "unarchive.setting", "clear", "pin", "mute", "mark.read"),
			field("dateTime", "string", true, false, "RFC3339 UTC timestamp of the event or of the processing."),
			field("chatJid", "string", false, false, "Chat JID when the subtype has a specific chat."),
			field("fromFullSync", "boolean", false, false, "Whether it came from a full synchronization, when available."),
			field("additionalProperties", "object", false, false, "Flattened fields of the original whatsmeow event."),
		},
		PossibleValues: []PossibleEnum{{Field: "type", Values: []string{"blocklist", "blocklist.change", "archive", "unarchive.setting", "clear", "pin", "mute", "mark.read"}}},
		Example: envelope(dbtypes.WebhookEventChatsUpdated, map[string]any{
			"type":     "archive",
			"dateTime": "2026-07-04T18:00:00Z",
			"chatJid":  "5511999999999@s.whatsapp.net",
			"archived": true,
		}),
		Notes: []string{"UserStatusMute events are documented under settings.update, because the current registration routes them to that external event."},
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_events.go",
		},
	}
}

func chatsDeleteDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventChatsDeleted),
		Flag:           "chatsDeleted",
		Description:    "A chat was deleted or cleared.",
		InternalEvents: []string{"*events.DeleteChat"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "ChatDeletedWebhookData",
		DynamicFields:  true,
		Fields: []Field{
			field("chatJid", "string", true, false, "Chat JID."),
			field("dateTime", "string", true, false, "RFC3339 timestamp of the processing."),
			field("deleteMedia", "boolean", false, false, "Whether local media was removed, when present."),
			field("additionalProperties", "object", false, false, "Flattened fields of the original action."),
		},
		Example: envelope(dbtypes.WebhookEventChatsDeleted, map[string]any{
			"chatJid":     "5511999999999@s.whatsapp.net",
			"dateTime":    "2026-07-04T18:00:00Z",
			"deleteMedia": false,
		}),
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_events.go",
		},
	}
}

func presenceUpdatedDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventPresenceUpdated),
		Flag:           "presenceUpdated",
		Description:    "User presence or chat presence was updated.",
		InternalEvents: []string{"*events.ChatPresence", "*events.Presence"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "PresenceUpdatedWebhookData",
		DynamicFields:  true,
		Fields: []Field{
			field("type", "string", false, false, "Fixed presence type in the payload coming from *events.Presence.", "presence"),
			field("chatJid", "string", false, false, "Chat JID in the ChatPresence payload."),
			field("senderJid", "string", false, false, "Sender JID in the ChatPresence payload."),
			field("state", "string", false, false, "Presence state in the ChatPresence payload."),
			field("media", "string", false, false, "Media type when the presence relates to media."),
			field("jid", "string", false, false, "JID in the Presence payload."),
			field("unavailable", "boolean", false, false, "Whether the user is unavailable, in the Presence payload."),
			field("lastSeen", "string", false, false, "Last seen when reported."),
			field("dateTime", "string", true, false, "RFC3339 timestamp of the processing."),
		},
		PossibleValues: []PossibleEnum{{Field: "type", Values: []string{"presence"}}},
		Example: envelope(dbtypes.WebhookEventPresenceUpdated, map[string]any{
			"chatJid":   "5511999999999@s.whatsapp.net",
			"senderJid": "5511999999999@s.whatsapp.net",
			"state":     "composing",
			"media":     "text",
			"dateTime":  "2026-07-04T18:00:00Z",
		}),
		Notes: []string{"The shape differs between ChatPresence and Presence; use the fields present in the payload you receive."},
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_events.go",
		},
	}
}

func groupsUpsertDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventGroupsUpsert),
		Flag:           "groupsUpsert",
		Description:    "A group was created, discovered or synchronized.",
		InternalEvents: []string{"*events.JoinedGroup"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "array",
		DataSchema:     "GroupUpsertWebhookData[]",
		Fields:         groupUpsertFields(),
		Example: envelope(dbtypes.WebhookEventGroupsUpsert, []any{
			map[string]any{
				"id":             "120363000000000000@g.us",
				"addressingMode": "pn",
				"owner":          "5531999999999@s.whatsapp.net",
				"subject":        "Group",
				"isCommunity":    false,
				"participants": []any{
					map[string]any{
						"id":           "5511999999999@s.whatsapp.net",
						"lid":          "279847268053216@lid",
						"isAdmin":      true,
						"isSuperAdmin": false,
						"admin":        "admin",
					},
				},
				"creation": 1783187000,
			},
		}),
		Notes: []string{"The payload is an array for compatibility with list contracts, even when a delivery carries a single group."},
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_extended_events.go",
		},
	}
}

func groupsUpdateDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventGroupsUpdated),
		Flag:           "groupsUpdated",
		Description:    "Group metadata was partially updated.",
		InternalEvents: []string{"*events.GroupInfo"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "array",
		DataSchema:     "GroupUpdateWebhookData[]",
		Fields:         groupUpdateFields(),
		Example: envelope(dbtypes.WebhookEventGroupsUpdated, []any{
			map[string]any{
				"partial": map[string]any{
					"id":          "120363000000000000@g.us",
					"subject":     "New group name",
					"announce":    true,
					"subjectTime": 1783188000,
				},
			},
		}),
		Notes: []string{"The current handler sends an array holding one item that carries partial."},
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_extended_events.go",
		},
	}
}

func groupsParticipantsUpdateDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventGroupsParticipantsUpdated),
		Flag:           "groupsParticipantsUpdated",
		Description:    "Group participants changed.",
		InternalEvents: []string{"*events.GroupInfo"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "GroupParticipantsUpdatedWebhookData",
		Fields: []Field{
			field("id", "string", true, false, "Group JID."),
			field("author", "string", true, false, "JID of the author of the change; empty string when absent."),
			field("authorPn", "string", false, false, "Phone number of the author when available; omitted when absent."),
			field("participants", "GroupParticipantWebhookData[]", true, false, "Participantes afetados."),
			field("action", "string", true, false, "Acao aplicada.", "add", "remove", "promote", "demote"),
		},
		PossibleValues: []PossibleEnum{{Field: "action", Values: []string{"add", "remove", "promote", "demote"}}},
		Example: envelope(dbtypes.WebhookEventGroupsParticipantsUpdated, map[string]any{
			"id":       "120363000000000000@g.us",
			"author":   "5531999999999@s.whatsapp.net",
			"authorPn": "5531999999999",
			"participants": []any{
				map[string]any{
					"id":           "5511999999999@s.whatsapp.net",
					"isAdmin":      false,
					"isSuperAdmin": false,
					"admin":        nil,
				},
			},
			"action": "add",
		}),
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_extended_events.go",
		},
	}
}

func connectionUpdateDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventConnectionUpdated),
		Flag:           "connectionUpdated",
		Description:    "The instance connection state changed.",
		InternalEvents: []string{"*events.PairSuccess", "*events.PairError", "*events.Connected", "*events.Disconnected", "*events.LoggedOut", "*events.StreamReplaced", "*events.KeepAliveTimeout", "*events.KeepAliveRestored", "*events.ConnectFailure", "*events.ManualLoginReconnect", "*events.StreamError", "*events.CATRefreshError"},
		Persistence:    "The instance status is updated by the connection flows before or alongside the delivery, depending on the subtype.",
		DataType:       "object",
		DataSchema:     "ConnectionUpdateWebhookData",
		Fields: []Field{
			field("type", "string", true, false, "Normalized connection subtype.", "pair.success", "connected", "disconnected", "logged.out", "stream.replaced", "keepalive.timeout", "keepalive.restored", "connect.failure", "manual.reconnect", "pair.error", "stream.error", "cat.refresh.error"),
			field("connection", "string", true, false, "External connection state.", "connecting", "open", "close", "replaced", "timeout"),
			field("statusReason", "number", false, false, "Numeric reason code when non-zero; omitted when zero."),
			field("lastConnection", "string", false, false, "RFC3339 UTC timestamp when reported; omitted when absent."),
			field("message", "string", false, false, "Technical message when reported; omitted when empty."),
		},
		PossibleValues: []PossibleEnum{
			{Field: "type", Values: []string{"pair.success", "connected", "disconnected", "logged.out", "stream.replaced", "keepalive.timeout", "keepalive.restored", "connect.failure", "manual.reconnect", "pair.error", "stream.error", "cat.refresh.error"}},
			{Field: "connection", Values: []string{"connecting", "open", "close", "replaced", "timeout"}},
		},
		Example: envelope(dbtypes.WebhookEventConnectionUpdated, map[string]any{
			"type":           "connected",
			"connection":     "open",
			"lastConnection": "2026-07-04T18:50:00Z",
		}),
		Notes: []string{"`statusReason`, `lastConnection` and `message` use `omitempty`; when they are zero or empty they do not appear in the JSON."},
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/webhook/payload.go",
		},
	}
}

func statusInstanceDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventStatusInstance),
		Flag:           "statusInstance",
		Description:    "Operational state events or warnings from the instance.",
		InternalEvents: []string{"*events.ClientOutdated", "*events.TemporaryBan", "*events.OfflineSyncPreview", "*events.OfflineSyncCompleted", "*events.PrivacySettings", "*events.AppState", "*events.AppStateSyncComplete", "*events.AppStateSyncError", "*events.AccountReachoutTimelock"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "InstanceStatusWebhookData",
		DynamicFields:  true,
		Fields: []Field{
			field("type", "string", true, false, "Subtype of the instance status."),
			field("status", "string", false, false, "Textual status of the subtype; omitted when empty."),
			field("message", "string", false, false, "Technical or human-readable message; omitted when empty."),
			field("data", "object", false, false, "Additional data for the subtype; omitted when absent."),
		},
		PossibleValues: []PossibleEnum{{Field: "type", Values: []string{"client.outdated", "temporary.ban", "offline.sync.preview", "offline.sync.completed", "privacy.settings", "app.state", "app.state.sync.completed", "app.state.sync.error", "account.reachout.timelock"}}},
		Example: envelope(dbtypes.WebhookEventStatusInstance, map[string]any{
			"type":   "offline.sync.completed",
			"status": "completed",
			"data": map[string]any{
				"count": 185,
			},
		}),
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/webhook/payload.go",
		},
	}
}

func newsLetterDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventNewsletter),
		Flag:           "newsLetter",
		Description:    "Events related to newsletters and channels.",
		InternalEvents: []string{"*events.NewsletterJoin", "*events.NewsletterLeave", "*events.NewsletterLiveUpdate", "*events.NewsletterMessageMeta", "*events.NewsletterMuteChange"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "NewsLetterWebhookData",
		DynamicFields:  true,
		Fields: []Field{
			field("type", "string", true, false, "Subtype of the newsletter event.", "join", "leave", "live.update", "message.meta", "mute.change"),
			field("newsletterJid", "string", false, false, "Newsletter JID when the source carries an id or jid."),
			field("dateTime", "string", true, false, "RFC3339 timestamp of the processing."),
			field("additionalProperties", "object", false, false, "Flattened fields of the original event."),
		},
		PossibleValues: []PossibleEnum{{Field: "type", Values: []string{"join", "leave", "live.update", "message.meta", "mute.change"}}},
		Example: envelope(dbtypes.WebhookEventNewsletter, map[string]any{
			"type":          "mute.change",
			"newsletterJid": "120363000000000000@newsletter",
			"muted":         true,
			"dateTime":      "2026-07-04T18:00:00Z",
		}),
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_extended_events.go",
		},
	}
}

func callUpsertDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventCallUpsert),
		Flag:           "callUpsert",
		Description:    "A voice or video call was updated.",
		InternalEvents: []string{"*events.CallOffer", "*events.CallAccept", "*events.CallOfferNotice", "*events.CallPreAccept", "*events.CallTransport", "*events.CallTerminate", "*events.CallReject", "*events.CallRelayLatency", "*events.UnknownCallEvent"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "CallUpsertWebhookData",
		Fields: []Field{
			field("chatId", "string", true, false, "Chat JID of the call."),
			field("from", "string", true, false, "Source JID."),
			field("callerPn", "string | null", true, true, "Phone number of the caller when available."),
			field("isGroup", "boolean | null", true, true, "Whether it is a group call, when the normalizer can infer it."),
			field("groupJid", "string | null", true, true, "Group JID when available."),
			field("id", "string", true, false, "Call id."),
			field("date", "string", true, false, "RFC3339 timestamp of the call or of the processing."),
			field("isVideo", "boolean | null", true, true, "Whether it is a video call, when the normalizer can infer it."),
			field("status", "string", true, false, "Normalized call status.", "offer", "ringing", "preaccept", "transport", "relaylatency", "timeout", "reject", "accept", "terminate", "unknown"),
			field("offline", "boolean", true, false, "Whether the event arrived as offline."),
			field("latencyMs", "number | null", true, true, "Latency in milliseconds when reported."),
		},
		PossibleValues: []PossibleEnum{{Field: "status", Values: []string{"offer", "ringing", "preaccept", "transport", "relaylatency", "timeout", "reject", "accept", "terminate", "unknown"}}},
		Example: envelope(dbtypes.WebhookEventCallUpsert, map[string]any{
			"chatId":    "5511999999999@s.whatsapp.net",
			"from":      "5511999999999@s.whatsapp.net",
			"callerPn":  "5511999999999",
			"isGroup":   false,
			"groupJid":  nil,
			"id":        "3EB0C4D0A1",
			"date":      "2026-07-04T19:05:00Z",
			"isVideo":   false,
			"status":    "offer",
			"offline":   false,
			"latencyMs": nil,
		}),
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_extended_events.go",
			"internal/webhook/payload.go",
		},
	}
}

func labelsAssociationDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventLabelsAssociation),
		Flag:           "labelsAssociation",
		Description:    "A label was associated with or removed from a chat or message.",
		InternalEvents: []string{"*events.LabelAssociationChat", "*events.LabelAssociationMessage"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "LabelsAssociationWebhookData",
		DynamicFields:  true,
		Fields: []Field{
			field("type", "string", true, false, "Association type.", "chat", "message"),
			field("chatJid", "string", true, false, "Chat JID."),
			field("messageId", "string", false, false, "Message id when type=message."),
			field("labelId", "string", true, false, "Label id."),
			field("action", "string", false, false, "Action inferred when labeled is present.", "add", "remove"),
			field("dateTime", "string", true, false, "RFC3339 timestamp of the processing."),
			field("additionalProperties", "object", false, false, "Flattened fields of the original event."),
		},
		PossibleValues: []PossibleEnum{
			{Field: "type", Values: []string{"chat", "message"}},
			{Field: "action", Values: []string{"add", "remove"}},
		},
		Example: envelope(dbtypes.WebhookEventLabelsAssociation, map[string]any{
			"type":     "chat",
			"chatJid":  "5511999999999@s.whatsapp.net",
			"labelId":  "7",
			"action":   "add",
			"dateTime": "2026-07-04T18:00:00Z",
		}),
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/webhook/payload.go",
		},
	}
}

func labelsEditDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventLabelsEdit),
		Flag:           "labelsEdit",
		Description:    "A label was created, changed or removed.",
		InternalEvents: []string{"*events.LabelEdit"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "LabelsEditWebhookData",
		DynamicFields:  true,
		Fields: []Field{
			field("id", "string", true, false, "Label id, derived from labelId."),
			field("name", "string", false, false, "Label name when reported."),
			field("color", "number", false, false, "Label color when reported."),
			field("deleted", "boolean", false, false, "Whether the label was removed, when reported."),
			field("additionalProperties", "object", false, false, "Flattened fields of the original event."),
		},
		Example: envelope(dbtypes.WebhookEventLabelsEdit, map[string]any{
			"id":      "12",
			"name":    "Cliente",
			"color":   3,
			"deleted": false,
		}),
		Notes: []string{"The normalizer adds neither a `type` nor a `dateTime` field for this event."},
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_extended_events.go",
		},
	}
}

func profilePictureUpdateDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventProfilePictureUpdated),
		Flag:           "profilePictureUpdated",
		Description:    "The profile picture of the instance itself or of another JID was updated.",
		InternalEvents: []string{"*events.Picture"},
		Persistence:    "When the JID is the instance itself, the instance profilePicUrl is updated before delivery. For other JIDs there is no specific persistence.",
		DataType:       "object",
		DataSchema:     "ProfilePictureUpdatedWebhookData",
		Fields: []Field{
			field("jid", "string", true, false, "JID whose picture changed."),
			field("author", "string", false, false, "Author JID when reported; omitted when empty."),
			field("dateTime", "string", true, false, "RFC3339 UTC timestamp of the event or of the processing."),
			field("remove", "boolean", true, false, "Whether the picture was removed."),
			field("pictureId", "string", false, false, "Picture id when reported; omitted when empty."),
			field("isGroup", "boolean", true, false, "Whether the JID belongs to a group."),
		},
		Example: envelope(dbtypes.WebhookEventProfilePictureUpdated, map[string]any{
			"jid":       "120363000000000000@g.us",
			"author":    "5531999999999@s.whatsapp.net",
			"dateTime":  "2026-07-04T18:00:00Z",
			"remove":    false,
			"pictureId": "pic-123",
			"isGroup":   true,
		}),
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/webhook/payload.go",
		},
	}
}

func userAboutUpdateDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventUserAboutUpdated),
		Flag:           "userAboutUpdated",
		Description:    "A user's about text was updated.",
		InternalEvents: []string{"*events.UserAbout"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "UserAboutUpdatedWebhookData",
		Fields: []Field{
			field("jid", "string", true, false, "User JID."),
			field("status", "string", false, false, "About text when reported."),
			field("dateTime", "string", true, false, "RFC3339 timestamp of the processing."),
		},
		Example: envelope(dbtypes.WebhookEventUserAboutUpdated, map[string]any{
			"jid":      "5511999999999@s.whatsapp.net",
			"status":   "Disponivel",
			"dateTime": "2026-07-04T18:00:00Z",
		}),
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/webhook/payload.go",
		},
	}
}

func identityUpdateDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventIdentityUpdated),
		Flag:           "identityUpdated",
		Description:    "A contact's cryptographic identity changed.",
		InternalEvents: []string{"*events.IdentityChange"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "IdentityUpdatedWebhookData",
		Fields: []Field{
			field("jid", "string", true, false, "JID cuja identidade mudou."),
			field("dateTime", "string", true, false, "RFC3339 UTC timestamp of the event or of the processing."),
			field("implicit", "boolean", true, false, "Whether whatsmeow reported the change as implicit."),
		},
		Example: envelope(dbtypes.WebhookEventIdentityUpdated, map[string]any{
			"jid":      "5511999999999@s.whatsapp.net",
			"dateTime": "2026-07-04T18:00:00Z",
			"implicit": true,
		}),
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/webhook/payload.go",
		},
	}
}

func mediaRetryDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventMediaRetry),
		Flag:           "mediaRetry",
		Description:    "Result or error of a media retry attempt.",
		InternalEvents: []string{"*events.MediaRetry"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "MediaRetryWebhookData",
		Fields: []Field{
			field("keyId", "string", true, false, "Message id."),
			field("chatJid", "string", true, false, "Chat JID."),
			field("senderJid", "string", false, false, "Sender JID when available; omitted when absent."),
			field("keyFromMe", "boolean", true, false, "Whether the message belongs to the instance itself."),
			field("hasCiphertext", "boolean", true, false, "Whether the event carried ciphertext."),
			field("errorCode", "number", false, false, "Error code when reported; omitted when absent."),
			field("dateTime", "string", true, false, "RFC3339 UTC timestamp of the event or of the processing."),
		},
		Example: envelope(dbtypes.WebhookEventMediaRetry, map[string]any{
			"keyId":         "ABC123",
			"chatJid":       "5511999999999@s.whatsapp.net",
			"senderJid":     "5511988888888@s.whatsapp.net",
			"keyFromMe":     false,
			"hasCiphertext": true,
			"errorCode":     404,
			"dateTime":      "2026-07-04T18:00:00Z",
		}),
		Notes: []string{"The ciphertext and IV whatsmeow received are not exposed in the webhook."},
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_events.go",
			"internal/webhook/payload.go",
		},
	}
}

func settingsUpdateDoc() EventDoc {
	return EventDoc{
		Name:           string(dbtypes.WebhookEventSettingsUpdated),
		Flag:           "settingsUpdated",
		Description:    "User or instance settings were updated.",
		InternalEvents: []string{"*events.PushNameSetting", "*events.UserStatusMute"},
		Persistence:    "Persists no specific data before delivering the webhook.",
		DataType:       "object",
		DataSchema:     "SettingsUpdatedWebhookData",
		Fields: []Field{
			field("type", "string", true, false, "Settings subtype.", "push.name", "user.status.mute"),
			field("jid", "string", false, false, "Affected JID when the subtype reports one."),
			field("name", "string", false, false, "Name set in the push.name subtype."),
			field("muted", "boolean", false, false, "Mute state in the user.status.mute subtype."),
			field("fromFullSync", "boolean", true, false, "Whether it came from a full synchronization."),
			field("dateTime", "string", true, false, "RFC3339 UTC timestamp of the event or of the processing."),
		},
		PossibleValues: []PossibleEnum{{Field: "type", Values: []string{"push.name", "user.status.mute"}}},
		Example: envelope(dbtypes.WebhookEventSettingsUpdated, map[string]any{
			"type":         "push.name",
			"name":         "My instance",
			"fromFullSync": false,
			"dateTime":     "2026-07-04T18:00:00Z",
		}),
		ImplementedIn: []string{
			"internal/whatsapp/service.go",
			"internal/whatsapp/webhook_events.go",
			"internal/webhook/payload.go",
		},
	}
}

func messageFields() []Field {
	return []Field{
		field("id", "number", true, false, "Internal message id."),
		field("keyId", "string", true, false, "External id or key of the message on WhatsApp."),
		field("keyRemoteJid", "string | null", true, true, "Remote JID of the message."),
		field("keyLid", "string | null", true, true, "Remote LID of the message."),
		field("keyFromMe", "boolean", true, false, "Whether the message was sent by the instance itself."),
		field("keyParticipant", "string | null", true, true, "Participant in group messages."),
		field("keyParticipantLid", "string | null", true, true, "LID of the participant in group messages."),
		field("pushName", "string | null", true, true, "Display name of the sender when known."),
		field("messageType", "string", true, false, "Normalized message type."),
		field("content", "object", true, false, "Normalized message content."),
		field("messageTimestamp", "number", true, false, "Unix timestamp in seconds."),
		field("device", "string | null", true, true, "Device or source inferred for the message."),
		field("isGroup", "boolean", true, false, "Whether the message belongs to a group."),
		field("metadata", "object | null", true, true, "Metadados adicionais normalizados."),
	}
}

func contactUpsertFields() []Field {
	return []Field{
		field("id", "number", true, false, "Internal id of the persisted contact."),
		field("remoteJid", "string", true, false, "Remote JID of the contact."),
		field("lid", "string | null", true, true, "LID of the contact when known."),
		field("pushName", "string | null", true, true, "Push name stored for the contact."),
		field("profilePicUrl", "string | null", true, true, "Profile picture URL when known."),
		field("action", "string", true, false, "Acao executada.", "upserted"),
	}
}

func contactUpdateFields() []Field {
	return []Field{
		field("id", "number", true, false, "Internal id of the persisted contact."),
		field("remoteJid", "string", true, false, "Remote JID of the contact."),
		field("lid", "string | null", true, true, "LID of the contact when known."),
		field("pushName", "string | null", false, true, "Updated push name when present."),
		field("businessName", "string | null", false, true, "Updated business name when present."),
		field("action", "string", true, false, "Acao executada.", "updated"),
		field("source", "string", true, false, "Source of the change.", "pushName", "businessName"),
	}
}

func groupParticipantFields() []Field {
	return []Field{
		field("id", "string", false, false, "Traditional JID of the participant when available; omitted when absent."),
		field("lid", "string", false, false, "LID of the participant when known; omitted when absent."),
		field("isAdmin", "boolean", true, false, "Whether the participant is an admin."),
		field("isSuperAdmin", "boolean", true, false, "Whether the participant is a super admin."),
		field("admin", "string | null", true, true, "Raw value of the admin level when reported."),
	}
}

func groupUpsertFields() []Field {
	return append([]Field{
		field("id", "string", true, false, "Group JID."),
		field("subject", "string", true, false, "Group name."),
		field("participants", "GroupParticipantWebhookData[]", true, false, "Known participants of the group."),
	}, groupPartialFieldSet()...)
}

func groupUpdateFields() []Field {
	return append([]Field{
		field("partial", "GroupPartialWebhookData", true, false, "Partial metadata changed on the group."),
	}, prefixFields("partial.", groupPartialFieldSet())...)
}

func groupPartialFieldSet() []Field {
	return []Field{
		field("notify", "string", false, false, "Notification name of the group when reported."),
		field("addressingMode", "string", false, false, "Addressing mode of the group when reported."),
		field("owner", "string", false, false, "Owner JID when reported."),
		field("ownerPn", "string", false, false, "Phone number of the owner when reported."),
		field("ownerUsername", "string", false, false, "Username of the owner when reported."),
		field("ownerCountryCode", "string", false, false, "Country code of the owner when reported."),
		field("subjectOwner", "string", false, false, "JID of whoever set the subject, when reported."),
		field("subjectOwnerPn", "string", false, false, "Phone number of whoever set the subject, when reported."),
		field("subjectOwnerUsername", "string", false, false, "Username of whoever set the subject, when reported."),
		field("subjectTime", "number", false, false, "Unix timestamp of the subject when reported."),
		field("creation", "number", false, false, "Unix timestamp of the creation when reported."),
		field("desc", "string", false, false, "Group description when reported."),
		field("descOwner", "string", false, false, "JID of whoever set the description, when reported."),
		field("descOwnerPn", "string", false, false, "Phone number of whoever set the description, when reported."),
		field("descOwnerUsername", "string", false, false, "Username of whoever set the description, when reported."),
		field("descId", "string", false, false, "Description id when reported."),
		field("descTime", "number", false, false, "Unix timestamp of the description when reported."),
		field("linkedParent", "string", false, false, "Parent group or community when reported."),
		field("restrict", "boolean", false, false, "Edit restriction when reported."),
		field("announce", "boolean", false, false, "Announcement mode when reported."),
		field("memberAddMode", "boolean", false, false, "Member-add mode when reported."),
		field("joinApprovalMode", "boolean", false, false, "Join-approval mode when reported."),
		field("isCommunity", "boolean", false, false, "Whether it is a community, when reported."),
		field("isCommunityAnnounce", "boolean", false, false, "Whether it is the community announcement group, when reported."),
		field("size", "number", false, false, "Group size when reported."),
		field("ephemeralDuration", "number", false, false, "Disappearing-message duration in seconds when reported."),
		field("inviteCode", "string", false, false, "Invite code when reported."),
		field("author", "string", false, false, "Author of the change when reported."),
		field("authorPn", "string", false, false, "Phone number of the author when reported."),
		field("authorUsername", "string", false, false, "Username of the author when reported."),
	}
}

func prefixFields(prefix string, fields []Field) []Field {
	prefixed := make([]Field, len(fields))
	for i, item := range fields {
		item.Name = prefix + item.Name
		prefixed[i] = item
	}
	return prefixed
}

func supportedEventNames() []string {
	events := dbtypes.SupportedWebhookEvents()
	names := make([]string, 0, len(events))
	for _, event := range events {
		names = append(names, string(event))
	}
	sort.Strings(names)
	return names
}

func field(name, typ string, required, nullable bool, description string, values ...string) Field {
	return Field{
		Name:        name,
		Type:        typ,
		Required:    required,
		Nullable:    nullable,
		Description: description,
		Values:      values,
	}
}

func envelope(event dbtypes.WebhookEvent, data any) map[string]any {
	return envelopeWithInstance(event, map[string]any{
		"id":                 1,
		"name":               "beplus",
		"connectionStatus":   "online",
		"ownerJid":           "5511999999999@s.whatsapp.net",
		"externalAttributes": map[string]any{},
	}, data)
}

func envelopeWithInstance(event dbtypes.WebhookEvent, instance map[string]any, data any) map[string]any {
	return map[string]any{
		"event":     string(event),
		"instance":  instance,
		"data":      data,
		"timestamp": "2026-07-04T18:00:00Z",
	}
}
