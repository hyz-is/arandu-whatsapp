package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/arandu-io/framework/security"
	hlog "github.com/arandu-io/hesape/log"
	"go.mau.fi/whatsmeow"
	watypes "go.mau.fi/whatsmeow/types"

	"github.com/hyz-is/arandu-whatsapp/internal/database/repository"
	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

const passkeyCommandTimeout = 30 * time.Second

type PasskeyClient interface {
	IsConnected() bool
	IsLoggedIn() bool
	HasLinkedDevice() bool
	GetPasskeyRequestOptions(ctx context.Context) (*watypes.WebAuthnPublicKey, error)
	SendPasskeyResponse(ctx context.Context, response *watypes.WebAuthnResponse) error
	SendPasskeyConfirmation(ctx context.Context) error
}

type whatsmeowPasskeyClient struct {
	client *whatsmeow.Client
}

func newWhatsmeowPasskeyClient(managed *ManagedWhatsAppClient) PasskeyClient {
	if managed == nil || managed.Client == nil {
		return nil
	}
	return whatsmeowPasskeyClient{client: managed.Client}
}

func (c whatsmeowPasskeyClient) IsConnected() bool {
	return c.client != nil && c.client.IsConnected()
}

func (c whatsmeowPasskeyClient) IsLoggedIn() bool {
	return c.client != nil && c.client.IsLoggedIn()
}

func (c whatsmeowPasskeyClient) HasLinkedDevice() bool {
	return c.client != nil && c.client.Store != nil && c.client.Store.ID != nil
}

func (c whatsmeowPasskeyClient) GetPasskeyRequestOptions(ctx context.Context) (*watypes.WebAuthnPublicKey, error) {
	if c.client == nil {
		return nil, ErrClientNotConnected
	}
	return c.client.DangerousInternals().GetPasskeyRequestOptions(ctx)
}

func (c whatsmeowPasskeyClient) SendPasskeyResponse(ctx context.Context, response *watypes.WebAuthnResponse) error {
	if c.client == nil {
		return ErrClientNotConnected
	}
	return c.client.SendPasskeyResponse(ctx, response)
}

func (c whatsmeowPasskeyClient) SendPasskeyConfirmation(ctx context.Context) error {
	if c.client == nil {
		return ErrClientNotConnected
	}
	return c.client.SendPasskeyConfirmation(ctx)
}

type PasskeyChallengeResult struct {
	RequestID string
	State     PasskeyPairingState
	ExpiresAt time.Time
	PublicKey *watypes.WebAuthnPublicKey
}

type SubmitPasskeyAssertionRequest struct {
	RequestID string                   `json:"requestId" validate:"required,uuid"`
	Assertion watypes.WebAuthnResponse `json:"assertion" validate:"required"`
}

type PasskeyAssertionResult struct {
	State   PasskeyPairingState
	Message string
}

func (s *Service) RequestPasskeyChallenge(ctx context.Context, grant security.Grant, instanceName string) (PasskeyChallengeResult, error) {
	instance, err := s.authenticatePasskeyInstance(ctx, grant, instanceName)
	if err != nil {
		return PasskeyChallengeResult{}, err
	}
	session, managed, client, err := s.activePasskeySession(instance.Instance.ID)
	if err != nil {
		return PasskeyChallengeResult{}, err
	}
	if err := validatePasskeyClientReady(client); err != nil {
		return PasskeyChallengeResult{}, err
	}
	if !session.passkeyCommandMu.TryLock() {
		return PasskeyChallengeResult{}, ErrInvalidPairingState
	}
	defer session.passkeyCommandMu.Unlock()

	if snapshot, ok, err := session.beginPasskeyChallengeFetch(); err != nil {
		return PasskeyChallengeResult{}, err
	} else if ok {
		return PasskeyChallengeResult{
			RequestID: snapshot.RequestID,
			State:     snapshot.State,
			ExpiresAt: snapshot.ExpiresAt,
			PublicKey: snapshot.PublicKey,
		}, nil
	}

	commandCtx, cancel := context.WithTimeout(sessionContext(session), passkeyCommandTimeout)
	defer cancel()
	startedAt := time.Now()
	publicKey, err := client.GetPasskeyRequestOptions(commandCtx)
	if err != nil {
		session.markPasskeyFailed(err)
		hlog.For(ctx).WarnContext(ctx, "passkey challenge fetch failed",
			"component", "whatsapp_service", "error", err, "instance_id", managed.InstanceID, "instance_name", managed.InstanceName,
			"event", "connection.passkey.challenge.fetch_failed", "duration", time.Since(startedAt))
		return PasskeyChallengeResult{}, fmt.Errorf("%w: challenge", ErrPasskeyServiceUnavailable)
	}
	if publicKey == nil {
		session.markPasskeyFailed(ErrPasskeyNotAvailable)
		return PasskeyChallengeResult{}, ErrPasskeyNotAvailable
	}
	snapshot := session.setPasskeyChallenge(publicKey)
	s.publishPasskeyRequired(managed, snapshot)
	hlog.For(ctx).InfoContext(ctx, "passkey challenge available",
		"component", "whatsapp_service", "instance_id", managed.InstanceID, "instance_name", managed.InstanceName,
		"request_id", snapshot.RequestID, "passkey_state", string(snapshot.State), "event", "connection.passkey.challenge.created",
		"expires_at", snapshot.ExpiresAt, "duration", time.Since(startedAt))
	return PasskeyChallengeResult{
		RequestID: snapshot.RequestID,
		State:     snapshot.State,
		ExpiresAt: snapshot.ExpiresAt,
		PublicKey: snapshot.PublicKey,
	}, nil
}

func (s *Service) SubmitPasskeyAssertion(ctx context.Context, grant security.Grant, instanceName string, request SubmitPasskeyAssertionRequest) (PasskeyAssertionResult, error) {
	instance, err := s.authenticatePasskeyInstance(ctx, grant, instanceName)
	if err != nil {
		return PasskeyAssertionResult{}, err
	}
	session, managed, client, err := s.activePasskeySession(instance.Instance.ID)
	if err != nil {
		return PasskeyAssertionResult{}, err
	}
	if err := validatePasskeyClientReady(client); err != nil {
		return PasskeyAssertionResult{}, err
	}
	if !session.passkeyCommandMu.TryLock() {
		return PasskeyAssertionResult{}, ErrInvalidPairingState
	}
	defer session.passkeyCommandMu.Unlock()

	if err := session.reservePasskeyAssertion(request.RequestID, request.Assertion); err != nil {
		return PasskeyAssertionResult{}, err
	}

	commandCtx, cancel := context.WithTimeout(sessionContext(session), passkeyCommandTimeout)
	defer cancel()
	startedAt := time.Now()
	if err := client.SendPasskeyResponse(commandCtx, &request.Assertion); err != nil {
		session.markPasskeyFailed(err)
		s.publishPasskeyFailed(managed, "PASSKEY_PAIRING_FAILED", "Passkey pairing could not be completed.")
		s.updateQRFailure(managed.RuntimeGrant, mustAtoi64(managed.InstanceID), dbtypes.InstanceConnectionStatusConnectionError, "passkey_response_error", nil)
		hlog.For(ctx).WarnContext(ctx, "passkey assertion submission failed",
			"component", "whatsapp_service", "error", err, "instance_id", managed.InstanceID, "instance_name", managed.InstanceName,
			"request_id", request.RequestID, "passkey_state", string(PasskeyStateFailed),
			"event", "connection.passkey.assertion.failed", "duration", time.Since(startedAt))
		return PasskeyAssertionResult{}, fmt.Errorf("%w: assertion", ErrPasskeyServiceUnavailable)
	}
	session.markPasskeyAwaitingConfirmation()
	hlog.For(ctx).InfoContext(ctx, "passkey assertion submitted",
		"component", "whatsapp_service", "instance_id", managed.InstanceID, "instance_name", managed.InstanceName,
		"request_id", request.RequestID, "passkey_state", string(PasskeyStateAwaitingConfirmation),
		"event", "connection.passkey.assertion.submitted", "duration", time.Since(startedAt))
	return PasskeyAssertionResult{
		State:   PasskeyStateAwaitingConfirmation,
		Message: "A assertion foi enviada ao WhatsApp.",
	}, nil
}

func (s *Service) activePasskeySession(instanceID int64) (*pairingSession, *ManagedWhatsAppClient, PasskeyClient, error) {
	id := strconv.FormatInt(instanceID, 10)
	session, ok := s.pairings.get(id)
	if !ok || session == nil {
		return nil, nil, nil, ErrPairingSessionNotFound
	}
	managed, ok := s.hub.GetByInstanceID(id)
	if !ok || managed == nil {
		return nil, nil, nil, ErrClientNotConnected
	}
	clientFactory := s.passkeyClient
	if clientFactory == nil {
		clientFactory = newWhatsmeowPasskeyClient
	}
	client := clientFactory(managed)
	if client == nil {
		return nil, nil, nil, ErrClientNotConnected
	}
	return session, managed, client, nil
}

func (s *Service) authenticatePasskeyInstance(ctx context.Context, grant security.Grant, instanceName string) (dbtypes.InstanceRecord, error) {
	instance, err := s.authenticateInstance(ctx, grant, instanceName)
	if errors.Is(err, repository.ErrInstanceNotFound) {
		return dbtypes.InstanceRecord{}, fmt.Errorf("%w: %w", ErrPasskeyInstanceNotFound, err)
	}
	return instance, err
}

func validatePasskeyClientReady(client PasskeyClient) error {
	if client == nil || !client.IsConnected() {
		return ErrClientNotConnected
	}
	if client.IsLoggedIn() || client.HasLinkedDevice() {
		return ErrInstanceConnected
	}
	return nil
}

func sessionContext(session *pairingSession) context.Context {
	if session != nil && session.ctx != nil {
		return session.ctx
	}
	return context.Background()
}
