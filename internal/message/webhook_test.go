package message

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
)

func TestDispatchSendMessageWebhookUsesPersistedMessage(t *testing.T) {
	webhooks := &fakeMessageWebhooks{}
	service := &MessageService{webhooks: webhooks}
	owner := "5531999999999@s.whatsapp.net"
	remote := "5531988888888@s.whatsapp.net"
	message := types.Message{
		ID:               77,
		KeyID:            "msg-1",
		KeyRemoteJid:     &remote,
		KeyFromMe:        true,
		MessageType:      "extendedTextMessage",
		Content:          json.RawMessage(`{"text":"hello"}`),
		MessageTimestamp: 1783170000,
		Device:           types.DeviceMessageWeb,
		InstanceID:       42,
	}
	instance := types.Instance{
		ID:                 42,
		Name:               "beplus",
		ConnectionStatus:   types.InstanceConnectionStatusOnline,
		WhatsAppOwnerJid:   &owner,
		ExternalAttributes: json.RawMessage(`{"tenantId":"019f"}`),
	}

	service.dispatchSendMessageWebhook(context.Background(), security.SystemGrant(authz.ActionMessageSend, "acme"), instance, message)

	if webhooks.calls != 1 || webhooks.event != types.WebhookEventSendMessage {
		t.Fatalf("expected send.message dispatch, got calls=%d event=%s", webhooks.calls, webhooks.event)
	}
	data, ok := webhooks.data.(webhooksvc.MessageUpsertWebhookData)
	if !ok {
		t.Fatalf("unexpected webhook data type %T", webhooks.data)
	}
	if data.ID != 77 || data.Content["text"] != "hello" {
		t.Fatalf("unexpected message data: %#v", data)
	}
	if webhooks.instance.ExternalAttributes["tenantId"] != "019f" {
		t.Fatalf("externalAttributes not preserved: %#v", webhooks.instance.ExternalAttributes)
	}
}

type fakeMessageWebhooks struct {
	calls    int
	instance webhooksvc.WebhookInstance
	event    types.WebhookEvent
	data     any
	err      error
}

func (f *fakeMessageWebhooks) Dispatch(_ context.Context, _ security.Grant, instance webhooksvc.WebhookInstance, event types.WebhookEvent, data any) error {
	f.calls++
	f.instance = instance
	f.event = event
	f.data = data
	return f.err
}
func (f *fakeMessageWebhooks) Shutdown(context.Context) error {
	return nil
}
