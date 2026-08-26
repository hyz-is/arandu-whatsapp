// Package requests holds the typed inputs accepted by the WhatsApp module.
package requests

import (
	"encoding/json"

	"github.com/hyz-is/arandu-whatsapp/internal/chat"
	"github.com/hyz-is/arandu-whatsapp/internal/group"
	httprequest "github.com/hyz-is/arandu-whatsapp/internal/http/request"
	"github.com/hyz-is/arandu-whatsapp/internal/message"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
	internalwhatsapp "github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
)

// CreateInstance is the explicit instance creation contract.
type CreateInstance struct {
	// Name is the stable public name. A generated name is used when it is absent.
	Name *string `json:"name,omitempty"`
	// Description is optional human-readable instance metadata.
	Description *string `json:"description,omitempty"`
	// ExternalAttributes holds caller-defined JSON object metadata.
	ExternalAttributes json.RawMessage `json:"externalAttributes,omitempty"`
}

// PairPhone is the JSON-safe input for phone-code pairing.
type PairPhone struct {
	// PhoneNumber is the international phone number to pair.
	PhoneNumber string `json:"phoneNumber"`
}

// SubmitPasskeyAssertion is the input for a Passkey assertion.
type SubmitPasskeyAssertion = internalwhatsapp.SubmitPasskeyAssertionRequest

// SetWebhook is the input for creating or updating a webhook.
type SetWebhook = webhooksvc.SetInput

// WebhookPayload is the JSON request accepted by the webhook controller.
type WebhookPayload = httprequest.SetWebhookRequest

// MessageOptions configures optional outbound message behavior.
type MessageOptions = message.MessageOptions

// TextMessage contains text-message content.
type TextMessage = httprequest.TextMessage

// LinkMessage contains link-message content and optional preview metadata.
type LinkMessage = httprequest.LinkMessage

// MediaMessage contains outbound media content.
type MediaMessage = httprequest.MediaMessage

// AudioMessage contains outbound audio content.
type AudioMessage = httprequest.AudioMessageRequest

// ContactMessage contains one outbound contact card.
type ContactMessage = httprequest.ContactMessage

// LocationMessage contains outbound geographic coordinates and metadata.
type LocationMessage = httprequest.LocationMessage

// ReactionMessage contains an outbound reaction and its target key.
type ReactionMessage = httprequest.ReactionMessage

// ReactionKey identifies the message targeted by a reaction.
type ReactionKey = httprequest.ReactionKey

// SendText is the input for sending a text message.
type SendText = message.SendTextRequest

// SendLink is the input for sending a link message.
type SendLink = message.SendLinkRequest

// SendMedia is the input for sending media.
type SendMedia = message.SendMediaRequest

// SendWhatsAppAudio is the input for sending WhatsApp audio.
type SendWhatsAppAudio = message.SendWhatsAppAudioRequest

// SendContact is the input for sending a contact card.
type SendContact = message.SendContactRequest

// SendLocation is the input for sending a location.
type SendLocation = message.SendLocationRequest

// SendReaction is the input for sending a message reaction.
type SendReaction = message.SendReactionRequest

// CheckWhatsAppNumbers is the input for checking WhatsApp contacts.
type CheckWhatsAppNumbers = chat.WhatsAppNumbersRequest

// FindMessages is the input for listing stored messages.
type FindMessages = chat.FindMessagesRequest

// FindMessagesWhere contains optional stored-message filters.
type FindMessagesWhere = chat.FindMessagesWhere

// FlexibleBool accepts boolean JSON values in native or string form.
type FlexibleBool = chat.FlexibleBool

// ReadMessages is the input for marking messages as read.
type ReadMessages = chat.ReadMessagesRequest

// ArchiveChat is the input for changing a chat archive state.
type ArchiveChat = chat.ArchiveChatRequest

// MessageReference identifies the last message used by a chat operation.
type MessageReference = chat.MessageReferenceDTO

// MessageKey identifies a WhatsApp message.
type MessageKey = chat.MessageKeyDTO

// FetchProfilePicture is the input for fetching a profile picture.
type FetchProfilePicture = chat.FetchProfilePictureRequest

// RejectCall is the input for rejecting a call.
type RejectCall = chat.RejectCallRequest

// EditMessage is the input for editing a sent message.
type EditMessage = chat.EditMessageRequest

// MessageIdentifier identifies a stored message by numeric or WhatsApp key ID.
type MessageIdentifier = chat.MessageIdentifier

// DownloadMedia is the input for downloading message media.
type DownloadMedia = chat.MediaDataRequest

// MediaDataMode identifies how a media download request selects its message.
type MediaDataMode = chat.MediaDataMode

// Media data modes identify the supported message lookup strategies.
const (
	MediaDataModeID      = chat.MediaDataModeID
	MediaDataModeKeyID   = chat.MediaDataModeKeyID
	MediaDataModePayload = chat.MediaDataModePayload
)

// CreateGroup is the input for creating a group.
type CreateGroup = group.CreateRequest

// UpdateGroupPicture is the input for replacing a group picture.
type UpdateGroupPicture = group.UpdatePictureRequest

// UpdateGroupParticipant is the input for changing group participants.
type UpdateGroupParticipant = group.UpdateParticipantRequest
