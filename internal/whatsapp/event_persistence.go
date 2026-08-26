package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	hlog "github.com/arandu-io/hesape/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/arandu-io/framework/security"
	"github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
	"github.com/hyz-is/arandu-whatsapp/internal/whatsapp/address"
)

const (
	defaultInitialContactSyncDelay = 30 * time.Second
	defaultContactProfileWorkers   = 5
	defaultReceiptRetryAttempts    = 3
	defaultReceiptRetryDelay       = 100 * time.Millisecond
)

type EventPersistenceConfig struct {
	SaveDataNewMessage       bool
	SaveMessageUpdate        bool
	SaveDataContacts         bool
	InitialContactSyncDelay  time.Duration
	ContactProfileWorkers    int
	ProfilePictureTimeout    time.Duration
	ReceiptRetryAttempts     int
	ReceiptRetryInitialDelay time.Duration
}

type EventPersistenceService struct {
	cfg            EventPersistenceConfig
	messages       repository.MessageRepository
	messageUpdates repository.MessageUpdateRepository
	contacts       repository.ContactRepository
	instances      webhookInstanceFinder
	webhooks       webhooksvc.WebhookManager
	normalizer     MessageEventNormalizer
}

type webhookInstanceFinder interface {
	FindByName(ctx context.Context, grant security.Grant, name string) (dbtypes.InstanceRecord, error)
}

func NewEventPersistenceService(
	cfg EventPersistenceConfig,
	messages repository.MessageRepository,
	messageUpdates repository.MessageUpdateRepository,
	contacts repository.ContactRepository,
) *EventPersistenceService {
	if cfg.InitialContactSyncDelay <= 0 {
		cfg.InitialContactSyncDelay = defaultInitialContactSyncDelay
	}
	if cfg.ContactProfileWorkers <= 0 {
		cfg.ContactProfileWorkers = defaultContactProfileWorkers
	}
	if cfg.ReceiptRetryAttempts <= 0 {
		cfg.ReceiptRetryAttempts = defaultReceiptRetryAttempts
	}
	if cfg.ReceiptRetryInitialDelay <= 0 {
		cfg.ReceiptRetryInitialDelay = defaultReceiptRetryDelay
	}
	if cfg.ProfilePictureTimeout <= 0 {
		cfg.ProfilePictureTimeout = 15 * time.Second
	}
	return &EventPersistenceService{
		cfg:            cfg,
		messages:       messages,
		messageUpdates: messageUpdates,
		contacts:       contacts,
		normalizer:     NewMessageEventNormalizer(),
	}
}

func (s *EventPersistenceService) SetWebhookDispatcher(instances webhookInstanceFinder, webhooks webhooksvc.WebhookManager) {
	s.instances = instances
	s.webhooks = webhooks
}

func (s *EventPersistenceService) HandleMessage(ctx context.Context, managed *ManagedWhatsAppClient, event *events.Message) {
	instanceID := mustAtoi64(managed.InstanceID)
	message, err := s.normalizer.NormalizeMessage(instanceID, event)
	if err != nil {
		hlog.For(ctx).WarnContext(ctx, "message event not persisted",
			"component", "event_persistence", "error", err, "instance_id", instanceID, "instance_name", managed.InstanceName, "event", "message")
		return
	}
	if !s.cfg.SaveDataNewMessage {
		return
	}
	if err := s.messages.CreateOrIgnore(ctx, managed.RuntimeGrant, message); err != nil {
		hlog.For(ctx).ErrorContext(ctx, "failed to persist message event",
			"component", "event_persistence", "error", err, "event", "message", "operation", "message.create_or_ignore",
			"instance_id", instanceID, "instance_name", managed.InstanceName, "message_key_id", message.KeyID,
			"remote_jid", address.MaskAddress(stringValue(message.KeyRemoteJid)))
	} else {
		hlog.For(ctx).DebugContext(ctx, "new message",
			"component", "event_persistence", "instance_name", managed.InstanceName,
			"message_key_id", message.KeyID, "message_type", message.MessageType)
		s.dispatchMessageUpsertWebhook(ctx, managed, message.KeyID)
	}
}

func (s *EventPersistenceService) HandleFBMessage(ctx context.Context, managed *ManagedWhatsAppClient, event *events.FBMessage) {
	instanceID := mustAtoi64(managed.InstanceID)
	message, err := s.normalizer.NormalizeFBMessage(instanceID, event)
	if err != nil {
		hlog.For(ctx).WarnContext(ctx, "fb message event not persisted",
			"component", "event_persistence", "error", err, "instance_id", instanceID, "instance_name", managed.InstanceName, "event", "fb_message")
		return
	}
	if !s.cfg.SaveDataNewMessage {
		return
	}
	if err := s.messages.CreateOrIgnore(ctx, managed.RuntimeGrant, message); err != nil {
		hlog.For(ctx).ErrorContext(ctx, "failed to persist fb message event",
			"component", "event_persistence", "error", err, "event", "fb_message", "operation", "message.create_or_ignore",
			"instance_id", instanceID, "instance_name", managed.InstanceName, "message_key_id", message.KeyID,
			"remote_jid", address.MaskAddress(stringValue(message.KeyRemoteJid)))
	} else {
		s.dispatchMessageUpsertWebhook(ctx, managed, message.KeyID)
	}
}

func (s *EventPersistenceService) HandleReceipt(ctx context.Context, managed *ManagedWhatsAppClient, event *events.Receipt) {
	if event == nil {
		return
	}
	instanceID := mustAtoi64(managed.InstanceID)
	status := normalizeReceiptStatus(event.Type)
	dateTime := event.Timestamp
	if dateTime.IsZero() {
		dateTime = time.Now().UTC()
	}
	for _, messageID := range event.MessageIDs {
		keyID := string(messageID)
		if strings.TrimSpace(keyID) == "" {
			continue
		}
		var message dbtypes.Message
		if s.cfg.SaveMessageUpdate {
			m, err := s.findMessageWithRetry(ctx, managed.RuntimeGrant, instanceID, keyID)
			message = m
			if err != nil {
				if errors.Is(err, repository.ErrMessageNotFound) {
					hlog.For(ctx).WarnContext(ctx, "message not found for receipt",
						"component", "event_persistence", "event", "receipt", "instance_id", instanceID, "instance_name", managed.InstanceName,
						"message_key_id", keyID, "receipt_type", string(event.Type), "timestamp", dateTime)
					hlog.For(ctx).WarnContext(ctx, "webhook source entity not found",
						"component", "event_persistence", "event", string(dbtypes.WebhookEventMessagesUpdated), "instance_id", instanceID,
						"instance_name", managed.InstanceName, "message_key", keyID)
					continue
				}
				hlog.For(ctx).ErrorContext(ctx, "failed to find message for receipt",
					"component", "event_persistence", "error", err, "event", "receipt", "instance_id", instanceID,
					"instance_name", managed.InstanceName, "message_key_id", keyID)
				continue
			}
			if err := s.messageUpdates.CreateOrIgnore(ctx, managed.RuntimeGrant, dbtypes.CreateMessageUpdateInput{
				DateTime:  dateTime,
				Status:    status,
				MessageID: message.ID,
			}); err != nil {
				hlog.For(ctx).ErrorContext(ctx, "failed to persist receipt",
					"component", "event_persistence", "error", err, "event", "receipt", "operation", "message_update.create_or_ignore",
					"instance_id", instanceID, "instance_name", managed.InstanceName, "message_key_id", keyID)
				continue
			}
		}

		s.dispatchWebhook(ctx, managed, dbtypes.WebhookEventMessagesUpdated, webhooksvc.NewMessageUpdateWebhookData(message.ID, keyID, status, dateTime))
	}
}

func (s *EventPersistenceService) findMessageWithRetry(ctx context.Context, grant security.Grant, instanceID int64, keyID string) (dbtypes.Message, error) {
	delay := s.cfg.ReceiptRetryInitialDelay
	var lastErr error
	for attempt := 0; attempt < s.cfg.ReceiptRetryAttempts; attempt++ {
		message, err := s.messages.FindByKeyIDForInstance(ctx, grant, instanceID, keyID)
		if err == nil {
			return message, nil
		}
		lastErr = err
		if !errors.Is(err, repository.ErrMessageNotFound) || attempt == s.cfg.ReceiptRetryAttempts-1 {
			return dbtypes.Message{}, err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return dbtypes.Message{}, ctx.Err()
		case <-timer.C:
		}
		delay *= 2
	}
	return dbtypes.Message{}, lastErr
}

func (s *EventPersistenceService) StartInitialContactSync(ctx context.Context, managed *ManagedWhatsAppClient) {
	if !s.cfg.SaveDataContacts || managed == nil || managed.Client == nil {
		return
	}
	timer := time.NewTimer(s.cfg.InitialContactSyncDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	if ctx.Err() != nil || !managed.IsReady() || managed.Client.Store == nil || managed.Client.Store.Contacts == nil {
		return
	}
	contacts, err := managed.Client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		hlog.For(ctx).WarnContext(ctx, "failed to load WhatsApp contacts",
			"component", "event_persistence", "error", err, "event", "contact_sync",
			"instance_id", managed.InstanceID, "instance_name", managed.InstanceName)
		return
	}
	s.syncContacts(ctx, managed, contacts)
}

func (s *EventPersistenceService) syncContacts(ctx context.Context, managed *ManagedWhatsAppClient, contacts map[types.JID]types.ContactInfo) {
	jobs := make(chan normalizedContact)
	var wg sync.WaitGroup
	workers := s.cfg.ContactProfileWorkers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for contact := range jobs {
				s.persistContact(ctx, managed, contact, true)
			}
		}()
	}
	for jid, info := range contacts {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case jobs <- normalizeStoreContact(mustAtoi64(managed.InstanceID), jid, info):
		}
	}
	close(jobs)
	wg.Wait()
}

func (s *EventPersistenceService) HandleContact(ctx context.Context, managed *ManagedWhatsAppClient, event *events.Contact) {
	if event == nil {
		return
	}
	contact := normalizeContactEvent(mustAtoi64(managed.InstanceID), event)
	if contact.RemoteJid == "" {
		return
	}
	if !s.cfg.SaveDataContacts {
		return
	}
	persisted, ok := s.persistContact(ctx, managed, contact, true)
	if !ok {
		return
	}
	s.dispatchWebhook(ctx, managed, dbtypes.WebhookEventContactsUpsert, webhooksvc.NewContactUpsertWebhookData(persisted, contact.LID, "upserted"))
}

func (s *EventPersistenceService) HandlePushName(ctx context.Context, managed *ManagedWhatsAppClient, event *events.PushName) {
	if event == nil || !s.cfg.SaveDataContacts {
		return
	}
	remote := preferredContactJID(event.JID, event.JIDAlt)
	contact := normalizedContact{
		InstanceID: mustAtoi64(managed.InstanceID),
		JID:        remote,
		LID:        stringPtrFromJID(firstLIDJID(event.JIDAlt, event.JID)),
		RemoteJid:  jidString(remote),
		PushName:   event.NewPushName,
	}
	s.persistAndDispatchContactUpdate(ctx, managed, event, contact)
}

func (s *EventPersistenceService) HandleBusinessName(ctx context.Context, managed *ManagedWhatsAppClient, event *events.BusinessName) {
	if event == nil || !s.cfg.SaveDataContacts {
		return
	}
	remote := preferredContactJID(event.JID, types.EmptyJID)
	contact := normalizedContact{
		InstanceID: mustAtoi64(managed.InstanceID),
		JID:        remote,
		LID:        stringPtrFromJID(firstLIDJID(event.JID)),
		RemoteJid:  jidString(remote),
	}
	s.persistAndDispatchContactUpdate(ctx, managed, event, contact)
}

func (s *EventPersistenceService) persistAndDispatchContactUpdate(ctx context.Context, managed *ManagedWhatsAppClient, event any, contact normalizedContact) {
	if contact.RemoteJid == "" {
		return
	}
	persisted, ok := s.persistContact(ctx, managed, contact, false)
	if !ok {
		return
	}
	dto := webhooksvc.ContactUpdateWebhookData{
		ID:        int64(persisted.ID),
		RemoteJID: persisted.RemoteJid,
		LID:       contact.LID,
		PushName:  persisted.PushName,
		Action:    "updated",
		Source:    "unknown",
	}
	data, err := NewContactUpdateNormalizer().Normalize(event, dto)
	if err != nil {
		hlog.For(ctx).WarnContext(ctx, "webhook event normalization failed",
			"component", "event_persistence", "error", err, "event", string(dbtypes.WebhookEventContactsUpdated),
			"instance_id", contact.InstanceID, "instance_name", managed.InstanceName, "source_event", fmt.Sprintf("%T", event))
		return
	}
	s.dispatchWebhook(ctx, managed, dbtypes.WebhookEventContactsUpdated, data)
}

func (s *EventPersistenceService) persistContact(ctx context.Context, managed *ManagedWhatsAppClient, contact normalizedContact, fetchProfilePicture bool) (dbtypes.Contact, bool) {
	if contact.RemoteJid == "" {
		return dbtypes.Contact{}, false
	}
	if fetchProfilePicture && managed != nil && managed.IsReady() {
		contact.ProfilePicURL = s.profilePictureURL(ctx, managed.Client, contact.JID)
	}
	input := dbtypes.CreateContactInput{
		RemoteJid:     contact.RemoteJid,
		PushName:      stringPtr(contact.PushName),
		ProfilePicUrl: stringPtr(contact.ProfilePicURL),
		InstanceID:    contact.InstanceID,
	}
	persisted, err := s.contacts.Upsert(ctx, managed.RuntimeGrant, input)
	if err != nil {
		hlog.For(ctx).ErrorContext(ctx, "failed to persist contact",
			"component", "event_persistence", "error", err, "event", "contact", "operation", "contact.upsert",
			"instance_id", contact.InstanceID, "instance_name", managed.InstanceName,
			"remote_jid", address.MaskAddress(contact.RemoteJid))
		return dbtypes.Contact{}, false
	}
	return persisted, true
}

func (s *EventPersistenceService) profilePictureURL(ctx context.Context, client *whatsmeow.Client, jid types.JID) string {
	if client == nil || jid.IsEmpty() {
		return ""
	}
	profileCtx, cancel := context.WithTimeout(ctx, s.cfg.ProfilePictureTimeout)
	defer cancel()
	info, err := client.GetProfilePictureInfo(profileCtx, jid, nil)
	if err != nil || info == nil {
		return ""
	}
	return info.URL
}

type normalizedContact struct {
	InstanceID    int64
	JID           types.JID
	LID           *string
	RemoteJid     string
	PushName      string
	ProfilePicURL string
}

func normalizeStoreContact(instanceID int64, jid types.JID, info types.ContactInfo) normalizedContact {
	remote := preferredContactJID(jid, types.EmptyJID)
	return normalizedContact{
		InstanceID: instanceID,
		JID:        remote,
		RemoteJid:  jidString(remote),
		PushName:   firstNonEmpty(info.PushName, info.FullName, info.FirstName, info.BusinessName),
	}
}

func normalizeContactEvent(instanceID int64, event *events.Contact) normalizedContact {
	pnJID := jidFromString(contactActionPNJID(event.Action))
	lidJID := jidFromString(contactActionLIDJID(event.Action))
	remote := preferredContactJID(event.JID, pnJID)
	if remote.IsEmpty() {
		remote = preferredContactJID(pnJID, lidJID)
	}
	return normalizedContact{
		InstanceID: instanceID,
		JID:        remote,
		LID:        stringPtrFromJID(lidJID),
		RemoteJid:  jidString(remote),
		PushName:   firstNonEmpty(contactActionFullName(event.Action), contactActionFirstName(event.Action), contactActionUsername(event.Action)),
	}
}

func (s *EventPersistenceService) dispatchMessageUpsertWebhook(ctx context.Context, managed *ManagedWhatsAppClient, keyID string) {
	instanceID := mustAtoi64(managed.InstanceID)
	message, err := s.messages.FindByKeyIDForInstance(ctx, managed.RuntimeGrant, instanceID, keyID)
	if err != nil {
		if errors.Is(err, repository.ErrMessageNotFound) {
			hlog.For(ctx).WarnContext(ctx, "webhook source entity not found",
				"component", "event_persistence", "event", string(dbtypes.WebhookEventMessagesUpsert), "instance_id", instanceID,
				"instance_name", managed.InstanceName, "message_key", keyID)
			return
		}
		hlog.For(ctx).WarnContext(ctx, "webhook source entity not loaded",
			"component", "event_persistence", "error", err, "event", string(dbtypes.WebhookEventMessagesUpsert), "instance_id", instanceID,
			"instance_name", managed.InstanceName, "message_key", keyID)
		return
	}
	s.dispatchWebhook(ctx, managed, dbtypes.WebhookEventMessagesUpsert, webhooksvc.NewMessageUpsertWebhookData(message))
}

func (s *EventPersistenceService) dispatchWebhook(ctx context.Context, managed *ManagedWhatsAppClient, event dbtypes.WebhookEvent, data any) {
	if s.webhooks == nil || s.instances == nil || managed == nil {
		return
	}
	if ctx == nil {
		ctx = managed.Context
		if ctx == nil {
			ctx = context.Background()
		}
	}
	instance, err := s.instances.FindByName(ctx, managed.RuntimeGrant, managed.InstanceName)
	if err != nil {
		hlog.For(ctx).WarnContext(ctx, "webhook instance snapshot not loaded",
			"component", "event_persistence", "error", err, "event", string(event),
			"instance_id", managed.InstanceID, "instance_name", managed.InstanceName)
		return
	}
	if err := s.webhooks.Dispatch(ctx, managed.RuntimeGrant, webhooksvc.NewWebhookInstance(instance.Instance), event, data); err != nil {
		hlog.For(ctx).WarnContext(ctx, "webhook dispatch not queued",
			"component", "event_persistence", "error", err, "event", string(event),
			"instance_id", managed.InstanceID, "instance_name", managed.InstanceName)
	}
}

func preferredContactJID(primary types.JID, fallback types.JID) types.JID {
	for _, candidate := range []types.JID{primary, fallback} {
		if candidate.IsEmpty() {
			continue
		}
		switch candidate.Server {
		case types.DefaultUserServer, types.GroupServer, types.NewsletterServer:
			return candidate
		}
	}
	for _, candidate := range []types.JID{primary, fallback} {
		if !candidate.IsEmpty() {
			return candidate
		}
	}
	return types.EmptyJID
}

func normalizeReceiptStatus(value types.ReceiptType) string {
	switch value {
	case types.ReceiptTypeDelivered:
		return "delivered"
	case types.ReceiptTypeSender:
		return "sent"
	case types.ReceiptTypeRead, types.ReceiptTypeReadSelf:
		return "read"
	case types.ReceiptTypePlayed, types.ReceiptTypePlayedSelf:
		return "played"
	case types.ReceiptTypeServerError:
		return "server_error"
	case types.ReceiptTypeRetry:
		return "retry"
	default:
		return "unknown"
	}
}

func contactActionFullName(action *waSyncAction.ContactAction) string {
	if action == nil {
		return ""
	}
	return action.GetFullName()
}

func contactActionFirstName(action *waSyncAction.ContactAction) string {
	if action == nil {
		return ""
	}
	return action.GetFirstName()
}

func contactActionUsername(action *waSyncAction.ContactAction) string {
	if action == nil {
		return ""
	}
	return action.GetUsername()
}

func contactActionPNJID(action *waSyncAction.ContactAction) string {
	if action == nil {
		return ""
	}
	return action.GetPnJID()
}

func contactActionLIDJID(action *waSyncAction.ContactAction) string {
	if action == nil {
		return ""
	}
	return action.GetLidJID()
}

func jidString(jid types.JID) string {
	if jid.IsEmpty() {
		return ""
	}
	return jid.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
