package feature_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database/migrations"

	whatsapp "github.com/hyz-is/arandu-whatsapp"
)

const appKey = "0123456789abcdef0123456789abcdef"

func mount(t *testing.T, cfg whatsapp.Config) (*whatsapp.Module, *fhttp.Router) {
	t.Helper()
	sessions := security.NewSessionStore([]byte(appKey), time.Hour, false, security.NewMemoryBackend())
	module, err := whatsapp.New(cfg, data.Wrap(nil, data.DialectSQLite), sessions)
	if err != nil {
		t.Fatalf("build module: %v", err)
	}
	router := fhttp.NewRouter()
	module.Routes(router.ForModule(module.Name()))
	return module, router
}

func TestModuleRegistersCanonicalRouteSurface(t *testing.T) {
	t.Parallel()
	_, router := mount(t, whatsapp.Config{Tenant: "acme"})
	want := canonicalRoutes(whatsapp.DefaultPrefix)
	got := make([]string, 0, len(router.Routes()))
	names := make(map[string]struct{}, len(router.Routes()))
	for _, route := range router.Routes() {
		if route.Module != "whatsapp" {
			t.Errorf("route %s %s has module %q", route.Method, route.Pattern, route.Module)
		}
		name := route.RouteName()
		if name == "" {
			t.Errorf("route %s %s has no name", route.Method, route.Pattern)
		}
		if _, exists := names[name]; exists {
			t.Errorf("duplicate route name %q", name)
		}
		names[name] = struct{}{}
		got = append(got, route.Method+" "+route.Pattern)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("registered routes:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(got) != 36 {
		t.Fatalf("registered %d routes, want 36", len(got))
	}
	contractPath := filepath.Join("..", "..", "docs", "openapi.yaml")
	contract, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read %s: %v", contractPath, err)
	}
	text := string(contract)
	if count := strings.Count(text, "operationId:"); count != len(names) {
		t.Fatalf("OpenAPI declares %d operationIds, want %d", count, len(names))
	}
	for name := range names {
		if count := strings.Count(text, "operationId: "+name+"\n"); count != 1 {
			t.Errorf("OpenAPI operationId %q occurs %d times", name, count)
		}
	}
	if strings.Contains(text, "/connection/phone/{phone}") {
		t.Error("OpenAPI still exposes a phone number in the route path")
	}
	if !strings.Contains(text, "/instances/{instance}/connection/phone:\n") ||
		!strings.Contains(text, "$ref: '#/components/schemas/PhonePairingRequest'") {
		t.Error("OpenAPI does not declare the JSON phone-pairing request")
	}
	for _, field := range []string{"nextCursor:", "perPage:", "maximum: 200"} {
		if !strings.Contains(text, field) {
			t.Errorf("OpenAPI instance pagination is missing %q", field)
		}
	}
	for _, stale := range []string{
		"id: {type: integer, format: int32",
		"instanceId: {type: integer, format: int32",
	} {
		if strings.Contains(text, stale) {
			t.Errorf("OpenAPI still declares a database identifier as int32: %q", stale)
		}
	}
	if !strings.Contains(text, "id:\n          type: integer\n          format: int64") {
		t.Error("OpenAPI instance identifier is not declared as int64")
	}
}

func TestCustomPrefixAppliesToEveryRoute(t *testing.T) {
	t.Parallel()
	_, router := mount(t, whatsapp.Config{Tenant: "acme", Prefix: "/communications"})
	for _, route := range router.Routes() {
		if !strings.HasPrefix(route.Pattern, "/communications/instances") {
			t.Errorf("route escaped custom prefix: %s %s", route.Method, route.Pattern)
		}
	}
}

func TestGuestAndLegacyBearerCannotReachPersistence(t *testing.T) {
	t.Parallel()
	_, router := mount(t, whatsapp.Config{Tenant: "acme"})
	requests := guestRouteRequests(t)
	for _, item := range requests {
		req := httptest.NewRequest(item.method, item.target, bytes.NewReader(item.body))
		if item.contentType != "" {
			req.Header.Set("Content-Type", item.contentType)
		}
		req.Header.Set("Authorization", "Bearer legacy-instance-token")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s %s answered %d, want 403: %s", item.method, item.target, response.Code, response.Body.String())
		}
	}
}

type routeRequest struct {
	method      string
	target      string
	body        []byte
	contentType string
}

func guestRouteRequests(t *testing.T) []routeRequest {
	t.Helper()
	jsonBody := []byte(`{}`)
	jsonRequest := func(method, target string) routeRequest {
		return routeRequest{method: method, target: target, body: jsonBody, contentType: "application/json"}
	}
	request := func(method, target string) routeRequest {
		return routeRequest{method: method, target: target}
	}
	mediaBody, mediaType := multipartRequest(t, map[string]string{
		"number": "5531999999999", "mediaType": "image",
	})
	audioBody, audioType := multipartRequest(t, map[string]string{"number": "5531999999999"})
	base := "/whatsapp/instances/demo"
	return []routeRequest{
		jsonRequest(http.MethodPost, "/whatsapp/instances"),
		request(http.MethodGet, "/whatsapp/instances"),
		request(http.MethodGet, base),
		request(http.MethodDelete, base+"?force=false"),
		request(http.MethodPost, base+"/connection/qr"),
		{method: http.MethodPost, target: base + "/connection/phone", body: []byte(`{"phoneNumber":"5531999999999"}`), contentType: "application/json"},
		request(http.MethodPost, base+"/connection/passkey/challenge"),
		{method: http.MethodPost, target: base + "/connection/passkey/assertion", body: []byte(`{"requestId":"7bbaf109-e0cc-44de-a434-8d48dfd5cb7b","assertion":{}}`), contentType: "application/json"},
		request(http.MethodGet, base+"/connection"),
		request(http.MethodDelete, base+"/connection"),
		jsonRequest(http.MethodPut, base+"/webhook"),
		request(http.MethodGet, base+"/webhook"),
		jsonRequest(http.MethodPost, base+"/messages/text"),
		jsonRequest(http.MethodPost, base+"/messages/link"),
		jsonRequest(http.MethodPost, base+"/messages/media"),
		{method: http.MethodPost, target: base + "/messages/media/file", body: mediaBody, contentType: mediaType},
		jsonRequest(http.MethodPost, base+"/messages/audio"),
		{method: http.MethodPost, target: base + "/messages/audio/file", body: audioBody, contentType: audioType},
		jsonRequest(http.MethodPost, base+"/messages/contact"),
		jsonRequest(http.MethodPost, base+"/messages/location"),
		jsonRequest(http.MethodPost, base+"/messages/reaction"),
		jsonRequest(http.MethodPost, base+"/messages/search"),
		jsonRequest(http.MethodPatch, base+"/messages/read"),
		request(http.MethodDelete, base+"/messages/1"),
		jsonRequest(http.MethodPut, base+"/messages/message-key"),
		{method: http.MethodPost, target: base + "/messages/media/download", body: []byte(`{"id":1}`), contentType: "application/json"},
		jsonRequest(http.MethodPost, base+"/contacts/check"),
		jsonRequest(http.MethodPost, base+"/contacts/profile-picture"),
		jsonRequest(http.MethodPut, base+"/chats/archive"),
		jsonRequest(http.MethodPost, base+"/calls/reject"),
		jsonRequest(http.MethodPost, base+"/groups"),
		jsonRequest(http.MethodPut, base+"/groups/group@g.us/picture"),
		request(http.MethodGet, base+"/groups/group@g.us/invite"),
		request(http.MethodDelete, base+"/groups/group@g.us/invite"),
		jsonRequest(http.MethodPatch, base+"/groups/group@g.us/participants"),
		request(http.MethodDelete, base+"/groups/group@g.us"),
	}
}

func multipartRequest(t *testing.T, fields map[string]string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("attachment", "attachment.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("attachment")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func TestRemovedLegacyRoutesAreAbsent(t *testing.T) {
	t.Parallel()
	_, router := mount(t, whatsapp.Config{Tenant: "acme"})
	for _, target := range []string{
		"/whatsapp/instance/refreshToken/demo",
		"/whatsapp/instance/create",
		"/whatsapp/instance/fetchInstances",
		"/whatsapp/health",
		"/whatsapp/ready",
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("removed route %s answered %d", target, recorder.Code)
		}
	}

	recorder := httptest.NewRecorder()
	legacyPhone := httptest.NewRequest(http.MethodPost, "/whatsapp/instances/demo/connection/phone/5531999999999", strings.NewReader(`{"phoneNumber":"5531999999999"}`))
	legacyPhone.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, legacyPhone)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("removed phone-in-path route answered %d", recorder.Code)
	}
}

func TestNewRefusesInvalidWiringAndAcceptsStructuralHandle(t *testing.T) {
	t.Parallel()
	sessions := security.NewSessionStore([]byte(appKey), time.Hour, false, security.NewMemoryBackend())
	handle := data.Wrap(nil, data.DialectSQLite)
	valid := whatsapp.Config{Tenant: "acme"}
	if _, err := whatsapp.New(whatsapp.Config{}, handle, sessions); err == nil {
		t.Error("configuration without tenant was accepted")
	}
	if _, err := whatsapp.New(valid, nil, sessions); err == nil {
		t.Error("nil database was accepted")
	}
	if _, err := whatsapp.New(valid, handle, nil); err == nil {
		t.Error("nil session store was accepted")
	}
	if _, err := whatsapp.New(valid, data.Wrap(nil, data.DialectMySQL), sessions); err == nil {
		t.Error("unsupported MySQL sqlstore was accepted")
	}
	if _, err := whatsapp.New(valid, handle, sessions); err != nil {
		t.Fatalf("valid structural wiring was refused: %v", err)
	}
}

func TestModuleDeclaresCanonicalMigrations(t *testing.T) {
	t.Parallel()
	module, _ := mount(t, whatsapp.Config{Tenant: "acme"})
	declared := module.Migrations()
	want := []string{
		"20260825_0001_create_whatsapp_tables",
		"20260825_0002_upgrade_whatsmeow_store",
		"20260825_0003_create_webhook_deliveries",
		"20260825_0004_create_message_jobs",
	}
	if len(declared) != len(want) {
		t.Fatalf("declared %d migrations, want %d", len(declared), len(want))
	}
	for index, migration := range declared {
		if migration.GetName() != want[index] {
			t.Errorf("migration %d = %q, want %q", index, migration.GetName(), want[index])
		}
	}
	if _, ok := declared[0].(migrations.ReversibleMigration); !ok {
		t.Error("the package-owned schema migration is not reversible")
	}
	if _, ok := declared[1].(migrations.ReversibleMigration); ok {
		t.Error("the delegated WhatsMeow migration falsely declares a Down method")
	}
	if _, ok := declared[2].(migrations.ReversibleMigration); !ok {
		t.Error("the webhook delivery migration is not reversible")
	}
	if _, ok := declared[3].(migrations.ReversibleMigration); !ok {
		t.Error("the message job migration is not reversible")
	}
	if declared[1].WithinTransaction() {
		t.Error("the delegated WhatsMeow migration must run outside Arandu's transaction")
	}
}

func canonicalRoutes(prefix string) []string {
	i := prefix + "/instances"
	return []string{
		"POST " + i, "GET " + i,
		"GET " + i + "/{instance}", "DELETE " + i + "/{instance}",
		"POST " + i + "/{instance}/connection/qr",
		"POST " + i + "/{instance}/connection/phone",
		"POST " + i + "/{instance}/connection/passkey/challenge",
		"POST " + i + "/{instance}/connection/passkey/assertion",
		"GET " + i + "/{instance}/connection", "DELETE " + i + "/{instance}/connection",
		"PUT " + i + "/{instance}/webhook", "GET " + i + "/{instance}/webhook",
		"POST " + i + "/{instance}/messages/text", "POST " + i + "/{instance}/messages/link",
		"POST " + i + "/{instance}/messages/media", "POST " + i + "/{instance}/messages/media/file",
		"POST " + i + "/{instance}/messages/audio", "POST " + i + "/{instance}/messages/audio/file",
		"POST " + i + "/{instance}/messages/contact", "POST " + i + "/{instance}/messages/location",
		"POST " + i + "/{instance}/messages/reaction", "POST " + i + "/{instance}/messages/search",
		"PATCH " + i + "/{instance}/messages/read", "DELETE " + i + "/{instance}/messages/{message}",
		"PUT " + i + "/{instance}/messages/{message}", "POST " + i + "/{instance}/messages/media/download",
		"POST " + i + "/{instance}/contacts/check", "POST " + i + "/{instance}/contacts/profile-picture",
		"PUT " + i + "/{instance}/chats/archive", "POST " + i + "/{instance}/calls/reject",
		"POST " + i + "/{instance}/groups", "PUT " + i + "/{instance}/groups/{group}/picture",
		"GET " + i + "/{instance}/groups/{group}/invite", "DELETE " + i + "/{instance}/groups/{group}/invite",
		"PATCH " + i + "/{instance}/groups/{group}/participants", "DELETE " + i + "/{instance}/groups/{group}",
	}
}
