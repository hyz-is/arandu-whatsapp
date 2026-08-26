package message

import (
	"context"
	"testing"

	hlog "github.com/arandu-io/hesape/log"
	wae2e "go.mau.fi/whatsmeow/proto/waE2E"
	watypes "go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func TestMentionedJIDsFromParticipantsDeduplicatesAndIgnoresInvalid(t *testing.T) {
	logger, records := hlog.Capture()
	ctx := hlog.Into(context.Background(), logger)
	participants := []watypes.GroupParticipant{
		{JID: watypes.NewJID("5531999999999", watypes.DefaultUserServer)},
		{JID: watypes.NewJID("5531999999999", watypes.DefaultUserServer)},
		{PhoneNumber: watypes.NewJID("5531888888888", watypes.DefaultUserServer)},
		{JID: watypes.JID{}},
		{JID: watypes.NewJID("120363000000000000", watypes.GroupServer)},
	}

	remoteJID := watypes.NewJID("120363000000000000", watypes.GroupServer)
	got := mentionedJIDsFromParticipants(ctx, participants, "process-1", "beplus", remoteJID)

	if len(got) != 2 {
		t.Fatalf("expected 2 mentioned JIDs, got %#v", got)
	}
	if got[0] != "5531999999999@s.whatsapp.net" || got[1] != "5531888888888@s.whatsapp.net" {
		t.Fatalf("unexpected mentioned JIDs: %#v", got)
	}
	if records.Len() == 0 {
		t.Fatal("expected invalid-participant warning")
	}
	for _, record := range records.All() {
		if got := record.Attrs["remote_jid"]; got == remoteJID.String() {
			t.Fatalf("participant warning exposed complete JID %q", got)
		}
	}
}

func TestApplyMentionedJIDsPreservesQuotedContextAndText(t *testing.T) {
	text := "Aviso importante para todos."
	message := &wae2e.Message{ExtendedTextMessage: &wae2e.ExtendedTextMessage{
		Text: proto.String(text),
		ContextInfo: &wae2e.ContextInfo{
			StanzaID:     proto.String("quoted-id"),
			RemoteJID:    proto.String("120363000000000000@g.us"),
			MentionedJID: []string{"5531777777777@s.whatsapp.net"},
		},
	}}

	applyMentionedJIDs(message, []string{"5531888888888@s.whatsapp.net", "5531777777777@s.whatsapp.net"})

	if got := message.GetExtendedTextMessage().GetText(); got != text {
		t.Fatalf("text was changed: %q", got)
	}
	info := message.GetExtendedTextMessage().GetContextInfo()
	if info.GetStanzaID() != "quoted-id" || info.GetRemoteJID() != "120363000000000000@g.us" {
		t.Fatalf("quoted context not preserved: %#v", info)
	}
	want := []string{"5531777777777@s.whatsapp.net", "5531888888888@s.whatsapp.net"}
	if len(info.MentionedJID) != len(want) {
		t.Fatalf("mentioned count mismatch: %#v", info.MentionedJID)
	}
	for i := range want {
		if info.MentionedJID[i] != want[i] {
			t.Fatalf("mentioned[%d]: want %q got %q", i, want[i], info.MentionedJID[i])
		}
	}
}
