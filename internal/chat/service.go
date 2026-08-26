package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/arandu-io/framework/security"
	hlog "github.com/arandu-io/hesape/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	wacommon "go.mau.fi/whatsmeow/proto/waCommon"
	wae2e "go.mau.fi/whatsmeow/proto/waE2E"
	watypes "go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
	"github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
	"github.com/hyz-is/arandu-whatsapp/internal/whatsapp/address"
)

var formattedNumberCleaner = strings.NewReplacer("+", "", " ", "", "-", "", "(", "", ")", "", ".", "")
var digitsOnlyPattern = regexp.MustCompile(`^\d{8,20}$`)

type ConnectedClientResolver interface {
	ResolveConnectedClient(ctx context.Context, grant security.Grant, instanceName string) (*whatsapp.ManagedWhatsAppClient, error)
}

type Service interface {
	CheckWhatsAppNumbers(ctx context.Context, grant security.Grant, instanceName string, input WhatsAppNumbersRequest) ([]WhatsAppNumberResponse, error)
	FindMessages(ctx context.Context, grant security.Grant, instanceName string, input FindMessagesRequest) (dbtypes.MessageListResult, error)
	ReadMessages(ctx context.Context, grant security.Grant, instanceName string, input ReadMessagesRequest) error
	ArchiveChat(ctx context.Context, grant security.Grant, instanceName string, input ArchiveChatRequest) error
	DeleteMessageForEveryone(ctx context.Context, grant security.Grant, instanceName string, messageID int64) error
	FetchProfilePicture(ctx context.Context, grant security.Grant, instanceName string, input FetchProfilePictureRequest) (*string, error)
	RejectCall(ctx context.Context, grant security.Grant, instanceName string, input RejectCallRequest) error
	EditMessage(ctx context.Context, grant security.Grant, instanceName string, input EditMessageRequest) (dbtypes.Message, error)
	MediaData(ctx context.Context, grant security.Grant, instanceName string, input MediaDataRequest) (MediaDownloadResult, error)
}

type ChatService struct {
	instances     repository.InstanceRepository
	messages      repository.MessageRepository
	clients       ConnectedClientResolver
	resolver      address.Resolver
	numbersLimit  int
	maxMediaBytes int64
}

func NewService(
	instances repository.InstanceRepository,
	messages repository.MessageRepository,
	clients ConnectedClientResolver,
	resolver address.Resolver,
) *ChatService {
	return &ChatService{
		instances:     instances,
		messages:      messages,
		clients:       clients,
		resolver:      resolver,
		numbersLimit:  DefaultWhatsAppNumbersLimit,
		maxMediaBytes: DefaultMaxMediaBytes,
	}
}

func (s *ChatService) CheckWhatsAppNumbers(ctx context.Context, grant security.Grant, instanceName string, input WhatsAppNumbersRequest) ([]WhatsAppNumberResponse, error) {
	if err := validateWhatsAppNumbers(input, s.numbersLimit); err != nil {
		return nil, err
	}
	instance, client, err := s.authorizedClient(ctx, grant, instanceName)
	if err != nil {
		return nil, err
	}

	items := make([]numberLookup, len(input.Numbers))
	queries := make([]string, 0, len(input.Numbers))
	queryIndexes := make(map[string][]int, len(input.Numbers))
	for i, value := range input.Numbers {
		item, err := normalizeNumberLookup(value)
		if err != nil {
			return nil, err
		}
		items[i] = item
		if item.queryPhone != "" {
			query := "+" + item.queryPhone
			queries = append(queries, query)
			queryIndexes[item.queryPhone] = append(queryIndexes[item.queryPhone], i)
		}
	}

	result := make([]WhatsAppNumberResponse, len(items))
	for i, item := range items {
		result[i] = WhatsAppNumberResponse{JID: item.normalized, Exists: item.assumeExists}
	}
	if len(queries) > 0 {
		responses, err := client.IsOnWhatsApp(ctx, queries)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrRemoteOperation, err)
		}
		for _, response := range responses {
			indexes := queryIndexes[strings.TrimPrefix(response.Query, "+")]
			for _, index := range indexes {
				if response.IsIn && !response.JID.IsEmpty() {
					result[index].JID = response.JID.ToNonAD().String()
					result[index].Exists = true
				} else {
					result[index].Exists = false
				}
			}
		}
	}

	hlog.For(ctx).InfoContext(ctx, "WhatsApp numbers checked",
		"component", "chat_service",
		"instance_id", instance.ID,
		"instance_name", instance.Name,
		"operation", "whatsapp-numbers",
		"numbers_count", len(input.Numbers),
	)
	return result, nil
}

func (s *ChatService) FindMessages(ctx context.Context, grant security.Grant, instanceName string, input FindMessagesRequest) (dbtypes.MessageListResult, error) {
	if err := validateFindMessages(&input); err != nil {
		return dbtypes.MessageListResult{}, err
	}
	instance, err := s.authenticateInstance(ctx, grant, instanceName)
	if err != nil {
		return dbtypes.MessageListResult{}, err
	}
	result, err := s.messages.ListPage(ctx, grant, instance.ID, dbtypes.ListMessagesPageInput{
		Limit:   input.Offset,
		Page:    input.Page,
		Filters: findMessagesFilters(input.Where),
	})
	if err != nil {
		return dbtypes.MessageListResult{}, err
	}
	hlog.For(ctx).InfoContext(ctx, "messages listed",
		"component", "chat_service",
		"instance_id", instance.ID,
		"instance_name", instance.Name,
		"operation", "find-messages",
		"limit", input.Offset,
		"page", input.Page,
	)
	return result, nil
}

func (s *ChatService) ReadMessages(ctx context.Context, grant security.Grant, instanceName string, input ReadMessagesRequest) error {
	if err := validateReadMessages(input); err != nil {
		return err
	}
	instance, client, err := s.authorizedClient(ctx, grant, instanceName)
	if err != nil {
		return err
	}
	if len(input.IDs) > 0 {
		return s.readMessagesByDatabaseIDs(ctx, grant, instance, client, input.IDs)
	}
	chatJID, err := parseChatJID(*input.Chat)
	if err != nil {
		return err
	}
	senderJID, err := parseSenderJID(*input.Sender)
	if err != nil {
		return err
	}
	ids := messageIDs(input.MessageIDs)
	if len(ids) == 0 {
		return ErrInvalidRequestMode
	}
	if err := client.MarkRead(ctx, ids, time.Now().UTC(), chatJID, senderJID); err != nil {
		return fmt.Errorf("%w: %w", ErrRemoteOperation, err)
	}
	hlog.For(ctx).InfoContext(ctx, "messages marked read",
		"component", "chat_service",
		"instance_id", instance.ID,
		"instance_name", instance.Name,
		"operation", "read-messages",
		"remote_jid", address.MaskAddress(chatJID.String()),
		"read_messages_count", len(ids),
	)
	return nil
}

func findMessagesFilters(where FindMessagesWhere) dbtypes.MessageFilters {
	keyID := trimmedString(firstString(where.KeyID, where.KeyIDCamel))
	keyRemoteJid := trimmedString(where.KeyRemoteJid)
	messageType := trimmedString(where.MessageType)
	messageStatus := trimmedString(where.MessageStatus)

	var keyFromMe *bool
	if where.KeyFromMe != nil {
		value := bool(*where.KeyFromMe)
		keyFromMe = &value
	}
	var device *dbtypes.DeviceMessage
	if value := trimmedString(where.Device); value != nil {
		value := dbtypes.DeviceMessage(*value)
		device = &value
	}

	return dbtypes.MessageFilters{
		ID:                  where.ID,
		KeyID:               keyID,
		KeyRemoteJid:        keyRemoteJid,
		KeyFromMe:           keyFromMe,
		MessageType:         messageType,
		Device:              device,
		MessageStatus:       messageStatus,
		MessageTimestampGTE: where.MessageTimestampGTE,
		MessageTimestampLTE: where.MessageTimestampLTE,
	}
}

func firstString(values ...*string) *string {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func trimmedString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (s *ChatService) ArchiveChat(ctx context.Context, grant security.Grant, instanceName string, input ArchiveChatRequest) error {
	if err := validateArchiveChat(input); err != nil {
		return err
	}
	instance, client, err := s.authorizedClient(ctx, grant, instanceName)
	if err != nil {
		return err
	}
	remote, err := parseChatJID(input.LastMessage.Key.RemoteJID)
	if err != nil {
		return err
	}
	key := &wacommon.MessageKey{
		RemoteJID: proto.String(remote.String()),
		FromMe:    proto.Bool(*input.LastMessage.Key.FromMe),
		ID:        proto.String(strings.TrimSpace(input.LastMessage.Key.ID)),
	}
	if err := client.SendAppState(ctx, appstate.BuildArchive(remote, *input.Archive, time.Time{}, key)); err != nil {
		return fmt.Errorf("%w: %w", ErrRemoteOperation, err)
	}
	hlog.For(ctx).InfoContext(ctx, "chat archive state updated",
		"component", "chat_service",
		"instance_id", instance.ID,
		"instance_name", instance.Name,
		"operation", "archive-chat",
		"remote_jid", address.MaskAddress(remote.String()),
		"archive", *input.Archive,
	)
	return nil
}

func (s *ChatService) DeleteMessageForEveryone(ctx context.Context, grant security.Grant, instanceName string, messageID int64) error {
	if messageID <= 0 {
		return ValidationError{Messages: []string{"id must be a positive integer"}}
	}
	instance, client, err := s.authorizedClient(ctx, grant, instanceName)
	if err != nil {
		return err
	}
	message, err := s.messages.FindByIDForInstance(ctx, grant, instance.ID, messageID)
	if err != nil {
		return err
	}
	if !message.KeyFromMe {
		return ErrMessageNotOutgoing
	}
	chatJID, err := remoteJIDFromMessage(message)
	if err != nil {
		return err
	}
	if strings.TrimSpace(message.KeyID) == "" {
		return ErrInvalidRecipient
	}
	if _, err := client.SendMessage(ctx, chatJID, client.BuildRevoke(chatJID, watypes.EmptyJID, watypes.MessageID(message.KeyID))); err != nil {
		return fmt.Errorf("%w: %w", ErrRemoteOperation, err)
	}
	content, err := markDeletedContent(message.Content, time.Now().UTC())
	if err != nil {
		return err
	}
	if _, err := s.messages.UpdateContentForInstance(ctx, grant, instance.ID, message.ID, content); err != nil {
		hlog.For(ctx).ErrorContext(ctx, "message revoked remotely but local metadata update failed",
			"component", "chat_service", "error", err, "instance_id", instance.ID, "message_id", message.ID)
		return ErrDatabaseOperation
	}
	hlog.For(ctx).InfoContext(ctx, "message revoked for everyone",
		"component", "chat_service",
		"instance_id", instance.ID,
		"instance_name", instance.Name,
		"operation", "delete-message",
		"remote_jid", address.MaskAddress(chatJID.String()),
		"key_id", message.KeyID,
	)
	return nil
}

func (s *ChatService) FetchProfilePicture(ctx context.Context, grant security.Grant, instanceName string, input FetchProfilePictureRequest) (*string, error) {
	if err := validateFetchProfilePicture(input); err != nil {
		return nil, err
	}
	instance, client, err := s.authorizedClient(ctx, grant, instanceName)
	if err != nil {
		return nil, err
	}
	recipient, err := input.ResolveRecipient()
	if err != nil {
		return nil, err
	}
	jid, err := s.resolveRecipientJID(ctx, grant, client, instance.ID, recipient)
	if err != nil {
		return nil, err
	}
	info, err := client.GetProfilePictureInfo(ctx, jid, nil)
	if err != nil {
		if errors.Is(err, whatsmeow.ErrProfilePictureNotSet) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %w", ErrRemoteOperation, err)
	}
	if info == nil || strings.TrimSpace(info.URL) == "" {
		return nil, nil
	}
	url := info.URL
	hlog.For(ctx).InfoContext(ctx, "profile picture URL fetched",
		"component", "chat_service",
		"instance_id", instance.ID,
		"instance_name", instance.Name,
		"operation", "fetch-profile-picture",
		"remote_jid", address.MaskAddress(jid.String()),
	)
	return &url, nil
}

func (s *ChatService) RejectCall(ctx context.Context, grant security.Grant, instanceName string, input RejectCallRequest) error {
	if err := validateRejectCall(input); err != nil {
		return err
	}
	instance, client, err := s.authorizedClient(ctx, grant, instanceName)
	if err != nil {
		return err
	}
	callFrom, err := parseSenderJID(input.CallFrom)
	if err != nil {
		return err
	}
	if err := client.RejectCall(ctx, callFrom, strings.TrimSpace(input.CallID)); err != nil {
		return fmt.Errorf("%w: %w", ErrRemoteOperation, err)
	}
	hlog.For(ctx).InfoContext(ctx, "call rejected",
		"component", "chat_service",
		"instance_id", instance.ID,
		"instance_name", instance.Name,
		"operation", "reject-call",
		"remote_jid", address.MaskAddress(callFrom.String()),
		"call_id", input.CallID,
	)
	return nil
}

func (s *ChatService) EditMessage(ctx context.Context, grant security.Grant, instanceName string, input EditMessageRequest) (dbtypes.Message, error) {
	if err := validateEditMessage(input); err != nil {
		return dbtypes.Message{}, err
	}
	instance, client, err := s.authorizedClient(ctx, grant, instanceName)
	if err != nil {
		return dbtypes.Message{}, err
	}
	message, err := s.findOutgoingMessage(ctx, grant, instance.ID, input.ID)
	if err != nil {
		return dbtypes.Message{}, err
	}
	if !editableMessageType(message.MessageType) {
		return dbtypes.Message{}, ErrMessageNotEditable
	}
	chatJID, err := remoteJIDFromMessage(message)
	if err != nil {
		return dbtypes.Message{}, err
	}
	if strings.TrimSpace(message.KeyID) == "" {
		return dbtypes.Message{}, ErrInvalidRecipient
	}

	_ = client.SendChatPresence(ctx, chatJID, watypes.ChatPresenceComposing, watypes.ChatPresenceMediaText)
	edit := &wae2e.Message{
		ProtocolMessage: &wae2e.ProtocolMessage{
			Type: wae2e.ProtocolMessage_MESSAGE_EDIT.Enum(),
			Key:  client.BuildMessageKey(chatJID, watypes.EmptyJID, watypes.MessageID(message.KeyID)),
			EditedMessage: &wae2e.Message{
				ExtendedTextMessage: &wae2e.ExtendedTextMessage{Text: proto.String(input.Text)},
			},
		},
	}
	if _, err := client.SendMessage(ctx, chatJID, edit); err != nil {
		return dbtypes.Message{}, fmt.Errorf("%w: %w", ErrRemoteOperation, err)
	}
	content, err := markEditedContent(message.Content, input.Text, time.Now().UTC())
	if err != nil {
		return dbtypes.Message{}, err
	}
	updated, err := s.messages.UpdateContentForInstance(ctx, grant, instance.ID, message.ID, content)
	if err != nil {
		hlog.For(ctx).ErrorContext(ctx, "message edited remotely but local update failed",
			"component", "chat_service", "error", err, "instance_id", instance.ID, "message_id", message.ID, "key_id", message.KeyID)
		return dbtypes.Message{}, ErrDatabaseOperation
	}
	hlog.For(ctx).InfoContext(ctx, "message edited",
		"component", "chat_service",
		"instance_id", instance.ID,
		"instance_name", instance.Name,
		"operation", "edit-message",
		"remote_jid", address.MaskAddress(chatJID.String()),
		"key_id", message.KeyID,
		"identifier_type", identifierType(input.ID),
	)
	return updated, nil
}

func (s *ChatService) readMessagesByDatabaseIDs(ctx context.Context, grant security.Grant, instance dbtypes.Instance, client *whatsmeow.Client, ids []int64) error {
	messages, err := s.messages.FindByIDsForInstance(ctx, grant, instance.ID, ids)
	if err != nil {
		return err
	}
	if len(messages) != len(ids) {
		return repository.ErrMessageNotFound
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].ID < messages[j].ID })
	groups := make(map[string]readReceiptGroup)
	for _, message := range messages {
		chatJID, err := remoteJIDFromMessage(message)
		if err != nil {
			return err
		}
		senderJID, err := senderJIDFromMessage(message)
		if err != nil {
			return err
		}
		key := chatJID.String() + "|" + senderJID.String()
		group := groups[key]
		group.chat = chatJID
		group.sender = senderJID
		group.ids = append(group.ids, watypes.MessageID(message.KeyID))
		groups[key] = group
	}
	if len(groups) == 0 {
		return ErrInvalidRequestMode
	}
	for _, group := range groups {
		if err := client.MarkRead(ctx, group.ids, time.Now().UTC(), group.chat, group.sender); err != nil {
			return fmt.Errorf("%w: %w", ErrRemoteOperation, err)
		}
	}
	if err := s.messages.MarkReadForInstance(ctx, grant, instance.ID, ids); err != nil {
		return ErrDatabaseOperation
	}
	hlog.For(ctx).InfoContext(ctx, "messages marked read",
		"component", "chat_service",
		"instance_id", instance.ID,
		"instance_name", instance.Name,
		"operation", "read-messages",
		"read_messages_count", len(ids),
	)
	return nil
}

func (s *ChatService) authorizedClient(ctx context.Context, grant security.Grant, instanceName string) (dbtypes.Instance, *whatsmeow.Client, error) {
	instance, err := s.authenticateInstance(ctx, grant, instanceName)
	if err != nil {
		return dbtypes.Instance{}, nil, err
	}
	managed, err := s.clients.ResolveConnectedClient(ctx, grant, instance.Name)
	if err != nil {
		return dbtypes.Instance{}, nil, err
	}
	if managed == nil || managed.Client == nil || managed.Client.Store == nil ||
		managed.Client.Store.ID == nil || !managed.Client.IsConnected() || !managed.Client.IsLoggedIn() {
		return dbtypes.Instance{}, nil, ErrInstanceDisconnected
	}
	return instance, managed.Client, nil
}

func (s *ChatService) authenticateInstance(ctx context.Context, grant security.Grant, instanceName string) (dbtypes.Instance, error) {
	name := strings.TrimSpace(instanceName)
	if name == "" {
		return dbtypes.Instance{}, repository.ErrInvalidInput
	}
	instance, err := s.instances.FindByName(ctx, grant, name)
	if err != nil {
		return dbtypes.Instance{}, err
	}
	if instance.Instance.Status != dbtypes.InstanceStatusOnline {
		if instance.Instance.ConnectionStatus == dbtypes.InstanceConnectionStatusLoggedOut {
			active := dbtypes.InstanceStatusOnline
			if err := s.instances.UpdateStatus(ctx, grant, instance.Instance.ID, active); err != nil {
				return dbtypes.Instance{}, err
			}
			instance.Instance.Status = active
			return instance.Instance, nil
		}
		return dbtypes.Instance{}, whatsapp.ErrInstanceInactive
	}
	return instance.Instance, nil
}

func (s *ChatService) resolveRecipientJID(ctx context.Context, grant security.Grant, client *whatsmeow.Client, instanceID int64, raw string) (watypes.JID, error) {
	if s.resolver != nil {
		resolved, err := s.resolver.Resolve(ctx, grant, client, address.ResolveInput{InstanceID: instanceID, Address: raw})
		if err == nil {
			return resolved.CanonicalJID, nil
		}
		if strings.Contains(raw, "@") {
			return parseChatJID(raw)
		}
		return watypes.JID{}, err
	}
	item, err := normalizeNumberLookup(raw)
	if err != nil {
		return watypes.JID{}, err
	}
	if item.directJID != nil {
		return *item.directJID, nil
	}
	return watypes.NewJID(item.queryPhone, watypes.DefaultUserServer), nil
}

func (s *ChatService) findOutgoingMessage(ctx context.Context, grant security.Grant, instanceID int64, identifier MessageIdentifier) (dbtypes.Message, error) {
	if identifier.NumericID != nil {
		if *identifier.NumericID <= 0 {
			return dbtypes.Message{}, ValidationError{Messages: []string{"id must be a positive integer"}}
		}
		return s.messages.FindOutgoingByIDForInstance(ctx, grant, instanceID, *identifier.NumericID)
	}
	if identifier.KeyID != nil && strings.TrimSpace(*identifier.KeyID) != "" {
		return s.messages.FindOutgoingByKeyIDForInstance(ctx, grant, instanceID, strings.TrimSpace(*identifier.KeyID))
	}
	return dbtypes.Message{}, ValidationError{Messages: []string{"id is required"}}
}

type numberLookup struct {
	normalized   string
	queryPhone   string
	directJID    *watypes.JID
	assumeExists bool
}

func normalizeNumberLookup(raw string) (numberLookup, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return numberLookup{}, ErrInvalidRecipient
	}
	if strings.Contains(value, "@") {
		jid, err := watypes.ParseJID(value)
		if err != nil {
			return numberLookup{}, fmt.Errorf("%w: %w", ErrInvalidRecipient, err)
		}
		if jid.Server == watypes.GroupServer {
			return numberLookup{}, fmt.Errorf("%w: groups are not supported", ErrInvalidRecipient)
		}
		jid = jid.ToNonAD()
		switch jid.Server {
		case watypes.DefaultUserServer, watypes.LegacyUserServer:
			if !digitsOnlyPattern.MatchString(jid.User) {
				return numberLookup{}, ErrInvalidRecipient
			}
			return numberLookup{normalized: watypes.NewJID(jid.User, watypes.DefaultUserServer).String(), queryPhone: jid.User}, nil
		case watypes.HiddenUserServer:
			return numberLookup{normalized: jid.String(), directJID: &jid, assumeExists: true}, nil
		default:
			return numberLookup{}, fmt.Errorf("%w: unsupported jid server", ErrInvalidRecipient)
		}
	}
	number := formattedNumberCleaner.Replace(value)
	if !digitsOnlyPattern.MatchString(number) {
		return numberLookup{}, ErrInvalidRecipient
	}
	return numberLookup{normalized: number, queryPhone: number}, nil
}

func parseChatJID(value string) (watypes.JID, error) {
	normalized, err := address.NormalizeAddress(value)
	if err != nil {
		return watypes.JID{}, fmt.Errorf("%w: %w", ErrInvalidRecipient, err)
	}
	if !strings.Contains(normalized, "@") {
		return watypes.NewJID(normalized, watypes.DefaultUserServer), nil
	}
	jid, err := watypes.ParseJID(normalized)
	if err != nil {
		return watypes.JID{}, fmt.Errorf("%w: %w", ErrInvalidRecipient, err)
	}
	jid = jid.ToNonAD()
	switch jid.Server {
	case watypes.DefaultUserServer, watypes.HiddenUserServer, watypes.GroupServer:
		return jid, nil
	default:
		return watypes.JID{}, fmt.Errorf("%w: unsupported jid server", ErrInvalidRecipient)
	}
}

func parseSenderJID(value string) (watypes.JID, error) {
	jid, err := parseChatJID(value)
	if err != nil {
		return watypes.JID{}, err
	}
	if jid.Server == watypes.GroupServer {
		return watypes.JID{}, fmt.Errorf("%w: sender cannot be a group", ErrInvalidRecipient)
	}
	return jid, nil
}

func remoteJIDFromMessage(message dbtypes.Message) (watypes.JID, error) {
	if message.KeyRemoteJid == nil || strings.TrimSpace(*message.KeyRemoteJid) == "" {
		return watypes.JID{}, ErrInvalidRecipient
	}
	return parseChatJID(*message.KeyRemoteJid)
}

func senderJIDFromMessage(message dbtypes.Message) (watypes.JID, error) {
	if message.KeyParticipant != nil && strings.TrimSpace(*message.KeyParticipant) != "" {
		return parseSenderJID(*message.KeyParticipant)
	}
	if !message.KeyFromMe && message.KeyRemoteJid != nil {
		remote, err := parseChatJID(*message.KeyRemoteJid)
		if err != nil {
			return watypes.JID{}, err
		}
		if remote.Server != watypes.GroupServer {
			return watypes.EmptyJID, nil
		}
	}
	return watypes.EmptyJID, nil
}

func messageIDs(values []string) []watypes.MessageID {
	ids := make([]watypes.MessageID, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			ids = append(ids, watypes.MessageID(value))
		}
	}
	return ids
}

type readReceiptGroup struct {
	chat   watypes.JID
	sender watypes.JID
	ids    []watypes.MessageID
}

func markDeletedContent(raw json.RawMessage, deletedAt time.Time) (json.RawMessage, error) {
	content := contentMap(raw)
	content["deletedForEveryone"] = true
	content["deletedAt"] = deletedAt.Format(time.RFC3339Nano)
	return json.Marshal(content)
}

func markEditedContent(raw json.RawMessage, text string, editedAt time.Time) (json.RawMessage, error) {
	content := contentMap(raw)
	if previous, ok := content["text"].(string); ok && previous != "" {
		content["previousText"] = previous
	}
	content["text"] = text
	content["edited"] = true
	content["editedAt"] = editedAt.Format(time.RFC3339Nano)
	return json.Marshal(content)
}

func contentMap(raw json.RawMessage) map[string]any {
	content := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &content)
	}
	return content
}

func editableMessageType(messageType string) bool {
	switch messageType {
	case "conversation", "extendedTextMessage":
		return true
	default:
		return false
	}
}

func identifierType(identifier MessageIdentifier) string {
	if identifier.NumericID != nil {
		return "numeric"
	}
	if identifier.KeyID != nil {
		return "keyId"
	}
	return "unknown"
}

func Int64FromQuery(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, ValidationError{Messages: []string{"id is required"}}
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, ValidationError{Messages: []string{"id must be a positive integer"}}
	}
	return id, nil
}
