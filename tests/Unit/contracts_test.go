package unit_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	whatsapp "github.com/hyz-is/arandu-whatsapp"
)

func TestPublicRequestContractsAreConstructible(t *testing.T) {
	t.Parallel()

	number := "5511999999999"
	message := whatsapp.SendTextRequest{
		Number:      &number,
		TextMessage: &whatsapp.TextMessage{Text: "hello"},
		Options:     &whatsapp.MessageOptions{},
	}
	webhook := whatsapp.WebhookSetInput{
		URL:       "https://example.test/webhooks/whatsapp",
		Events:    map[string]bool{"messages.upsert": true},
		EventsSet: true,
	}
	passkey := whatsapp.SubmitPasskeyAssertionRequest{
		RequestID: "018f5e15-8a3b-7d1c-9a84-9f66c79f2692",
	}
	pairing := whatsapp.PhonePairingInput{PhoneNumber: number}
	list := whatsapp.InstanceListQuery{Limit: 50}
	validation := whatsapp.ValidationError{Messages: []string{"invalid"}}

	if message.TextMessage.Text == "" || webhook.URL == "" || passkey.RequestID == "" || pairing.PhoneNumber == "" || list.Limit != 50 || validation.Error() == "" {
		t.Fatal("public request contracts lost their values")
	}
}

func TestPublicErrorsSupportErrorsIsFromExternalConsumers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repository := whatsapp.NewInstanceRepository(data.Wrap(nil, data.DialectSQLite))
	listGrant := security.SystemGrant(whatsapp.ActionInstanceList, "acme")
	if _, err := repository.ListPage(ctx, listGrant, whatsapp.InstanceListQuery{Limit: 1, Cursor: "invalid"}); !errors.Is(err, whatsapp.ErrInvalidCursor) {
		t.Fatalf("invalid cursor error = %v", err)
	}
	if _, err := repository.List(ctx, listGrant, data.Query{Limit: 1, Sort: "name"}); !errors.Is(err, whatsapp.ErrInvalidInput) {
		t.Fatalf("unsupported sort error = %v", err)
	}
	if _, err := repository.Find(ctx, security.Grant{}, 1); !errors.Is(err, whatsapp.ErrForbidden) {
		t.Fatalf("invalid Grant error = %v", err)
	}

	sessions := security.NewSessionStore([]byte("0123456789abcdef0123456789abcdef"), time.Hour, false, security.NewMemoryBackend())
	module, err := whatsapp.New(whatsapp.Config{
		Tenant: "acme",
		Policy: whatsapp.PolicyConfig{Roles: map[security.Action][]string{
			whatsapp.ActionInstanceCreate: {"admin"},
			whatsapp.ActionConnectionPair: {"admin"},
		}},
	}, data.Wrap(nil, data.DialectSQLite), sessions)
	if err != nil {
		t.Fatal(err)
	}
	actor := security.Subject{ID: "user-1", Tenant: "acme", Roles: []string{"admin"}, Verified: true}
	name := strings.Repeat("x", 256)
	if _, err := module.Service().CreateInstance(ctx, actor, whatsapp.CreateInstanceInput{Name: &name}); !errors.Is(err, whatsapp.ErrInvalidInput) {
		t.Fatalf("invalid instance error = %v", err)
	}
	if _, err := module.Service().ConnectPhone(ctx, actor, "demo", whatsapp.PhonePairingInput{PhoneNumber: "short"}); !errors.Is(err, whatsapp.ErrInvalidPhoneNumber) {
		t.Fatalf("invalid phone error = %v", err)
	}

	for name, sentinel := range map[string]error{
		"instance not found": whatsapp.ErrInstanceNotFound,
		"webhook conflict":   whatsapp.ErrWebhookAlreadyExists,
		"message send":       whatsapp.ErrMessageSendFailed,
		"chat media":         whatsapp.ErrChatMediaDownloadFailed,
		"group operation":    whatsapp.ErrGroupRemoteOperation,
		"passkey":            whatsapp.ErrInvalidPasskeyAssertion,
	} {
		if sentinel == nil || !errors.Is(sentinel, sentinel) {
			t.Errorf("public sentinel %q is unusable", name)
		}
	}
}
