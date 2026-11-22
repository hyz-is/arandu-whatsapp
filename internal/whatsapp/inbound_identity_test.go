package whatsapp

import (
	"context"
	"errors"
	"testing"

	"github.com/arandu-io/framework/security"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	watypes "go.mau.fi/whatsmeow/types"

	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

type fakePhoneResolver struct {
	mapping map[string]watypes.JID
	err     error
	calls   []string
}

func (f *fakePhoneResolver) GetPNForLID(_ context.Context, lid watypes.JID) (watypes.JID, error) {
	f.calls = append(f.calls, lid.String())
	if f.err != nil {
		return watypes.EmptyJID, f.err
	}
	return f.mapping[lid.String()], nil
}

// fakeLIDStore satisfies store.LIDStore with the one lookup the module uses.
type fakeLIDStore struct {
	store.LIDStore
	fakePhoneResolver
}

func (f *fakeLIDStore) GetPNForLID(ctx context.Context, lid watypes.JID) (watypes.JID, error) {
	return f.fakePhoneResolver.GetPNForLID(ctx, lid)
}

func inboundInfo(source watypes.MessageSource) watypes.MessageInfo {
	return watypes.MessageInfo{MessageSource: source}
}

func TestResolveInboundDetailsReadsAPhoneSenderWithoutTheStore(t *testing.T) {
	phone := watypes.NewJID("5511999990001", watypes.DefaultUserServer)
	resolver := &fakePhoneResolver{}
	details := ResolveInboundDetails(context.Background(), inboundInfo(watypes.MessageSource{Chat: phone, Sender: phone}), watypes.EmptyJID, resolver)
	if details.SenderPhone != "5511999990001" || details.SenderJID != "5511999990001@s.whatsapp.net" || details.SenderLID != "" {
		t.Fatalf("unexpected details %#v", details)
	}
	if len(resolver.calls) != 0 {
		t.Fatalf("the store was asked about a sender that already had a number: %v", resolver.calls)
	}
}

func TestResolveInboundDetailsTakesThePhoneFromTheAlternativeAddress(t *testing.T) {
	lid := watypes.NewJID("98765432101234", watypes.HiddenUserServer)
	phone := watypes.NewJID("5511999990002", watypes.DefaultUserServer)
	resolver := &fakePhoneResolver{}
	details := ResolveInboundDetails(context.Background(), inboundInfo(watypes.MessageSource{Chat: lid, Sender: lid, SenderAlt: phone}), watypes.EmptyJID, resolver)
	if details.SenderPhone != "5511999990002" || details.SenderLID != "98765432101234@lid" || details.SenderJID != "98765432101234@lid" {
		t.Fatalf("unexpected details %#v", details)
	}
	if len(resolver.calls) != 0 {
		t.Fatalf("the store was asked although SenderAlt carried the number: %v", resolver.calls)
	}
}

func TestResolveInboundDetailsResolvesAPrivateLIDThroughTheStore(t *testing.T) {
	lid := watypes.NewJID("98765432101234", watypes.HiddenUserServer)
	resolver := &fakePhoneResolver{mapping: map[string]watypes.JID{
		"98765432101234@lid": watypes.NewJID("5511999990003", watypes.DefaultUserServer),
	}}
	details := ResolveInboundDetails(context.Background(), inboundInfo(watypes.MessageSource{Chat: lid, Sender: lid}), watypes.EmptyJID, resolver)
	if details.SenderPhone != "5511999990003" || details.SenderLID != "98765432101234@lid" {
		t.Fatalf("unexpected details %#v", details)
	}
}

func TestResolveInboundDetailsLeavesThePhoneEmptyWhenTheLIDIsUnknownOrTheStoreFails(t *testing.T) {
	lid := watypes.NewJID("98765432101234", watypes.HiddenUserServer)
	info := inboundInfo(watypes.MessageSource{Chat: lid, Sender: lid})

	unknown := ResolveInboundDetails(context.Background(), info, watypes.EmptyJID, &fakePhoneResolver{})
	if unknown.SenderPhone != "" || unknown.SenderLID != "98765432101234@lid" {
		t.Fatalf("an unmapped LID produced %#v", unknown)
	}
	failing := ResolveInboundDetails(context.Background(), info, watypes.EmptyJID, &fakePhoneResolver{err: errors.New("store down")})
	if failing.SenderPhone != "" {
		t.Fatalf("a failing store produced %#v", failing)
	}
	none := ResolveInboundDetails(context.Background(), info, watypes.EmptyJID, nil)
	if none.SenderPhone != "" {
		t.Fatalf("a missing store produced %#v", none)
	}
}

func TestResolveInboundDetailsNamesTheGroupAndItsLIDParticipant(t *testing.T) {
	group := watypes.NewJID("120363000000000001", watypes.GroupServer)
	lid := watypes.NewJID("98765432101234", watypes.HiddenUserServer)
	resolver := &fakePhoneResolver{mapping: map[string]watypes.JID{
		"98765432101234@lid": watypes.NewJID("5511999990004", watypes.DefaultUserServer),
	}}
	details := ResolveInboundDetails(context.Background(), inboundInfo(watypes.MessageSource{Chat: group, Sender: lid, IsGroup: true}), watypes.EmptyJID, resolver)
	if details.GroupJID != "120363000000000001@g.us" || details.IsBroadcast || details.BroadcastListJID != "" {
		t.Fatalf("unexpected group details %#v", details)
	}
	if details.SenderPhone != "5511999990004" {
		t.Fatalf("the participant's number was not resolved: %#v", details)
	}
}

func TestResolveInboundDetailsFlagsABroadcastListAndNotAGroup(t *testing.T) {
	list := watypes.NewJID("1700000000", watypes.BroadcastServer)
	phone := watypes.NewJID("5511999990005", watypes.DefaultUserServer)
	details := ResolveInboundDetails(context.Background(), inboundInfo(watypes.MessageSource{Chat: list, Sender: phone, IsGroup: true}), watypes.EmptyJID, nil)
	if !details.IsBroadcast || details.BroadcastListJID != "1700000000@broadcast" || details.GroupJID != "" {
		t.Fatalf("unexpected broadcast details %#v", details)
	}
	if details.SenderPhone != "5511999990005" {
		t.Fatalf("unexpected sender %#v", details)
	}

	status := ResolveInboundDetails(context.Background(), inboundInfo(watypes.MessageSource{Chat: watypes.StatusBroadcastJID, Sender: phone, IsGroup: true}), watypes.EmptyJID, nil)
	if status.IsBroadcast {
		t.Fatalf("a status update was reported as a broadcast list: %#v", status)
	}

	direct := ResolveInboundDetails(context.Background(), inboundInfo(watypes.MessageSource{Chat: phone, Sender: phone}), watypes.EmptyJID, nil)
	if direct.IsBroadcast || direct.GroupJID != "" || direct.BroadcastListJID != "" {
		t.Fatalf("a direct chat carried an origin: %#v", direct)
	}
}

func TestResolveInboundDetailsUsesThePairedAccountForAMessageItSent(t *testing.T) {
	own := watypes.JID{User: "5511999990006", Device: 7, Server: watypes.DefaultUserServer}
	peer := watypes.NewJID("98765432101234", watypes.HiddenUserServer)
	resolver := &fakePhoneResolver{}
	details := ResolveInboundDetails(context.Background(), inboundInfo(watypes.MessageSource{Chat: peer, Sender: own, IsFromMe: true}), own, resolver)
	if details.SenderPhone != "5511999990006" || details.SenderJID != "5511999990006@s.whatsapp.net" {
		t.Fatalf("unexpected details %#v", details)
	}
}

func TestHandleMessageTellsTheHandlerTheSenderPhoneResolvedFromTheClientStore(t *testing.T) {
	lid := watypes.NewJID("98765432101234", watypes.HiddenUserServer)
	messages := &fakePersistenceMessages{findResults: []findMessageResult{{message: dbtypes.Message{ID: 9, KeyID: "msg-lid"}}}}
	service := NewEventPersistenceService(EventPersistenceConfig{SaveDataNewMessage: true}, messages, &fakePersistenceMessageUpdates{}, &fakePersistenceContacts{})
	service.SetWebhookDispatcher(fakeInstanceFinder(), &fakePersistenceWebhooks{})
	var got InboundDetails
	service.AddInboundHandler(func(_ context.Context, _ security.Grant, _ string, _ dbtypes.CreateMessageInput, details InboundDetails) {
		got = details
	})

	managed := eventManagedClient()
	managed.Client = &whatsmeow.Client{Store: &store.Device{LIDs: &fakeLIDStore{fakePhoneResolver: fakePhoneResolver{mapping: map[string]watypes.JID{
		lid.String(): watypes.NewJID("5511999990007", watypes.DefaultUserServer),
	}}}}}
	event := textEvent("msg-lid")
	event.Info.Chat = lid
	event.Info.Sender = lid
	service.HandleMessage(context.Background(), managed, event)

	if got.SenderPhone != "5511999990007" || got.SenderLID != lid.String() {
		t.Fatalf("the handler was told %#v", got)
	}
}
