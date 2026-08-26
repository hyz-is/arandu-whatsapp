// Package config contains the runtime settings consumed by the WhatsApp
// implementation. The public package translates its typed Config into these
// values; this package never reads environment variables.
package config

import (
	"strings"
	"time"
)

const (
	// DefaultSessionPhoneClient is the platform reported to WhatsApp when the
	// application does not choose one.
	DefaultSessionPhoneClient = "DESKTOP"
	// DefaultSessionPhoneName is the device name reported to WhatsApp when the
	// application does not choose one.
	DefaultSessionPhoneName = "Arandu"
)

// WhatsAppConfig controls pairing, reconnects and address resolution.
type WhatsAppConfig struct {
	QRCodeLimit                 int
	QRCodeExpirationTime        time.Duration
	QRCodeLightColor            string
	QRCodeDarkColor             string
	SessionPhoneClient          string
	SessionPhoneName            string
	PairingTimeout              time.Duration
	AutoReconnect               bool
	StartupReconnectConcurrency int
	ConnectTimeout              time.Duration
	ReconnectInitialDelay       time.Duration
	ReconnectMaxDelay           time.Duration
	ProfilePictureTimeout       time.Duration
	AddressCacheTTL             time.Duration
}

// MaximumPairingTime returns the explicit pairing timeout or the lifetime of
// all configured QR codes.
func (c WhatsAppConfig) MaximumPairingTime() time.Duration {
	if c.PairingTimeout > 0 {
		return c.PairingTimeout
	}
	return time.Duration(c.QRCodeLimit) * c.QRCodeExpirationTime
}

// WhatsAppSessionConfig is retained as the implementation's store selector.
// Arandu supplies the already-open database, so an empty value means that
// shared handle and a non-empty URL is intentionally unsupported by the public
// package.
type WhatsAppSessionConfig struct {
	PostgresURL string
}

// PostgresDSN returns the dedicated URL when present, otherwise the main URL.
func (c WhatsAppSessionConfig) PostgresDSN(mainDatabaseURL string) string {
	if value := strings.TrimSpace(c.PostgresURL); value != "" {
		return value
	}
	return strings.TrimSpace(mainDatabaseURL)
}
