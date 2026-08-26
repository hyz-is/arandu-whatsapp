package whatsapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

// Instance is the public WhatsApp instance entity protected by InstancePolicy.
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

func instanceFromInternal(item dbtypes.Instance) Instance {
	return Instance{
		ID: item.ID, TenantID: item.TenantID, Name: item.Name,
		Description: item.Description, Status: string(item.Status),
		ConnectionStatus: string(item.ConnectionStatus), OwnerJID: item.OwnerJid,
		ProfilePictureURL: item.ProfilePicUrl, WhatsAppDeviceJID: item.WhatsAppDeviceJid,
		WhatsAppOwnerJID: item.WhatsAppOwnerJid, WhatsAppPhone: item.WhatsAppPhone,
		LastConnectedAt: item.LastConnectedAt, LastDisconnectedAt: item.LastDisconnectedAt,
		ConnectionAttempts: item.ConnectionAttempts, CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt, ExternalAttributes: item.ExternalAttributes,
	}
}

// InstanceResource declares the fields that may leave the module. Tenant and
// the WhatsMeow device-store key are intentionally absent.
type InstanceResource struct{ instance Instance }

// NewInstanceResource wraps one instance for an Arandu JSON response.
func NewInstanceResource(instance Instance) InstanceResource {
	return InstanceResource{instance: instance}
}

// ToArray returns the public representation of an instance.
func (r InstanceResource) ToArray() map[string]any {
	return map[string]any{
		"id": r.instance.ID, "name": r.instance.Name,
		"description": r.instance.Description, "status": r.instance.Status,
		"connectionStatus": r.instance.ConnectionStatus, "ownerJid": r.instance.OwnerJID,
		"profilePicUrl": r.instance.ProfilePictureURL, "whatsappOwnerJid": r.instance.WhatsAppOwnerJID,
		"whatsappPhoneNumber": r.instance.WhatsAppPhone,
		"lastConnectedAt":     r.instance.LastConnectedAt,
		"lastDisconnectedAt":  r.instance.LastDisconnectedAt,
		"connectionAttempts":  r.instance.ConnectionAttempts,
		"createdAt":           r.instance.CreatedAt.UTC(), "updatedAt": r.instance.UpdatedAt.UTC(),
		"externalAttributes": jsonValue(r.instance.ExternalAttributes),
	}
}

// With returns no top-level metadata.
func (InstanceResource) With() map[string]any { return nil }

// InstanceCollection is a tenant-scoped instance page.
type InstanceCollection struct{ page InstancePage }

// NewInstanceCollection wraps an instance page for an Arandu JSON response.
func NewInstanceCollection(page InstancePage) InstanceCollection {
	return InstanceCollection{page: page}
}

// ToArray returns the page items and forward-pagination metadata.
func (c InstanceCollection) ToArray() map[string]any {
	items := make([]map[string]any, 0, len(c.page.Items))
	for _, item := range c.page.Items {
		items = append(items, NewInstanceResource(item).ToArray())
	}
	var nextCursor any
	if c.page.NextCursor != "" {
		nextCursor = c.page.NextCursor
	}
	return map[string]any{
		"items": items, "perPage": c.page.PerPage, "nextCursor": nextCursor,
	}
}

// With returns no top-level metadata.
func (InstanceCollection) With() map[string]any { return nil }

func jsonValue(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return map[string]any{}
	}
	return value
}

func structFields(value any) (map[string]any, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode response payload: %w", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, fmt.Errorf("decode response payload: %w", err)
	}
	if fields == nil {
		return nil, errors.New("response payload must be a JSON object")
	}
	return fields, nil
}
