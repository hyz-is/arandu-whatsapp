package webhook

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/cache"

	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

// DefaultConfigurationCacheTTL is how long a webhook configuration read is
// reused when the caller asked for caching without naming a window.
//
// It is short on purpose. The configuration decides where an event goes and
// whether it goes at all, so the cache is a bound on how long this process may
// act on a change another process made -- not a place to keep the answer.
const DefaultConfigurationCacheTTL = 5 * time.Second

// configurationCacheNamespace keeps these entries away from every other cache
// key of the application. The tenant is added by the cache itself.
//
// A namespace is one key segment, so it is lowercase letters, digits, - and _
// and nothing else. A dotted name is refused, and it is refused at dispatch,
// where the refusal is a webhook that did not go out.
const configurationCacheNamespace = "whatsapp_webhook_configuration"

// cachedConfiguration is what a lookup answered, including the answer that
// there is no configuration.
//
// The negative answer is cached because it is the common one: an application
// that only uses the global webhook has no per-instance row, and leaving that
// uncached means every event it dispatches reads the database to be told so
// again.
type cachedConfiguration struct {
	Found   bool          `json:"found"`
	Webhook types.Webhook `json:"webhook"`
}

// configurationCache reuses webhook configuration lookups for a bounded window.
//
// A nil cache, or a ttl of zero, reads through every time, which is what an
// application gets until it asks for something else.
type configurationCache struct {
	repository *cache.Repository
	ttl        time.Duration
}

func newConfigurationCache(repository *cache.Repository, ttl time.Duration) configurationCache {
	if repository == nil || ttl <= 0 {
		return configurationCache{}
	}
	return configurationCache{repository: repository.Namespace(configurationCacheNamespace), ttl: ttl}
}

func (c configurationCache) enabled() bool { return c.repository != nil && c.ttl > 0 }

func configurationCacheKey(instanceID int64) string {
	return strconv.FormatInt(instanceID, 10)
}

// lookup answers from the cache when it can, and otherwise calls read and
// remembers what it said.
//
// A cache that fails is not a lookup that fails: the value read is still the
// value, so a write-back error is dropped rather than turned into a webhook
// that was not dispatched.
func (c configurationCache) lookup(
	ctx context.Context,
	grant security.Grant,
	instanceID int64,
	read func(context.Context) (types.Webhook, error),
) (types.Webhook, error) {
	if !c.enabled() {
		return read(ctx)
	}
	key := configurationCacheKey(instanceID)
	entry, err := cache.Remember(ctx, c.repository, grant, key, c.ttl,
		func(inner context.Context) (cachedConfiguration, error) {
			webhook, readErr := read(inner)
			if errors.Is(readErr, errWebhookConfigurationNotFound) {
				return cachedConfiguration{}, nil
			}
			if readErr != nil {
				return cachedConfiguration{}, readErr
			}
			return cachedConfiguration{Found: true, Webhook: webhook}, nil
		})
	if err != nil {
		return types.Webhook{}, err
	}
	if !entry.Found {
		return types.Webhook{}, errWebhookConfigurationNotFound
	}
	return entry.Webhook, nil
}

// forget drops the entry for one instance, so a configuration this process
// wrote is in effect on the next event rather than at the end of the window.
func (c configurationCache) forget(ctx context.Context, grant security.Grant, instanceID int64) {
	if !c.enabled() {
		return
	}
	_ = c.repository.Forget(ctx, grant, configurationCacheKey(instanceID))
}
