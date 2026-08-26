package whatsapp

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/arandu-io/framework/security"

	internalconfig "github.com/hyz-is/arandu-whatsapp/internal/config"
	"github.com/hyz-is/arandu-whatsapp/internal/message"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
)

const (
	// DefaultPrefix is where the module mounts its routes when Config.Prefix is
	// empty.
	DefaultPrefix = "/whatsapp"
	// DefaultWebhookRetention is the default lifetime of durable delivery
	// snapshots, including failed deliveries that could otherwise retain PII.
	DefaultWebhookRetention = webhooksvc.DefaultDeliveryRetention
	// DefaultProcessingRetention is the default lifetime of a recoverable
	// mention-all snapshot and its native queue jobs.
	DefaultProcessingRetention = message.DefaultProcessingRetention
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Config is the complete typed configuration for the WhatsApp module. The
// package never reads environment variables; the host application owns that
// translation and passes the result here.
type Config struct {
	// Tenant is the complete runtime scope of this module instance. Requests
	// carrying a different tenant are refused before authorization or SQL, and
	// all reconnect/webhook background work uses this tenant.
	Tenant string
	// Prefix is the absolute path under which the module mounts its routes.
	Prefix string
	// WhatsApp controls pairing, reconnect and device behavior.
	WhatsApp WhatsAppConfig
	// Persistence selects the inbound data retained by the module.
	Persistence PersistenceConfig
	// Webhooks controls outbound webhook delivery.
	Webhooks WebhookConfig
	// Processing controls durable mention-all execution and retention.
	Processing ProcessingConfig
	// Media controls downloads, transcoding and temporary files.
	Media MediaConfig
	// Policy maps explicitly opened actions to application roles.
	Policy PolicyConfig
}

// PolicyConfig maps each explicitly enabled action to the roles allowed to
// perform it. An empty map is the safe default and denies every action.
type PolicyConfig struct {
	// Roles lists the roles allowed for each action. Missing actions are denied.
	Roles map[security.Action][]string
}

// WhatsAppConfig controls device identity, pairing, reconnect and address
// resolution.
type WhatsAppConfig struct {
	// QRCodeLimit is the maximum number of QR codes emitted by one pairing flow.
	QRCodeLimit int
	// QRCodeExpiration is how long one QR code remains usable.
	QRCodeExpiration time.Duration
	// QRCodeLightColor is the generated QR code background in #RRGGBB form.
	QRCodeLightColor string
	// QRCodeDarkColor is the generated QR code foreground in #RRGGBB form.
	QRCodeDarkColor string
	// SessionPhoneClient identifies the emulated WhatsApp client class.
	SessionPhoneClient string
	// SessionPhoneName is the device name WhatsApp displays for the session.
	SessionPhoneName string
	// PairingTimeout bounds one QR, phone-code or Passkey pairing flow.
	PairingTimeout time.Duration
	// AutoReconnect restores eligible sessions when the module starts.
	AutoReconnect bool
	// StartupReconnectConcurrency bounds parallel session restoration.
	StartupReconnectConcurrency int
	// ConnectTimeout bounds one WhatsApp connection attempt.
	ConnectTimeout time.Duration
	// ReconnectInitialDelay is the first delay between reconnect attempts.
	ReconnectInitialDelay time.Duration
	// ReconnectMaxDelay caps reconnect backoff.
	ReconnectMaxDelay time.Duration
	// ProfilePictureTimeout bounds profile-picture lookups.
	ProfilePictureTimeout time.Duration
	// AddressCacheTTL controls persisted phone-to-JID resolution freshness.
	AddressCacheTTL time.Duration
}

// PersistenceConfig selects which asynchronous WhatsApp events are retained.
type PersistenceConfig struct {
	// Messages retains inbound and outbound message snapshots.
	Messages bool
	// MessageUpdates retains receipt and status transitions.
	MessageUpdates bool
	// Contacts retains contact snapshots received from WhatsApp.
	Contacts bool
}

// WebhookConfig controls the optional global destination. Durable delivery is
// processed by the host application's native queue worker.
type WebhookConfig struct {
	// GlobalURL receives every enabled event in addition to instance webhooks.
	GlobalURL string
	// GlobalEnabled enables delivery to GlobalURL.
	GlobalEnabled bool
	// SigningSecret signs every outbound delivery with HMAC-SHA256. It is
	// required before any global or instance webhook can be enabled and must
	// contain at least 32 bytes after surrounding whitespace is ignored.
	SigningSecret string
	// Retention is the maximum lifetime of delivery snapshots. Zero uses
	// DefaultWebhookRetention.
	Retention time.Duration
	// Workers is retained for source compatibility and is ignored. Configure
	// worker concurrency with `aru queue:work --workers=N`.
	//
	// Deprecated: use the host queue worker concurrency instead.
	Workers int
	// QueueSize is retained for source compatibility and is ignored because the
	// native database queue is durable.
	//
	// Deprecated: the native database queue has no in-memory queue size.
	QueueSize int
}

// ProcessingConfig controls durable mention-all execution.
type ProcessingConfig struct {
	// Workers is retained for source compatibility and is ignored. Configure
	// concurrency with `aru queue:work --workers=N`.
	//
	// Deprecated: use the host queue worker concurrency instead.
	Workers int
	// QueueSize is retained for source compatibility and is ignored because the
	// native database queue is durable.
	//
	// Deprecated: the native database queue has no in-memory queue size.
	QueueSize int
	// ProcessingTimeout bounds the complete mention-all operation.
	ProcessingTimeout time.Duration
	// GroupInfoTimeout bounds the participant lookup used by mention-all.
	GroupInfoTimeout time.Duration
	// SendTimeout bounds the final WhatsApp send operation.
	SendTimeout time.Duration
	// Retention bounds how long a failed or abandoned mention-all snapshot and
	// its recoverable dead letter remain available. Zero uses
	// DefaultProcessingRetention.
	Retention time.Duration
}

// MediaConfig declares external executable paths and bounded temporary-media
// settings. Empty executable paths use the conventional command names.
type MediaConfig struct {
	// FFmpegPath is the ffmpeg executable path or command name.
	FFmpegPath string
	// FFprobePath is the ffprobe executable path or command name.
	FFprobePath string
	// TempDir is the parent for secure transient media directories.
	TempDir string
	// MaxInputBytes is the maximum accepted media input size.
	MaxInputBytes int64
	// MaxDurationSeconds is the maximum accepted audio duration.
	MaxDurationSeconds uint32
	// ProcessingTimeout bounds one media transformation.
	ProcessingTimeout time.Duration
}

// Validate reports unusable configuration before any worker or network client
// is started.
func (c Config) Validate() error {
	if !security.ValidTenant(c.Tenant) {
		return fmt.Errorf("whatsapp: Config.Tenant %q is invalid; use lowercase letters, digits, - and _, up to 64 characters", c.Tenant)
	}
	if c.Prefix != "" && (!strings.HasPrefix(c.Prefix, "/") || c.Prefix == "/") {
		return fmt.Errorf("whatsapp: Config.Prefix %q must be an absolute non-root path", c.Prefix)
	}
	for name, value := range map[string]int{
		"WhatsApp.QRCodeLimit":                 c.WhatsApp.QRCodeLimit,
		"WhatsApp.StartupReconnectConcurrency": c.WhatsApp.StartupReconnectConcurrency,
	} {
		if value < 0 {
			return fmt.Errorf("whatsapp: Config.%s cannot be negative", name)
		}
	}
	for name, duration := range map[string]time.Duration{
		"WhatsApp.QRCodeExpiration":      c.WhatsApp.QRCodeExpiration,
		"WhatsApp.PairingTimeout":        c.WhatsApp.PairingTimeout,
		"WhatsApp.ConnectTimeout":        c.WhatsApp.ConnectTimeout,
		"WhatsApp.ReconnectInitialDelay": c.WhatsApp.ReconnectInitialDelay,
		"WhatsApp.ReconnectMaxDelay":     c.WhatsApp.ReconnectMaxDelay,
		"WhatsApp.ProfilePictureTimeout": c.WhatsApp.ProfilePictureTimeout,
		"WhatsApp.AddressCacheTTL":       c.WhatsApp.AddressCacheTTL,
		"Processing.ProcessingTimeout":   c.Processing.ProcessingTimeout,
		"Processing.GroupInfoTimeout":    c.Processing.GroupInfoTimeout,
		"Processing.SendTimeout":         c.Processing.SendTimeout,
		"Processing.Retention":           c.Processing.Retention,
		"Media.ProcessingTimeout":        c.Media.ProcessingTimeout,
		"Webhooks.Retention":             c.Webhooks.Retention,
	} {
		if duration < 0 {
			return fmt.Errorf("whatsapp: Config.%s cannot be negative", name)
		}
	}
	if c.Media.MaxInputBytes < 0 {
		return fmt.Errorf("whatsapp: Config.Media.MaxInputBytes cannot be negative")
	}
	if c.WhatsApp.QRCodeLightColor != "" && !hexColor.MatchString(c.WhatsApp.QRCodeLightColor) {
		return fmt.Errorf("whatsapp: Config.WhatsApp.QRCodeLightColor must be #RRGGBB")
	}
	if c.WhatsApp.QRCodeDarkColor != "" && !hexColor.MatchString(c.WhatsApp.QRCodeDarkColor) {
		return fmt.Errorf("whatsapp: Config.WhatsApp.QRCodeDarkColor must be #RRGGBB")
	}
	if c.WhatsApp.ReconnectInitialDelay > 0 && c.WhatsApp.ReconnectMaxDelay > 0 &&
		c.WhatsApp.ReconnectMaxDelay < c.WhatsApp.ReconnectInitialDelay {
		return fmt.Errorf("whatsapp: Config.WhatsApp.ReconnectMaxDelay must be at least ReconnectInitialDelay")
	}
	if c.Webhooks.GlobalEnabled && strings.TrimSpace(c.Webhooks.GlobalURL) == "" {
		return fmt.Errorf("whatsapp: Config.Webhooks.GlobalURL is required when GlobalEnabled is true")
	}
	if c.Webhooks.GlobalEnabled && c.Webhooks.SigningSecret == "" {
		return fmt.Errorf("whatsapp: Config.Webhooks.SigningSecret is required when GlobalEnabled is true")
	}
	if c.Webhooks.SigningSecret != "" && len([]byte(strings.TrimSpace(c.Webhooks.SigningSecret))) < 32 {
		return fmt.Errorf("whatsapp: Config.Webhooks.SigningSecret must contain at least 32 bytes after trimming surrounding whitespace")
	}
	if raw := strings.TrimSpace(c.Webhooks.GlobalURL); raw != "" {
		if len(raw) > webhooksvc.MaxURLLength {
			return fmt.Errorf("whatsapp: Config.Webhooks.GlobalURL cannot exceed %d bytes", webhooksvc.MaxURLLength)
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("whatsapp: Config.Webhooks.GlobalURL must be an absolute HTTP or HTTPS URL")
		}
	}
	for action, roles := range c.Policy.Roles {
		if !isWhatsAppAction(action) {
			return fmt.Errorf("whatsapp: Config.Policy contains unknown action %q", action)
		}
		for _, role := range roles {
			if strings.TrimSpace(role) == "" {
				return fmt.Errorf("whatsapp: Config.Policy action %q contains an empty role", action)
			}
		}
	}
	return nil
}

func (c Config) withDefaults() Config {
	if c.Prefix == "" {
		c.Prefix = DefaultPrefix
	}
	c.Prefix = strings.TrimSuffix(c.Prefix, "/")
	wa := &c.WhatsApp
	if wa.QRCodeLimit == 0 {
		wa.QRCodeLimit = 5
	}
	if wa.QRCodeExpiration == 0 {
		wa.QRCodeExpiration = 30 * time.Second
	}
	if wa.QRCodeLightColor == "" {
		wa.QRCodeLightColor = "#ffffff"
	}
	if wa.QRCodeDarkColor == "" {
		wa.QRCodeDarkColor = "#198754"
	}
	if wa.SessionPhoneClient == "" {
		wa.SessionPhoneClient = internalconfig.DefaultSessionPhoneClient
	}
	if wa.SessionPhoneName == "" {
		wa.SessionPhoneName = internalconfig.DefaultSessionPhoneName
	}
	if wa.PairingTimeout == 0 {
		wa.PairingTimeout = 3 * time.Minute
	}
	if wa.StartupReconnectConcurrency == 0 {
		wa.StartupReconnectConcurrency = 5
	}
	if wa.ConnectTimeout == 0 {
		wa.ConnectTimeout = 30 * time.Second
	}
	if wa.ReconnectInitialDelay == 0 {
		wa.ReconnectInitialDelay = 2 * time.Second
	}
	if wa.ReconnectMaxDelay == 0 {
		wa.ReconnectMaxDelay = 60 * time.Second
	}
	if wa.ProfilePictureTimeout == 0 {
		wa.ProfilePictureTimeout = 15 * time.Second
	}
	if wa.AddressCacheTTL == 0 {
		wa.AddressCacheTTL = 168 * time.Hour
	}
	if c.Webhooks.Retention == 0 {
		c.Webhooks.Retention = DefaultWebhookRetention
	}
	processing := message.DefaultProcessingConfig()
	if c.Processing.ProcessingTimeout == 0 {
		c.Processing.ProcessingTimeout = processing.ProcessingTimeout
	}
	if c.Processing.GroupInfoTimeout == 0 {
		c.Processing.GroupInfoTimeout = processing.GroupInfoTimeout
	}
	if c.Processing.SendTimeout == 0 {
		c.Processing.SendTimeout = processing.SendTimeout
	}
	if c.Processing.Retention == 0 {
		c.Processing.Retention = processing.Retention
	}
	media := message.DefaultAudioConfig()
	if c.Media.FFmpegPath == "" {
		c.Media.FFmpegPath = "ffmpeg"
	}
	if c.Media.FFprobePath == "" {
		c.Media.FFprobePath = "ffprobe"
	}
	if c.Media.MaxInputBytes == 0 {
		c.Media.MaxInputBytes = media.MaxInputBytes
	}
	if c.Media.MaxDurationSeconds == 0 {
		c.Media.MaxDurationSeconds = media.MaxDurationSeconds
	}
	if c.Media.ProcessingTimeout == 0 {
		c.Media.ProcessingTimeout = media.ProcessingTimeout
	}
	return c
}

func (c Config) internalWhatsApp() internalconfig.WhatsAppConfig {
	return internalconfig.WhatsAppConfig{
		QRCodeLimit: c.WhatsApp.QRCodeLimit, QRCodeExpirationTime: c.WhatsApp.QRCodeExpiration,
		QRCodeLightColor: c.WhatsApp.QRCodeLightColor, QRCodeDarkColor: c.WhatsApp.QRCodeDarkColor,
		SessionPhoneClient: c.WhatsApp.SessionPhoneClient, SessionPhoneName: c.WhatsApp.SessionPhoneName,
		PairingTimeout: c.WhatsApp.PairingTimeout, AutoReconnect: c.WhatsApp.AutoReconnect,
		StartupReconnectConcurrency: c.WhatsApp.StartupReconnectConcurrency,
		ConnectTimeout:              c.WhatsApp.ConnectTimeout, ReconnectInitialDelay: c.WhatsApp.ReconnectInitialDelay,
		ReconnectMaxDelay: c.WhatsApp.ReconnectMaxDelay, ProfilePictureTimeout: c.WhatsApp.ProfilePictureTimeout,
		AddressCacheTTL: c.WhatsApp.AddressCacheTTL,
	}
}
