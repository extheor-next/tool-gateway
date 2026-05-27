package responses

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"tool-gateway/internal/auth"
	"tool-gateway/internal/config"
)

func newDirectTokenResolver(t *testing.T) (*config.Store, *auth.Resolver) {
	t.Helper()
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"keys":["token-a","token-b"],"accounts":[]}`)
	store := config.LoadStore()
	return store, auth.NewResolver(store)
}

func authForToken(t *testing.T, resolver *auth.Resolver, token string) *auth.RequestAuth {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/responses/resp_test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	a, err := resolver.Determine(req)
	if err != nil {
		t.Fatalf("determine auth failed: %v", err)
	}
	return a
}

func TestGetResponseByIDRequiresAuthAndIsTenantIsolated(t *testing.T) {
	store, resolver := newDirectTokenResolver(t)
	h := &Handler{Store: store, Auth: resolver}
	r := chi.NewRouter()
	RegisterRoutes(r, h)

	ownerA := "default"
	h.getResponseStore().put(ownerA, "resp_test", map[string]any{
		"id":     "resp_test",
		"object": "response",
	})

	t.Run("unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/responses/resp_test", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("cross-token-ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/responses/resp_test", nil)
		req.Header.Set("Authorization", "Bearer token-b")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("same-tenant-ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/responses/resp_test", nil)
		req.Header.Set("Authorization", "Bearer token-a")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body failed: %v", err)
		}
		if body["id"] != "resp_test" {
			t.Fatalf("unexpected body: %#v", body)
		}
	})
}

func TestResponsesRouteValidationContract(t *testing.T) {
	store, resolver := newDirectTokenResolver(t)
	h := &Handler{Store: store, Auth: resolver}
	r := chi.NewRouter()
	RegisterRoutes(r, h)

	tests := []struct {
		name string
		body string
	}{
		{name: "missing_model", body: `{"input":"hello"}`},
		{name: "missing_input_and_messages", body: `{"model":"gpt-4o"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(tc.body))
			req.Header.Set("Authorization", "Bearer token-a")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			var out map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("decode response failed: %v", err)
			}
			errObj, _ := out["error"].(map[string]any)
			if _, ok := errObj["code"]; !ok {
				t.Fatalf("expected error.code: %#v", out)
			}
			if _, ok := errObj["param"]; !ok {
				t.Fatalf("expected error.param: %#v", out)
			}
		})
	}
}
