package auth

import (
	"net/http"
	"testing"

	"tool-gateway/internal/config"
)

func newTestResolver(t *testing.T) *Resolver {
	t.Helper()
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"keys":["valid-key"]}`)
	store := config.LoadStore()
	return NewResolver(store)
}

func TestDetermineWithXAPIKeyRejectsUnlistedToken(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("x-api-key", "bad-key")

	_, err := r.Determine(req)
	if err != ErrInvalidAPIKey {
		t.Fatalf("expected invalid api key error, got %v", err)
	}
}

func TestDetermineWithXAPIKeyValidKeyPasses(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("x-api-key", "valid-key")

	auth, err := r.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	if auth.CallerID == "" {
		t.Fatalf("expected caller id to be populated")
	}
}

func TestDetermineCallerSameAsDetermine(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodGet, "/v1/responses/resp_1", nil)
	req.Header.Set("x-api-key", "valid-key")

	a, err := r.DetermineCaller(req)
	if err != nil {
		t.Fatalf("determine caller failed: %v", err)
	}
	if a.CallerID == "" {
		t.Fatalf("expected caller id to be populated")
	}
}

func TestCallerTokenIDStable(t *testing.T) {
	a := callerTokenID("token-a")
	b := callerTokenID("token-a")
	c := callerTokenID("token-b")
	if a == "" || b == "" || c == "" {
		t.Fatalf("expected non-empty caller ids")
	}
	if a != b {
		t.Fatalf("expected stable caller id, got %q and %q", a, b)
	}
	if a == c {
		t.Fatalf("expected different caller id for different tokens")
	}
}

func TestDetermineMissingToken(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	_, err := r.Determine(req)
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if err != ErrUnauthorized {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetermineWithQueryKeyRejectsUnlistedToken(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent?key=bad-query-key", nil)

	_, err := r.Determine(req)
	if err != ErrInvalidAPIKey {
		t.Fatalf("expected invalid api key error, got %v", err)
	}
}

func TestDetermineWithXGoogAPIKeyRejectsUnlistedToken(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse", nil)
	req.Header.Set("x-goog-api-key", "bad-goog-key")

	_, err := r.Determine(req)
	if err != ErrInvalidAPIKey {
		t.Fatalf("expected invalid api key error, got %v", err)
	}
}

func TestDetermineWithAPIKeyQueryParamRejectsUnlistedToken(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent?api_key=bad-api-key", nil)

	_, err := r.Determine(req)
	if err != ErrInvalidAPIKey {
		t.Fatalf("expected invalid api key error, got %v", err)
	}
}

func TestDetermineHeaderTokenPrecedenceOverQueryKey(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent?key=query-key", nil)
	req.Header.Set("x-api-key", "valid-key")

	a, err := r.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	if a.CallerID == "" {
		t.Fatalf("expected caller id to be populated")
	}
}

func TestDetermineCallerMissingToken(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest(http.MethodGet, "/v1/responses/resp_1", nil)

	_, err := r.DetermineCaller(req)
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if err != ErrUnauthorized {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetermineBearerPrefix(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Bearer valid-key")
	auth, err := r.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	if auth.CallerID == "" {
		t.Fatal("expected caller id")
	}
}

func TestDetermineBearerCaseInsensitive(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "BEARER valid-key")
	auth, err := r.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	if auth.CallerID == "" {
		t.Fatal("expected caller id")
	}
}

func TestDetermineBearerEmptyToken(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Bearer ")
	_, err := r.Determine(req)
	if err != ErrUnauthorized {
		t.Fatalf("expected unauthorized for empty bearer token, got %v", err)
	}
}

func TestDetermineNonBearerAuthIgnored(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Basic abc123")
	_, err := r.Determine(req)
	if err != ErrUnauthorized {
		t.Fatalf("expected unauthorized for non-bearer auth, got %v", err)
	}
}

func TestDetermineBearerPreferredOverXAPIKey(t *testing.T) {
	r := newTestResolver(t)
	req, _ := http.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Bearer valid-key")
	req.Header.Set("x-api-key", "bad-key")
	auth, err := r.Determine(req)
	if err != nil {
		t.Fatalf("determine failed: %v", err)
	}
	if auth.CallerID == "" {
		t.Fatal("expected caller id")
	}
}
