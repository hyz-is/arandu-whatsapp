package address

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	hlog "github.com/arandu-io/hesape/log"
	"go.mau.fi/whatsmeow/types"
	"golang.org/x/sync/singleflight"
)

type ResolutionSource string

const (
	ResolutionSourceDirect   ResolutionSource = "direct"
	ResolutionSourceCache    ResolutionSource = "cache"
	ResolutionSourceWhatsApp ResolutionSource = "whatsapp"
	ResolutionSourceLegacy   ResolutionSource = "legacy"
)

var (
	ErrInvalidAddress         = errors.New("invalid whatsapp address")
	ErrRecipientNotOnWhatsApp = errors.New("recipient not on whatsapp")
	ErrAmbiguousRecipient     = errors.New("ambiguous whatsapp recipient")
	ErrAddressMappingNotFound = errors.New("address mapping not found")
)

type ResolveInput struct {
	InstanceID int64
	Address    string
}

type ResolveResult struct {
	Input        string
	Normalized   string
	Candidates   []string
	CanonicalJID types.JID
	Source       ResolutionSource
	RemovedNinth bool
}

type Resolver interface {
	Resolve(ctx context.Context, grant security.Grant, client WhatsAppLookup, input ResolveInput) (ResolveResult, error)
}

type WhatsAppLookup interface {
	IsOnWhatsApp(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error)
}

type AddressMappingRepository interface {
	FindByAlias(ctx context.Context, grant security.Grant, instanceID int64, alias string) (*AddressMapping, error)
	Upsert(ctx context.Context, grant security.Grant, mapping AddressMapping) error
	DeleteByCanonicalJID(ctx context.Context, grant security.Grant, instanceID int64, canonicalJID string) error
}

type AddressMapping struct {
	InstanceID      int64
	NormalizedPhone string
	CanonicalJID    string
	LIDJID          *string
	Aliases         []string
	ResolvedAt      time.Time
	ExpiresAt       time.Time
}

type DefaultResolver struct {
	repository AddressMappingRepository
	ttl        time.Duration
	group      singleflight.Group
	now        func() time.Time
}

func NewResolver(repository AddressMappingRepository, ttl time.Duration) *DefaultResolver {
	if ttl <= 0 {
		ttl = 168 * time.Hour
	}
	return &DefaultResolver{
		repository: repository,
		ttl:        ttl,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (r *DefaultResolver) Resolve(ctx context.Context, grant security.Grant, client WhatsAppLookup, input ResolveInput) (ResolveResult, error) {
	parsed, err := parseAddress(input.Address)
	if err != nil {
		return ResolveResult{}, err
	}
	if parsed.direct {
		return ResolveResult{
			Input:        input.Address,
			Normalized:   parsed.jid.String(),
			CanonicalJID: parsed.jid,
			Source:       ResolutionSourceDirect,
		}, nil
	}
	if client == nil {
		return ResolveResult{}, fmt.Errorf("%w: whatsapp client is required", ErrInvalidAddress)
	}

	key := data.Tenant(grant) + ":" + strconv.FormatInt(input.InstanceID, 10) + ":" + parsed.number
	value, err, _ := r.group.Do(key, func() (any, error) {
		return r.resolveNumber(ctx, grant, client, input, parsed.number)
	})
	if err != nil {
		return ResolveResult{}, err
	}
	return value.(ResolveResult), nil
}

func (r *DefaultResolver) resolveNumber(ctx context.Context, grant security.Grant, client WhatsAppLookup, input ResolveInput, normalized string) (ResolveResult, error) {
	candidates := BuildCandidates(normalized)
	removedNinth := len(candidates) > 1 && len(candidates[1]) < len(candidates[0])

	if mapping, ok := r.findCached(ctx, grant, input.InstanceID, aliasesForCandidates(candidates)); ok {
		jid, err := types.ParseJID(mapping.CanonicalJID)
		if err != nil || jid.IsEmpty() {
			_ = r.repository.DeleteByCanonicalJID(ctx, grant, input.InstanceID, mapping.CanonicalJID)
		} else {
			return ResolveResult{
				Input:        input.Address,
				Normalized:   normalized,
				Candidates:   candidates,
				CanonicalJID: jid.ToNonAD(),
				Source:       ResolutionSourceCache,
				RemovedNinth: removedNinth,
			}, nil
		}
	}

	queries := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		queries = append(queries, "+"+candidate)
	}
	responses, err := client.IsOnWhatsApp(ctx, queries)
	if err != nil {
		return ResolveResult{}, err
	}

	byJID := make(map[string]types.JID)
	for _, response := range responses {
		if !response.IsIn || response.JID.IsEmpty() {
			continue
		}
		jid := response.JID.ToNonAD()
		byJID[jid.String()] = jid
	}
	if len(byJID) == 0 {
		return ResolveResult{}, ErrRecipientNotOnWhatsApp
	}
	if len(byJID) > 1 {
		hlog.For(ctx).DebugContext(ctx, "ambiguous WhatsApp address resolution",
			"component", "address_resolver",
			"instance_id", input.InstanceID,
			"input", MaskAddress(input.Address),
			"candidates", maskAll(candidates),
		)
		return ResolveResult{}, ErrAmbiguousRecipient
	}

	var canonical types.JID
	for _, jid := range byJID {
		canonical = jid
	}

	source := ResolutionSourceWhatsApp
	if removedNinth && canonical.User == candidates[1] {
		source = ResolutionSourceLegacy
	}
	result := ResolveResult{
		Input:        input.Address,
		Normalized:   normalized,
		Candidates:   candidates,
		CanonicalJID: canonical,
		Source:       source,
		RemovedNinth: removedNinth,
	}

	if r.repository != nil {
		now := r.now()
		aliases := aliasesForCandidates(candidates)
		aliases = appendUnique(aliases, canonical.String())
		if err := r.repository.Upsert(ctx, grant, AddressMapping{
			InstanceID:      input.InstanceID,
			NormalizedPhone: normalized,
			CanonicalJID:    canonical.String(),
			Aliases:         aliases,
			ResolvedAt:      now,
			ExpiresAt:       now.Add(r.ttl),
		}); err != nil {
			hlog.For(ctx).DebugContext(ctx, "failed to persist WhatsApp address mapping",
				"component", "address_resolver", "error", err, "instance_id", input.InstanceID)
		}
	}

	hlog.For(ctx).DebugContext(ctx, "resolved WhatsApp address",
		"component", "address_resolver",
		"instance_id", input.InstanceID,
		"input", MaskAddress(input.Address),
		"candidates", maskAll(candidates),
		"canonical_jid", MaskAddress(canonical.String()),
		"source", string(source),
		"removed_ninth_digit", removedNinth,
	)

	return result, nil
}

func (r *DefaultResolver) findCached(ctx context.Context, grant security.Grant, instanceID int64, aliases []string) (*AddressMapping, bool) {
	if r.repository == nil {
		return nil, false
	}
	now := r.now()
	for _, alias := range aliases {
		mapping, err := r.repository.FindByAlias(ctx, grant, instanceID, alias)
		if err != nil {
			if !errors.Is(err, ErrAddressMappingNotFound) {
				hlog.For(ctx).DebugContext(ctx, "failed to read WhatsApp address mapping",
					"component", "address_resolver", "error", err, "instance_id", instanceID)
			}
			continue
		}
		if mapping.ExpiresAt.After(now) {
			return mapping, true
		}
		_ = r.repository.DeleteByCanonicalJID(ctx, grant, instanceID, mapping.CanonicalJID)
	}
	return nil, false
}

func aliasesForCandidates(candidates []string) []string {
	aliases := make([]string, 0, len(candidates)*2)
	for _, candidate := range candidates {
		aliases = appendUnique(aliases, candidate)
		aliases = appendUnique(aliases, candidate+"@"+types.DefaultUserServer)
	}
	return aliases
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func maskAll(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, MaskAddress(value))
	}
	return out
}
