package types

import (
	"encoding/json"
	"time"
)

type DeviceMessage string

const (
	DeviceMessageIOS     DeviceMessage = "ios"
	DeviceMessageAndroid DeviceMessage = "android"
	DeviceMessageWeb     DeviceMessage = "web"
	DeviceMessageUnknown DeviceMessage = "unknown"
	DeviceMessageDesktop DeviceMessage = "desktop"
)

func (d DeviceMessage) IsValid() bool {
	switch d {
	case DeviceMessageIOS, DeviceMessageAndroid, DeviceMessageWeb, DeviceMessageUnknown, DeviceMessageDesktop:
		return true
	default:
		return false
	}
}

type CursorDirection string

const (
	CursorDirectionNext     CursorDirection = "next"
	CursorDirectionPrevious CursorDirection = "previous"
)

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

type MessageUpdate struct {
	ID        int64     `json:"id"`
	DateTime  time.Time `json:"dateTime"`
	Status    string    `json:"status"`
	MessageID int64     `json:"messageId"`
}

type MessageUpdateSummary struct {
	Status   string    `json:"status"`
	DateTime time.Time `json:"dateTime"`
}

type MessageWithUpdates struct {
	Message
	MessageUpdate []MessageUpdateSummary `json:"MessageUpdate"`
}

type CreateMessageInput struct {
	KeyID             string
	KeyRemoteJid      *string
	KeyLid            *string
	KeyFromMe         bool
	KeyParticipant    *string
	KeyParticipantLid *string
	PushName          *string
	MessageType       string
	Content           json.RawMessage
	MessageTimestamp  int32
	Device            DeviceMessage
	IsGroup           *bool
	InstanceID        int64
	Metadata          json.RawMessage
}

type MessageFilters struct {
	ID                  *int64
	KeyID               *string
	KeyRemoteJid        *string
	KeyFromMe           *bool
	MessageType         *string
	Device              *DeviceMessage
	MessageStatus       *string
	MessageTimestampGTE *int32
	MessageTimestampLTE *int32
}

type ListMessagesInput struct {
	Cursor    *int64
	Limit     int32
	Direction CursorDirection
	Filters   MessageFilters
}

type ListMessagesPageInput struct {
	Limit   int32
	Page    int32
	Filters MessageFilters
}

type MessageListResult struct {
	Messages MessagePage `json:"messages"`
}

type MessagePage struct {
	Total       int64                `json:"total"`
	Pages       int64                `json:"pages"`
	CurrentPage int64                `json:"currentPage"`
	Records     []MessageWithUpdates `json:"records"`
}

type CreateMessageUpdateInput struct {
	DateTime  time.Time
	Status    string
	MessageID int64
}
