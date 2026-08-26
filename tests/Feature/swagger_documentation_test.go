package feature_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"
	swagger "github.com/hyz-is/arandu-swagger"

	whatsapp "github.com/hyz-is/arandu-whatsapp"
)

func TestSwaggerDocumentationMatchesPublicRouteSurface(t *testing.T) {
	for _, test := range []struct {
		name   string
		prefix string
	}{
		{name: "default prefix"},
		{name: "custom prefix", prefix: "/communications"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, router, document := mountSwaggerDocumentation(t, test.prefix)

			runtimeRoutes := make(map[string]string)
			for _, route := range router.Routes() {
				if route.Module != "whatsapp" {
					t.Errorf("route %s %s has module %q", route.Method, route.Pattern, route.Module)
					continue
				}
				key := route.Method + " " + route.Pattern
				runtimeRoutes[key] = route.RouteName()
			}
			if len(runtimeRoutes) != 36 {
				t.Fatalf("runtime registered %d WhatsApp routes, want 36", len(runtimeRoutes))
			}

			documented := swaggerOperations(document)
			if len(documented) != 36 {
				t.Fatalf("OpenAPI generated %d operations, want 36", len(documented))
			}
			if len(documented) != len(runtimeRoutes) {
				t.Fatalf("OpenAPI generated %d operations for %d runtime routes", len(documented), len(runtimeRoutes))
			}

			for key, routeName := range runtimeRoutes {
				operation := documented[key]
				if operation == nil {
					t.Errorf("OpenAPI is missing runtime route %s (%s)", key, routeName)
					continue
				}
				if operation.OperationID != routeName {
					t.Errorf("%s operationId = %q, want %q", key, operation.OperationID, routeName)
				}
				if strings.TrimSpace(operation.Summary) == "" {
					t.Errorf("%s has no summary", key)
				}
				if len(operation.Tags) != 1 || strings.TrimSpace(operation.Tags[0]) == "" {
					t.Errorf("%s tags = %#v, want one non-empty tag", key, operation.Tags)
				}
				if len(operation.Responses) == 0 {
					t.Errorf("%s has no responses", key)
				}
				assertAranduSessionSecurity(t, key, operation)
			}

			for key := range documented {
				if _, exists := runtimeRoutes[key]; !exists {
					t.Errorf("OpenAPI contains non-runtime WhatsApp operation %s", key)
				}
			}

			effectivePrefix := test.prefix
			if effectivePrefix == "" {
				effectivePrefix = whatsapp.DefaultPrefix
			}
			for path := range document.Paths {
				if !strings.HasPrefix(path, effectivePrefix+"/instances") {
					t.Errorf("OpenAPI path %q escaped effective prefix %q", path, effectivePrefix)
				}
			}
		})
	}
}

func TestSwaggerDocumentationPreservesComplexComponents(t *testing.T) {
	_, _, _, document := mountSwaggerDocumentation(t, "")
	if document.Components == nil {
		t.Fatal("OpenAPI has no reusable components")
	}

	for name := range document.Components.Schemas {
		if !strings.HasPrefix(name, "WhatsApp") {
			t.Errorf("schema component %q is not module-scoped", name)
		}
	}
	for name := range document.Components.Parameters {
		if !strings.HasPrefix(name, "WhatsApp") {
			t.Errorf("parameter component %q is not module-scoped", name)
		}
	}
	for name := range document.Components.Responses {
		if !strings.HasPrefix(name, "WhatsApp") {
			t.Errorf("response component %q is not module-scoped", name)
		}
	}

	instance := decodedSwaggerSchema(t, document, "WhatsAppInstance")
	instanceID := swaggerSchemaProperty(t, instance, "id")
	if instanceID["type"] != "integer" || instanceID["format"] != "int64" {
		t.Errorf("WhatsAppInstance.id = %#v, want integer/int64", instanceID)
	}

	recipient := decodedSwaggerSchema(t, document, "WhatsAppRecipientFields")
	oneOf, ok := recipient["oneOf"].([]any)
	if !ok || len(oneOf) != 3 {
		t.Errorf("WhatsAppRecipientFields.oneOf = %#v, want three exclusive aliases", recipient["oneOf"])
	}

	textRequest := decodedSwaggerSchema(t, document, "WhatsAppSendTextRequest")
	allOf, ok := textRequest["allOf"].([]any)
	if !ok || len(allOf) != 2 {
		t.Errorf("WhatsAppSendTextRequest.allOf = %#v, want recipient and message schemas", textRequest["allOf"])
	}

	messageOptions := decodedSwaggerSchema(t, document, "WhatsAppMessageOptions")
	externalAttributes := swaggerSchemaProperty(t, messageOptions, "externalAttributes")
	if externalAttributes["additionalProperties"] != true {
		t.Errorf("WhatsAppMessageOptions.externalAttributes = %#v, want an open object", externalAttributes)
	}

	mediaFile := decodedSwaggerSchema(t, document, "WhatsAppSendMediaFileRequest")
	attachment := swaggerSchemaProperty(t, mediaFile, "attachment")
	if attachment["type"] != "string" || attachment["format"] != "binary" {
		t.Errorf("WhatsAppSendMediaFileRequest.attachment = %#v, want string/binary", attachment)
	}

	scheme := document.Components.SecuritySchemes["aranduSession"]
	if scheme == nil {
		t.Fatal("OpenAPI is missing aranduSession security scheme")
	}
	if scheme.Type != swagger.SecuritySchemeAPIKey || scheme.In != swagger.APIKeyInCookie || scheme.Name != security.SessionCookieName {
		t.Errorf("aranduSession = %#v, want API key in cookie %q", scheme, security.SessionCookieName)
	}

	phone := swaggerOperationAt(t, document, http.MethodPost, whatsapp.DefaultPrefix+"/instances/{instance}/connection/phone")
	assertRequestSchemaReference(t, phone, "application/json", "WhatsAppPhonePairingRequest", true)

	mediaUpload := swaggerOperationAt(t, document, http.MethodPost, whatsapp.DefaultPrefix+"/instances/{instance}/messages/media/file")
	assertRequestSchemaReference(t, mediaUpload, "multipart/form-data", "WhatsAppSendMediaFileRequest", true)

	messageSearch := swaggerOperationAt(t, document, http.MethodPost, whatsapp.DefaultPrefix+"/instances/{instance}/messages/search")
	assertRequestSchemaReference(t, messageSearch, "application/json", "WhatsAppMessageSearchRequest", false)

	download := swaggerOperationAt(t, document, http.MethodPost, whatsapp.DefaultPrefix+"/instances/{instance}/messages/media/download")
	response := download.Responses["200"]
	if response == nil {
		t.Fatal("media download has no 200 response")
	}
	for _, contentType := range []string{"application/octet-stream", "multipart/form-data"} {
		if response.Content[contentType] == nil {
			t.Errorf("media download 200 response is missing %q", contentType)
		}
	}

	list := swaggerOperationAt(t, document, http.MethodGet, whatsapp.DefaultPrefix+"/instances")
	queries := make(map[string]*swagger.Parameter)
	for _, parameter := range list.Parameters {
		if parameter != nil && parameter.In == swagger.ParameterInQuery {
			queries[parameter.Name] = parameter
		}
	}
	for _, name := range []string{"instanceName", "limit", "cursor"} {
		if queries[name] == nil {
			t.Errorf("instance listing is missing %q query parameter", name)
		}
	}
	if limit := queries["limit"]; limit != nil && limit.Schema != nil {
		definition := decodedSchema(t, *limit.Schema, "instances.index limit")
		if definition["minimum"] != float64(1) || definition["maximum"] != float64(200) || definition["default"] != float64(200) {
			t.Errorf("instances.index limit schema = %#v, want range 1..200 and default 200", definition)
		}
	}
}

func TestDocumentationConstructorsPreserveCompatibilityAndRejectTypedNil(t *testing.T) {
	sessions := security.NewSessionStore([]byte(appKey), time.Hour, false, security.NewMemoryBackend())
	db := data.Wrap(nil, data.DialectSQLite)

	module, err := whatsapp.New(whatsapp.Config{Tenant: "acme"}, db, sessions)
	if err != nil {
		t.Fatalf("traditional New() error = %v", err)
	}
	router := fhttp.NewRouter()
	module.Routes(router.ForModule(module.Name()))
	if got := len(router.Routes()); got != 36 {
		t.Fatalf("traditional New() registered %d routes, want 36", got)
	}

	var typedNil *swagger.Module
	if _, err := whatsapp.NewWithDocumentation(whatsapp.Config{Tenant: "acme"}, db, sessions, typedNil); err == nil {
		t.Fatal("NewWithDocumentation accepted a typed nil Swagger module")
	} else if !strings.Contains(err.Error(), "needs a documentation registry") {
		t.Fatalf("typed nil error = %q, want documentation registry error", err)
	}
}

func mountSwaggerDocumentation(t *testing.T, prefix string) (*whatsapp.Module, *swagger.Module, *fhttp.Router, *swagger.Document) {
	t.Helper()
	docs, err := swagger.New(swagger.Config{
		Enabled:     true,
		Title:       "Arandu WhatsApp Module API",
		Version:     "unreleased",
		DisableUI:   true,
		DisableSpec: true,
		Filter: swagger.RouteFilter{
			IncludeModules: []string{"whatsapp"},
		},
	})
	if err != nil {
		t.Fatalf("build Swagger module: %v", err)
	}
	sessions := security.NewSessionStore([]byte(appKey), time.Hour, false, security.NewMemoryBackend())
	module, err := whatsapp.NewWithDocumentation(
		whatsapp.Config{Tenant: "acme", Prefix: prefix},
		data.Wrap(nil, data.DialectSQLite),
		sessions,
		docs,
	)
	if err != nil {
		t.Fatalf("build documented WhatsApp module: %v", err)
	}
	router := fhttp.NewRouter()
	module.Routes(router.ForModule(module.Name()))
	docs.Routes(router.ForModule(docs.Name()))
	document, err := docs.Generate()
	if err != nil {
		t.Fatalf("generate OpenAPI document: %v", err)
	}
	return module, docs, router, document
}

func swaggerOperations(document *swagger.Document) map[string]*swagger.Operation {
	operations := make(map[string]*swagger.Operation)
	for path, item := range document.Paths {
		if item == nil {
			continue
		}
		for _, candidate := range []struct {
			method    string
			operation *swagger.Operation
		}{
			{method: http.MethodGet, operation: item.Get},
			{method: http.MethodPut, operation: item.Put},
			{method: http.MethodPost, operation: item.Post},
			{method: http.MethodDelete, operation: item.Delete},
			{method: http.MethodOptions, operation: item.Options},
			{method: http.MethodHead, operation: item.Head},
			{method: http.MethodPatch, operation: item.Patch},
			{method: http.MethodTrace, operation: item.Trace},
		} {
			if candidate.operation != nil {
				operations[candidate.method+" "+path] = candidate.operation
			}
		}
	}
	return operations
}

func swaggerOperationAt(t *testing.T, document *swagger.Document, method, path string) *swagger.Operation {
	t.Helper()
	operation := swaggerOperations(document)[method+" "+path]
	if operation == nil {
		t.Fatalf("OpenAPI operation %s %s is missing", method, path)
	}
	return operation
}

func assertAranduSessionSecurity(t *testing.T, key string, operation *swagger.Operation) {
	t.Helper()
	if len(operation.Security) != 1 {
		t.Errorf("%s security = %#v, want one aranduSession requirement", key, operation.Security)
		return
	}
	scopes, exists := operation.Security[0]["aranduSession"]
	if !exists || len(scopes) != 0 {
		t.Errorf("%s security = %#v, want aranduSession without scopes", key, operation.Security)
	}
}

func decodedSwaggerSchema(t *testing.T, document *swagger.Document, name string) map[string]any {
	t.Helper()
	schema, exists := document.Components.Schemas[name]
	if !exists {
		t.Fatalf("OpenAPI schema component %q is missing", name)
	}
	return decodedSchema(t, schema, name)
}

func decodedSchema(t *testing.T, schema swagger.Schema, label string) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal OpenAPI schema %s: %v", label, err)
	}
	var definition map[string]any
	if err := json.Unmarshal(encoded, &definition); err != nil {
		t.Fatalf("decode OpenAPI schema %s: %v", label, err)
	}
	return definition
}

func swaggerSchemaProperty(t *testing.T, schema map[string]any, name string) map[string]any {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema has no object properties: %#v", schema)
	}
	property, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("schema property %q is missing: %#v", name, properties)
	}
	return property
}

func assertRequestSchemaReference(t *testing.T, operation *swagger.Operation, contentType, component string, required bool) {
	t.Helper()
	if operation.RequestBody == nil {
		t.Fatalf("%s operation has no request body", operation.OperationID)
	}
	if operation.RequestBody.Required != required {
		t.Errorf("%s request body required = %v, want %v", operation.OperationID, operation.RequestBody.Required, required)
	}
	media := operation.RequestBody.Content[contentType]
	if media == nil || media.Schema == nil {
		t.Fatalf("%s request body has no %q schema", operation.OperationID, contentType)
	}
	want := "#/components/schemas/" + component
	if got := media.Schema.Reference(); got != want {
		t.Errorf("%s request schema reference = %q, want %q", operation.OperationID, got, want)
	}
}
