package whatsapp

import (
	"context"
	"strings"

	watypes "go.mau.fi/whatsmeow/types"
)

// PhoneNumberResolver answers which phone-number address a private LID
// address belongs to. WhatsMeow's LID store satisfies it.
type PhoneNumberResolver interface {
	GetPNForLID(ctx context.Context, lid watypes.JID) (watypes.JID, error)
}

// InboundDetails is who and where an inbound message came from, as the
// message event and the LID store know it.
type InboundDetails struct {
	// SenderJID is the sender's address as WhatsApp delivered it, without a
	// device suffix: a phone-number JID or a private LID.
	SenderJID string
	// SenderLID is the sender's private LID address when WhatsApp delivered
	// one, and empty otherwise.
	SenderLID string
	// SenderPhone is the sender's phone number as digits only, with the
	// country code and no plus sign, when it is known; empty when a private
	// LID has no mapping yet.
	SenderPhone string
	// IsBroadcast is a message delivered through a broadcast list.
	IsBroadcast bool
	// BroadcastListJID is the broadcast list the message came through.
	BroadcastListJID string
	// GroupJID is the group the message was posted in, empty outside a group.
	GroupJID string
}

// ResolveInboundDetails reads the sender identity, the broadcast origin and
// the group of one message. own is the paired account's address, used as the
// sender of a message the paired phone sent; lids may be nil, and a lookup
// that fails leaves the phone number empty rather than failing the message.
func ResolveInboundDetails(ctx context.Context, info watypes.MessageInfo, own watypes.JID, lids PhoneNumberResolver) InboundDetails {
	details := InboundDetails{}
	if info.Chat.IsBroadcastList() {
		details.IsBroadcast = true
		details.BroadcastListJID = info.Chat.ToNonAD().String()
	}
	if info.Chat.Server == watypes.GroupServer {
		details.GroupJID = info.Chat.ToNonAD().String()
	}

	sender := info.Sender
	if info.IsFromMe && !own.IsEmpty() {
		sender = own
	}
	if !sender.IsEmpty() {
		details.SenderJID = sender.ToNonAD().String()
	}
	if lid := firstLIDJID(sender, info.SenderAlt); !lid.IsEmpty() {
		details.SenderLID = lid.ToNonAD().String()
	}

	details.SenderPhone = phoneDigits(firstPhoneJID(sender, info.SenderAlt))
	if details.SenderPhone == "" && lids != nil && !info.IsFromMe {
		for _, candidate := range []watypes.JID{sender, info.SenderAlt} {
			if !isLID(candidate) {
				continue
			}
			pn, err := lids.GetPNForLID(ctx, candidate.ToNonAD())
			if err != nil {
				continue
			}
			if phone := phoneDigits(pn); phone != "" {
				details.SenderPhone = phone
				break
			}
		}
	}
	return details
}

func firstPhoneJID(values ...watypes.JID) watypes.JID {
	for _, value := range values {
		if value.Server == watypes.DefaultUserServer && value.User != "" {
			return value
		}
	}
	return watypes.EmptyJID
}

func phoneDigits(jid watypes.JID) string {
	if jid.Server != watypes.DefaultUserServer || jid.User == "" {
		return ""
	}
	user := strings.TrimSpace(jid.User)
	for _, char := range user {
		if char < '0' || char > '9' {
			return ""
		}
	}
	return user
}
