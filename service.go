package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/str"

	"github.com/hyz-is/arandu-whatsapp/internal/chat"
	internalrepo "github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	"github.com/hyz-is/arandu-whatsapp/internal/group"
	"github.com/hyz-is/arandu-whatsapp/internal/message"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
	internalwhatsapp "github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
)

// CreateInstanceInput is the explicit instance creation contract.
type CreateInstanceInput struct {
	// Name is the stable public name. A generated name is used when it is absent.
	Name *string `json:"name,omitempty"`
	// Description is optional human-readable instance metadata.
	Description *string `json:"description,omitempty"`
	// ExternalAttributes holds caller-defined JSON object metadata.
	ExternalAttributes json.RawMessage `json:"externalAttributes,omitempty"`
}

// PhonePairingInput is the JSON-safe input for phone-code pairing.
type PhonePairingInput struct {
	// PhoneNumber is the international phone number to pair.
	PhoneNumber string `json:"phoneNumber"`
}

// Service is the single authorized entry point used by HTTP handlers.
type Service struct {
	tenant      string
	repository  *InstanceRepository
	instances   internalrepo.InstanceRepository
	policy      InstancePolicy
	connections internalwhatsapp.ConnectionService
	messages    message.Service
	chats       chat.Service
	groups      group.Service
	webhooks    webhooksvc.Service
}

func newService(
	tenant string,
	repository *InstanceRepository,
	instances internalrepo.InstanceRepository,
	policy InstancePolicy,
	connections internalwhatsapp.ConnectionService,
	messages message.Service,
	chats chat.Service,
	groups group.Service,
	webhooks webhooksvc.Service,
) *Service {
	return &Service{
		tenant: tenant, repository: repository, instances: instances, policy: policy,
		connections: connections, messages: messages, chats: chats,
		groups: groups, webhooks: webhooks,
	}
}

// CreateInstance validates, authorizes and creates one instance.
func (s *Service) CreateInstance(ctx context.Context, actor security.Subject, input CreateInstanceInput) (Instance, error) {
	if err := s.checkTenant(actor); err != nil {
		return Instance{}, err
	}
	name := ""
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
	}
	if name == "" {
		name = "instance-" + str.UUID()
	}
	if len(name) > 255 {
		return Instance{}, internalrepo.ErrInvalidInput
	}
	if input.Description != nil {
		value := strings.TrimSpace(*input.Description)
		if len(value) > 255 {
			return Instance{}, internalrepo.ErrInvalidInput
		}
		input.Description = &value
	}
	if err := validateJSONObject(input.ExternalAttributes); err != nil {
		return Instance{}, err
	}
	candidate := Instance{TenantID: actor.Tenant, Name: name, Description: input.Description, ExternalAttributes: input.ExternalAttributes}
	grant, err := security.Authorize(ctx, s.policy, actor, ActionInstanceCreate, candidate)
	if err != nil {
		return Instance{}, err
	}
	return s.repository.Create(ctx, grant, candidate)
}

// ListInstances returns one page of instances from the actor tenant.
func (s *Service) ListInstances(ctx context.Context, actor security.Subject, query InstanceListQuery) (InstancePage, error) {
	if err := s.checkTenant(actor); err != nil {
		return InstancePage{}, err
	}
	grant, err := security.Authorize(ctx, s.policy, actor, ActionInstanceList, Instance{TenantID: actor.Tenant})
	if err != nil {
		return InstancePage{}, err
	}
	return s.repository.ListPage(ctx, grant, query)
}

// FindInstance returns one instance after both collection and record checks.
func (s *Service) FindInstance(ctx context.Context, actor security.Subject, name string) (Instance, error) {
	_, instance, err := s.authorizedInstance(ctx, actor, ActionInstanceView, name)
	return instance, err
}

// ConnectQRCode starts QR-code pairing for an instance.
func (s *Service) ConnectQRCode(ctx context.Context, actor security.Subject, name string) (QRCodeConnectionResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionConnectionPair, name)
	if err != nil {
		return QRCodeConnectionResult{}, err
	}
	return s.connections.ConnectQRCode(ctx, grant, name)
}

// ConnectPhone starts phone-code pairing for an instance.
func (s *Service) ConnectPhone(ctx context.Context, actor security.Subject, name string, input PhonePairingInput) (PhonePairingResult, error) {
	if err := s.checkTenant(actor); err != nil {
		return PhonePairingResult{}, err
	}
	rawPhone := strings.TrimSpace(input.PhoneNumber)
	if rawPhone == "" || len(rawPhone) > 32 {
		return PhonePairingResult{}, ErrInvalidPhoneNumber
	}
	phone, err := internalwhatsapp.NormalizePhoneNumber(rawPhone)
	if err != nil {
		return PhonePairingResult{}, err
	}
	grant, _, err := s.authorizedInstance(ctx, actor, ActionConnectionPair, name)
	if err != nil {
		return PhonePairingResult{}, err
	}
	return s.connections.ConnectPhone(ctx, grant, name, phone)
}

// PasskeyChallenge requests the current Passkey pairing challenge.
func (s *Service) PasskeyChallenge(ctx context.Context, actor security.Subject, name string) (PasskeyChallengeResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionConnectionPair, name)
	if err != nil {
		return PasskeyChallengeResult{}, err
	}
	return s.connections.RequestPasskeyChallenge(ctx, grant, name)
}

// PasskeyAssertion submits an assertion for the active Passkey challenge.
func (s *Service) PasskeyAssertion(ctx context.Context, actor security.Subject, name string, input SubmitPasskeyAssertionRequest) (PasskeyAssertionResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionConnectionPair, name)
	if err != nil {
		return PasskeyAssertionResult{}, err
	}
	return s.connections.SubmitPasskeyAssertion(ctx, grant, name, input)
}

// ConnectionState returns the current connection state of an instance.
func (s *Service) ConnectionState(ctx context.Context, actor security.Subject, name string) (ConnectionStateResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionConnectionView, name)
	if err != nil {
		return ConnectionStateResult{}, err
	}
	return s.connections.ConnectionState(ctx, grant, name)
}

// Logout disconnects an instance and removes its active WhatsApp session.
func (s *Service) Logout(ctx context.Context, actor security.Subject, name string) (LogoutResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionConnectionLogout, name)
	if err != nil {
		return LogoutResult{}, err
	}
	return s.connections.Logout(ctx, grant, name)
}

// DeleteInstance deletes an instance and optionally forces dependency removal.
func (s *Service) DeleteInstance(ctx context.Context, actor security.Subject, name string, force bool) (DeleteResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionInstanceDelete, name)
	if err != nil {
		return DeleteResult{}, err
	}
	return s.connections.DeleteInstance(ctx, grant, name, force)
}

// SetWebhook creates or updates the webhook configuration for an instance.
func (s *Service) SetWebhook(ctx context.Context, actor security.Subject, name string, input WebhookSetInput) (Webhook, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionWebhookSet, name)
	if err != nil {
		return Webhook{}, err
	}
	return s.webhooks.Set(ctx, grant, name, input)
}

// FindWebhook returns the webhook configuration for an instance.
func (s *Service) FindWebhook(ctx context.Context, actor security.Subject, name string) (Webhook, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionWebhookView, name)
	if err != nil {
		return Webhook{}, err
	}
	return s.webhooks.Find(ctx, grant, name)
}

// SendText sends a text message through an instance.
func (s *Service) SendText(ctx context.Context, actor security.Subject, name string, input SendTextRequest) (SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionMessageSend, name)
	if err != nil {
		return SendResult{}, err
	}
	return s.messages.SendText(ctx, grant, name, input)
}

// SendLink sends a link message through an instance.
func (s *Service) SendLink(ctx context.Context, actor security.Subject, name string, input SendLinkRequest) (SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionMessageSend, name)
	if err != nil {
		return SendResult{}, err
	}
	return s.messages.SendLink(ctx, grant, name, input)
}

// SendMedia sends media described by a structured request.
func (s *Service) SendMedia(ctx context.Context, actor security.Subject, name string, input SendMediaRequest) (SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionMessageSend, name)
	if err != nil {
		return SendResult{}, err
	}
	return s.messages.SendMedia(ctx, grant, name, input)
}

// SendMediaFile sends media from a multipart upload.
func (s *Service) SendMediaFile(ctx context.Context, actor security.Subject, name, number string, file multipart.File, header *multipart.FileHeader, mediaType string, caption *string, options *MessageOptions) (SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionMessageSend, name)
	if err != nil {
		return SendResult{}, err
	}
	return s.messages.SendMediaFile(ctx, grant, name, number, file, header, mediaType, caption, options)
}

// SendAudio sends an audio message described by a structured request.
func (s *Service) SendAudio(ctx context.Context, actor security.Subject, name string, input SendWhatsAppAudioRequest) (SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionMessageSend, name)
	if err != nil {
		return SendResult{}, err
	}
	return s.messages.SendWhatsAppAudio(ctx, grant, name, input)
}

// SendAudioFile sends an audio message from a multipart upload.
func (s *Service) SendAudioFile(ctx context.Context, actor security.Subject, name, number string, file multipart.File, header *multipart.FileHeader, options *MessageOptions) (SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionMessageSend, name)
	if err != nil {
		return SendResult{}, err
	}
	return s.messages.SendWhatsAppAudioFile(ctx, grant, name, number, file, header, options)
}

// SendContact sends a contact card through an instance.
func (s *Service) SendContact(ctx context.Context, actor security.Subject, name string, input SendContactRequest) (SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionMessageSend, name)
	if err != nil {
		return SendResult{}, err
	}
	return s.messages.SendContact(ctx, grant, name, input)
}

// SendLocation sends a location message through an instance.
func (s *Service) SendLocation(ctx context.Context, actor security.Subject, name string, input SendLocationRequest) (SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionMessageSend, name)
	if err != nil {
		return SendResult{}, err
	}
	return s.messages.SendLocation(ctx, grant, name, input)
}

// SendReaction sends a reaction to an existing message.
func (s *Service) SendReaction(ctx context.Context, actor security.Subject, name string, input SendReactionRequest) (SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionMessageSend, name)
	if err != nil {
		return SendResult{}, err
	}
	return s.messages.SendReaction(ctx, grant, name, input)
}

// CheckContacts reports which supplied contacts are registered on WhatsApp.
func (s *Service) CheckContacts(ctx context.Context, actor security.Subject, name string, input WhatsAppNumbersRequest) ([]WhatsAppNumberResponse, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionContactCheck, name)
	if err != nil {
		return nil, err
	}
	return s.chats.CheckWhatsAppNumbers(ctx, grant, name, input)
}

// FindMessages returns messages matching the supplied query.
func (s *Service) FindMessages(ctx context.Context, actor security.Subject, name string, input FindMessagesRequest) (MessageListResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionMessageList, name)
	if err != nil {
		return MessageListResult{}, err
	}
	return s.chats.FindMessages(ctx, grant, name, input)
}

// ReadMessages marks the selected messages as read.
func (s *Service) ReadMessages(ctx context.Context, actor security.Subject, name string, input ReadMessagesRequest) error {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionMessageRead, name)
	if err != nil {
		return err
	}
	return s.chats.ReadMessages(ctx, grant, name, input)
}

// ArchiveChat changes the archive state of a chat.
func (s *Service) ArchiveChat(ctx context.Context, actor security.Subject, name string, input ArchiveChatRequest) error {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionChatArchive, name)
	if err != nil {
		return err
	}
	return s.chats.ArchiveChat(ctx, grant, name, input)
}

// DeleteMessage deletes a message for every participant.
func (s *Service) DeleteMessage(ctx context.Context, actor security.Subject, name string, id int64) error {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionMessageDelete, name)
	if err != nil {
		return err
	}
	return s.chats.DeleteMessageForEveryone(ctx, grant, name, id)
}

// ProfilePicture returns the profile-picture URL for a contact or group.
func (s *Service) ProfilePicture(ctx context.Context, actor security.Subject, name string, input FetchProfilePictureRequest) (*string, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionProfilePictureView, name)
	if err != nil {
		return nil, err
	}
	return s.chats.FetchProfilePicture(ctx, grant, name, input)
}

// RejectCall rejects an incoming WhatsApp call.
func (s *Service) RejectCall(ctx context.Context, actor security.Subject, name string, input RejectCallRequest) error {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionCallReject, name)
	if err != nil {
		return err
	}
	return s.chats.RejectCall(ctx, grant, name, input)
}

// EditMessage replaces the content of a sent message.
func (s *Service) EditMessage(ctx context.Context, actor security.Subject, name string, input EditMessageRequest) (Message, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionMessageEdit, name)
	if err != nil {
		return Message{}, err
	}
	return s.chats.EditMessage(ctx, grant, name, input)
}

// MediaData downloads media attached to a message.
func (s *Service) MediaData(ctx context.Context, actor security.Subject, name string, input MediaDataRequest) (MediaDownloadResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionMessageMediaDownload, name)
	if err != nil {
		return MediaDownloadResult{}, err
	}
	return s.chats.MediaData(ctx, grant, name, input)
}

// CreateGroup creates a WhatsApp group.
func (s *Service) CreateGroup(ctx context.Context, actor security.Subject, name string, input GroupCreateRequest) (GroupInfoResponse, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionGroupCreate, name)
	if err != nil {
		return GroupInfoResponse{}, err
	}
	return s.groups.Create(ctx, grant, name, input)
}

// UpdateGroupPicture replaces a group's profile picture.
func (s *Service) UpdateGroupPicture(ctx context.Context, actor security.Subject, name string, input GroupUpdatePictureRequest) (GroupInfoResponse, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionGroupPictureUpdate, name)
	if err != nil {
		return GroupInfoResponse{}, err
	}
	return s.groups.UpdatePicture(ctx, grant, name, input)
}

// GroupInviteCode returns the active invite code for a group.
func (s *Service) GroupInviteCode(ctx context.Context, actor security.Subject, name, groupJID string) (GroupInviteCodeResponse, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionGroupInviteView, name)
	if err != nil {
		return GroupInviteCodeResponse{}, err
	}
	return s.groups.InviteCode(ctx, grant, name, groupJID)
}

// RevokeGroupInvite revokes the active invite code for a group.
func (s *Service) RevokeGroupInvite(ctx context.Context, actor security.Subject, name, groupJID string) error {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionGroupInviteRevoke, name)
	if err != nil {
		return err
	}
	return s.groups.RevokeInviteCode(ctx, grant, name, groupJID)
}

// UpdateGroupParticipants applies a participant membership or role change.
func (s *Service) UpdateGroupParticipants(ctx context.Context, actor security.Subject, name, groupJID string, input GroupUpdateParticipantRequest) error {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionGroupParticipantUpdate, name)
	if err != nil {
		return err
	}
	return s.groups.UpdateParticipant(ctx, grant, name, groupJID, input)
}

// LeaveGroup removes the connected account from a group.
func (s *Service) LeaveGroup(ctx context.Context, actor security.Subject, name, groupJID string) error {
	grant, _, err := s.authorizedInstance(ctx, actor, ActionGroupLeave, name)
	if err != nil {
		return err
	}
	return s.groups.Leave(ctx, grant, name, groupJID)
}

func (s *Service) authorizedInstance(ctx context.Context, actor security.Subject, action security.Action, rawName string) (security.Grant, Instance, error) {
	if err := s.checkTenant(actor); err != nil {
		return security.Grant{}, Instance{}, err
	}
	name := strings.TrimSpace(rawName)
	if name == "" || len(name) > 255 {
		return security.Grant{}, Instance{}, internalrepo.ErrInvalidInput
	}
	probe := Instance{TenantID: actor.Tenant}
	grant, err := security.Authorize(ctx, s.policy, actor, action, probe)
	if err != nil {
		return security.Grant{}, Instance{}, err
	}
	item, err := s.instances.FindByName(ctx, grant, name)
	if err != nil {
		return security.Grant{}, Instance{}, err
	}
	instance := instanceFromInternal(item.Instance)
	grant, err = security.Authorize(ctx, s.policy, actor, action, instance)
	if err != nil {
		return security.Grant{}, Instance{}, err
	}
	return grant, instance, nil
}

func (s *Service) checkTenant(actor security.Subject) error {
	if actor.Tenant == "" || actor.Tenant != s.tenant {
		return fmt.Errorf("%w: subject is outside the WhatsApp module tenant", security.ErrForbidden)
	}
	return nil
}

func validateJSONObject(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("%w: externalAttributes", internalrepo.ErrInvalidJSON)
	}
	if _, ok := value.(map[string]any); !ok {
		return internalrepo.ErrInvalidJSON
	}
	return nil
}
