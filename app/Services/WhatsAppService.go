// Package services holds the authorized business rules of the WhatsApp module.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/str"

	enums "github.com/hyz-is/arandu-whatsapp/app/Enums"
	requests "github.com/hyz-is/arandu-whatsapp/app/Http/Requests"
	models "github.com/hyz-is/arandu-whatsapp/app/Models"
	policies "github.com/hyz-is/arandu-whatsapp/app/Policies"
	repositories "github.com/hyz-is/arandu-whatsapp/app/Repositories"
	"github.com/hyz-is/arandu-whatsapp/internal/chat"
	"github.com/hyz-is/arandu-whatsapp/internal/group"
	"github.com/hyz-is/arandu-whatsapp/internal/message"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
	internalwhatsapp "github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
)

// Service is the single authorized entry point used by HTTP handlers.
type Service struct {
	tenant      string
	repository  *repositories.InstanceRepository
	policy      policies.InstancePolicy
	connections internalwhatsapp.ConnectionService
	messages    message.Service
	chats       chat.Service
	groups      group.Service
	webhooks    webhooksvc.Service
}

// NewService returns the authorized WhatsApp domain service.
func NewService(
	tenant string,
	repository *repositories.InstanceRepository,
	policy policies.InstancePolicy,
	connections internalwhatsapp.ConnectionService,
	messages message.Service,
	chats chat.Service,
	groups group.Service,
	webhooks webhooksvc.Service,
) *Service {
	return &Service{
		tenant: tenant, repository: repository, policy: policy,
		connections: connections, messages: messages, chats: chats,
		groups: groups, webhooks: webhooks,
	}
}

// CreateInstance validates, authorizes and creates one instance.
func (s *Service) CreateInstance(ctx context.Context, actor security.Subject, input requests.CreateInstance) (models.Instance, error) {
	if err := s.checkTenant(actor); err != nil {
		return models.Instance{}, err
	}
	name := ""
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
	}
	if name == "" {
		name = "instance-" + str.UUID()
	}
	if len(name) > 255 {
		return models.Instance{}, models.ErrInvalidInput
	}
	if input.Description != nil {
		value := strings.TrimSpace(*input.Description)
		if len(value) > 255 {
			return models.Instance{}, models.ErrInvalidInput
		}
		input.Description = &value
	}
	if err := validateJSONObject(input.ExternalAttributes); err != nil {
		return models.Instance{}, err
	}
	candidate := models.Instance{TenantID: actor.Tenant, Name: name, Description: input.Description, ExternalAttributes: input.ExternalAttributes}
	grant, err := security.Authorize(ctx, s.policy, actor, enums.ActionInstanceCreate, candidate)
	if err != nil {
		return models.Instance{}, err
	}
	return s.repository.Create(ctx, grant, candidate)
}

// ListInstances returns one page of instances from the actor tenant.
func (s *Service) ListInstances(ctx context.Context, actor security.Subject, query models.InstanceListQuery) (models.InstancePage, error) {
	if err := s.checkTenant(actor); err != nil {
		return models.InstancePage{}, err
	}
	grant, err := security.Authorize(ctx, s.policy, actor, enums.ActionInstanceList, models.Instance{TenantID: actor.Tenant})
	if err != nil {
		return models.InstancePage{}, err
	}
	return s.repository.ListPage(ctx, grant, query)
}

// FindInstance returns one instance after both collection and record checks.
func (s *Service) FindInstance(ctx context.Context, actor security.Subject, name string) (models.Instance, error) {
	_, instance, err := s.authorizedInstance(ctx, actor, enums.ActionInstanceView, name)
	return instance, err
}

// ConnectQRCode starts QR-code pairing for an instance.
func (s *Service) ConnectQRCode(ctx context.Context, actor security.Subject, name string) (models.QRCodeConnectionResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionConnectionPair, name)
	if err != nil {
		return models.QRCodeConnectionResult{}, err
	}
	return s.connections.ConnectQRCode(ctx, grant, name)
}

// ConnectPhone starts phone-code pairing for an instance.
func (s *Service) ConnectPhone(ctx context.Context, actor security.Subject, name string, input requests.PairPhone) (models.PhonePairingResult, error) {
	if err := s.checkTenant(actor); err != nil {
		return models.PhonePairingResult{}, err
	}
	rawPhone := strings.TrimSpace(input.PhoneNumber)
	if rawPhone == "" || len(rawPhone) > 32 {
		return models.PhonePairingResult{}, internalwhatsapp.ErrInvalidPhoneNumber
	}
	phone, err := internalwhatsapp.NormalizePhoneNumber(rawPhone)
	if err != nil {
		return models.PhonePairingResult{}, err
	}
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionConnectionPair, name)
	if err != nil {
		return models.PhonePairingResult{}, err
	}
	return s.connections.ConnectPhone(ctx, grant, name, phone)
}

// PasskeyChallenge requests the current Passkey pairing challenge.
func (s *Service) PasskeyChallenge(ctx context.Context, actor security.Subject, name string) (models.PasskeyChallengeResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionConnectionPair, name)
	if err != nil {
		return models.PasskeyChallengeResult{}, err
	}
	return s.connections.RequestPasskeyChallenge(ctx, grant, name)
}

// PasskeyAssertion submits an assertion for the active Passkey challenge.
func (s *Service) PasskeyAssertion(ctx context.Context, actor security.Subject, name string, input requests.SubmitPasskeyAssertion) (models.PasskeyAssertionResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionConnectionPair, name)
	if err != nil {
		return models.PasskeyAssertionResult{}, err
	}
	return s.connections.SubmitPasskeyAssertion(ctx, grant, name, input)
}

// ConnectionState returns the current connection state of an instance.
func (s *Service) ConnectionState(ctx context.Context, actor security.Subject, name string) (models.ConnectionStateResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionConnectionView, name)
	if err != nil {
		return models.ConnectionStateResult{}, err
	}
	return s.connections.ConnectionState(ctx, grant, name)
}

// Logout disconnects an instance and removes its active WhatsApp session.
func (s *Service) Logout(ctx context.Context, actor security.Subject, name string) (models.LogoutResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionConnectionLogout, name)
	if err != nil {
		return models.LogoutResult{}, err
	}
	return s.connections.Logout(ctx, grant, name)
}

// DeleteInstance deletes an instance and optionally forces dependency removal.
func (s *Service) DeleteInstance(ctx context.Context, actor security.Subject, name string, force bool) (models.DeleteResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionInstanceDelete, name)
	if err != nil {
		return models.DeleteResult{}, err
	}
	return s.connections.DeleteInstance(ctx, grant, name, force)
}

// SetWebhook creates or updates the webhook configuration for an instance.
func (s *Service) SetWebhook(ctx context.Context, actor security.Subject, name string, input requests.SetWebhook) (models.Webhook, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionWebhookSet, name)
	if err != nil {
		return models.Webhook{}, err
	}
	item, err := s.webhooks.Set(ctx, grant, name, input)
	if err != nil {
		return models.Webhook{}, err
	}
	return repositories.WebhookFromDatabase(item), nil
}

// FindWebhook returns the webhook configuration for an instance.
func (s *Service) FindWebhook(ctx context.Context, actor security.Subject, name string) (models.Webhook, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionWebhookView, name)
	if err != nil {
		return models.Webhook{}, err
	}
	item, err := s.webhooks.Find(ctx, grant, name)
	if err != nil {
		return models.Webhook{}, err
	}
	return repositories.WebhookFromDatabase(item), nil
}

// SendText sends a text message through an instance.
func (s *Service) SendText(ctx context.Context, actor security.Subject, name string, input requests.SendText) (models.SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionMessageSend, name)
	if err != nil {
		return models.SendResult{}, err
	}
	return s.messages.SendText(ctx, grant, name, input)
}

// SendLink sends a link message through an instance.
func (s *Service) SendLink(ctx context.Context, actor security.Subject, name string, input requests.SendLink) (models.SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionMessageSend, name)
	if err != nil {
		return models.SendResult{}, err
	}
	return s.messages.SendLink(ctx, grant, name, input)
}

// SendMedia sends media described by a structured request.
func (s *Service) SendMedia(ctx context.Context, actor security.Subject, name string, input requests.SendMedia) (models.SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionMessageSend, name)
	if err != nil {
		return models.SendResult{}, err
	}
	return s.messages.SendMedia(ctx, grant, name, input)
}

// SendMediaFile sends media from a multipart upload.
func (s *Service) SendMediaFile(ctx context.Context, actor security.Subject, name, number string, file multipart.File, header *multipart.FileHeader, mediaType string, caption *string, options *requests.MessageOptions) (models.SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionMessageSend, name)
	if err != nil {
		return models.SendResult{}, err
	}
	return s.messages.SendMediaFile(ctx, grant, name, number, file, header, mediaType, caption, options)
}

// SendAudio sends an audio message described by a structured request.
func (s *Service) SendAudio(ctx context.Context, actor security.Subject, name string, input requests.SendWhatsAppAudio) (models.SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionMessageSend, name)
	if err != nil {
		return models.SendResult{}, err
	}
	return s.messages.SendWhatsAppAudio(ctx, grant, name, input)
}

// SendAudioFile sends an audio message from a multipart upload.
func (s *Service) SendAudioFile(ctx context.Context, actor security.Subject, name, number string, file multipart.File, header *multipart.FileHeader, options *requests.MessageOptions) (models.SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionMessageSend, name)
	if err != nil {
		return models.SendResult{}, err
	}
	return s.messages.SendWhatsAppAudioFile(ctx, grant, name, number, file, header, options)
}

// SendContact sends a contact card through an instance.
func (s *Service) SendContact(ctx context.Context, actor security.Subject, name string, input requests.SendContact) (models.SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionMessageSend, name)
	if err != nil {
		return models.SendResult{}, err
	}
	return s.messages.SendContact(ctx, grant, name, input)
}

// SendLocation sends a location message through an instance.
func (s *Service) SendLocation(ctx context.Context, actor security.Subject, name string, input requests.SendLocation) (models.SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionMessageSend, name)
	if err != nil {
		return models.SendResult{}, err
	}
	return s.messages.SendLocation(ctx, grant, name, input)
}

// SendReaction sends a reaction to an existing message.
func (s *Service) SendReaction(ctx context.Context, actor security.Subject, name string, input requests.SendReaction) (models.SendResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionMessageSend, name)
	if err != nil {
		return models.SendResult{}, err
	}
	return s.messages.SendReaction(ctx, grant, name, input)
}

// CheckContacts reports which supplied contacts are registered on WhatsApp.
func (s *Service) CheckContacts(ctx context.Context, actor security.Subject, name string, input requests.CheckWhatsAppNumbers) ([]models.WhatsAppNumber, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionContactCheck, name)
	if err != nil {
		return nil, err
	}
	return s.chats.CheckWhatsAppNumbers(ctx, grant, name, input)
}

// FindMessages returns messages matching the supplied query.
func (s *Service) FindMessages(ctx context.Context, actor security.Subject, name string, input requests.FindMessages) (models.MessageListResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionMessageList, name)
	if err != nil {
		return models.MessageListResult{}, err
	}
	result, err := s.chats.FindMessages(ctx, grant, name, input)
	if err != nil {
		return models.MessageListResult{}, err
	}
	return repositories.MessageListFromDatabase(result), nil
}

// ReadMessages marks the selected messages as read.
func (s *Service) ReadMessages(ctx context.Context, actor security.Subject, name string, input requests.ReadMessages) error {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionMessageRead, name)
	if err != nil {
		return err
	}
	return s.chats.ReadMessages(ctx, grant, name, input)
}

// ArchiveChat changes the archive state of a chat.
func (s *Service) ArchiveChat(ctx context.Context, actor security.Subject, name string, input requests.ArchiveChat) error {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionChatArchive, name)
	if err != nil {
		return err
	}
	return s.chats.ArchiveChat(ctx, grant, name, input)
}

// DeleteMessage deletes a message for every participant.
func (s *Service) DeleteMessage(ctx context.Context, actor security.Subject, name string, id int64) error {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionMessageDelete, name)
	if err != nil {
		return err
	}
	return s.chats.DeleteMessageForEveryone(ctx, grant, name, id)
}

// ProfilePicture returns the profile-picture URL for a contact or group.
func (s *Service) ProfilePicture(ctx context.Context, actor security.Subject, name string, input requests.FetchProfilePicture) (*string, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionProfilePictureView, name)
	if err != nil {
		return nil, err
	}
	return s.chats.FetchProfilePicture(ctx, grant, name, input)
}

// RejectCall rejects an incoming WhatsApp call.
func (s *Service) RejectCall(ctx context.Context, actor security.Subject, name string, input requests.RejectCall) error {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionCallReject, name)
	if err != nil {
		return err
	}
	return s.chats.RejectCall(ctx, grant, name, input)
}

// EditMessage replaces the content of a sent message.
func (s *Service) EditMessage(ctx context.Context, actor security.Subject, name string, input requests.EditMessage) (models.Message, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionMessageEdit, name)
	if err != nil {
		return models.Message{}, err
	}
	message, err := s.chats.EditMessage(ctx, grant, name, input)
	if err != nil {
		return models.Message{}, err
	}
	return repositories.MessageFromDatabase(message), nil
}

// MediaData downloads media attached to a message.
func (s *Service) MediaData(ctx context.Context, actor security.Subject, name string, input requests.DownloadMedia) (models.MediaDownloadResult, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionMessageMediaDownload, name)
	if err != nil {
		return models.MediaDownloadResult{}, err
	}
	return s.chats.MediaData(ctx, grant, name, input)
}

// CreateGroup creates a WhatsApp group.
func (s *Service) CreateGroup(ctx context.Context, actor security.Subject, name string, input requests.CreateGroup) (models.GroupInfo, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionGroupCreate, name)
	if err != nil {
		return models.GroupInfo{}, err
	}
	return s.groups.Create(ctx, grant, name, input)
}

// UpdateGroupPicture replaces a group's profile picture.
func (s *Service) UpdateGroupPicture(ctx context.Context, actor security.Subject, name string, input requests.UpdateGroupPicture) (models.GroupInfo, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionGroupPictureUpdate, name)
	if err != nil {
		return models.GroupInfo{}, err
	}
	return s.groups.UpdatePicture(ctx, grant, name, input)
}

// GroupInviteCode returns the active invite code for a group.
func (s *Service) GroupInviteCode(ctx context.Context, actor security.Subject, name, groupJID string) (models.GroupInviteCode, error) {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionGroupInviteView, name)
	if err != nil {
		return models.GroupInviteCode{}, err
	}
	return s.groups.InviteCode(ctx, grant, name, groupJID)
}

// RevokeGroupInvite revokes the active invite code for a group.
func (s *Service) RevokeGroupInvite(ctx context.Context, actor security.Subject, name, groupJID string) error {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionGroupInviteRevoke, name)
	if err != nil {
		return err
	}
	return s.groups.RevokeInviteCode(ctx, grant, name, groupJID)
}

// UpdateGroupParticipants applies a participant membership or role change.
func (s *Service) UpdateGroupParticipants(ctx context.Context, actor security.Subject, name, groupJID string, input requests.UpdateGroupParticipant) error {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionGroupParticipantUpdate, name)
	if err != nil {
		return err
	}
	return s.groups.UpdateParticipant(ctx, grant, name, groupJID, input)
}

// LeaveGroup removes the connected account from a group.
func (s *Service) LeaveGroup(ctx context.Context, actor security.Subject, name, groupJID string) error {
	grant, _, err := s.authorizedInstance(ctx, actor, enums.ActionGroupLeave, name)
	if err != nil {
		return err
	}
	return s.groups.Leave(ctx, grant, name, groupJID)
}

func (s *Service) authorizedInstance(ctx context.Context, actor security.Subject, action security.Action, rawName string) (security.Grant, models.Instance, error) {
	if err := s.checkTenant(actor); err != nil {
		return security.Grant{}, models.Instance{}, err
	}
	name := strings.TrimSpace(rawName)
	if name == "" || len(name) > 255 {
		return security.Grant{}, models.Instance{}, models.ErrInvalidInput
	}
	probe := models.Instance{TenantID: actor.Tenant}
	grant, err := security.Authorize(ctx, s.policy, actor, action, probe)
	if err != nil {
		return security.Grant{}, models.Instance{}, err
	}
	instance, err := s.repository.ResolveByName(ctx, grant, name)
	if err != nil {
		return security.Grant{}, models.Instance{}, err
	}
	grant, err = security.Authorize(ctx, s.policy, actor, action, instance)
	if err != nil {
		return security.Grant{}, models.Instance{}, err
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
		return fmt.Errorf("%w: externalAttributes", models.ErrInvalidJSON)
	}
	if _, ok := value.(map[string]any); !ok {
		return models.ErrInvalidJSON
	}
	return nil
}
