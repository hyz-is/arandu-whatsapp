package group

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	httpclient "github.com/arandu-io/hesape/http/client"
)

func TestGroupImageClientRejectsInternalAddresses(t *testing.T) {
	service := NewService(nil, nil)
	_, err := service.downloadImage(context.Background(), "http://127.0.0.1:1/image.jpg")
	if !errors.Is(err, httpclient.ErrInternalAddress) {
		t.Fatalf("expected ErrInternalAddress, got %v", err)
	}
}

func TestGroupImageClientUsesNativeResponseLimitAndCallerContext(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request-context")
	contextPreserved := false
	transport := groupRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		contextPreserved = req.Context().Value(contextKey{}) == "request-context"
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: maxGroupPictureBytes + 2,
			Body:          io.NopCloser(strings.NewReader("oversized")),
			Header:        make(http.Header),
		}, nil
	})
	service := NewService(nil, nil)
	service.http = newGroupHTTPClient(httpclient.NewFactory(&http.Client{Transport: transport}))

	_, err := service.downloadImage(ctx, "https://example.com/image.jpg")
	if !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("expected ErrImageTooLarge, got %v", err)
	}
	if !contextPreserved {
		t.Fatal("expected the caller context to reach the transport")
	}
}

type groupRoundTripFunc func(*http.Request) (*http.Response, error)

func (f groupRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
