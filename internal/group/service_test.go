package group

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	httpclient "github.com/arandu-io/hesape/http/client"
	watypes "go.mau.fi/whatsmeow/types"
)

func TestGroupImageClientRejectsInternalAddresses(t *testing.T) {
	service := NewService(nil, nil)
	_, err := service.downloadImage(context.Background(), "http://127.0.0.1:1/image.jpg")
	if !errors.Is(err, httpclient.ErrInternalAddress) {
		t.Fatalf("expected ErrInternalAddress, got %v", err)
	}
}

func TestGroupImageClientUsesNativeResponseLimitAndCallerContext(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request-context")
	contextPreserved := false
	transport := groupRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		contextPreserved = req.Context().Value(contextKey{}) == "request-context"
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: maxGroupPictureBytes + 2,
			Body:          io.NopCloser(strings.NewReader("oversized")),
			Header:        make(http.Header),
		}, nil
	})
	service := NewService(nil, nil)
	service.http = newGroupHTTPClient(httpclient.NewFactory(&http.Client{Transport: transport}))

	_, err := service.downloadImage(ctx, "https://example.com/image.jpg")
	if !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("expected ErrImageTooLarge, got %v", err)
	}
	if !contextPreserved {
		t.Fatal("expected the caller context to reach the transport")
	}
}

type groupRoundTripFunc func(*http.Request) (*http.Response, error)

func (f groupRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGroupSummaryResponsesKeepJIDSubjectAndSizeWithoutParticipants(t *testing.T) {
	groups := []*watypes.GroupInfo{
		nil,
		{
			JID: watypes.NewJID("120363000000000001", watypes.GroupServer), OwnerPN: watypes.NewJID("5511999990001", watypes.DefaultUserServer),
			GroupName: watypes.GroupName{Name: "Fornecedores A"}, ParticipantCount: 12,
			Participants: []watypes.GroupParticipant{{JID: watypes.NewJID("5511999990002", watypes.DefaultUserServer)}},
		},
		{
			JID: watypes.NewJID("120363000000000002", watypes.GroupServer), GroupName: watypes.GroupName{Name: "Sem contagem"},
			Participants: []watypes.GroupParticipant{{}, {}},
		},
	}
	got := groupSummaryResponses(groups)
	if len(got) != 2 {
		t.Fatalf("got %d summaries, want 2: %#v", len(got), got)
	}
	if got[0].ID != "120363000000000001@g.us" || got[0].Subject != "Fornecedores A" || got[0].Size != 12 || got[0].Owner != "5511999990001@s.whatsapp.net" {
		t.Fatalf("unexpected first summary %#v", got[0])
	}
	if got[1].Size != 2 {
		t.Fatalf("a group with no count must fall back to its participants: %#v", got[1])
	}
	if empty := groupSummaryResponses(nil); empty == nil || len(empty) != 0 {
		t.Fatalf("no groups must answer an empty list, got %#v", empty)
	}
}
