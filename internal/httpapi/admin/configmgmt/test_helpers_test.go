package configmgmt

import (
	"testing"

	"tool-gateway/internal/config"
)

func newAdminTestHandler(t *testing.T, raw string) *Handler {
	t.Helper()
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", raw)
	store := config.LoadStore()
	return &Handler{Store: store}
}
