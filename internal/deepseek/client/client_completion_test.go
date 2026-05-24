package client

import (
	"context"
	"errors"
	"net/http"
	"testing"

)

func TestCallCompletionDoesNotFallbackForNonIdempotentCompletion(t *testing.T) {
	var fallbackCalled bool
	client := &Client{
		stream: doerFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("ambiguous completion write failure")
		}),
		fallbackS: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			fallbackCalled = true
			return &http.Response{StatusCode: http.StatusOK}, nil
		})},
	}
	client.deepseekKey = "token"
	_, err := client.CallCompletion(
		context.Background(),
		map[string]any{"prompt": "hello"},
		"pow",
	)
	if err == nil {
		t.Fatal("expected completion error")
	}
	if fallbackCalled {
		t.Fatal("completion fallback should not be called for a non-idempotent request")
	}
}
