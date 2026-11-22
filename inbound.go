package whatsapp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/arandu-io/framework/security"

	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
	internalwhatsapp "github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
)

// InboundMessage is one message an instance received or sent from the paired
// phone, as it was persisted.
type InboundMessage struct {
	// InstanceName is the instance it arrived on, unique inside its tenant.
	InstanceName string
	// KeyID is WhatsApp's identifier of the message.
	KeyID string
	// RemoteJID is the chat: the contact, or the group.
	RemoteJID string
	// Participant is who sent it inside a group, and empty in a direct chat.
	Participant string
	// PushName is the name the sender shows, when WhatsApp sent one.
	PushName string
	// FromMe is a message the paired phone itself sent.
	FromMe bool
	// IsGroup is WhatsApp's own flag: true for a group chat and for a
	// broadcast list. Use GroupJID to tell a group apart and IsBroadcast to
	// tell a broadcast list apart.
	IsGroup bool
	// GroupJID is the group the message was posted in, and empty outside a
	// group.
	GroupJID string
	// IsBroadcast is a message delivered through a broadcast list.
	IsBroadcast bool
	// BroadcastListJID is the broadcast list the message came through, and
	// empty when it did not come through one.
	BroadcastListJID string
	// SenderJID is the sender's address as WhatsApp delivered it, without a
	// device suffix: a phone-number JID or a private LID. For a message the
	// paired phone sent, it is the paired account.
	SenderJID string
	// SenderLID is the sender's private LID address when WhatsApp delivered
	// one, and empty otherwise.
	SenderLID string
	// SenderPhone is the sender's phone number as digits only, with the
	// country code and no plus sign. It is the number WhatsApp delivered, or
	// the one the LID store maps a private LID to, and it is empty while a
	// private LID has no known number.
	SenderPhone string
	// MessageType is the kind of content, such as "conversation" for text.
	MessageType string
	// Content is the normalized content, as the messages table stores it.
	Content json.RawMessage
	// At is when WhatsApp says the message was sent.
	At time.Time
}

// InboundListener is told about every message an instance persisted, after
// it was persisted, with the runtime Grant of that instance's tenant: the
// tenant the listener writes under is the Grant's, and a listener never sees a
// message of another tenant under it.
//
// It runs on the instance's event goroutine. A listener that does slow work
// hands it to the application's queue; a panic is recovered and logged.
type InboundListener func(ctx context.Context, grant security.Grant, message InboundMessage)

// OnInbound registers a listener for inbound messages. It is wiring: call it
// after New and before Start.
func (m *Module) OnInbound(fn InboundListener) {
	if fn == nil || m.connections == nil {
		return
	}
	m.connections.AddInboundHandler(func(ctx context.Context, grant security.Grant, instanceName string, msg dbtypes.CreateMessageInput, details internalwhatsapp.InboundDetails) {
		fn(ctx, grant, inboundOf(instanceName, msg, details))
	})
}

func inboundOf(instanceName string, msg dbtypes.CreateMessageInput, details internalwhatsapp.InboundDetails) InboundMessage {
	out := InboundMessage{
		InstanceName: instanceName, KeyID: msg.KeyID, FromMe: msg.KeyFromMe,
		MessageType: msg.MessageType, Content: msg.Content,
		GroupJID: details.GroupJID, IsBroadcast: details.IsBroadcast, BroadcastListJID: details.BroadcastListJID,
		SenderJID: details.SenderJID, SenderLID: details.SenderLID, SenderPhone: details.SenderPhone,
	}
	if msg.KeyRemoteJid != nil {
		out.RemoteJID = *msg.KeyRemoteJid
	}
	if msg.KeyParticipant != nil {
		out.Participant = *msg.KeyParticipant
	}
	if msg.PushName != nil {
		out.PushName = *msg.PushName
	}
	if msg.IsGroup != nil {
		out.IsGroup = *msg.IsGroup
	}
	if msg.MessageTimestamp > 0 {
		out.At = time.Unix(int64(msg.MessageTimestamp), 0).UTC()
	}
	return out
}
