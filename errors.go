package whatsapp

import (
	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-whatsapp/internal/chat"
	internalrepo "github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	"github.com/hyz-is/arandu-whatsapp/internal/group"
	"github.com/hyz-is/arandu-whatsapp/internal/message"
	internalwhatsapp "github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
	"github.com/hyz-is/arandu-whatsapp/internal/whatsapp/address"
)

var (
	// ErrForbidden means authorization, tenant scope or Grant validation refused an operation.
	ErrForbidden = security.ErrForbidden
	// ErrInstanceNotFound means the requested instance does not exist in the Grant tenant.
	ErrInstanceNotFound = internalrepo.ErrInstanceNotFound
	// ErrInstanceNameAlreadyExists means the Grant tenant already owns the requested name.
	ErrInstanceNameAlreadyExists = internalrepo.ErrInstanceNameAlreadyExists
	// ErrInstanceHasDependencies means a non-forced delete found owned records.
	ErrInstanceHasDependencies = internalrepo.ErrInstanceHasDependencies
	// ErrWhatsAppDeviceAlreadyLinked means the device is already assigned to another instance.
	ErrWhatsAppDeviceAlreadyLinked = internalrepo.ErrWhatsAppDeviceAlreadyLinked
	// ErrWebhookAlreadyExists means an instance already owns a webhook configuration.
	ErrWebhookAlreadyExists = internalrepo.ErrWebhookAlreadyExists
	// ErrWebhookNotFound means the requested webhook configuration does not exist.
	ErrWebhookNotFound = internalrepo.ErrWebhookNotFound
	// ErrMessageNotFound means the requested persisted message does not exist.
	ErrMessageNotFound = internalrepo.ErrMessageNotFound
	// ErrInvalidWebhookEvent means a webhook event name is unsupported.
	ErrInvalidWebhookEvent = internalrepo.ErrInvalidWebhookEvent
	// ErrInvalidWebhookURL means a webhook destination is not acceptable.
	ErrInvalidWebhookURL = internalrepo.ErrInvalidWebhookURL
	// ErrInvalidJSON means a JSON field is syntactically invalid or has the wrong shape.
	ErrInvalidJSON = internalrepo.ErrInvalidJSON
	// ErrInvalidEnum means a persisted enum value is unsupported.
	ErrInvalidEnum = internalrepo.ErrInvalidEnum
	// ErrInvalidInput means a public operation received invalid input.
	ErrInvalidInput = internalrepo.ErrInvalidInput
	// ErrInvalidCursor means an instance page cursor is malformed or outside the scoped result set.
	ErrInvalidCursor = internalrepo.ErrInvalidCursor

	// ErrConnectionInProgress means another pairing or connection attempt owns the instance.
	ErrConnectionInProgress = internalwhatsapp.ErrConnectionInProgress
	// ErrInstanceConnected means the instance is already connected.
	ErrInstanceConnected = internalwhatsapp.ErrInstanceConnected
	// ErrQRCodeTimeout means QR pairing did not produce a code before its deadline.
	ErrQRCodeTimeout = internalwhatsapp.ErrQRCodeTimeout
	// ErrQRChannelClosed means the QR event channel closed before pairing completed.
	ErrQRChannelClosed = internalwhatsapp.ErrQRChannelClosed
	// ErrPairingFailed means WhatsApp rejected or aborted pairing.
	ErrPairingFailed = internalwhatsapp.ErrPairingFailed
	// ErrClientOutdated means the WhatsApp client must be upgraded before connecting.
	ErrClientOutdated = internalwhatsapp.ErrClientOutdated
	// ErrWhatsAppUnavailable means the upstream WhatsApp service is unavailable.
	ErrWhatsAppUnavailable = internalwhatsapp.ErrWhatsAppUnavailable
	// ErrInvalidPhoneNumber means a phone number cannot be normalized for pairing.
	ErrInvalidPhoneNumber = internalwhatsapp.ErrInvalidPhoneNumber
	// ErrSessionMissing means the instance has no usable WhatsApp session.
	ErrSessionMissing = internalwhatsapp.ErrSessionMissing
	// ErrClientNotConnected means the operation requires an active WhatsApp client.
	ErrClientNotConnected = internalwhatsapp.ErrClientNotConnected
	// ErrInstanceInactive means the instance is not enabled for the requested operation.
	ErrInstanceInactive = internalwhatsapp.ErrInstanceInactive
	// ErrDeviceMismatch means the connected WhatsApp device does not match the instance.
	ErrDeviceMismatch = internalwhatsapp.ErrDeviceMismatch
	// ErrPasskeyInstanceNotFound means Passkey pairing cannot find the requested instance.
	ErrPasskeyInstanceNotFound = internalwhatsapp.ErrPasskeyInstanceNotFound
	// ErrPairingSessionNotFound means no Passkey pairing session exists for the request.
	ErrPairingSessionNotFound = internalwhatsapp.ErrPairingSessionNotFound
	// ErrPairingSessionNotActive means the Passkey pairing session cannot accept the operation.
	ErrPairingSessionNotActive = internalwhatsapp.ErrPairingSessionNotActive
	// ErrInvalidPairingState means the requested transition is invalid for the pairing state.
	ErrInvalidPairingState = internalwhatsapp.ErrInvalidPairingState
	// ErrPasskeyRequestMismatch means an assertion does not belong to the active request.
	ErrPasskeyRequestMismatch = internalwhatsapp.ErrPasskeyRequestMismatch
	// ErrPasskeyChallengeAlreadyUsed means the challenge has already accepted an assertion.
	ErrPasskeyChallengeAlreadyUsed = internalwhatsapp.ErrPasskeyChallengeAlreadyUsed
	// ErrPasskeyChallengeExpired means the Passkey challenge is no longer valid.
	ErrPasskeyChallengeExpired = internalwhatsapp.ErrPasskeyChallengeExpired
	// ErrInvalidPasskeyAssertion means the submitted assertion is invalid.
	ErrInvalidPasskeyAssertion = internalwhatsapp.ErrInvalidPasskeyAssertion
	// ErrPasskeyNotAvailable means the instance cannot use Passkey pairing.
	ErrPasskeyNotAvailable = internalwhatsapp.ErrPasskeyNotAvailable
	// ErrPasskeyServiceUnavailable means the Passkey integration is unavailable.
	ErrPasskeyServiceUnavailable = internalwhatsapp.ErrPasskeyServiceUnavailable

	// ErrInvalidMessageRequest means an outbound message request is invalid.
	ErrInvalidMessageRequest = message.ErrInvalidRequest
	// ErrRecipientInvalid means the outbound recipient is invalid.
	ErrRecipientInvalid = message.ErrRecipientInvalid
	// ErrPresenceInvalid means the requested outbound presence is invalid.
	ErrPresenceInvalid = message.ErrPresenceInvalid
	// ErrDelayInvalid means the requested outbound delay is invalid.
	ErrDelayInvalid = message.ErrDelayInvalid
	// ErrQuotedMessageInvalid means quoted-message input is invalid.
	ErrQuotedMessageInvalid = message.ErrQuotedMessageInvalid
	// ErrQuotedMessageLookup means the quoted message could not be loaded.
	ErrQuotedMessageLookup = message.ErrQuotedMessageLookup
	// ErrMessagePersistenceFailed means WhatsApp accepted a message that could not be persisted.
	ErrMessagePersistenceFailed = message.ErrPersistenceFailed
	// ErrMediaDownloadFailed means outbound media could not be downloaded.
	ErrMediaDownloadFailed = message.ErrDownloadFailed
	// ErrMediaUploadFailed means outbound media could not be uploaded to WhatsApp.
	ErrMediaUploadFailed = message.ErrUploadFailed
	// ErrMessageSendFailed means WhatsApp did not accept the outbound message.
	ErrMessageSendFailed = message.ErrSendFailed
	// ErrPayloadTooLarge means a message or attachment exceeds the configured limit.
	ErrPayloadTooLarge = message.ErrPayloadTooLarge
	// ErrUnsupportedMediaType means an outbound message uses an unsupported media type.
	ErrUnsupportedMediaType = message.ErrUnsupportedMediaType
	// ErrAudioProcessing means audio conversion or inspection failed.
	ErrAudioProcessing = message.ErrAudioProcessing
	// ErrInvalidAudioDuration means an audio duration is missing or outside the accepted range.
	ErrInvalidAudioDuration = message.ErrInvalidAudioDuration
	// ErrMentionAllRequiresGroup means mention-all was requested for a non-group recipient.
	ErrMentionAllRequiresGroup = message.ErrMentionAllRequiresGroup
	// ErrMentionAllUnsupported means the message type cannot carry mention-all metadata.
	ErrMentionAllUnsupported = message.ErrMentionAllUnsupported
	// ErrGroupInfoFetchFailed means mention processing could not load group information.
	ErrGroupInfoFetchFailed = message.ErrGroupInfoFetchFailed
	// ErrGroupHasNoParticipants means mention-all found no group participants.
	ErrGroupHasNoParticipants = message.ErrGroupHasNoParticipants
	// ErrGroupMentionProcessing means mention-all processing failed.
	ErrGroupMentionProcessing = message.ErrGroupMentionProcessing

	// ErrChatInstanceDisconnected means a chat operation requires a connected instance.
	ErrChatInstanceDisconnected = chat.ErrInstanceDisconnected
	// ErrMessageNotOutgoing means the instance did not send the targeted message.
	ErrMessageNotOutgoing = chat.ErrMessageNotOutgoing
	// ErrMessageNotEditable means WhatsApp does not permit editing the targeted message.
	ErrMessageNotEditable = chat.ErrMessageNotEditable
	// ErrChatInvalidRecipient means a chat operation received an invalid recipient.
	ErrChatInvalidRecipient = chat.ErrInvalidRecipient
	// ErrInvalidRequestMode means a message search mode is unsupported.
	ErrInvalidRequestMode = chat.ErrInvalidRequestMode
	// ErrDatabaseOperation means a chat operation failed while accessing persistence.
	ErrDatabaseOperation = chat.ErrDatabaseOperation
	// ErrChatRemoteOperation means a remote WhatsApp chat operation failed.
	ErrChatRemoteOperation = chat.ErrRemoteOperation
	// ErrInvalidMediaRequest means a media download request is invalid.
	ErrInvalidMediaRequest = chat.ErrInvalidMediaRequest
	// ErrMediaMessageNotFound means the requested media message does not exist.
	ErrMediaMessageNotFound = chat.ErrMediaMessageNotFound
	// ErrChatUnsupportedMediaType means stored media has an unsupported type.
	ErrChatUnsupportedMediaType = chat.ErrUnsupportedMediaType
	// ErrInvalidMediaContent means stored media content is invalid.
	ErrInvalidMediaContent = chat.ErrInvalidMediaContent
	// ErrMessageIsNotMedia means the requested message has no downloadable media.
	ErrMessageIsNotMedia = chat.ErrMessageIsNotMedia
	// ErrChatMediaDownloadFailed means WhatsApp could not return stored media.
	ErrChatMediaDownloadFailed = chat.ErrMediaDownloadFailed
	// ErrMediaTooLarge means downloaded chat media exceeds the configured limit.
	ErrMediaTooLarge = chat.ErrMediaTooLarge

	// ErrGroupInstanceDisconnected means a group operation requires a connected instance.
	ErrGroupInstanceDisconnected = group.ErrInstanceDisconnected
	// ErrInvalidGroupJID means the supplied group address is invalid.
	ErrInvalidGroupJID = group.ErrInvalidGroupJID
	// ErrInvalidParticipant means a group participant address is invalid.
	ErrInvalidParticipant = group.ErrInvalidParticipant
	// ErrInvalidGroupRequest means a group operation request is invalid.
	ErrInvalidGroupRequest = group.ErrInvalidRequest
	// ErrGroupRemoteOperation means a remote WhatsApp group operation failed.
	ErrGroupRemoteOperation = group.ErrRemoteOperation
	// ErrGroupDownloadFailed means a group image could not be downloaded.
	ErrGroupDownloadFailed = group.ErrDownloadFailed
	// ErrGroupImageTooLarge means a group image exceeds the configured limit.
	ErrGroupImageTooLarge = group.ErrImageTooLarge

	// ErrInvalidAddress means a WhatsApp address cannot be parsed or normalized.
	ErrInvalidAddress = address.ErrInvalidAddress
	// ErrRecipientNotOnWhatsApp means the resolved recipient is not registered on WhatsApp.
	ErrRecipientNotOnWhatsApp = address.ErrRecipientNotOnWhatsApp
	// ErrAmbiguousRecipient means more than one canonical recipient matches the input.
	ErrAmbiguousRecipient = address.ErrAmbiguousRecipient
	// ErrAddressMappingNotFound means no persisted alias mapping exists.
	ErrAddressMappingNotFound = address.ErrAddressMappingNotFound
)

// InstanceDependenciesError reports the owned record counts that blocked deletion.
type InstanceDependenciesError = internalrepo.InstanceDependenciesError

// ValidationError carries semantic request validation messages from chat operations.
type ValidationError = chat.ValidationError
