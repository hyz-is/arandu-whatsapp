package docs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

func JSON(doc Document) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func Markdown(doc Document) (string, error) {
	var buffer bytes.Buffer

	writeLine(&buffer, "# Webhooks")
	writeLine(&buffer, "")
	writeLine(&buffer, "Technical documentation of the webhooks implemented in the executable code as it stands.")
	writeLine(&buffer, "")

	writeLine(&buffer, "## Contents")
	for _, section := range []string{
		"Overview",
		"Configuration",
		"Event map",
		"HTTP headers",
		"Standard envelope",
		"Instance structure",
		"Delivery and error handling",
		"Events",
		"Unsupported or ignored events",
	} {
		writeLine(&buffer, "- [%s](#%s)", section, anchor(section))
	}
	writeLine(&buffer, "")

	writeLine(&buffer, "## Overview")
	writeLine(&buffer, "")
	writeLine(&buffer, "Webhooks are asynchronous HTTP `POST` requests the application sends to a URL the consumer configures. Every delivery carries a common envelope holding the external event name, the instance the event came from, the event-specific `data` and the `timestamp` the webhook was created at.")
	writeLine(&buffer, "")
	writeLine(&buffer, "There are two possible destinations. With the default `Config.Prefix`, the instance webhook is configured through `PUT /whatsapp/instances/{instance}/webhook`; the dispatcher reads that configuration straight from the database and creates deliveries only for events whose flags are enabled in `events`. The global webhook is configured through `Config.Webhooks.GlobalURL` and `Config.Webhooks.GlobalEnabled`; when enabled it receives every supported event, without applying the instance flags.")
	writeLine(&buffer, "")
	writeLine(&buffer, "For each enabled destination, the module saves an immutable snapshot of the URL, body and headers in `whatsapp_webhook_deliveries` and, in the same transaction, inserts a job carrying only the `deliveryId`. If the instance has no webhook enabled and the global webhook is disabled, the event is dropped without an error.")
	writeLine(&buffer, "")
	writeLine(&buffer, "Deliveries use Hesape's native `DatabaseQueue`. The application registers the handler with `Module.RegisterJobHandlers`, includes `whatsapp.WebhookQueueName` among the queues `jobs.NewModule` watches, and runs `aru queue:work --queue=whatsapp-webhooks --workers=N`. HTTP `2xx` responses are a success; network errors, timeouts and non-`2xx` responses return an error so the job retries with the same `X-Arandu-Delivery-ID`. The relative order between events is not guaranteed.")
	writeLine(&buffer, "")
	writeLine(&buffer, "- Document version: `%s`.", doc.Version)
	writeLine(&buffer, "- Official events documented: `%d`.", len(doc.Events))
	writeLine(&buffer, "- Constants package: `%s`.", doc.GeneratedFrom.ConstantsPackage)
	writeLine(&buffer, "- Dispatcher: `%s`.", doc.GeneratedFrom.Dispatcher)
	writeLine(&buffer, "- Audited whatsmeow version: `%s`.", doc.GeneratedFrom.WhatsmeowVersion)
	writeLine(&buffer, "")
	writeLine(&buffer, "Examples of events with static fields are verified with `hesape/jsonschema` v0.12. For events marked as carrying dynamic fields, the presence and root type of `data` are still verified, but the object is neither closed nor validated field by field: that builder cannot express open objects, and closing these would reject legitimate properties coming from whatsmeow.")
	writeLine(&buffer, "")

	writeLine(&buffer, "## Configuration")
	writeLine(&buffer, "")
	writeLine(&buffer, "### Typed configuration")
	writeLine(&buffer, "")
	writeLine(&buffer, "The module reads no environment variables. The host application provides:")
	writeLine(&buffer, "")
	writeLine(&buffer, "| Field | Type | Default | Description |")
	writeLine(&buffer, "| --- | --- | --- | --- |")
	writeLine(&buffer, "| `Config.Webhooks.GlobalURL` | URL | empty | HTTP or HTTPS URL of the global webhook. |")
	writeLine(&buffer, "| `Config.Webhooks.GlobalEnabled` | boolean | `false` | Enables the global webhook; requires `GlobalURL`. |")
	writeLine(&buffer, "| `Config.Webhooks.SigningSecret` | string | empty | HMAC secret of at least 32 bytes; required before enabling any webhook. |")
	writeLine(&buffer, "| `Config.Webhooks.Retention` | duration | `720h` (30 days) | Longest a snapshot is retained, failures included; zero uses the default. |")
	writeLine(&buffer, "| `Config.Webhooks.Workers` | integer | ignored | Obsolete; set `--workers=N` on `aru queue:work` instead. |")
	writeLine(&buffer, "| `Config.Webhooks.QueueSize` | integer | ignored | Obsolete; the native queue is durable in the database. |")
	writeLine(&buffer, "")
	writeLine(&buffer, "The host application also has to apply the `DatabaseQueue` migrations, register the module's handler on the worker and consume the `whatsapp-webhooks` queue.")
	writeLine(&buffer, "")
	writeLine(&buffer, "### Instance webhook")
	writeLine(&buffer, "")
	writeLine(&buffer, "To configure or update it:")
	writeLine(&buffer, "")
	writeLine(&buffer, "```http")
	writeLine(&buffer, "PUT /whatsapp/instances/beplus/webhook HTTP/1.1")
	writeLine(&buffer, "Cookie: arandu_session=<host-session>")
	writeLine(&buffer, "Content-Type: application/json")
	writeLine(&buffer, "```")
	writeLine(&buffer, "")
	writeLine(&buffer, "To read it:")
	writeLine(&buffer, "")
	writeLine(&buffer, "```http")
	writeLine(&buffer, "GET /whatsapp/instances/beplus/webhook HTTP/1.1")
	writeLine(&buffer, "Cookie: arandu_session=<host-session>")
	writeLine(&buffer, "```")
	writeLine(&buffer, "")
	writeLine(&buffer, "Both routes authenticate through the Arandu session and require Grants issued by the policy for `ActionWebhookSet` and `ActionWebhookView` respectively. Configuration responses use the Arandu envelope:")
	if err := writeJSONBlock(&buffer, map[string]any{"data": instanceConfigExample(doc)}); err != nil {
		return "", err
	}
	writeLine(&buffer, "")
	writeLine(&buffer, "`url` has to use `http` or `https` and be at most 500 characters long. An absent `enabled` is taken as `true` on creation and update. When `events` is omitted the existing flags are preserved; when `events` is `{}` the flags are removed. Unknown fields in `events` are rejected.")
	writeLine(&buffer, "")

	writeLine(&buffer, "## Event map")
	writeLine(&buffer, "")
	writeLine(&buffer, "| Flag | External event | Description |")
	writeLine(&buffer, "| --- | --- | --- |")
	for _, event := range doc.Events {
		writeLine(&buffer, "| `%s` | `%s` | %s |", event.Flag, event.Name, escapeTable(event.Description))
	}
	writeLine(&buffer, "")

	writeLine(&buffer, "## HTTP headers")
	writeLine(&buffer, "")
	writeLine(&buffer, "| Header | Example | Description |")
	writeLine(&buffer, "| --- | --- | --- |")
	for _, header := range doc.Headers {
		writeLine(&buffer, "| `%s` | `%s` | %s |", header.Name, escapeCode(header.Value), escapeTable(header.Description))
	}
	writeLine(&buffer, "")
	writeLine(&buffer, "An example of the request the consumer receives:")
	writeLine(&buffer, "")
	writeLine(&buffer, "```http")
	writeLine(&buffer, "POST /webhooks/beplus HTTP/1.1")
	writeLine(&buffer, "Host: example.com")
	writeLine(&buffer, "Content-Type: application/json")
	writeLine(&buffer, "User-Agent: Arandu-WhatsApp/1.0")
	writeLine(&buffer, "x-request-id: 019f0000-0000-7000-8000-000000000000")
	writeLine(&buffer, "x-owner-jid: 5531999999999@s.whatsapp.net")
	writeLine(&buffer, "x-instance-name: beplus")
	writeLine(&buffer, "x-instance-id: 1")
	writeLine(&buffer, "x-webhook-event: messages.upsert")
	writeLine(&buffer, "X-Arandu-Delivery-ID: 019f0000-0000-7000-8000-000000000001")
	writeLine(&buffer, "X-Arandu-Timestamp: 1785876000")
	writeLine(&buffer, "X-Arandu-Signature: sha256=<hex>")
	writeLine(&buffer, "```")
	writeLine(&buffer, "")
	writeLine(&buffer, "`x-owner-jid` can be an empty string when the instance is not connected yet, or when the owner is not stored in the snapshot the dispatcher used.")
	writeLine(&buffer, "")

	writeLine(&buffer, "## Standard envelope")
	writeLine(&buffer, "")
	if err := writeJSONBlock(&buffer, map[string]any{
		"event":     "event.name",
		"instance":  sampleInstance(false),
		"data":      map[string]any{},
		"timestamp": "2026-07-04T18:00:00Z",
	}); err != nil {
		return "", err
	}
	writeLine(&buffer, "")
	writeLine(&buffer, "`event` is the external event name. `instance` holds the minimal snapshot of the instance responsible for the event. `data` holds the event-specific payload and can be an object or an array. `timestamp` is generated when the envelope is built, in RFC3339 UTC.")
	writeLine(&buffer, "")

	writeLine(&buffer, "## Instance structure")
	writeLine(&buffer, "")
	if err := writeJSONBlock(&buffer, map[string]any{
		"id":                 1,
		"name":               "beplus",
		"connectionStatus":   "online",
		"ownerJid":           "5531999999999@s.whatsapp.net",
		"externalAttributes": map[string]any{},
	}); err != nil {
		return "", err
	}
	writeLine(&buffer, "")
	writeLine(&buffer, "`id` is the internal numeric identifier of the instance. `name` is the name used in the routes. `connectionStatus` carries the persisted connection values, such as `offline`, `connecting`, `qr_code`, `pairing_code`, `pairing`, `online`, `reconnecting`, `disconnected`, `connection_timeout`, `logged_out`, `session_missing`, `stream_replaced`, `keepalive_timeout`, `client_outdated`, `temporary_ban` and `connection_error`. `ownerJid` is a `string` or `null` in the body; in the `x-owner-jid` header, a null value becomes an empty string. `externalAttributes` is always a JSON object; absent, `null` or invalid values are serialized as `{}`.")
	writeLine(&buffer, "")

	writeLine(&buffer, "## Delivery and error handling")
	writeLine(&buffer, "")
	if err := writeJSONBlock(&buffer, map[string]any{
		"delivery": map[string]any{
			"method":            doc.Delivery.Method,
			"contentType":       "application/json",
			"successStatus":     "200-299",
			"timeoutSeconds":    15,
			"queue":             doc.Delivery.Queue,
			"job":               doc.Delivery.Job,
			"maxTries":          doc.Delivery.MaxTries,
			"backoff":           doc.Delivery.Backoff,
			"idempotencyHeader": doc.Delivery.IdempotencyHeader,
			"allowedSchemes":    doc.Delivery.AllowedWebhookSchemes,
			"ordering":          "not_guaranteed",
		},
	}); err != nil {
		return "", err
	}
	writeLine(&buffer, "")
	for _, item := range doc.ErrorHandling {
		writeLine(&buffer, "- %s", item)
	}
	for _, item := range doc.Ordering {
		writeLine(&buffer, "- %s", item)
	}
	for _, item := range doc.Security {
		writeLine(&buffer, "- %s", item)
	}
	writeLine(&buffer, "- Creating the snapshot and inserting the job share a `data.Transaction`: either both commit or both roll back.")
	writeLine(&buffer, "- A completed delivery keeps only the tombstone idempotency needs; the URL, body, headers and response are erased immediately.")
	writeLine(&buffer, "- Snapshots in any state expire after `Config.Webhooks.Retention`; orphaned jobs end as a no-op, so a manual redrive only exists inside that window.")
	writeLine(&buffer, "- `Start` and `Close` neither create nor stop webhook workers; the queue's lifecycle belongs to the host application.")
	writeLine(&buffer, "")

	writeLine(&buffer, "## Events")
	writeLine(&buffer, "")
	for _, event := range doc.Events {
		if err := writeEvent(&buffer, event); err != nil {
			return "", err
		}
	}

	writeLine(&buffer, "## Unsupported or ignored events")
	writeLine(&buffer, "")
	writeLine(&buffer, "| Internal event | Status | Reason |")
	writeLine(&buffer, "| --- | --- | --- |")
	for _, event := range doc.IgnoredEvents {
		writeLine(&buffer, "| `%s` | `%s` | %s |", event.Name, event.Status, escapeTable(event.Description))
	}
	writeLine(&buffer, "")

	return strings.TrimRight(buffer.String(), "\n") + "\n", nil
}

func writeEvent(buffer *bytes.Buffer, event EventDoc) error {
	writeLine(buffer, "### `%s`", event.Name)
	writeLine(buffer, "")
	writeLine(buffer, "%s", event.Description)
	writeLine(buffer, "")
	writeLine(buffer, "**Flag:** `%s`", event.Flag)
	writeLine(buffer, "")
	writeLine(buffer, "**Internal events:** `%s`", strings.Join(event.InternalEvents, "`, `"))
	writeLine(buffer, "")
	writeLine(buffer, "**Persistence:** %s", event.Persistence)
	if event.RequiresPersistenceFlag != "" {
		writeLine(buffer, "")
		writeLine(buffer, "**Persistence flag:** `%s`", event.RequiresPersistenceFlag)
	}
	writeLine(buffer, "")
	writeLine(buffer, "**Type of `data`:** `%s`", event.DataType)
	writeLine(buffer, "")
	writeLine(buffer, "**DTO/normalizer:** `%s`", event.DataSchema)
	writeLine(buffer, "")
	writeLine(buffer, "**Dynamic fields:** %s", yesNo(event.DynamicFields))
	writeLine(buffer, "")
	writeLine(buffer, "**Implemented in:** `%s`", strings.Join(event.ImplementedIn, "`, `"))
	writeLine(buffer, "")

	writeLine(buffer, "#### Request")
	writeLine(buffer, "")
	writeLine(buffer, "```http")
	writeLine(buffer, "POST /webhooks/beplus HTTP/1.1")
	writeLine(buffer, "Content-Type: application/json")
	writeLine(buffer, "x-webhook-event: %s", event.Name)
	writeLine(buffer, "```")
	writeLine(buffer, "")

	writeLine(buffer, "#### Body")
	if err := writeJSONBlock(buffer, event.Example); err != nil {
		return fmt.Errorf("write example for %s: %w", event.Name, err)
	}
	writeLine(buffer, "")

	writeLine(buffer, "#### Fields of `data`")
	writeLine(buffer, "")
	writeFieldBullets(buffer, event.Fields)
	writeLine(buffer, "")

	if len(event.PossibleValues) > 0 {
		writeLine(buffer, "#### Possible values")
		writeLine(buffer, "")
		for _, possible := range event.PossibleValues {
			writeLine(buffer, "- `%s`: `%s`", possible.Field, strings.Join(possible.Values, "`, `"))
		}
		writeLine(buffer, "")
	}

	writeLine(buffer, "#### Notes")
	writeLine(buffer, "")
	if len(event.Notes) == 0 {
		writeLine(buffer, "- No additional notes.")
	} else {
		for _, note := range event.Notes {
			writeLine(buffer, "- %s", note)
		}
	}
	writeLine(buffer, "")
	return nil
}

func writeFieldBullets(buffer *bytes.Buffer, fields []Field) {
	for _, item := range fields {
		requirement := "optional"
		if item.Required {
			requirement = "required"
		}
		nullability := "does not accept `null`"
		if item.Nullable {
			nullability = "accepts `null`"
		}
		line := fmt.Sprintf("- `%s`: `%s`, %s, %s. %s", item.Name, item.Type, requirement, nullability, item.Description)
		if len(item.Values) > 0 {
			line += fmt.Sprintf(" Possible values: `%s`.", strings.Join(item.Values, "`, `"))
		}
		writeLine(buffer, "%s", line)
	}
}

func writeJSONBlock(buffer *bytes.Buffer, value any) error {
	example, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	writeLine(buffer, "")
	writeLine(buffer, "```json")
	writeLine(buffer, "%s", string(example))
	writeLine(buffer, "```")
	return nil
}

func instanceConfigExample(doc Document) map[string]any {
	events := make(map[string]bool, len(doc.Events))
	for _, event := range doc.Events {
		events[event.Flag] = true
	}
	return map[string]any{
		"url":     "https://example.com/webhooks/beplus",
		"enabled": true,
		"events":  events,
	}
}

func sampleInstance(ownerNull bool) map[string]any {
	var owner any = "5531999999999@s.whatsapp.net"
	if ownerNull {
		owner = nil
	}
	return map[string]any{
		"id":                 1,
		"name":               "beplus",
		"connectionStatus":   "online",
		"ownerJid":           owner,
		"externalAttributes": map[string]any{},
	}
}

func writeLine(buffer *bytes.Buffer, format string, args ...any) {
	if len(args) == 0 {
		buffer.WriteString(format)
	} else {
		buffer.WriteString(fmt.Sprintf(format, args...))
	}
	buffer.WriteByte('\n')
}

func anchor(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, " ", "-")
	return value
}

func escapeTable(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "|", "\\|")
}

func escapeCode(value string) string {
	return strings.ReplaceAll(value, "`", "\\`")
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func ValidateDocument(doc Document) error {
	if !sort.SliceIsSorted(doc.Events, func(i, j int) bool {
		return doc.Events[i].Name < doc.Events[j].Name
	}) {
		return fmt.Errorf("events must be sorted alphabetically by name")
	}

	names := map[string]struct{}{}
	flags := map[string]struct{}{}
	for _, event := range doc.Events {
		if event.Name == "" {
			return fmt.Errorf("event with empty name")
		}
		if event.Flag == "" {
			return fmt.Errorf("event %s has empty flag", event.Name)
		}
		if len(event.Fields) == 0 {
			return fmt.Errorf("event %s has no fields", event.Name)
		}
		if _, ok := names[event.Name]; ok {
			return fmt.Errorf("duplicate event name %s", event.Name)
		}
		names[event.Name] = struct{}{}
		if _, ok := flags[event.Flag]; ok {
			return fmt.Errorf("duplicate event flag %s", event.Flag)
		}
		flags[event.Flag] = struct{}{}
		exampleEvent, ok := event.Example["event"].(string)
		if !ok || exampleEvent != event.Name {
			return fmt.Errorf("event %s example has event=%v", event.Name, event.Example["event"])
		}
		if _, ok := event.Example["timestamp"].(string); !ok {
			return fmt.Errorf("event %s example has no timestamp", event.Name)
		}
		if err := validateStaticEventExample(event); err != nil {
			return err
		}
		instance, ok := event.Example["instance"].(map[string]any)
		if !ok {
			return fmt.Errorf("event %s example has invalid instance", event.Name)
		}
		if id, ok := instance["id"].(int); !ok || id <= 0 {
			return fmt.Errorf("event %s example has invalid numeric instance.id", event.Name)
		}
		external, ok := instance["externalAttributes"]
		if !ok || external == nil || reflect.TypeOf(external).Kind() != reflect.Map {
			return fmt.Errorf("event %s example has invalid externalAttributes", event.Name)
		}
		if _, err := json.Marshal(event.Example); err != nil {
			return fmt.Errorf("event %s example is not JSON serializable: %w", event.Name, err)
		}
	}
	return nil
}
