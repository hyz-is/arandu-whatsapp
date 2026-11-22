package whatsapp

import (
	"encoding/json"
	"testing"

	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
	internalwhatsapp "github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
)

func TestInboundOfCarriesTheSenderIdentityAndTheOrigin(t *testing.T) {
	group := "120363000000000001@g.us"
	isGroup := true
	msg := dbtypes.CreateMessageInput{
		KeyID: "k1", KeyRemoteJid: &group, IsGroup: &isGroup,
		MessageType: "documentMessage", Content: json.RawMessage(`{}`), MessageTimestamp: 10,
	}
	details := internalwhatsapp.InboundDetails{
		SenderJID: "98765432101234@lid", SenderLID: "98765432101234@lid", SenderPhone: "5511999990001",
		GroupJID: group,
	}
	got := inboundOf("main", msg, details)
	if got.SenderPhone != "5511999990001" || got.SenderLID != "98765432101234@lid" || got.SenderJID != "98765432101234@lid" {
		t.Fatalf("sender identity lost: %#v", got)
	}
	if got.GroupJID != group || got.IsBroadcast || got.BroadcastListJID != "" {
		t.Fatalf("origin wrong: %#v", got)
	}
	if got.RemoteJID != group || !got.IsGroup || got.KeyID != "k1" || got.At.Unix() != 10 {
		t.Fatalf("the existing fields changed: %#v", got)
	}

	broadcast := inboundOf("main", msg, internalwhatsapp.InboundDetails{IsBroadcast: true, BroadcastListJID: "1700000000@broadcast"})
	if !broadcast.IsBroadcast || broadcast.BroadcastListJID != "1700000000@broadcast" || broadcast.GroupJID != "" {
		t.Fatalf("broadcast origin wrong: %#v", broadcast)
	}
}
