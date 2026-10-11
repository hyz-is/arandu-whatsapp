package whatsapp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/arandu-io/framework/security"

	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
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
	// IsGroup is a message in a group chat.
	IsGroup bool
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
	m.connections.AddInboundHandler(func(ctx context.Context, grant security.Grant, instanceName string, msg dbtypes.CreateMessageInput) {
		fn(ctx, grant, inboundOf(instanceName, msg))
	})
}

func inboundOf(instanceName string, msg dbtypes.CreateMessageInput) InboundMessage {
	out := InboundMessage{
		InstanceName: instanceName, KeyID: msg.KeyID, FromMe: msg.KeyFromMe,
		MessageType: msg.MessageType, Content: msg.Content,
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
