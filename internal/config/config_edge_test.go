package config

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// ─── GetModelConfig edge cases ───────────────────────────────────────

func TestGetModelConfigDeepSeekChat(t *testing.T) {
	thinking, search, ok := GetModelConfig("deepseek-v4-flash")
	if !ok {
		t.Fatal("expected ok for deepseek-v4-flash")
	}
	if !thinking || search {
		t.Fatalf("expected thinking=true search=false for deepseek-v4-flash, got thinking=%v search=%v", thinking, search)
	}
}

func TestGetModelConfigDeepSeekChatNoThinking(t *testing.T) {
	thinking, search, ok := GetModelConfig("deepseek-v4-flash-nothinking")
	if !ok {
		t.Fatal("expected ok for deepseek-v4-flash-nothinking")
	}
	if thinking || search {
		t.Fatalf("expected thinking=false search=false for deepseek-v4-flash-nothinking, got thinking=%v search=%v", thinking, search)
	}
}

func TestGetModelConfigDeepSeekReasoner(t *testing.T) {
	thinking, search, ok := GetModelConfig("deepseek-v4-pro")
	if !ok {
		t.Fatal("expected ok for deepseek-v4-pro")
	}
	if !thinking || search {
		t.Fatalf("expected thinking=true search=false, got thinking=%v search=%v", thinking, search)
	}
}

func TestGetModelConfigDeepSeekChatSearch(t *testing.T) {
	thinking, search, ok := GetModelConfig("deepseek-v4-flash-search")
	if !ok {
		t.Fatal("expected ok for deepseek-v4-flash-search")
	}
	if !thinking || !search {
		t.Fatalf("expected thinking=true search=true, got thinking=%v search=%v", thinking, search)
	}
}

func TestGetModelConfigDeepSeekReasonerSearch(t *testing.T) {
	thinking, search, ok := GetModelConfig("deepseek-v4-pro-search")
	if !ok {
		t.Fatal("expected ok for deepseek-v4-pro-search")
	}
	if !thinking || !search {
		t.Fatalf("expected both true, got thinking=%v search=%v", thinking, search)
	}
}

func TestGetModelConfigDeepSeekExpertChat(t *testing.T) {
	thinking, search, ok := GetModelConfig("deepseek-v4-pro")
	if !ok {
		t.Fatal("expected ok for deepseek-v4-pro")
	}
	if !thinking || search {
		t.Fatalf("expected thinking=true search=false for deepseek-v4-pro, got thinking=%v search=%v", thinking, search)
	}
}

func TestGetModelConfigDeepSeekExpertReasonerSearch(t *testing.T) {
	thinking, search, ok := GetModelConfig("deepseek-v4-pro-search")
	if !ok {
		t.Fatal("expected ok for deepseek-v4-pro-search")
	}
	if !thinking || !search {
		t.Fatalf("expected both true, got thinking=%v search=%v", thinking, search)
	}
}

func TestGetModelConfigDeepSeekVision(t *testing.T) {
	thinking, search, ok := GetModelConfig("deepseek-v4-vision")
	if !ok {
		t.Fatal("expected ok for deepseek-v4-vision")
	}
	if !thinking || search {
		t.Fatalf("expected thinking=true search=false, got thinking=%v search=%v", thinking, search)
	}
}

func TestGetModelConfigDeepSeekVisionSearchUnsupported(t *testing.T) {
	_, _, ok := GetModelConfig("deepseek-v4-vision-search")
	if ok {
		t.Fatal("expected deepseek-v4-vision-search to be unsupported")
	}
}

func TestGetModelTypeDefaultExpertAndVision(t *testing.T) {
	defaultType, ok := GetModelType("deepseek-v4-flash")
	if !ok || defaultType != "default" {
		t.Fatalf("expected default model_type, got ok=%v model_type=%q", ok, defaultType)
	}
	defaultNoThinkingType, ok := GetModelType("deepseek-v4-flash-nothinking")
	if !ok || defaultNoThinkingType != "default" {
		t.Fatalf("expected default model_type for nothinking, got ok=%v model_type=%q", ok, defaultNoThinkingType)
	}
	expertType, ok := GetModelType("deepseek-v4-pro")
	if !ok || expertType != "expert" {
		t.Fatalf("expected expert model_type, got ok=%v model_type=%q", ok, expertType)
	}
	visionType, ok := GetModelType("deepseek-v4-vision")
	if !ok || visionType != "vision" {
		t.Fatalf("expected vision model_type, got ok=%v model_type=%q", ok, visionType)
	}
}

func TestGetModelConfigCaseInsensitive(t *testing.T) {
	thinking, search, ok := GetModelConfig("DeepSeek-V4-Flash")
	if !ok {
		t.Fatal("expected ok for case-insensitive deepseek-v4-flash")
	}
	if !thinking || search {
		t.Fatalf("expected thinking=true search=false for case-insensitive deepseek-v4-flash")
	}
}

func TestGetModelConfigUnknownModel(t *testing.T) {
	_, _, ok := GetModelConfig("gpt-4")
	if ok {
		t.Fatal("expected not ok for unknown model")
	}
}

func TestGetModelConfigEmpty(t *testing.T) {
	_, _, ok := GetModelConfig("")
	if ok {
		t.Fatal("expected not ok for empty model")
	}
}

// ─── lower function ──────────────────────────────────────────────────

func TestLowerFunction(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello", "hello"},
		{"ALLCAPS", "allcaps"},
		{"already-lower", "already-lower"},
		{"Mixed-CASE-123", "mixed-case-123"},
		{"", ""},
	}
	for _, tc := range tests {
		got := lower(tc.input)
		if got != tc.expected {
			t.Errorf("lower(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// ─── Config.MarshalJSON / UnmarshalJSON roundtrip ────────────────────

func TestConfigJSONRoundtrip(t *testing.T) {
	cfg := Config{
		Keys:         []string{"key1", "key2"},
		ModelAliases: map[string]string{"Claude-Sonnet-4-6": "DeepSeek-V4-Flash"},
		AutoDelete: AutoDeleteConfig{
			Mode: "single",
		},
		Runtime: RuntimeConfig{
			TokenRefreshIntervalHours: 12,
		},
		Vercel: VercelConfig{
			Token:     " vercel-token ",
			ProjectID: " prj_123 ",
			TeamID:    " team_123 ",
		},
		VercelSyncHash: "hash123",
		VercelSyncTime: 1234567890,
		AdditionalFields: map[string]any{
			"custom_field": "custom_value",
		},
	}

	data, err := cfg.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(decoded.Keys) != 2 || decoded.Keys[0] != "key1" {
		t.Fatalf("unexpected keys: %#v", decoded.Keys)
	}
	if decoded.ModelAliases["claude-sonnet-4-6"] != "deepseek-v4-flash" {
		t.Fatalf("unexpected normalized model aliases: %#v", decoded.ModelAliases)
	}
	if decoded.Runtime.TokenRefreshIntervalHours != 12 {
		t.Fatalf("unexpected runtime refresh interval: %#v", decoded.Runtime.TokenRefreshIntervalHours)
	}
	if decoded.AutoDelete.Mode != "single" {
		t.Fatalf("unexpected auto delete mode: %#v", decoded.AutoDelete.Mode)
	}
	if decoded.Vercel.Token != "vercel-token" || decoded.Vercel.ProjectID != "prj_123" || decoded.Vercel.TeamID != "team_123" {
		t.Fatalf("unexpected vercel config: %#v", decoded.Vercel)
	}
	if decoded.VercelSyncHash != "hash123" {
		t.Fatalf("unexpected vercel sync hash: %q", decoded.VercelSyncHash)
	}
	if decoded.AdditionalFields["custom_field"] != "custom_value" {
		t.Fatalf("unexpected additional fields: %#v", decoded.AdditionalFields)
	}
}

func TestAutoDeleteModeResolution(t *testing.T) {
	tests := []struct {
		name string
		cfg  AutoDeleteConfig
		want string
	}{
		{name: "default", cfg: AutoDeleteConfig{}, want: "none"},
		{name: "legacy all", cfg: AutoDeleteConfig{Sessions: true}, want: "all"},
		{name: "single", cfg: AutoDeleteConfig{Mode: "single"}, want: "single"},
		{name: "all", cfg: AutoDeleteConfig{Mode: "all"}, want: "all"},
		{name: "none", cfg: AutoDeleteConfig{Mode: "none"}, want: "none"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &Store{cfg: Config{AutoDelete: tc.cfg}}
			if got := store.AutoDeleteMode(); got != tc.want {
				t.Fatalf("AutoDeleteMode()=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestConfigUnmarshalJSONPreservesUnknownFields(t *testing.T) {
	raw := `{"keys":["k1"],"my_custom_field":"hello","number_field":42}`
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if cfg.AdditionalFields["my_custom_field"] != "hello" {
		t.Fatalf("expected custom field preserved, got %#v", cfg.AdditionalFields)
	}
	// number_field should also be preserved
	if cfg.AdditionalFields["number_field"] != float64(42) {
		t.Fatalf("expected number field preserved, got %#v", cfg.AdditionalFields["number_field"])
	}
}

func TestConfigUnmarshalJSONIgnoresRemovedLegacyModelMappings(t *testing.T) {
	raw := `{"keys":["k1"],"claude_mapping":{"fast":"deepseek-v4-pro"},"claude_model_mapping":{"slow":"deepseek-v4-pro"}}`
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(cfg.ModelAliases) != 0 {
		t.Fatalf("expected removed legacy mappings to be ignored, got %#v", cfg.ModelAliases)
	}
	if _, ok := cfg.AdditionalFields["claude_mapping"]; ok {
		t.Fatalf("expected removed legacy field not to persist in additional fields: %#v", cfg.AdditionalFields)
	}
	if _, ok := cfg.AdditionalFields["claude_model_mapping"]; ok {
		t.Fatalf("expected removed legacy field not to persist in additional fields: %#v", cfg.AdditionalFields)
	}
}

func TestConfigUnmarshalJSONIgnoresRemovedHistorySplit(t *testing.T) {
	raw := `{"keys":["k1"],"history_split":{"enabled":true,"trigger_after_turns":2}}`
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if _, ok := cfg.AdditionalFields["history_split"]; ok {
		t.Fatalf("expected removed legacy field not to persist in additional fields: %#v", cfg.AdditionalFields)
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if strings.Contains(string(out), "history_split") {
		t.Fatalf("expected removed history_split field not to marshal, got %s", out)
	}
}

// ─── Config.Clone ────────────────────────────────────────────────────

func TestConfigCloneIsDeepCopy(t *testing.T) {
	cfg := Config{
		Keys:             []string{"key1"},
		ModelAliases:     map[string]string{"claude-sonnet-4-6": "deepseek-v4-flash"},
		AdditionalFields: map[string]any{"custom": "value"},
	}

	cloned := cfg.Clone()

	// Modify original
	cfg.Keys[0] = "modified"
	cfg.ModelAliases["claude-sonnet-4-6"] = "modified-model"

	// Cloned should not be affected
	if cloned.Keys[0] != "key1" {
		t.Fatalf("clone keys was affected by original change: %#v", cloned.Keys)
	}
	if cloned.ModelAliases["claude-sonnet-4-6"] != "deepseek-v4-flash" {
		t.Fatalf("clone model aliases was affected: %#v", cloned.ModelAliases)
	}
}

func TestConfigCloneNilMaps(t *testing.T) {
	cfg := Config{
		Keys: []string{"k"},
	}
	cloned := cfg.Clone()
	if len(cloned.Keys) != 1 {
		t.Fatalf("unexpected keys length: %d", len(cloned.Keys))
	}
}

// ─── normalizeConfigInput ────────────────────────────────────────────

func TestNormalizeConfigInputStripsQuotes(t *testing.T) {
	got := normalizeConfigInput(`"base64:abc"`)
	if strings.HasPrefix(got, `"`) || strings.HasSuffix(got, `"`) {
		t.Fatalf("expected quotes stripped, got %q", got)
	}
}

func TestNormalizeConfigInputStripsSingleQuotes(t *testing.T) {
	got := normalizeConfigInput("'some-value'")
	if strings.HasPrefix(got, "'") || strings.HasSuffix(got, "'") {
		t.Fatalf("expected single quotes stripped, got %q", got)
	}
}

func TestNormalizeConfigInputTrimsWhitespace(t *testing.T) {
	got := normalizeConfigInput("  hello  ")
	if got != "hello" {
		t.Fatalf("expected trimmed, got %q", got)
	}
}

// ─── parseConfigString edge cases ────────────────────────────────────

func TestParseConfigStringPlainJSON(t *testing.T) {
	cfg, err := parseConfigString(`{"keys":["k1"]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Keys) != 1 || cfg.Keys[0] != "k1" {
		t.Fatalf("unexpected keys: %#v", cfg.Keys)
	}
}

func TestParseConfigStringBase64Prefix(t *testing.T) {
	rawJSON := `{"keys":["base64-key"]}`
	b64 := base64.StdEncoding.EncodeToString([]byte(rawJSON))
	cfg, err := parseConfigString("base64:" + b64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Keys) != 1 || cfg.Keys[0] != "base64-key" {
		t.Fatalf("unexpected keys: %#v", cfg.Keys)
	}
}

func TestParseConfigStringInvalidBase64(t *testing.T) {
	_, err := parseConfigString("base64:!!!invalid!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestParseConfigStringEmptyString(t *testing.T) {
	_, err := parseConfigString("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}

// ─── Store methods ───────────────────────────────────────────────────

func TestStoreIgnoresRemovedCompatConfig(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"keys":["k1"],"compat":{"strip_reference_markers":false}}`)
	store := LoadStore()

	snap := store.Snapshot()
	data, err := snap.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if _, ok := out["compat"]; ok {
		t.Fatalf("expected removed compat field not to marshal, got %#v", out)
	}
}

// ─── OpenAIModelsResponse / ClaudeModelsResponse ─────────────────────

func TestOpenAIModelsResponse(t *testing.T) {
	resp := OpenAIModelsResponse()
	if resp["object"] != "list" {
		t.Fatalf("unexpected object: %v", resp["object"])
	}
	data, ok := resp["data"].([]ModelInfo)
	if !ok {
		t.Fatalf("unexpected data type: %T", resp["data"])
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty models list")
	}
	expected := map[string]bool{
		"deepseek-v4-flash":                   false,
		"deepseek-v4-flash-nothinking":        false,
		"deepseek-v4-pro":                     false,
		"deepseek-v4-pro-nothinking":          false,
		"deepseek-v4-flash-search":            false,
		"deepseek-v4-flash-search-nothinking": false,
		"deepseek-v4-pro-search":              false,
		"deepseek-v4-pro-search-nothinking":   false,
		"deepseek-v4-vision":                  false,
		"deepseek-v4-vision-nothinking":       false,
	}
	for _, model := range data {
		if _, ok := expected[model.ID]; ok {
			expected[model.ID] = true
		}
	}
	for id, seen := range expected {
		if !seen {
			t.Fatalf("expected OpenAI model list to include %s", id)
		}
	}
}

func TestClaudeModelsResponse(t *testing.T) {
	resp := ClaudeModelsResponse()
	if resp["object"] != "list" {
		t.Fatalf("unexpected object: %v", resp["object"])
	}
	data, ok := resp["data"].([]ModelInfo)
	if !ok {
		t.Fatalf("unexpected data type: %T", resp["data"])
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty models list")
	}
}
