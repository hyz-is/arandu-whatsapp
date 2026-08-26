package whatsapp

import (
	"database/sql"
	"testing"

	"github.com/arandu-io/framework/data"
)

func TestNewWhatsAppSessionContainerRequiresDatabase(t *testing.T) {
	_, err := NewWhatsAppSessionContainer(nil, nil)
	if err == nil {
		t.Fatal("expected error when no shared database is configured")
	}
}

func TestNewWhatsAppSessionContainerRejectsUnsupportedDialect(t *testing.T) {
	db := data.Wrap(&sql.DB{}, data.DialectMySQL)
	_, err := NewWhatsAppSessionContainer(db, nil)
	if err == nil {
		t.Fatal("expected MySQL to be rejected by the WhatsMeow store")
	}
}

func TestNewWhatsAppSessionContainerWrapsSQLiteDatabase(t *testing.T) {
	db := data.Wrap(&sql.DB{}, data.DialectSQLite)
	container, err := NewWhatsAppSessionContainer(db, nil)
	if err != nil {
		t.Fatalf("NewWhatsAppSessionContainer() error = %v", err)
	}
	if container == nil {
		t.Fatal("expected a sqlstore container")
	}
}
