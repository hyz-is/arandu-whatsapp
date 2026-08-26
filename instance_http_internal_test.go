package whatsapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"
)

func TestInstanceHTTPPaginationAndPhoneBodyContract(t *testing.T) {
	db := migratedSQLite(t, "instance-http-contract")
	handle := data.Wrap(db, data.DialectSQLite)
	sessions := security.NewSessionStore([]byte(lifecycleTestKey), time.Hour, false, security.NewMemoryBackend())
	module, err := New(Config{
		Tenant: "acme",
		Policy: PolicyConfig{Roles: map[security.Action][]string{
			ActionInstanceList:   {"admin"},
			ActionConnectionPair: {"admin"},
		}},
	}, handle, sessions)
	if err != nil {
		t.Fatal(err)
	}
	repository := NewInstanceRepository(handle)
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		if _, err := repository.Create(context.Background(), security.SystemGrant(ActionInstanceCreate, "acme"), Instance{Name: name}); err != nil {
			t.Fatal(err)
		}
	}

	router := fhttp.NewRouter()
	module.Routes(router.ForModule(module.Name()))
	actor := security.Subject{ID: "user-1", Tenant: "acme", Roles: []string{"admin"}, Verified: true}
	sessionResponse := httptest.NewRecorder()
	if _, err := sessions.Start(context.Background(), sessionResponse, actor); err != nil {
		t.Fatal(err)
	}
	cookies := sessionResponse.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("session store did not issue a cookie")
	}

	requestPage := func(target string) (int, map[string]any) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.AddCookie(cookies[0])
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		var payload map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode %s response: %v (%s)", target, err, response.Body.String())
		}
		return response.Code, payload
	}

	status, first := requestPage("/whatsapp/instances?limit=2")
	if status != http.StatusOK {
		t.Fatalf("first page status = %d, payload %#v", status, first)
	}
	firstData := responseData(t, first)
	items, ok := firstData["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("first page items = %#v", firstData["items"])
	}
	if firstData["perPage"] != float64(2) {
		t.Fatalf("first page perPage = %#v", firstData["perPage"])
	}
	cursor, ok := firstData["nextCursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("first page nextCursor = %#v", firstData["nextCursor"])
	}

	status, second := requestPage("/whatsapp/instances?limit=2&cursor=" + url.QueryEscape(cursor))
	if status != http.StatusOK {
		t.Fatalf("second page status = %d, payload %#v", status, second)
	}
	secondData := responseData(t, second)
	items, ok = secondData["items"].([]any)
	if !ok || len(items) != 1 || secondData["nextCursor"] != nil {
		t.Fatalf("second page data = %#v", secondData)
	}

	status, invalid := requestPage("/whatsapp/instances?limit=2&cursor=invalid")
	if status != http.StatusBadRequest {
		t.Fatalf("invalid cursor status = %d, payload %#v", status, invalid)
	}
	for _, target := range []string{"/whatsapp/instances?limit=0", "/whatsapp/instances?limit=201"} {
		status, invalid = requestPage(target)
		if status != http.StatusBadRequest {
			t.Fatalf("invalid limit %s status = %d, payload %#v", target, status, invalid)
		}
	}

	phoneRequest := httptest.NewRequest(http.MethodPost, "/whatsapp/instances/demo/connection/phone", strings.NewReader(`{"phoneNumber":"short"}`))
	phoneRequest.Header.Set("Content-Type", "application/json")
	phoneRequest.AddCookie(cookies[0])
	phoneResponse := httptest.NewRecorder()
	router.ServeHTTP(phoneResponse, phoneRequest)
	if phoneResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid phone body status = %d: %s", phoneResponse.Code, phoneResponse.Body.String())
	}
}

func responseData(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("response has no data object: %#v", payload)
	}
	return data
}
