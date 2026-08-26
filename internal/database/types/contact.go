package types

import "time"

type Contact struct {
	ID            int64     `json:"id"`
	RemoteJid     string    `json:"remoteJid"`
	PushName      *string   `json:"pushName"`
	ProfilePicUrl *string   `json:"profilePicUrl"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	InstanceID    int64     `json:"instanceId"`
}

type CreateContactInput struct {
	RemoteJid     string
	PushName      *string
	ProfilePicUrl *string
	InstanceID    int64
}

type ContactFilters struct {
	ID        *int64
	RemoteJid *string
	PushName  *string
}
