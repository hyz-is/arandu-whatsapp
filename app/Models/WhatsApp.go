package models

import (
	"github.com/hyz-is/arandu-whatsapp/internal/chat"
	"github.com/hyz-is/arandu-whatsapp/internal/group"
	"github.com/hyz-is/arandu-whatsapp/internal/message"
	internalwhatsapp "github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
)

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

// PasskeyAssertionResult describes the outcome of a Passkey assertion.
type PasskeyAssertionResult = internalwhatsapp.PasskeyAssertionResult

// ConnectionStateResult describes the current connection state of an instance.
type ConnectionStateResult = internalwhatsapp.ConnectionStateResult

// LogoutResult describes the outcome of an instance logout.
type LogoutResult = internalwhatsapp.LogoutResult

// DeleteResult describes the outcome of deleting an instance.
type DeleteResult = internalwhatsapp.DeleteResult

// SendResult describes a persisted outbound message.
type SendResult = message.SendResult

// WhatsAppNumber describes one WhatsApp contact check.
type WhatsAppNumber = chat.WhatsAppNumberResponse

// ValidationError carries semantic request validation messages.
type ValidationError = chat.ValidationError

// MediaDownloadResult describes downloaded message media.
type MediaDownloadResult = chat.MediaDownloadResult

// MediaMetadata describes downloaded media content.
type MediaMetadata = chat.MediaMetadata

// GroupInfo describes a WhatsApp group.
type GroupInfo = group.InfoResponse

// GroupParticipant describes one group participant.
type GroupParticipant = group.ParticipantResponse

// GroupInviteCode describes a group's active invite code.
type GroupInviteCode = group.InviteCodeResponse
