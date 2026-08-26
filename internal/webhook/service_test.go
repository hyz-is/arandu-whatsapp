package webhook

import (
	"context"
	"errors"
	"testing"

	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

func TestNormalizeWebhookURL(t *testing.T) {
	for _, value := range []string{
		"https://example.com/webhook",
		"http://internal.local/hook",
	} {
		if _, err := normalizeWebhookURL(value); err != nil {
			t.Fatalf("expected valid URL %q, got %v", value, err)
		}
	}

	for _, value := range []string{
		"",
		"not-a-url",
		"ftp://example.com/webhook",
		"https:///missing-host",
	} {
		if _, err := normalizeWebhookURL(value); !errors.Is(err, repository.ErrInvalidWebhookURL) {
			t.Fatalf("expected invalid URL for %q, got %v", value, err)
		}
	}
}

func TestValidateEventsRejectsUnknownEventAndPreservesFalse(t *testing.T) {
	if err := validateEvents(map[string]bool{
		"messagesUpsert":            false,
		"connectionUpdated":         true,
		"historySync":               true,
		"contactsUpdated":           true,
		"groupsUpdated":             true,
		"callUpsert":                true,
		"labelsAssociation":         true,
		"labelsEdit":                true,
		"groupsParticipantsUpdated": true,
		"groupsUpsert":              true,
		"newsLetter":                true,
		"messagesDeleted":           true,
		"profilePictureUpdated":     true,
		"settingsUpdated":           true,
	}); err != nil {
		t.Fatalf("expected valid events, got %v", err)
	}

	err := validateEvents(map[string]bool{"unknownEvent": false})
	if !errors.Is(err, repository.ErrInvalidWebhookEvent) {
		t.Fatalf("expected ErrInvalidWebhookEvent, got %v", err)
	}
}

// failingEventsRepository is a real webhook repository whose event upsert
// fails. It is what stands in for the failure the transaction exists to undo:
// the row is already written when the events cannot be.
type failingEventsRepository struct {
	repository.WebhookRepository
	err error
}

func (r failingEventsRepository) UpsertEvents(context.Context, security.Grant, int64, map[string]bool) (types.Webhook, error) {
	return types.Webhook{}, r.err
}

func TestSetStoresTheWebhookAndItsEventsTogether(t *testing.T) {
	testDB := newWebhookTestDB(t, false)
	testDB.insertInstance(t)

	base := repository.NewBase(testDB.db)
	failure := errors.New("events unavailable")
	service := NewService(
		testDB.db,
		repository.NewInstanceRepository(base),
		failingEventsRepository{WebhookRepository: repository.NewWebhookRepository(base), err: failure},
		nil, 0,
		true,
	)

	enabled := true
	_, err := service.Set(context.Background(), testWebhookSetGrant(), "beplus", SetInput{
		URL:       "https://example.com/hook",
		Enabled:   &enabled,
		Events:    map[string]bool{"connectionUpdated": true},
		EventsSet: true,
	})
	if !errors.Is(err, failure) {
		t.Fatalf("Set() error = %v, want %v", err, failure)
	}

	if count := testDB.count(t, "whatsapp_webhooks"); count != 0 {
		t.Fatalf("whatsapp_webhooks rows = %d, want 0: the row outlived the events it was created with", count)
	}
}

func TestSetLeavesAnExistingWebhookUntouchedWhenItsEventsFail(t *testing.T) {
	testDB := newWebhookTestDB(t, false)
	testDB.insertInstance(t)
	testDB.insertWebhook(t, "https://original.example/hook", true, types.WebhookEvents{ConnectionUpdated: true})

	base := repository.NewBase(testDB.db)
	failure := errors.New("events unavailable")
	service := NewService(
		testDB.db,
		repository.NewInstanceRepository(base),
		failingEventsRepository{WebhookRepository: repository.NewWebhookRepository(base), err: failure},
		nil, 0,
		true,
	)

	enabled := true
	if _, err := service.Set(context.Background(), testWebhookSetGrant(), "beplus", SetInput{
		URL:       "https://replacement.example/hook",
		Enabled:   &enabled,
		Events:    map[string]bool{"messagesUpsert": true},
		EventsSet: true,
	}); !errors.Is(err, failure) {
		t.Fatalf("Set() error = %v, want %v", err, failure)
	}

	var storedURL string
	if err := testDB.raw.QueryRow(`SELECT url FROM whatsapp_webhooks WHERE id = ?`, 10).Scan(&storedURL); err != nil {
		t.Fatal(err)
	}
	if storedURL != "https://original.example/hook" {
		t.Fatalf("stored url = %q, want the original: the update outlived the events it was made for", storedURL)
	}
}

// testWebhookSetGrant is what the HTTP surface hands the service: the action
// the route is authorized for, not the runtime action the dispatcher carries.
func testWebhookSetGrant() security.Grant {
	return security.SystemGrant(authz.ActionWebhookSet, "acme")
}
