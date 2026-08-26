package unit_test

import (
	"encoding/json"
	"testing"
	"time"

	repositories "github.com/hyz-is/arandu-whatsapp/app/Repositories"
	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

func TestPersistenceAdaptersPreservePublicJSON(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 25, 12, 30, 0, 0, time.UTC)
	remote := "5511999999999@s.whatsapp.net"
	group := true

	webhook := dbtypes.Webhook{
		ID: 7, URL: "https://example.test/webhooks", Enabled: true,
		Events: json.RawMessage(`{"messagesUpsert":true}`), CreatedAt: now,
		UpdatedAt: now.Add(time.Minute), InstanceID: 11,
	}
	assertSameJSON(t, webhook, repositories.WebhookFromDatabase(webhook))

	message := dbtypes.Message{
		ID: 17, KeyID: "message-key", KeyRemoteJid: &remote, KeyFromMe: true,
		MessageType: "conversation", Content: json.RawMessage(`{"text":"hello"}`),
		MessageTimestamp: 123, Device: dbtypes.DeviceMessageWeb, IsGroup: &group,
		InstanceID: 11, Metadata: json.RawMessage(`{"source":"test"}`),
		ExternalAttributes: map[string]any{"trace": "abc"},
	}
	assertSameJSON(t, message, repositories.MessageFromDatabase(message))

	page := dbtypes.MessageListResult{Messages: dbtypes.MessagePage{
		Total: 1, Pages: 1, CurrentPage: 1,
		Records: []dbtypes.MessageWithUpdates{{
			Message: message,
			MessageUpdate: []dbtypes.MessageUpdateSummary{{
				Status: "DELIVERY_ACK", DateTime: now,
			}},
		}},
	}}
	assertSameJSON(t, page, repositories.MessageListFromDatabase(page))
	assertSameJSON(t, dbtypes.MessageListResult{}, repositories.MessageListFromDatabase(dbtypes.MessageListResult{}))
}

func assertSameJSON(t *testing.T, want, got any) {
	t.Helper()
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("converted JSON changed\nwant: %s\n got: %s", wantJSON, gotJSON)
	}
}
