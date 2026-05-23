package accounts

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"tool-gateway/internal/account"
	"tool-gateway/internal/config"
	adminshared "tool-gateway/internal/httpapi/admin/shared"
)

func newHTTPAdminHarness(t *testing.T, rawConfig string, ds adminshared.CompletionBackend) http.Handler {
	t.Helper()
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", rawConfig)
	store := config.LoadStore()
	h := &Handler{
		Store:   store,
		Pool:    account.NewPool(store),
		Backend: ds,
	}
	r := chi.NewRouter()
	RegisterRoutes(r, h)
	return r
}

func adminReq(method, path string, body []byte) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	return req
}
