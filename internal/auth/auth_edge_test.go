package auth

import (
	"context"
	"net/http"
	"testing"
)

// ─── extractCallerToken edge cases ───────────────────────────────────

func TestExtractCallerTokenBearerPrefix(t *testing.T) {
	req, _ := http.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Bearer my-token")
	if got := extractCallerToken(req); got != "my-token" {
		t.Fatalf("expected my-token, got %q", got)
	}
}

func TestExtractCallerTokenBearerCaseInsensitive(t *testing.T) {
	req, _ := http.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "BEARER My-Token")
	if got := extractCallerToken(req); got != "My-Token" {
		t.Fatalf("expected My-Token, got %q", got)
	}
}

func TestExtractCallerTokenBearerEmpty(t *testing.T) {
	req, _ := http.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Bearer ")
	if got := extractCallerToken(req); got != "" {
		t.Fatalf("expected empty for 'Bearer ', got %q", got)
	}
}

func TestExtractCallerTokenXAPIKey(t *testing.T) {
	req, _ := http.NewRequest("POST", "/", nil)
	req.Header.Set("x-api-key", "x-api-key-token")
	if got := extractCallerToken(req); got != "x-api-key-token" {
		t.Fatalf("expected x-api-key-token, got %q", got)
	}
}

func TestExtractCallerTokenBearerPreferredOverXAPIKey(t *testing.T) {
	req, _ := http.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Bearer bearer-token")
	req.Header.Set("x-api-key", "x-api-key-token")
	if got := extractCallerToken(req); got != "bearer-token" {
		t.Fatalf("expected bearer-token, got %q", got)
	}
}

func TestExtractCallerTokenMissingHeaders(t *testing.T) {
	req, _ := http.NewRequest("POST", "/", nil)
	if got := extractCallerToken(req); got != "" {
		t.Fatalf("expected empty for missing headers, got %q", got)
	}
}

func TestExtractCallerTokenNonBearerAuth(t *testing.T) {
	req, _ := http.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Basic abc123")
	if got := extractCallerToken(req); got != "" {
		t.Fatalf("expected empty for Basic auth, got %q", got)
	}
}

// ─── Context helpers ─────────────────────────────────────────────────

func TestWithAuthAndFromContext(t *testing.T) {
	a := &RequestAuth{CallerID: "caller:test-token"}
	ctx := WithAuth(context.Background(), a)
	got, ok := FromContext(ctx)
	if !ok || got.CallerID != "caller:test-token" {
		t.Fatalf("expected caller id from context, got ok=%v caller=%q", ok, got.CallerID)
	}
}

func TestFromContextMissing(t *testing.T) {
	_, ok := FromContext(context.Background())
	if ok {
		t.Fatal("expected not ok from empty context")
	}
}

// ─── JWT edge cases ──────────────────────────────────────────────────

func TestVerifyJWTInvalidFormat(t *testing.T) {
	_, err := VerifyJWT("not-a-jwt")
	if err == nil {
		t.Fatal("expected error for invalid JWT format")
	}
}

func TestVerifyJWTInvalidSignature(t *testing.T) {
	token, _ := CreateJWT(1)
	parts := splitJWT(token)
	if len(parts) == 3 {
		tampered := parts[0] + "." + parts[1] + ".invalid_signature"
		_, err := VerifyJWT(tampered)
		if err == nil {
			t.Fatal("expected error for tampered signature")
		}
	}
}

func TestVerifyJWTExpired(t *testing.T) {
	_, err := VerifyJWT("eyJhbGciOiJIUzI1NiJ9.eyJleHAiOjF9.invalid")
	if err == nil {
		t.Fatal("expected error for expired/invalid JWT")
	}
}

func TestCreateJWTDefaultExpiry(t *testing.T) {
	token, err := CreateJWT(0) // should use default
	if err != nil {
		t.Fatalf("create jwt failed: %v", err)
	}
	_, err = VerifyJWT(token)
	if err != nil {
		t.Fatalf("verify jwt failed: %v", err)
	}
}

// ─── VerifyAdminRequest edge cases ───────────────────────────────────

func TestVerifyAdminRequestNoHeader(t *testing.T) {
	req, _ := http.NewRequest("GET", "/admin/config", nil)
	if err := VerifyAdminRequest(req); err == nil {
		t.Fatal("expected error for missing auth")
	}
}

func TestVerifyAdminRequestEmptyBearer(t *testing.T) {
	req, _ := http.NewRequest("GET", "/admin/config", nil)
	req.Header.Set("Authorization", "Bearer ")
	if err := VerifyAdminRequest(req); err == nil {
		t.Fatal("expected error for empty bearer")
	}
}

func TestVerifyAdminRequestWithAdminKey(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_ADMIN_KEY", "test-admin-key")
	req, _ := http.NewRequest("GET", "/admin/config", nil)
	req.Header.Set("Authorization", "Bearer test-admin-key")
	if err := VerifyAdminRequest(req); err != nil {
		t.Fatalf("expected admin key accepted: %v", err)
	}
}

func TestVerifyAdminRequestInvalidCredentials(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_ADMIN_KEY", "correct-key")
	req, _ := http.NewRequest("GET", "/admin/config", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	if err := VerifyAdminRequest(req); err == nil {
		t.Fatal("expected error for wrong key")
	}
}

func TestVerifyAdminRequestBasicAuth(t *testing.T) {
	req, _ := http.NewRequest("GET", "/admin/config", nil)
	req.Header.Set("Authorization", "Basic abc123")
	if err := VerifyAdminRequest(req); err == nil {
		t.Fatal("expected error for Basic auth")
	}
}

// helper
func splitJWT(token string) []string {
	result := make([]string, 0, 3)
	start := 0
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			result = append(result, token[start:i])
			start = i + 1
		}
	}
	result = append(result, token[start:])
	return result
}
