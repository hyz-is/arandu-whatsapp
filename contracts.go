package whatsapp

import (
	"github.com/hyz-is/arandu-whatsapp/internal/chat"
	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
	"github.com/hyz-is/arandu-whatsapp/internal/group"
	httprequest "github.com/hyz-is/arandu-whatsapp/internal/http/request"
	"github.com/hyz-is/arandu-whatsapp/internal/message"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
	internalwhatsapp "github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
)

// WebhookQueueName is the native queue the host worker and queue health module
// must monitor for webhook deliveries.
const WebhookQueueName = webhooksvc.WebhookQueueName

// MessageQueueName is the native queue the host worker and queue health module
// must monitor for durable mention-all sends.
const MessageQueueName = message.MessageQueueName

// QRCodeConnectionResult describes the outcome of QR-code pairing.
type QRCodeConnectionResult = internalwhatsapp.QRCodeConnectionResult

// PhonePairingResult describes the outcome of phone-code pairing.
type PhonePairingResult = internalwhatsapp.PhonePairingResult

// PasskeyChallengeResult describes a Passkey pairing challenge.
type PasskeyChallengeResult = internalwhatsapp.PasskeyChallengeResult

// PasskeyPairingState identifies a stage in Passkey pairing.
type PasskeyPairingState = internalwhatsapp.PasskeyPairingState

// Passkey pairing states describe the complete pairing lifecycle.
const (
	PasskeyStateIdle                 = internalwhatsapp.PasskeyStateIdle
	PasskeyStateFetchingChallenge    = internalwhatsapp.PasskeyStateFetchingChallenge
	PasskeyStateAwaitingAssertion    = internalwhatsapp.PasskeyStateAwaitingAssertion
	PasskeyStateSubmittingAssertion  = internalwhatsapp.PasskeyStateSubmittingAssertion
	PasskeyStateAwaitingConfirmation = internalwhatsapp.PasskeyStateAwaitingConfirmation
	PasskeyStateConfirmationSent     = internalwhatsapp.PasskeyStateConfirmationSent
	PasskeyStateCompleted            = internalwhatsapp.PasskeyStateCompleted
	PasskeyStateFailed               = internalwhatsapp.PasskeyStateFailed
	PasskeyStateExpired              = internalwhatsapp.PasskeyStateExpired
)

// SubmitPasskeyAssertionRequest is the input for a Passkey assertion.
type SubmitPasskeyAssertionRequest = internalwhatsapp.SubmitPasskeyAssertionRequest

// PasskeyAssertionResult describes the outcome of a Passkey assertion.
type PasskeyAssertionResult = internalwhatsapp.PasskeyAssertionResult

// ConnectionStateResult describes the current connection state of an instance.
type ConnectionStateResult = internalwhatsapp.ConnectionStateResult

// LogoutResult describes the outcome of an instance logout.
type LogoutResult = internalwhatsapp.LogoutResult

// DeleteResult describes the outcome of deleting an instance.
type DeleteResult = internalwhatsapp.DeleteResult

// WebhookSetInput is the input for creating or updating a webhook.
type WebhookSetInput = webhooksvc.SetInput

// Webhook is the persisted webhook configuration of an instance.
type Webhook = dbtypes.Webhook

// MessageOptions configures optional outbound message behavior.
type MessageOptions = message.MessageOptions

// TextMessage contains text-message content.
type TextMessage = httprequest.TextMessage

// LinkMessage contains link-message content and optional preview metadata.
type LinkMessage = httprequest.LinkMessage

// MediaMessage contains outbound media content.
type MediaMessage = httprequest.MediaMessage

// AudioMessageRequest contains outbound audio content.
type AudioMessageRequest = httprequest.AudioMessageRequest

// ContactMessage contains one outbound contact card.
type ContactMessage = httprequest.ContactMessage

// LocationMessage contains outbound geographic coordinates and metadata.
type LocationMessage = httprequest.LocationMessage

// ReactionMessage contains an outbound reaction and its target key.
type ReactionMessage = httprequest.ReactionMessage

// ReactionKey identifies the message targeted by a reaction.
type ReactionKey = httprequest.ReactionKey

// SendTextRequest is the input for sending a text message.
type SendTextRequest = message.SendTextRequest

// SendLinkRequest is the input for sending a link message.
type SendLinkRequest = message.SendLinkRequest

// SendMediaRequest is the input for sending media.
type SendMediaRequest = message.SendMediaRequest

// SendWhatsAppAudioRequest is the input for sending WhatsApp audio.
type SendWhatsAppAudioRequest = message.SendWhatsAppAudioRequest

// SendContactRequest is the input for sending a contact card.
type SendContactRequest = message.SendContactRequest

// SendLocationRequest is the input for sending a location.
type SendLocationRequest = message.SendLocationRequest

// SendReactionRequest is the input for sending a message reaction.
type SendReactionRequest = message.SendReactionRequest

// SendResult describes a persisted outbound message.
type SendResult = message.SendResult

// WhatsAppNumbersRequest is the input for checking WhatsApp contacts.
type WhatsAppNumbersRequest = chat.WhatsAppNumbersRequest

// WhatsAppNumberResponse describes one WhatsApp contact check.
type WhatsAppNumberResponse = chat.WhatsAppNumberResponse

// FindMessagesRequest is the input for listing stored messages.
type FindMessagesRequest = chat.FindMessagesRequest

// FindMessagesWhere contains optional stored-message filters.
type FindMessagesWhere = chat.FindMessagesWhere

// FlexibleBool accepts boolean JSON values in native or string form.
type FlexibleBool = chat.FlexibleBool

// MessageListResult is a page of stored messages.
type MessageListResult = dbtypes.MessageListResult

// MessagePage contains stored messages and page metadata.
type MessagePage = dbtypes.MessagePage

// MessageWithUpdates contains a stored message and its status history.
type MessageWithUpdates = dbtypes.MessageWithUpdates

// MessageUpdateSummary describes one message status transition.
type MessageUpdateSummary = dbtypes.MessageUpdateSummary

// DeviceMessage identifies the client class that produced a message.
type DeviceMessage = dbtypes.DeviceMessage

// Device message values identify supported WhatsApp client classes.
const (
	DeviceMessageIOS     = dbtypes.DeviceMessageIOS
	DeviceMessageAndroid = dbtypes.DeviceMessageAndroid
	DeviceMessageWeb     = dbtypes.DeviceMessageWeb
	DeviceMessageUnknown = dbtypes.DeviceMessageUnknown
	DeviceMessageDesktop = dbtypes.DeviceMessageDesktop
)

// ReadMessagesRequest is the input for marking messages as read.
type ReadMessagesRequest = chat.ReadMessagesRequest

// ArchiveChatRequest is the input for changing a chat archive state.
type ArchiveChatRequest = chat.ArchiveChatRequest

// MessageReference identifies the last message used by a chat operation.
type MessageReference = chat.MessageReferenceDTO

// MessageKey identifies a WhatsApp message.
type MessageKey = chat.MessageKeyDTO

// FetchProfilePictureRequest is the input for fetching a profile picture.
type FetchProfilePictureRequest = chat.FetchProfilePictureRequest

// RejectCallRequest is the input for rejecting a call.
type RejectCallRequest = chat.RejectCallRequest

// EditMessageRequest is the input for editing a sent message.
type EditMessageRequest = chat.EditMessageRequest

// MessageIdentifier identifies a stored message by numeric or WhatsApp key ID.
type MessageIdentifier = chat.MessageIdentifier

// Message is a persisted WhatsApp message.
type Message = dbtypes.Message

// MediaDataRequest is the input for downloading message media.
type MediaDataRequest = chat.MediaDataRequest

// MediaDownloadResult describes downloaded message media.
type MediaDownloadResult = chat.MediaDownloadResult

// MediaMetadata describes downloaded media content.
type MediaMetadata = chat.MediaMetadata

// MediaDataMode identifies how a media download request selects its message.
type MediaDataMode = chat.MediaDataMode

// Media data modes identify the supported message lookup strategies.
const (
	MediaDataModeID      = chat.MediaDataModeID
	MediaDataModeKeyID   = chat.MediaDataModeKeyID
	MediaDataModePayload = chat.MediaDataModePayload
)

// GroupCreateRequest is the input for creating a group.
type GroupCreateRequest = group.CreateRequest

// GroupInfoResponse describes a WhatsApp group.
type GroupInfoResponse = group.InfoResponse

// GroupParticipantResponse describes one group participant.
type GroupParticipantResponse = group.ParticipantResponse

// GroupUpdatePictureRequest is the input for replacing a group picture.
type GroupUpdatePictureRequest = group.UpdatePictureRequest

// GroupInviteCodeResponse describes a group's active invite code.
type GroupInviteCodeResponse = group.InviteCodeResponse

// GroupUpdateParticipantRequest is the input for changing group participants.
type GroupUpdateParticipantRequest = group.UpdateParticipantRequest
