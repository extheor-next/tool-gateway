package proxies

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	adminconfig "ds2api/internal/httpapi/admin/configmgmt"
	adminshared "ds2api/internal/httpapi/admin/shared"
)

type testingBackendMock struct{}

func (m *testingBackendMock) Login(_ context.Context, _ config.Account) (string, error) {
	return "token", nil
}
func (m *testingBackendMock) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "session-id", nil
}
func (m *testingBackendMock) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "pow", nil
}
func (m *testingBackendMock) CallCompletion(_ context.Context, _ *auth.RequestAuth, _ map[string]any, _ string, _ int) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}
func (m *testingBackendMock) DeleteAllSessionsForToken(_ context.Context, _ string) error { return nil }
func (m *testingBackendMock) GetSessionCountForToken(_ context.Context, _ string) (*dsclient.SessionStats, error) {
	return &dsclient.SessionStats{}, nil
}

func newHTTPAdminHarness(t *testing.T, rawConfig string, ds adminshared.CompletionBackend) http.Handler {
	t.Helper()
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", rawConfig)
	store := config.LoadStore()
	pool := account.NewPool(store)
	h := &Handler{Store: store, Pool: pool, Backend: ds}
	configHandler := &adminconfig.Handler{Store: store, Pool: pool, Backend: ds}
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
