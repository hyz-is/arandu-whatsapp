package docs_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
	webhooksvc "github.com/hyz-is/arandu-whatsapp/internal/webhook"
	webhookdocs "github.com/hyz-is/arandu-whatsapp/internal/webhook/docs"
)

func TestWebhookDocumentationCoverage(t *testing.T) {
	doc := webhookdocs.Build()
	if err := webhookdocs.ValidateDocument(doc); err != nil {
		t.Fatalf("ValidateDocument() error = %v", err)
	}

	assertEventsMatchOfficialFields(t, doc)

	expectedMarkdown, err := webhookdocs.Markdown(doc)
	if err != nil {
		t.Fatalf("Markdown() error = %v", err)
	}

	markdownPath := filepath.Join("..", "..", "..", "docs", "webhooks.md")
	actualMarkdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read %s: %v", markdownPath, err)
	}
	if string(actualMarkdown) != expectedMarkdown {
		t.Fatalf("%s is stale; run go run ./cmd/webhook-docs", markdownPath)
	}

	assertForbiddenContractFilesAbsent(t)
	assertDocumentedEventHeadings(t, string(actualMarkdown), doc)
	assertNoForbiddenEventNames(t, string(actualMarkdown))
}

func TestWebhookDocumentationJSONExamples(t *testing.T) {
	doc := webhookdocs.Build()
	markdownPath := filepath.Join("..", "..", "..", "docs", "webhooks.md")
	actualMarkdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read %s: %v", markdownPath, err)
	}

	blocks := jsonBlocks(string(actualMarkdown))
	minimumBlocks := len(doc.Events) + 4
	if len(blocks) < minimumBlocks {
		t.Fatalf("expected at least %d JSON blocks, got %d", minimumBlocks, len(blocks))
	}

	for idx, block := range blocks {
		var decoded any
		if err := json.Unmarshal([]byte(block), &decoded); err != nil {
			t.Fatalf("JSON block %d is invalid: %v\n%s", idx+1, err, block)
		}
	}
}

func TestWebhookDocumentationMatchesPayloadJSONTags(t *testing.T) {
	doc := webhookdocs.Build()
	events := make(map[string]webhookdocs.EventDoc, len(doc.Events))
	for _, event := range doc.Events {
		events[event.Name] = event
	}

	payloads := map[string]any{
		string(dbtypes.WebhookEventMessagesUpsert):  webhooksvc.MessageUpsertWebhookData{},
		string(dbtypes.WebhookEventMessagesUpdated): webhooksvc.MessageUpdateWebhookData{},
		string(dbtypes.WebhookEventMessagesDeleted): webhooksvc.MessageDeletedWebhookData{},
		string(dbtypes.WebhookEventMessagesStarred): webhooksvc.MessageStarredWebhookData{},
		string(dbtypes.WebhookEventSendMessage):     webhooksvc.MessageUpsertWebhookData{},
		string(dbtypes.WebhookEventMediaRetry):      webhooksvc.MediaRetryWebhookData{},
	}
	for name, payload := range payloads {
		event, ok := events[name]
		if !ok {
			t.Fatalf("event %s is missing", name)
		}
		got := make([]string, 0, len(event.Fields))
		for _, field := range event.Fields {
			got = append(got, field.Name)
		}
		want := jsonFieldNames(reflect.TypeOf(payload))
		sort.Strings(got)
		sort.Strings(want)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("event %s fields do not match %T JSON tags\ngot: %v\nwant: %v", name, payload, got, want)
		}
	}
}

func TestStaticWebhookExamplesRejectSchemaViolations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name: "required field",
			mutate: func(data map[string]any) {
				delete(data, "status")
			},
			want: "status is required",
		},
		{
			name: "undeclared field",
			mutate: func(data map[string]any) {
				data["futureField"] = true
			},
			want: "futureField is not a declared property",
		},
		{
			name: "enum",
			mutate: func(data map[string]any) {
				data["status"] = "invented"
			},
			want: "status must be one of",
		},
		{
			name: "type",
			mutate: func(data map[string]any) {
				data["id"] = "1024"
			},
			want: "id must be a number",
		},
		{
			name: "non-nullable field",
			mutate: func(data map[string]any) {
				data["status"] = nil
			},
			want: "status must not be null",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := webhookdocs.Build()
			event := findEvent(t, &doc, string(dbtypes.WebhookEventMessagesUpdated))
			if event.DynamicFields {
				t.Fatal("messages.updated unexpectedly declares dynamic fields")
			}
			data, ok := event.Example["data"].(map[string]any)
			if !ok {
				t.Fatal("messages.updated example data is not an object")
			}
			test.mutate(data)

			err := webhookdocs.ValidateDocument(doc)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateDocument() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestStaticWebhookExampleAcceptsDeclaredNull(t *testing.T) {
	doc := webhookdocs.Build()
	event := findEvent(t, &doc, string(dbtypes.WebhookEventMessagesUpsert))
	if event.DynamicFields {
		t.Fatal("messages.upsert unexpectedly declares dynamic fields")
	}
	data, ok := event.Example["data"].(map[string]any)
	if !ok {
		t.Fatal("messages.upsert example data is not an object")
	}
	data["keyLid"] = nil

	if err := webhookdocs.ValidateDocument(doc); err != nil {
		t.Fatalf("ValidateDocument() rejected a nullable field: %v", err)
	}
}

func TestDynamicWebhookExampleBypassesClosedFieldValidation(t *testing.T) {
	doc := webhookdocs.Build()
	event := findEvent(t, &doc, string(dbtypes.WebhookEventHistorySync))
	if !event.DynamicFields {
		t.Fatal("history.sync must remain a dynamic-field contract")
	}
	event.Example["data"] = map[string]any{
		"type":        42,
		"futureField": map[string]any{"nested": true},
	}

	if err := webhookdocs.ValidateDocument(doc); err != nil {
		t.Fatalf("ValidateDocument() closed a dynamic event: %v", err)
	}
}

func TestDynamicWebhookExampleStillRequiresDeclaredRootType(t *testing.T) {
	doc := webhookdocs.Build()
	event := findEvent(t, &doc, string(dbtypes.WebhookEventHistorySync))
	if !event.DynamicFields {
		t.Fatal("history.sync must remain a dynamic-field contract")
	}
	event.Example["data"] = []any{"not", "an", "object"}

	err := webhookdocs.ValidateDocument(doc)
	if err == nil || !strings.Contains(err.Error(), "must be an object") {
		t.Fatalf("ValidateDocument() error = %v, want a dynamic root-type error", err)
	}
}

func findEvent(t *testing.T, doc *webhookdocs.Document, name string) *webhookdocs.EventDoc {
	t.Helper()
	for index := range doc.Events {
		if doc.Events[index].Name == name {
			return &doc.Events[index]
		}
	}
	t.Fatalf("event %s is missing", name)
	return nil
}

func jsonFieldNames(typ reflect.Type) []string {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	fields := make([]string, 0, typ.NumField())
	for index := 0; index < typ.NumField(); index++ {
		tag := strings.Split(typ.Field(index).Tag.Get("json"), ",")[0]
		if tag != "" && tag != "-" {
			fields = append(fields, tag)
		}
	}
	return fields
}

func assertEventsMatchOfficialFields(t *testing.T, doc webhookdocs.Document) {
	t.Helper()

	fields := dbtypes.WebhookEventFields()
	if len(doc.Events) != len(fields) {
		t.Fatalf("event count mismatch: docs=%d official_fields=%d", len(doc.Events), len(fields))
	}

	eventToFlag := make(map[string]string, len(fields))
	for flag, event := range fields {
		eventToFlag[string(event)] = flag
	}

	seenNames := make([]string, 0, len(doc.Events))
	seenFlags := map[string]struct{}{}
	for _, event := range doc.Events {
		expectedFlag, ok := eventToFlag[event.Name]
		if !ok {
			t.Fatalf("event %s is not official", event.Name)
		}
		if event.Flag != expectedFlag {
			t.Fatalf("event %s flag mismatch: got %s want %s", event.Name, event.Flag, expectedFlag)
		}
		if _, ok := seenFlags[event.Flag]; ok {
			t.Fatalf("duplicate flag %s", event.Flag)
		}
		seenFlags[event.Flag] = struct{}{}
		seenNames = append(seenNames, event.Name)
	}

	if !sort.StringsAreSorted(seenNames) {
		t.Fatalf("events are not sorted alphabetically: %v", seenNames)
	}

	for _, event := range dbtypes.SupportedWebhookEvents() {
		if _, ok := eventToFlag[string(event)]; !ok {
			t.Fatalf("supported event %s has no field mapping", event)
		}
		if !contains(seenNames, string(event)) {
			t.Fatalf("supported event %s is missing from docs", event)
		}
	}
}

func assertForbiddenContractFilesAbsent(t *testing.T) {
	t.Helper()
	for _, path := range []string{
		filepath.Join("..", "..", "..", "docs", "webhooks.json"),
		filepath.Join("..", "..", "..", "docs", "webhooks.yaml"),
		filepath.Join("..", "..", "..", "docs", "webhooks.yml"),
	} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("forbidden webhook contract file exists: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", path, err)
		}
	}
}

func assertDocumentedEventHeadings(t *testing.T, markdown string, doc webhookdocs.Document) {
	t.Helper()
	got := eventHeadings(markdown)
	want := make([]string, 0, len(doc.Events))
	for _, event := range doc.Events {
		want = append(want, event.Name)
		if !strings.Contains(markdown, "**Flag:** `"+event.Flag+"`") {
			t.Fatalf("event %s flag %s is missing from markdown", event.Name, event.Flag)
		}
		if !strings.Contains(markdown, "x-webhook-event: "+event.Name) {
			t.Fatalf("event %s request example is missing", event.Name)
		}
	}
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("event headings mismatch\ngot:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func assertNoForbiddenEventNames(t *testing.T, markdown string) {
	t.Helper()
	for _, forbidden := range []string{
		"hystory.sync",
		"label.ssociation",
		"labels.ssociation",
		"group-participants.update",
		"groups.updated",
		"chats.deleted",
		"groupUpsert",
		"messagesUpdate",
		"instance.status",
	} {
		token := regexp.MustCompile(`(^|[^A-Za-z0-9_.-])` + regexp.QuoteMeta(forbidden) + `([^A-Za-z0-9_.-]|$)`)
		if token.MatchString(markdown) {
			t.Fatalf("forbidden non-official event/flag %q found in docs", forbidden)
		}
	}
}

func eventHeadings(markdown string) []string {
	re := regexp.MustCompile("(?m)^### `([^`]+)`$")
	matches := re.FindAllStringSubmatch(markdown, -1)
	headings := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		name := match[1]
		if _, ok := seen[name]; ok {
			headings = append(headings, "DUPLICATE:"+name)
			continue
		}
		seen[name] = struct{}{}
		headings = append(headings, name)
	}
	sort.Strings(headings)
	return headings
}

func jsonBlocks(markdown string) []string {
	re := regexp.MustCompile("(?s)```json\\s*\\n(.*?)\\n```")
	matches := re.FindAllStringSubmatch(markdown, -1)
	blocks := make([]string, 0, len(matches))
	for _, match := range matches {
		blocks = append(blocks, strings.TrimSpace(match[1]))
	}
	return blocks
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
