package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
	"github.com/hyz-is/arandu-whatsapp/internal/whatsapp/address"
)

func TestEverySQLDoorRejectsInvalidGrantBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()

	base := NewBase(data.Wrap(nil, data.DialectSQLite))
	instances := NewInstanceRepository(base)
	messages := NewMessageRepository(base)
	messageUpdates := NewMessageUpdateRepository(base)
	contacts := NewContactRepository(base)
	webhooks := NewWebhookRepository(base)
	addresses := NewAddressMappingRepository(base)
	ctx := context.Background()

	type sqlDoor struct {
		name          string
		allowedAction security.Action
		call          func(security.Grant) error
	}
	doors := []sqlDoor{
		{name: "instances/create", allowedAction: authz.ActionInstanceCreate, call: func(g security.Grant) error {
			_, err := instances.Create(ctx, g, types.CreateInstanceInput{})
			return err
		}},
		{name: "instances/find-by-name", allowedAction: authz.ActionInstanceView, call: func(g security.Grant) error { _, err := instances.FindByName(ctx, g, "instance"); return err }},
		{name: "instances/find-by-id", allowedAction: authz.ActionInstanceView, call: func(g security.Grant) error { _, err := instances.FindByID(ctx, g, 1); return err }},
		{name: "instances/list-page", allowedAction: authz.ActionInstanceList, call: func(g security.Grant) error {
			_, err := instances.ListPage(ctx, g, data.Query{Limit: 1}, nil)
			return err
		}},
		{name: "instances/fetch-details", allowedAction: authz.ActionInstanceView, call: func(g security.Grant) error { _, err := instances.FetchDetailsByName(ctx, g, "instance"); return err }},
		{name: "instances/find-auto-connect", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error { _, err := instances.FindAutoConnectInstances(ctx, g); return err }},
		{name: "instances/update", allowedAction: authz.ActionInstanceUpdate, call: func(g security.Grant) error {
			_, err := instances.Update(ctx, g, 1, types.UpdateInstanceInput{})
			return err
		}},
		{name: "instances/update-status", allowedAction: authz.ActionConnectionPair, call: func(g security.Grant) error { return instances.UpdateStatus(ctx, g, 1, types.InstanceStatusOnline) }},
		{name: "instances/update-connection-state", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error {
			return instances.UpdateConnectionState(ctx, g, types.UpdateConnectionStateInput{})
		}},
		{name: "instances/save-device", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error {
			return instances.SaveWhatsAppDevice(ctx, g, types.SaveWhatsAppDeviceInput{})
		}},
		{name: "instances/clear-device", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error { return instances.ClearWhatsAppDevice(ctx, g, 1) }},
		{name: "instances/update-profile-picture", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error { return instances.UpdateProfilePicture(ctx, g, 1, nil, nil) }},
		{name: "instances/acquire-lock", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error { _, err := instances.TryAcquireConnectionLock(ctx, g, "1"); return err }},
		{name: "instances/release-lock", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error { return instances.ReleaseConnectionLock(ctx, g, "1") }},
		{name: "instances/ensure-deletable", allowedAction: authz.ActionInstanceDelete, call: func(g security.Grant) error { return instances.EnsureDeletable(ctx, g, 1) }},
		{name: "instances/delete", allowedAction: authz.ActionInstanceDelete, call: func(g security.Grant) error { return instances.Delete(ctx, g, 1, false) }},

		{name: "messages/create", allowedAction: authz.ActionMessageSend, call: func(g security.Grant) error {
			_, err := messages.Create(ctx, g, types.CreateMessageInput{})
			return err
		}},
		{name: "messages/create-or-ignore", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error { return messages.CreateOrIgnore(ctx, g, types.CreateMessageInput{}) }},
		{name: "messages/find-by-id", allowedAction: authz.ActionMessageSend, call: func(g security.Grant) error { _, err := messages.FindByIDForInstance(ctx, g, 1, 1); return err }},
		{name: "messages/find-by-key", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error { _, err := messages.FindByKeyIDForInstance(ctx, g, 1, "key"); return err }},
		{name: "messages/find-by-ids", allowedAction: authz.ActionMessageRead, call: func(g security.Grant) error {
			_, err := messages.FindByIDsForInstance(ctx, g, 1, []int64{1})
			return err
		}},
		{name: "messages/find-outgoing-by-id", allowedAction: authz.ActionMessageEdit, call: func(g security.Grant) error { _, err := messages.FindOutgoingByIDForInstance(ctx, g, 1, 1); return err }},
		{name: "messages/find-outgoing-by-key", allowedAction: authz.ActionMessageEdit, call: func(g security.Grant) error {
			_, err := messages.FindOutgoingByKeyIDForInstance(ctx, g, 1, "key")
			return err
		}},
		{name: "messages/mark-read", allowedAction: authz.ActionMessageRead, call: func(g security.Grant) error { return messages.MarkReadForInstance(ctx, g, 1, []int64{1}) }},
		{name: "messages/update-content", allowedAction: authz.ActionMessageEdit, call: func(g security.Grant) error {
			_, err := messages.UpdateContentForInstance(ctx, g, 1, 1, nil)
			return err
		}},
		{name: "messages/count", allowedAction: authz.ActionMessageList, call: func(g security.Grant) error { _, err := messages.Count(ctx, g, 1, types.MessageFilters{}); return err }},
		{name: "messages/list", allowedAction: authz.ActionMessageList, call: func(g security.Grant) error {
			_, err := messages.List(ctx, g, 1, types.ListMessagesInput{})
			return err
		}},
		{name: "messages/list-page", allowedAction: authz.ActionMessageList, call: func(g security.Grant) error {
			_, err := messages.ListPage(ctx, g, 1, types.ListMessagesPageInput{})
			return err
		}},

		{name: "message-updates/create", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error {
			_, err := messageUpdates.Create(ctx, g, types.CreateMessageUpdateInput{})
			return err
		}},
		{name: "message-updates/create-or-ignore", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error {
			return messageUpdates.CreateOrIgnore(ctx, g, types.CreateMessageUpdateInput{})
		}},
		{name: "message-updates/list", allowedAction: authz.ActionMessageList, call: func(g security.Grant) error { _, err := messageUpdates.ListByMessageID(ctx, g, 1); return err }},

		{name: "contacts/create", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error {
			_, err := contacts.Create(ctx, g, types.CreateContactInput{})
			return err
		}},
		{name: "contacts/upsert", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error {
			_, err := contacts.Upsert(ctx, g, types.CreateContactInput{})
			return err
		}},
		{name: "contacts/list", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error { _, err := contacts.List(ctx, g, 1, types.ContactFilters{}); return err }},

		{name: "webhooks/create", allowedAction: authz.ActionWebhookSet, call: func(g security.Grant) error {
			_, err := webhooks.Create(ctx, g, types.CreateWebhookInput{})
			return err
		}},
		{name: "webhooks/find", allowedAction: authz.ActionWebhookView, call: func(g security.Grant) error { _, err := webhooks.FindByInstanceName(ctx, g, "instance"); return err }},
		{name: "webhooks/list-enabled", allowedAction: authz.ActionRuntime, call: func(g security.Grant) error { _, err := webhooks.ListEnabledWithInstance(ctx, g); return err }},
		{name: "webhooks/update", allowedAction: authz.ActionWebhookSet, call: func(g security.Grant) error {
			_, err := webhooks.Update(ctx, g, 1, types.UpdateWebhookInput{})
			return err
		}},
		{name: "webhooks/upsert-events", allowedAction: authz.ActionWebhookSet, call: func(g security.Grant) error { _, err := webhooks.UpsertEvents(ctx, g, 1, nil); return err }},

		{name: "addresses/find", allowedAction: authz.ActionMessageSend, call: func(g security.Grant) error { _, err := addresses.FindByAlias(ctx, g, 1, "alias"); return err }},
		{name: "addresses/upsert", allowedAction: authz.ActionMessageSend, call: func(g security.Grant) error { return addresses.Upsert(ctx, g, address.AddressMapping{}) }},
		{name: "addresses/delete", allowedAction: authz.ActionMessageSend, call: func(g security.Grant) error { return addresses.DeleteByCanonicalJID(ctx, g, 1, "jid") }},
	}

	for _, door := range doors {
		door := door
		t.Run(door.name, func(t *testing.T) {
			t.Parallel()
			grants := map[string]security.Grant{
				"zero":           {},
				"wrong-action":   security.SystemGrant(security.Action("whatsapp.invalid"), "acme"),
				"missing-tenant": security.SystemGrant(door.allowedAction, ""),
			}
			for name, grant := range grants {
				if err := door.call(grant); !errors.Is(err, security.ErrForbidden) {
					t.Errorf("%s Grant reached the nil database: %v", name, err)
				}
			}
		})
	}
}
