package whatsapp

import (
	"context"
	"fmt"
	"time"

	"github.com/arandu-io/framework/security"

	"github.com/hyz-is/arandu-whatsapp/app/Enums"
	appconfig "github.com/hyz-is/arandu-whatsapp/config"
)

// Instance actions identify permissions for instance lifecycle operations.
const (
	ActionInstanceCreate security.Action = enums.ActionInstanceCreate
	ActionInstanceList   security.Action = enums.ActionInstanceList
	ActionInstanceView   security.Action = enums.ActionInstanceView
	ActionInstanceUpdate security.Action = enums.ActionInstanceUpdate
	ActionInstanceDelete security.Action = enums.ActionInstanceDelete
)

// Connection actions identify permissions for pairing and session operations.
const (
	ActionConnectionPair   security.Action = enums.ActionConnectionPair
	ActionConnectionView   security.Action = enums.ActionConnectionView
	ActionConnectionLogout security.Action = enums.ActionConnectionLogout
)

// Webhook actions identify permissions for webhook configuration operations.
const (
	ActionWebhookSet  security.Action = enums.ActionWebhookSet
	ActionWebhookView security.Action = enums.ActionWebhookView
)

// Message actions identify permissions for message operations.
const (
	ActionMessageSend          security.Action = enums.ActionMessageSend
	ActionMessageList          security.Action = enums.ActionMessageList
	ActionMessageRead          security.Action = enums.ActionMessageRead
	ActionMessageDelete        security.Action = enums.ActionMessageDelete
	ActionMessageEdit          security.Action = enums.ActionMessageEdit
	ActionMessageMediaDownload security.Action = enums.ActionMessageMediaDownload
)

// Contact, chat, profile and call actions identify their respective permissions.
const (
	ActionContactCheck       security.Action = enums.ActionContactCheck
	ActionChatArchive        security.Action = enums.ActionChatArchive
	ActionProfilePictureView security.Action = enums.ActionProfilePictureView
	ActionCallReject         security.Action = enums.ActionCallReject
)

// Group actions identify permissions for group operations.
const (
	ActionGroupCreate            security.Action = enums.ActionGroupCreate
	ActionGroupPictureUpdate     security.Action = enums.ActionGroupPictureUpdate
	ActionGroupInviteView        security.Action = enums.ActionGroupInviteView
	ActionGroupInviteRevoke      security.Action = enums.ActionGroupInviteRevoke
	ActionGroupParticipantUpdate security.Action = enums.ActionGroupParticipantUpdate
	ActionGroupLeave             security.Action = enums.ActionGroupLeave
)

// ActionRuntime identifies the internal permission used by background work.
const ActionRuntime security.Action = enums.ActionRuntime

// Actions is a snapshot of the complete public permission vocabulary.
var Actions = append([]security.Action(nil), enums.Actions...)

// InstancePolicy protects every operation on WhatsApp instances and their
// children. No role is enabled unless Config.Policy explicitly lists it.
type InstancePolicy struct{ roles map[security.Action][]string }

// NewInstancePolicy builds a default-deny policy from typed role mappings.
func NewInstancePolicy(cfg PolicyConfig) InstancePolicy {
	roles := make(map[security.Action][]string, len(cfg.Roles))
	for action, allowed := range cfg.Roles {
		roles[action] = append([]string(nil), allowed...)
	}
	return InstancePolicy{roles: roles}
}

var _ security.Policy[Instance] = InstancePolicy{}

// Can decides whether a subject may perform an action on an instance.
func (p InstancePolicy) Can(_ context.Context, subject security.Subject, action security.Action, instance Instance) error {
	if instance.TenantID != "" && instance.TenantID != subject.Tenant {
		return fmt.Errorf("whatsapp instance belongs to another tenant")
	}
	for _, role := range p.roles[action] {
		if subject.HasRole(role) {
			return nil
		}
	}
	return fmt.Errorf("no configured role allows %s", action)
}

const (
	// DefaultPrefix is where the module mounts its routes when Config.Prefix is empty.
	DefaultPrefix = appconfig.DefaultPrefix
	// DefaultWebhookRetention is the default lifetime of durable delivery snapshots.
	DefaultWebhookRetention = appconfig.DefaultWebhookRetention
	// DefaultProcessingRetention is the default lifetime of recoverable processing snapshots.
	DefaultProcessingRetention = appconfig.DefaultProcessingRetention
)

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
	return c.toAppConfig().Validate()
}

func (c Config) withDefaults() Config {
	return fromAppConfig(appconfig.WithDefaults(c.toAppConfig()))
}

func (c Config) toAppConfig() appconfig.Config {
	return appconfig.Config{
		Tenant:      c.Tenant,
		Prefix:      c.Prefix,
		WhatsApp:    appconfig.WhatsAppConfig(c.WhatsApp),
		Persistence: appconfig.PersistenceConfig(c.Persistence),
		Webhooks:    appconfig.WebhookConfig(c.Webhooks),
		Processing:  appconfig.ProcessingConfig(c.Processing),
		Media:       appconfig.MediaConfig(c.Media),
		Policy:      appconfig.PolicyConfig(c.Policy),
	}
}

func fromAppConfig(c appconfig.Config) Config {
	return Config{
		Tenant:      c.Tenant,
		Prefix:      c.Prefix,
		WhatsApp:    WhatsAppConfig(c.WhatsApp),
		Persistence: PersistenceConfig(c.Persistence),
		Webhooks:    WebhookConfig(c.Webhooks),
		Processing:  ProcessingConfig(c.Processing),
		Media:       MediaConfig(c.Media),
		Policy:      PolicyConfig(c.Policy),
	}
}
