package proxies

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"tool-gateway/internal/config"
	adminconfig "tool-gateway/internal/httpapi/admin/configmgmt"
	adminshared "tool-gateway/internal/httpapi/admin/shared"
)

type testingBackendMock struct{}

func (m *testingBackendMock) CreateSession(_ context.Context, _ int) (string, error) {
	return "session-id", nil
}
func (m *testingBackendMock) GetPow(_ context.Context, _ int) (string, error) {
	return "pow", nil
}
func (m *testingBackendMock) CallCompletion(_ context.Context, _ map[string]any, _ string) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

func newHTTPAdminHarness(t *testing.T, rawConfig string, ds adminshared.CompletionBackend) http.Handler {
	t.Helper()
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", rawConfig)
	store := config.LoadStore()
	h := &Handler{Store: store, Backend: ds}
	configHandler := &adminconfig.Handler{Store: store, Backend: ds}
	r := chi.NewRouter()
	RegisterRoutes(r, h)
	r.Get("/config", configHandler.GetConfig)
	return r
}

func adminReq(method, path string, body []byte) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	return req
}
