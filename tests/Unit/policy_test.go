package unit_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	whatsapp "github.com/hyz-is/arandu-whatsapp"
)

func administrator(tenant string) security.Subject {
	return security.Subject{ID: "user-1", Tenant: tenant, Roles: []string{"admin"}, Verified: true}
}

func TestPolicyIsDefaultDenyForEveryPublicAction(t *testing.T) {
	t.Parallel()
	policy := whatsapp.NewInstancePolicy(whatsapp.PolicyConfig{})
	for _, action := range whatsapp.Actions {
		action := action
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()
			_, err := security.Authorize(context.Background(), policy, administrator("acme"), action,
				whatsapp.Instance{TenantID: "acme"})
			if !errors.Is(err, security.ErrForbidden) {
				t.Fatalf("default policy allowed %s: %v", action, err)
			}
		})
	}
}

func TestPolicyAllowsOnlyConfiguredRoleAndTenant(t *testing.T) {
	t.Parallel()
	policy := whatsapp.NewInstancePolicy(whatsapp.PolicyConfig{Roles: map[security.Action][]string{
		whatsapp.ActionInstanceView: {"admin"},
	}})
	if _, err := security.Authorize(context.Background(), policy, administrator("acme"),
		whatsapp.ActionInstanceView, whatsapp.Instance{TenantID: "acme"}); err != nil {
		t.Fatalf("configured role was refused: %v", err)
	}
	if _, err := security.Authorize(context.Background(), policy, administrator("acme"),
		whatsapp.ActionInstanceList, whatsapp.Instance{TenantID: "acme"}); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("unconfigured action was allowed: %v", err)
	}
	if _, err := security.Authorize(context.Background(), policy, administrator("acme"),
		whatsapp.ActionInstanceView, whatsapp.Instance{TenantID: "globex"}); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("cross-tenant record was allowed: %v", err)
	}
}

func TestPolicyOwnsAnImmutableRoleSnapshot(t *testing.T) {
	t.Parallel()
	roles := map[security.Action][]string{
		whatsapp.ActionInstanceView: {"admin"},
	}
	policy := whatsapp.NewInstancePolicy(whatsapp.PolicyConfig{Roles: roles})
	roles[whatsapp.ActionInstanceView][0] = "operator"
	delete(roles, whatsapp.ActionInstanceView)
	if _, err := security.Authorize(context.Background(), policy, administrator("acme"),
		whatsapp.ActionInstanceView, whatsapp.Instance{TenantID: "acme"}); err != nil {
		t.Fatalf("caller mutation changed the policy: %v", err)
	}
}

func nilRepository() *whatsapp.InstanceRepository {
	handle := data.Wrap(nil, data.DialectSQLite)
	return whatsapp.NewInstanceRepository(handle)
}

func TestRepositoryRejectsInvalidGrantsBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()
	repository := nilRepository()
	var none security.Grant
	if _, err := repository.Find(context.Background(), none, 1); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("Find accepted zero Grant: %v", err)
	}
	wrong := security.SystemGrant(whatsapp.ActionInstanceDelete, "acme")
	if _, err := repository.Find(context.Background(), wrong, 1); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("Find accepted wrong action: %v", err)
	}
	withoutTenant := security.SystemGrant(whatsapp.ActionInstanceView, "")
	if _, err := repository.Find(context.Background(), withoutTenant, 1); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("Find accepted Grant without tenant: %v", err)
	}
}

func TestModuleRefusesSubjectWithoutTenantBeforeDatabase(t *testing.T) {
	t.Parallel()
	sessions := security.NewSessionStore([]byte("0123456789abcdef0123456789abcdef"), time.Hour, false, security.NewMemoryBackend())
	module, err := whatsapp.New(whatsapp.Config{
		Tenant: "acme",
		Policy: whatsapp.PolicyConfig{Roles: map[security.Action][]string{
			whatsapp.ActionInstanceList: {"admin"},
		}},
	}, data.Wrap(nil, data.DialectSQLite), sessions)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.Service().ListInstances(context.Background(), administrator(""), whatsapp.InstanceListQuery{}); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("a subject with no tenant reached the module: %v", err)
	}
}

func TestConfigurationValidationAndDefaults(t *testing.T) {
	t.Parallel()
	invalid := []whatsapp.Config{
		{},
		{Tenant: "Acme"},
		{Tenant: "acme/reports"},
		{Tenant: "acme", Prefix: "whatsapp"},
		{Tenant: "acme", Webhooks: whatsapp.WebhookConfig{GlobalEnabled: true}},
		{Tenant: "acme", Webhooks: whatsapp.WebhookConfig{SigningSecret: "short"}},
		{Tenant: "acme", Webhooks: whatsapp.WebhookConfig{Retention: -time.Second}},
		{Tenant: "acme", Webhooks: whatsapp.WebhookConfig{GlobalURL: "https://example.com/" + strings.Repeat("x", 500)}},
		{Tenant: "acme", Processing: whatsapp.ProcessingConfig{SendTimeout: -time.Second}},
		{Tenant: "acme", Policy: whatsapp.PolicyConfig{Roles: map[security.Action][]string{"other.read": {"admin"}}}},
		{Tenant: "acme", Policy: whatsapp.PolicyConfig{Roles: map[security.Action][]string{whatsapp.ActionInstanceView: {""}}}},
	}
	for _, config := range invalid {
		if err := config.Validate(); err == nil {
			t.Errorf("invalid config was accepted: %#v", config)
		}
	}
	if err := (whatsapp.Config{Tenant: "acme"}).Validate(); err != nil {
		t.Fatalf("valid config was refused: %v", err)
	}
}

func TestAnActionListedInPolicyActionsIsDecidedByTheSubjectsOwnActions(t *testing.T) {
	t.Parallel()
	policy := whatsapp.NewInstancePolicy(whatsapp.PolicyConfig{Actions: []security.Action{whatsapp.ActionInstanceList}})
	carrying := security.Subject{ID: "u1", Tenant: "acme", Actions: []security.Action{whatsapp.ActionInstanceList, whatsapp.ActionInstanceDelete}}
	if err := policy.Can(context.Background(), carrying, whatsapp.ActionInstanceList, whatsapp.Instance{TenantID: "acme"}); err != nil {
		t.Fatalf("a subject carrying a listed action was refused: %v", err)
	}
	if err := policy.Can(context.Background(), carrying, whatsapp.ActionInstanceDelete, whatsapp.Instance{TenantID: "acme"}); err == nil {
		t.Fatal("an action the configuration does not list was allowed because the subject carries it")
	}
	without := security.Subject{ID: "u2", Tenant: "acme"}
	if err := policy.Can(context.Background(), without, whatsapp.ActionInstanceList, whatsapp.Instance{TenantID: "acme"}); err == nil {
		t.Fatal("a listed action was allowed to a subject that does not carry it")
	}
	if err := policy.Can(context.Background(), carrying, whatsapp.ActionInstanceList, whatsapp.Instance{TenantID: "globex"}); err == nil {
		t.Fatal("a carried action reached another tenant's instance")
	}
	guest := security.Guest("acme")
	guest.Actions = []security.Action{whatsapp.ActionInstanceList}
	if err := policy.Can(context.Background(), guest, whatsapp.ActionInstanceList, whatsapp.Instance{TenantID: "acme"}); err == nil {
		t.Fatal("a guest carrying the action was allowed")
	}
}
