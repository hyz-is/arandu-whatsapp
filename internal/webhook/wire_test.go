package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	hlog "github.com/arandu-io/hesape/log"
	"github.com/arandu-io/hesape/queue/jobs"
	hesapewebhook "github.com/arandu-io/hesape/webhook"

	"github.com/hyz-is/arandu-whatsapp/internal/authz"
	"github.com/hyz-is/arandu-whatsapp/internal/database/types"
)

// These tests hold what a webhook consumer receives from this module: the
// queued job, every header, the body bytes and the signature. The delivery
// engine underneath belongs to Hesape, and a change there must not be visible
// on the wire.

// wireBody is the exact body of a connection.update for testInstance with the
// clock fixed at wireClock.
const wireBody = `{"event":"connection.update","instance":{"id":1,"name":"beplus","connectionStatus":"online","ownerJid":"5531999999999@s.whatsapp.net","externalAttributes":{}},"data":{"state":"open"},"timestamp":"2026-10-15T12:00:00Z"}`

var wireClock = time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC)

// referenceSignature is the documented consumer recipe, written without
// Hesape: hex HMAC-SHA256 of "timestamp.deliveryID.body", prefixed "sha256=".
func referenceSignature(secret, timestamp, deliveryID string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + deliveryID + "."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestSignatureMatchesTheGoldenVector(t *testing.T) {
	// Computed outside Go:
	// printf '1760529600.delivery-1.{"event":"connection.update"}' |
	//   openssl dgst -sha256 -hmac 0123456789abcdef0123456789abcdef
	const want = "sha256=547608dfb4bcfa262776b828ff72a9718f008f6df892dc9b41aaafc8bf860ac9"
	body := []byte(`{"event":"connection.update"}`)
	if got := referenceSignature(testWebhookSigningSecret, "1760529600", "delivery-1", body); got != want {
		t.Fatalf("reference signature = %q, want %q", got, want)
	}
	if got := hesapewebhook.Sign([]byte(testWebhookSigningSecret), "1760529600", "delivery-1", body); got != want {
		t.Fatalf("Hesape signature = %q, want %q", got, want)
	}
}

func TestDeliveryWireIsPinned(t *testing.T) {
	db := newWebhookTestDB(t, true)
	db.insertInstance(t)
	db.insertWebhook(t, "https://instance.example/hook", true, types.WebhookEvents{ConnectionUpdated: true})
	type sent struct {
		method, url string
		header      http.Header
		body        []byte
	}
	var mu sync.Mutex
	var calls []sent
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		calls = append(calls, sent{r.Method, r.URL.String(), r.Header.Clone(), body})
		mu.Unlock()
		return testResponse(r, http.StatusNoContent, ""), nil
	})
	m := newTestManager(t, db.db, ManagerConfig{HTTPClient: &http.Client{Transport: transport}})
	m.now = func() time.Time { return wireClock }
	ctx := hlog.WithCollector(context.Background(), hlog.NewCollector("req-wire"))
	if err := m.Dispatch(ctx, testRuntimeGrant(), testInstance(), types.WebhookEventConnectionUpdated, map[string]string{"state": "open"}); err != nil {
		t.Fatal(err)
	}

	var deliveryID string
	if err := db.raw.QueryRow(`SELECT id FROM whatsapp_webhook_deliveries`).Scan(&deliveryID); err != nil {
		t.Fatal(err)
	}
	job := popDeliveryJob(t, db.db)
	if job.Queue != WebhookQueueName || job.Name != WebhookDeliveryJobName || job.Action != string(authz.ActionRuntime) {
		t.Fatalf("job queue=%q name=%q action=%q", job.Queue, job.Name, job.Action)
	}
	if want := `{"deliveryId":"` + deliveryID + `"}`; string(job.Payload) != want {
		t.Fatalf("job payload = %s, want %s", job.Payload, want)
	}
	if job.Attributes.Tries != 5 {
		t.Fatalf("job tries = %d, want 5", job.Attributes.Tries)
	}
	wantBackoff := []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}
	if !reflect.DeepEqual(job.Attributes.Backoff, wantBackoff) {
		t.Fatalf("job backoff = %v, want %v", job.Attributes.Backoff, wantBackoff)
	}

	before := time.Now().Unix()
	if err := registeredDeliveryHandler(t, m).Handle(context.Background(), jobs.GrantFor(job), job); err != nil {
		t.Fatal(err)
	}
	after := time.Now().Unix()
	if len(calls) != 1 {
		t.Fatalf("requests = %d, want 1", len(calls))
	}
	call := calls[0]
	if call.method != http.MethodPost || call.url != "https://instance.example/hook" {
		t.Fatalf("request = %s %s", call.method, call.url)
	}
	if string(call.body) != wireBody {
		t.Fatalf("body =\n%s\nwant\n%s", call.body, wireBody)
	}

	timestamp := call.header.Get("X-Arandu-Timestamp")
	signedAt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || signedAt < before || signedAt > after {
		t.Fatalf("X-Arandu-Timestamp = %q, want unix seconds in [%d, %d]", timestamp, before, after)
	}
	want := http.Header{
		"Content-Type":         {"application/json"},
		"User-Agent":           {"Arandu-WhatsApp/1.0"},
		"X-Request-Id":         {"req-wire"},
		"X-Owner-Jid":          {"5531999999999@s.whatsapp.net"},
		"X-Instance-Name":      {"beplus"},
		"X-Instance-Id":        {"1"},
		"X-Webhook-Event":      {"connection.update"},
		"X-Arandu-Delivery-Id": {deliveryID},
		"X-Arandu-Timestamp":   {timestamp},
		"X-Arandu-Signature":   {referenceSignature(testWebhookSigningSecret, timestamp, deliveryID, []byte(wireBody))},
	}
	if !reflect.DeepEqual(call.header, want) {
		t.Fatalf("headers =\n%v\nwant\n%v", call.header, want)
	}
}
