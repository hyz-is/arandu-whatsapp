// Package models holds the domain values shared across module responsibilities.
package models

import (
	"encoding/json"
	"time"
)

// Instance is the public WhatsApp instance entity protected by the instance policy.
type Instance struct {
	// ID is the instance identifier.
	ID int64
	// TenantID is the tenant that owns the instance.
	TenantID string
	// Name is the stable public instance name.
	Name string
	// Description is optional human-readable metadata.
	Description *string
	// Status is the configured instance status.
	Status string
	// ConnectionStatus is the latest persisted connection state.
	ConnectionStatus string
	// OwnerJID is the legacy owner address when one is stored.
	OwnerJID *string
	// ProfilePictureURL is the latest known profile-picture URL.
	ProfilePictureURL *string
	// WhatsAppDeviceJID is the paired device address.
	WhatsAppDeviceJID *string
	// WhatsAppOwnerJID is the paired account address without device information.
	WhatsAppOwnerJID *string
	// WhatsAppPhone is the paired account phone number.
	WhatsAppPhone *string
	// LastConnectedAt is when the instance most recently connected.
	LastConnectedAt *time.Time
	// LastDisconnectedAt is when the instance most recently disconnected.
	LastDisconnectedAt *time.Time
	// ConnectionAttempts is the number of consecutive connection attempts.
	ConnectionAttempts int32
	// CreatedAt is when the instance was created.
	CreatedAt time.Time
	// UpdatedAt is when the instance was last updated.
	UpdatedAt time.Time
	// ExternalAttributes holds caller-defined JSON object metadata.
	ExternalAttributes json.RawMessage
}

// InstanceListQuery selects one forward-only page of tenant instances.
type InstanceListQuery struct {
	// Name limits results to instance names containing this value.
	Name *string
	// Limit is the page size. Zero uses the default page limit.
	Limit int
	// Cursor is the opaque cursor returned by the preceding page.
	Cursor string
}

// InstancePage contains tenant instances and the cursor for the next page.
type InstancePage struct {
	// Items are the instances in this page.
	Items []Instance
	// NextCursor is empty when this is the final page.
	NextCursor string
	// PerPage is the normalized page size.
	PerPage int
}
