package models

import (
	"encoding/json"
	"time"
)

// DeviceMessage identifies the client class that produced a message.
type DeviceMessage string

// Device message values identify supported WhatsApp client classes.
const (
	DeviceMessageIOS     DeviceMessage = "ios"
	DeviceMessageAndroid DeviceMessage = "android"
	DeviceMessageWeb     DeviceMessage = "web"
	DeviceMessageUnknown DeviceMessage = "unknown"
	DeviceMessageDesktop DeviceMessage = "desktop"
)

// IsValid reports whether the device class is supported.
func (d DeviceMessage) IsValid() bool {
	switch d {
	case DeviceMessageIOS, DeviceMessageAndroid, DeviceMessageWeb, DeviceMessageUnknown, DeviceMessageDesktop:
		return true
	default:
		return false
	}
}

// Message is a persisted WhatsApp message exposed by the application.
type Message struct {
	ID                 int64           `json:"id"`
	KeyID              string          `json:"keyId"`
	KeyRemoteJid       *string         `json:"keyRemoteJid"`
	KeyLid             *string         `json:"keyLid"`
	KeyFromMe          bool            `json:"keyFromMe"`
	KeyParticipant     *string         `json:"keyParticipant"`
	KeyParticipantLid  *string         `json:"keyParticipantLid"`
	PushName           *string         `json:"pushName"`
	MessageType        string          `json:"messageType"`
	Content            json.RawMessage `json:"content"`
	MessageTimestamp   int32           `json:"messageTimestamp"`
	Device             DeviceMessage   `json:"device"`
	IsGroup            *bool           `json:"isGroup"`
	InstanceID         int64           `json:"instanceId"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
	ExternalAttributes map[string]any  `json:"externalAttributes,omitempty"`
}

// MessageUpdateSummary describes one message status transition.
type MessageUpdateSummary struct {
	Status   string    `json:"status"`
	DateTime time.Time `json:"dateTime"`
}

// MessageWithUpdates contains a stored message and its status history.
type MessageWithUpdates struct {
	Message
	MessageUpdate []MessageUpdateSummary `json:"MessageUpdate"`
}

// MessagePage contains stored messages and page metadata.
type MessagePage struct {
	Total       int64                `json:"total"`
	Pages       int64                `json:"pages"`
	CurrentPage int64                `json:"currentPage"`
	Records     []MessageWithUpdates `json:"records"`
}

// MessageListResult is a page of stored messages.
type MessageListResult struct {
	Messages MessagePage `json:"messages"`
}
