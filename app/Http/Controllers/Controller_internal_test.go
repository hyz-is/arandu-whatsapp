package controllers

import (
	"errors"
	"fmt"
	stdhttp "net/http"
	"reflect"
	"testing"

	"github.com/arandu-io/framework/security"
)

func TestPolicyErrorsUseGenericPublicMessage(t *testing.T) {
	err := fmt.Errorf("%w: tenant globex is outside the configured runtime scope", security.ErrForbidden)
	got := messagesForError(err, stdhttp.StatusForbidden)
	want := []string{"Forbidden"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("messagesForError() = %#v, want %#v", got, want)
	}
}

func TestStructFieldsPreservesJSONFieldNames(t *testing.T) {
	got, err := structFields(struct {
		ProcessID string `json:"processId"`
	}{ProcessID: "process-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got["processId"] != "process-1" {
		t.Fatalf("structFields() = %#v", got)
	}
}

func TestStructFieldsRejectsUnserializableAndNonObjectValues(t *testing.T) {
	for name, value := range map[string]any{
		"unserializable": struct{ Channel chan struct{} }{Channel: make(chan struct{})},
		"array":          []string{"not", "an", "object"},
		"scalar":         "not an object",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := structFields(value); err == nil {
				t.Fatalf("structFields(%T) succeeded", value)
			}
		})
	}
}

func TestStatusForWrappedPolicyError(t *testing.T) {
	err := fmt.Errorf("%w: internal detail", security.ErrForbidden)
	status, ok := statusForError(err)
	if !ok || status != stdhttp.StatusForbidden {
		t.Fatalf("statusForError() = %d, %v", status, ok)
	}
	if !errors.Is(err, security.ErrForbidden) {
		t.Fatal("test error lost policy sentinel")
	}
}
