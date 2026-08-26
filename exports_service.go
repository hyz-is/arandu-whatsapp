package whatsapp

import (
	"context"
	"mime/multipart"

	"github.com/arandu-io/framework/security"

	models "github.com/hyz-is/arandu-whatsapp/app/Models"
	services "github.com/hyz-is/arandu-whatsapp/app/Services"
	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

// Service is the nominal public facade over the native application service.
type Service struct{ native *services.Service }

func wrapService(native *services.Service) *Service {
	if native == nil {
		return nil
	}
	return &Service{native: native}
}

// CreateInstance validates, authorizes and creates one instance.
func (s *Service) CreateInstance(ctx context.Context, actor security.Subject, input CreateInstanceInput) (Instance, error) {
	instance, err := s.native.CreateInstance(ctx, actor, createInstanceInputToRequest(input))
	if err != nil {
		return Instance{}, err
	}
	return instanceFromModel(instance), nil
}

// ListInstances returns one page of instances from the actor tenant.
func (s *Service) ListInstances(ctx context.Context, actor security.Subject, query InstanceListQuery) (InstancePage, error) {
	page, err := s.native.ListInstances(ctx, actor, instanceListQueryToModel(query))
	if err != nil {
		return InstancePage{}, err
	}
	return instancePageFromModel(page), nil
}

// FindInstance returns one instance after both collection and record checks.
func (s *Service) FindInstance(ctx context.Context, actor security.Subject, name string) (Instance, error) {
	instance, err := s.native.FindInstance(ctx, actor, name)
	if err != nil {
		return Instance{}, err
	}
	return instanceFromModel(instance), nil
}

// ConnectQRCode starts QR-code pairing for an instance.
func (s *Service) ConnectQRCode(ctx context.Context, actor security.Subject, name string) (QRCodeConnectionResult, error) {
	return s.native.ConnectQRCode(ctx, actor, name)
}

// ConnectPhone starts phone-code pairing for an instance.
func (s *Service) ConnectPhone(ctx context.Context, actor security.Subject, name string, input PhonePairingInput) (PhonePairingResult, error) {
	return s.native.ConnectPhone(ctx, actor, name, phonePairingInputToRequest(input))
}

// PasskeyChallenge requests the current Passkey pairing challenge.
func (s *Service) PasskeyChallenge(ctx context.Context, actor security.Subject, name string) (PasskeyChallengeResult, error) {
	return s.native.PasskeyChallenge(ctx, actor, name)
}

// PasskeyAssertion submits an assertion for the active Passkey challenge.
func (s *Service) PasskeyAssertion(ctx context.Context, actor security.Subject, name string, input SubmitPasskeyAssertionRequest) (PasskeyAssertionResult, error) {
	return s.native.PasskeyAssertion(ctx, actor, name, input)
}

// ConnectionState returns the current connection state of an instance.
func (s *Service) ConnectionState(ctx context.Context, actor security.Subject, name string) (ConnectionStateResult, error) {
	return s.native.ConnectionState(ctx, actor, name)
}

// Logout disconnects an instance and removes its active WhatsApp session.
func (s *Service) Logout(ctx context.Context, actor security.Subject, name string) (LogoutResult, error) {
	return s.native.Logout(ctx, actor, name)
}

// DeleteInstance deletes an instance and optionally forces dependency removal.
func (s *Service) DeleteInstance(ctx context.Context, actor security.Subject, name string, force bool) (DeleteResult, error) {
	return s.native.DeleteInstance(ctx, actor, name, force)
}

// SetWebhook creates or updates the webhook configuration for an instance.
func (s *Service) SetWebhook(ctx context.Context, actor security.Subject, name string, input WebhookSetInput) (Webhook, error) {
	webhook, err := s.native.SetWebhook(ctx, actor, name, input)
	if err != nil {
		return Webhook{}, err
	}
	return webhookFromModel(webhook), nil
}

// FindWebhook returns the webhook configuration for an instance.
func (s *Service) FindWebhook(ctx context.Context, actor security.Subject, name string) (Webhook, error) {
	webhook, err := s.native.FindWebhook(ctx, actor, name)
	if err != nil {
		return Webhook{}, err
	}
	return webhookFromModel(webhook), nil
}

// SendText sends a text message through an instance.
func (s *Service) SendText(ctx context.Context, actor security.Subject, name string, input SendTextRequest) (SendResult, error) {
	return s.native.SendText(ctx, actor, name, input)
}

// SendLink sends a link message through an instance.
func (s *Service) SendLink(ctx context.Context, actor security.Subject, name string, input SendLinkRequest) (SendResult, error) {
	return s.native.SendLink(ctx, actor, name, input)
}

// SendMedia sends media described by a structured request.
func (s *Service) SendMedia(ctx context.Context, actor security.Subject, name string, input SendMediaRequest) (SendResult, error) {
	return s.native.SendMedia(ctx, actor, name, input)
}

// SendMediaFile sends media from a multipart upload.
func (s *Service) SendMediaFile(ctx context.Context, actor security.Subject, name, number string, file multipart.File, header *multipart.FileHeader, mediaType string, caption *string, options *MessageOptions) (SendResult, error) {
	return s.native.SendMediaFile(ctx, actor, name, number, file, header, mediaType, caption, options)
}

// SendAudio sends an audio message described by a structured request.
func (s *Service) SendAudio(ctx context.Context, actor security.Subject, name string, input SendWhatsAppAudioRequest) (SendResult, error) {
	return s.native.SendAudio(ctx, actor, name, input)
}

// SendAudioFile sends an audio message from a multipart upload.
func (s *Service) SendAudioFile(ctx context.Context, actor security.Subject, name, number string, file multipart.File, header *multipart.FileHeader, options *MessageOptions) (SendResult, error) {
	return s.native.SendAudioFile(ctx, actor, name, number, file, header, options)
}

// SendContact sends a contact card through an instance.
func (s *Service) SendContact(ctx context.Context, actor security.Subject, name string, input SendContactRequest) (SendResult, error) {
	return s.native.SendContact(ctx, actor, name, input)
}

// SendLocation sends a location message through an instance.
func (s *Service) SendLocation(ctx context.Context, actor security.Subject, name string, input SendLocationRequest) (SendResult, error) {
	return s.native.SendLocation(ctx, actor, name, input)
}

// SendReaction sends a reaction to an existing message.
func (s *Service) SendReaction(ctx context.Context, actor security.Subject, name string, input SendReactionRequest) (SendResult, error) {
	return s.native.SendReaction(ctx, actor, name, input)
}

// CheckContacts reports which supplied contacts are registered on WhatsApp.
func (s *Service) CheckContacts(ctx context.Context, actor security.Subject, name string, input WhatsAppNumbersRequest) ([]WhatsAppNumberResponse, error) {
	return s.native.CheckContacts(ctx, actor, name, input)
}

// FindMessages returns messages matching the supplied query.
func (s *Service) FindMessages(ctx context.Context, actor security.Subject, name string, input FindMessagesRequest) (MessageListResult, error) {
	result, err := s.native.FindMessages(ctx, actor, name, input)
	if err != nil {
		return MessageListResult{}, err
	}
	return messageListResultFromModel(result), nil
}

// ReadMessages marks the selected messages as read.
func (s *Service) ReadMessages(ctx context.Context, actor security.Subject, name string, input ReadMessagesRequest) error {
	return s.native.ReadMessages(ctx, actor, name, input)
}

// ArchiveChat changes the archive state of a chat.
func (s *Service) ArchiveChat(ctx context.Context, actor security.Subject, name string, input ArchiveChatRequest) error {
	return s.native.ArchiveChat(ctx, actor, name, input)
}

// DeleteMessage deletes a message for every participant.
func (s *Service) DeleteMessage(ctx context.Context, actor security.Subject, name string, id int64) error {
	return s.native.DeleteMessage(ctx, actor, name, id)
}

// ProfilePicture returns the profile-picture URL for a contact or group.
func (s *Service) ProfilePicture(ctx context.Context, actor security.Subject, name string, input FetchProfilePictureRequest) (*string, error) {
	return s.native.ProfilePicture(ctx, actor, name, input)
}

// RejectCall rejects an incoming WhatsApp call.
func (s *Service) RejectCall(ctx context.Context, actor security.Subject, name string, input RejectCallRequest) error {
	return s.native.RejectCall(ctx, actor, name, input)
}

// EditMessage replaces the content of a sent message.
func (s *Service) EditMessage(ctx context.Context, actor security.Subject, name string, input EditMessageRequest) (Message, error) {
	message, err := s.native.EditMessage(ctx, actor, name, input)
	if err != nil {
		return Message{}, err
	}
	return messageFromModel(message), nil
}

// MediaData downloads media attached to a message.
func (s *Service) MediaData(ctx context.Context, actor security.Subject, name string, input MediaDataRequest) (MediaDownloadResult, error) {
	return s.native.MediaData(ctx, actor, name, input)
}

// CreateGroup creates a WhatsApp group.
func (s *Service) CreateGroup(ctx context.Context, actor security.Subject, name string, input GroupCreateRequest) (GroupInfoResponse, error) {
	return s.native.CreateGroup(ctx, actor, name, input)
}

// UpdateGroupPicture replaces a group profile picture.
func (s *Service) UpdateGroupPicture(ctx context.Context, actor security.Subject, name string, input GroupUpdatePictureRequest) (GroupInfoResponse, error) {
	return s.native.UpdateGroupPicture(ctx, actor, name, input)
}

// GroupInviteCode returns the active invite code for a group.
func (s *Service) GroupInviteCode(ctx context.Context, actor security.Subject, name, groupJID string) (GroupInviteCodeResponse, error) {
	return s.native.GroupInviteCode(ctx, actor, name, groupJID)
}

// RevokeGroupInvite revokes the active invite code for a group.
func (s *Service) RevokeGroupInvite(ctx context.Context, actor security.Subject, name, groupJID string) error {
	return s.native.RevokeGroupInvite(ctx, actor, name, groupJID)
}

// UpdateGroupParticipants applies a participant membership or role change.
func (s *Service) UpdateGroupParticipants(ctx context.Context, actor security.Subject, name, groupJID string, input GroupUpdateParticipantRequest) error {
	return s.native.UpdateGroupParticipants(ctx, actor, name, groupJID, input)
}

// LeaveGroup removes the connected account from a group.
func (s *Service) LeaveGroup(ctx context.Context, actor security.Subject, name, groupJID string) error {
	return s.native.LeaveGroup(ctx, actor, name, groupJID)
}

func webhookFromModel(webhook models.Webhook) dbtypes.Webhook {
	return dbtypes.Webhook{ID: webhook.ID, URL: webhook.URL, Enabled: webhook.Enabled,
		Events: webhook.Events, CreatedAt: webhook.CreatedAt, UpdatedAt: webhook.UpdatedAt, InstanceID: webhook.InstanceID}
}

func messageFromModel(message models.Message) dbtypes.Message {
	return dbtypes.Message{
		ID: message.ID, KeyID: message.KeyID, KeyRemoteJid: message.KeyRemoteJid, KeyLid: message.KeyLid,
		KeyFromMe: message.KeyFromMe, KeyParticipant: message.KeyParticipant, KeyParticipantLid: message.KeyParticipantLid,
		PushName: message.PushName, MessageType: message.MessageType, Content: message.Content,
		MessageTimestamp: message.MessageTimestamp, Device: dbtypes.DeviceMessage(message.Device), IsGroup: message.IsGroup,
		InstanceID: message.InstanceID, Metadata: message.Metadata, ExternalAttributes: message.ExternalAttributes,
	}
}

func messageListResultFromModel(result models.MessageListResult) dbtypes.MessageListResult {
	records := make([]dbtypes.MessageWithUpdates, len(result.Messages.Records))
	if result.Messages.Records == nil {
		records = nil
	}
	for index, record := range result.Messages.Records {
		updates := make([]dbtypes.MessageUpdateSummary, len(record.MessageUpdate))
		if record.MessageUpdate == nil {
			updates = nil
		}
		for updateIndex, update := range record.MessageUpdate {
			updates[updateIndex] = dbtypes.MessageUpdateSummary{Status: update.Status, DateTime: update.DateTime}
		}
		records[index] = dbtypes.MessageWithUpdates{Message: messageFromModel(record.Message), MessageUpdate: updates}
	}
	return dbtypes.MessageListResult{Messages: dbtypes.MessagePage{
		Total: result.Messages.Total, Pages: result.Messages.Pages,
		CurrentPage: result.Messages.CurrentPage, Records: records,
	}}
}
