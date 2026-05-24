package config

import (
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLoadStorePreservesProxies(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{
		"proxies":[
			{
				"id":"proxy-sh-1",
				"name":"Shanghai Exit",
				"type":"socks5h",
				"host":"127.0.0.1",
				"port":1080,
				"username":"demo",
				"password":"secret"
			}
		]
	}`)

	store := LoadStore()
	snap := store.Snapshot()
	if len(snap.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(snap.Proxies))
	}
	if snap.Proxies[0].ID != "proxy-sh-1" {
		t.Fatalf("unexpected proxy id: %#v", snap.Proxies[0])
	}
	if snap.Proxies[0].Type != "socks5h" {
		t.Fatalf("unexpected proxy type: %#v", snap.Proxies[0])
	}
}

func TestExplicitMissingConfigPathBootstrapsEmptyFileBackedStore(t *testing.T) {
	path := t.TempDir() + "/config.json"

	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", "")
	t.Setenv("TOOL_GATEWAY_CONFIG_PATH", path)

	store, err := LoadStoreWithError()
	if err != nil {
		t.Fatalf("expected missing explicit config path to bootstrap, got: %v", err)
	}
	if store.IsEnvBacked() {
		t.Fatal("expected bootstrap store to be file-backed")
	}
	if store.ConfigPath() != path {
		t.Fatalf("ConfigPath() = %q, want %q", store.ConfigPath(), path)
	}
	if len(store.Keys()) != 0 {
		t.Fatalf("expected empty bootstrap config, got keys=%d", len(store.Keys()))
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected bootstrap not to create config until first save, stat err=%v", statErr)
	}

	if err := store.Update(func(c *Config) error {
		c.Keys = []string{"first-key"}
		return nil
	}); err != nil {
		t.Fatalf("update should persist bootstrap config: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected first update to write config: %v", err)
	}
	if !strings.Contains(string(content), "first-key") {
		t.Fatalf("expected saved config to contain first key, got: %s", content)
	}
}

func TestEnvBackedStoreWritebackBootstrapsMissingConfigFile(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	path := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(path)

	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"keys":["k1"]}`)
	t.Setenv("TOOL_GATEWAY_CONFIG_PATH", path)
	t.Setenv("TOOL_GATEWAY_ENV_WRITEBACK", "1")

	store := LoadStore()
	if store.IsEnvBacked() {
		t.Fatalf("expected writeback bootstrap to become file-backed immediately")
	}
	if err := store.Update(func(c *Config) error {
		c.Keys = append(c.Keys, "k2")
		return nil
	}); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	if !strings.Contains(string(content), "k1") {
		t.Fatalf("expected bootstrapped config to contain k1, got: %s", content)
	}
	if !strings.Contains(string(content), "k2") {
		t.Fatalf("expected persisted config to contain k2, got: %s", content)
	}

	reloaded := LoadStore()
	if reloaded.IsEnvBacked() {
		t.Fatalf("expected reloaded store to prefer persisted config file")
	}
	keys := reloaded.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys after reload, got %d", len(keys))
	}
}

func TestEnvBackedStoreWritebackDoesNotBootstrapOnInvalidEnvJSON(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	path := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(path)

	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", "{invalid-json")
	t.Setenv("TOOL_GATEWAY_CONFIG_PATH", path)
	t.Setenv("TOOL_GATEWAY_ENV_WRITEBACK", "1")

	cfg, fromEnv, loadErr := loadConfig()
	if loadErr == nil {
		t.Fatalf("expected loadConfig error for invalid env json")
	}
	if !fromEnv {
		t.Fatalf("expected fromEnv=true when parsing env config fails")
	}
	if len(cfg.Keys) != 0 {
		t.Fatalf("expected empty config on parse failure, got keys=%d", len(cfg.Keys))
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected no bootstrapped config file, stat err=%v", statErr)
	}
}

func TestEnvBackedStoreWritebackDoesNotBootstrapOnInvalidSemanticConfig(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	path := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(path)

	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{
		"keys":["k1"],
		"runtime":{"global_max_inflight":200001}
	}`)
	t.Setenv("TOOL_GATEWAY_CONFIG_PATH", path)
	t.Setenv("TOOL_GATEWAY_ENV_WRITEBACK", "1")

	_, fromEnv, _ := loadConfig()
	if !fromEnv {
		t.Fatalf("expected fromEnv=true when env config is the source")
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected invalid config not to be bootstrapped, stat err=%v", statErr)
	}
}

func TestEnvBackedStoreWritebackFallsBackToPersistedFileOnInvalidEnvJSON(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	path := tmp.Name()
	if _, err := tmp.WriteString(`{"keys":["file-key"]}`); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	_ = tmp.Close()

	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", "{invalid-json")
	t.Setenv("TOOL_GATEWAY_CONFIG_PATH", path)
	t.Setenv("TOOL_GATEWAY_ENV_WRITEBACK", "1")

	cfg, fromEnv, loadErr := loadConfig()
	if loadErr != nil {
		t.Fatalf("expected fallback to persisted file, got error: %v", loadErr)
	}
	if fromEnv {
		t.Fatalf("expected fallback to file-backed mode")
	}
	if len(cfg.Keys) != 1 || cfg.Keys[0] != "file-key" {
		t.Fatalf("unexpected keys after fallback: %#v", cfg.Keys)
	}
}

func TestLoadStoreRejectsInvalidFieldType(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"keys":"not-array"}`)
	store := LoadStore()
	if len(store.Keys()) != 0 {
		t.Fatalf("expected empty store when config type is invalid")
	}
}

func TestParseConfigStringSupportsQuotedBase64Prefix(t *testing.T) {
	rawJSON := `{"keys":["k1"]}`
	b64 := base64.StdEncoding.EncodeToString([]byte(rawJSON))
	cfg, err := parseConfigString(`"base64:` + b64 + `"`)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(cfg.Keys) != 1 || cfg.Keys[0] != "k1" {
		t.Fatalf("unexpected keys: %#v", cfg.Keys)
	}
}

func TestParseConfigStringSupportsRawURLBase64(t *testing.T) {
	rawJSON := `{"keys":["k-url"]}`
	b64 := base64.RawURLEncoding.EncodeToString([]byte(rawJSON))
	cfg, err := parseConfigString(b64)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(cfg.Keys) != 1 || cfg.Keys[0] != "k-url" {
		t.Fatalf("unexpected keys: %#v", cfg.Keys)
	}
}

func TestLoadConfigOnVercelWithoutConfigFileFallsBackToMemory(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", "")
	t.Setenv("TOOL_GATEWAY_CONFIG_PATH", "testdata/does-not-exist.json")

	cfg, fromEnv, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fromEnv {
		t.Fatalf("expected fromEnv=true for vercel fallback")
	}
	if len(cfg.Keys) != 0 {
		t.Fatalf("expected empty bootstrap config, got keys=%d", len(cfg.Keys))
	}
}

func TestNormalizeCredentialsPrefersStructuredAPIKeys(t *testing.T) {
	cfg := Config{
		Keys: []string{"legacy-key"},
		APIKeys: []APIKey{
			{Key: "structured-key", Name: "primary", Remark: "prod"},
		},
	}
	cfg.NormalizeCredentials()

	if len(cfg.Keys) != 1 || cfg.Keys[0] != "structured-key" {
		t.Fatalf("unexpected normalized keys: %#v", cfg.Keys)
	}
	if len(cfg.APIKeys) != 1 {
		t.Fatalf("unexpected normalized api keys: %#v", cfg.APIKeys)
	}
	if cfg.APIKeys[0].Key != "structured-key" || cfg.APIKeys[0].Name != "primary" || cfg.APIKeys[0].Remark != "prod" {
		t.Fatalf("unexpected structured api key metadata: %#v", cfg.APIKeys[0])
	}
}

func TestStoreModelAliasesIncludesDefaultsAndOverrides(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"keys":[],"model_aliases":{"claude-opus-4-6":"deepseek-v4-pro-search"}}`)
	store := LoadStore()
	aliases := store.ModelAliases()
	if aliases["claude-sonnet-4-6"] != "deepseek-v4-flash" {
		t.Fatalf("expected default alias to remain available, got %q", aliases["claude-sonnet-4-6"])
	}
	if aliases["claude-opus-4-6"] != "deepseek-v4-pro-search" {
		t.Fatalf("expected custom alias override, got %q", aliases["claude-opus-4-6"])
	}
}

func TestStoreModelAliasesDefault(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"keys":[]}`)
	store := LoadStore()
	aliases := store.ModelAliases()
	if aliases == nil {
		t.Fatal("expected non-nil aliases")
	}
	if aliases["claude-sonnet-4-6"] != "deepseek-v4-flash" {
		t.Fatalf("expected built-in alias, got %q", aliases["claude-sonnet-4-6"])
	}
}

func TestStoreSetVercelSync(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"keys":[]}`)
	store := LoadStore()
	if err := store.SetVercelSync("hash123", 1234567890); err != nil {
		t.Fatalf("setVercelSync error: %v", err)
	}
	snap := store.Snapshot()
	if snap.VercelSyncHash != "hash123" || snap.VercelSyncTime != 1234567890 {
		t.Fatalf("unexpected vercel sync: hash=%q time=%d", snap.VercelSyncHash, snap.VercelSyncTime)
	}
}

func TestStoreExportJSONAndBase64(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"keys":["export-key"]}`)
	store := LoadStore()
	jsonStr, b64Str, err := store.ExportJSONAndBase64()
	if err != nil {
		t.Fatalf("export error: %v", err)
	}
	if !strings.Contains(jsonStr, "export-key") {
		t.Fatalf("expected JSON to contain key: %q", jsonStr)
	}
	decoded, err := base64.StdEncoding.DecodeString(b64Str)
	if err != nil {
		t.Fatalf("base64 decode error: %v", err)
	}
	if !strings.Contains(string(decoded), "export-key") {
		t.Fatalf("expected base64-decoded to contain key: %q", string(decoded))
	}
}

func TestStoreSnapshotReturnsClone(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"keys":["k1"]}`)
	store := LoadStore()
	snap := store.Snapshot()
	snap.Keys[0] = "modified"
	if store.Keys()[0] != "k1" {
		t.Fatal("snapshot modification should not affect store")
	}
}

func TestStoreHasAPIKeyMultipleKeys(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"keys":["key1","key2","key3"]}`)
	store := LoadStore()
	if !store.HasAPIKey("key1") {
		t.Fatal("expected key1 found")
	}
	if !store.HasAPIKey("key2") {
		t.Fatal("expected key2 found")
	}
	if !store.HasAPIKey("key3") {
		t.Fatal("expected key3 found")
	}
	if store.HasAPIKey("nonexistent") {
		t.Fatal("expected nonexistent key not found")
	}
}

func TestStoreIsEnvBacked(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"keys":["k1"]}`)
	store := LoadStore()
	if !store.IsEnvBacked() {
		t.Fatal("expected env-backed store")
	}
}

func TestStoreReplace(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"keys":["k1"]}`)
	store := LoadStore()
	newCfg := Config{
		Keys: []string{"new-key"},
	}
	if err := store.Replace(newCfg); err != nil {
		t.Fatalf("replace error: %v", err)
	}
	if !store.HasAPIKey("new-key") {
		t.Fatal("expected new key after replace")
	}
	if store.HasAPIKey("k1") {
		t.Fatal("expected old key removed after replace")
	}
}

func TestStoreUpdate(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"keys":["k1"]}`)
	store := LoadStore()
	err := store.Update(func(cfg *Config) error {
		cfg.Keys = append(cfg.Keys, "k2")
		return nil
	})
	if err != nil {
		t.Fatalf("update error: %v", err)
	}
	if !store.HasAPIKey("k2") {
		t.Fatal("expected k2 after update")
	}
}

func TestStoreUpdateReconcilesAPIKeyMutations(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{
		"keys":["k1"],
		"api_keys":[{"key":"k1","name":"primary","remark":"prod"}]
	}`)
	store := LoadStore()

	if err := store.Update(func(cfg *Config) error {
		cfg.APIKeys = append(cfg.APIKeys, APIKey{Key: "k2", Name: "secondary", Remark: "staging"})
		return nil
	}); err != nil {
		t.Fatalf("add api key failed: %v", err)
	}

	snap := store.Snapshot()
	if len(snap.Keys) != 2 || snap.Keys[0] != "k1" || snap.Keys[1] != "k2" {
		t.Fatalf("unexpected keys after api key add: %#v", snap.Keys)
	}
	if len(snap.APIKeys) != 2 {
		t.Fatalf("unexpected api keys length after add: %#v", snap.APIKeys)
	}
	if snap.APIKeys[0].Name != "primary" || snap.APIKeys[0].Remark != "prod" {
		t.Fatalf("metadata for existing key was lost: %#v", snap.APIKeys[0])
	}
	if snap.APIKeys[1].Name != "secondary" || snap.APIKeys[1].Remark != "staging" {
		t.Fatalf("metadata for new key was lost: %#v", snap.APIKeys[1])
	}

	if err := store.Update(func(cfg *Config) error {
		cfg.APIKeys = append([]APIKey(nil), cfg.APIKeys[1:]...)
		return nil
	}); err != nil {
		t.Fatalf("delete api key failed: %v", err)
	}

	snap = store.Snapshot()
	if len(snap.Keys) != 1 || snap.Keys[0] != "k2" {
		t.Fatalf("unexpected keys after api key delete: %#v", snap.Keys)
	}
	if len(snap.APIKeys) != 1 || snap.APIKeys[0].Key != "k2" {
		t.Fatalf("unexpected api keys after delete: %#v", snap.APIKeys)
	}
}

func TestStoreUpdateReconcilesLegacyKeyMutations(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{
		"keys":["k1"],
		"api_keys":[{"key":"k1","name":"primary","remark":"prod"}]
	}`)
	store := LoadStore()

	if err := store.Update(func(cfg *Config) error {
		cfg.Keys = append(cfg.Keys, "k2")
		return nil
	}); err != nil {
		t.Fatalf("legacy key update failed: %v", err)
	}

	snap := store.Snapshot()
	if len(snap.Keys) != 2 || snap.Keys[0] != "k1" || snap.Keys[1] != "k2" {
		t.Fatalf("unexpected keys after legacy update: %#v", snap.Keys)
	}
	if len(snap.APIKeys) != 2 {
		t.Fatalf("unexpected api keys after legacy update: %#v", snap.APIKeys)
	}
	if snap.APIKeys[0].Name != "primary" || snap.APIKeys[0].Remark != "prod" {
		t.Fatalf("metadata for preserved key was lost: %#v", snap.APIKeys[0])
	}
	if snap.APIKeys[1].Key != "k2" || snap.APIKeys[1].Name != "" || snap.APIKeys[1].Remark != "" {
		t.Fatalf("new legacy key should stay metadata-free: %#v", snap.APIKeys[1])
	}
}

func TestLoadStoreIgnoresLegacyConfigJSONEnv(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	path := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(path)

	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", "")
	t.Setenv("CONFIG_JSON", `{"keys":["legacy-key"]}`)
	t.Setenv("TOOL_GATEWAY_CONFIG_PATH", path)

	store := LoadStore()
	if store.HasEnvConfigSource() {
		t.Fatal("expected legacy CONFIG_JSON to be ignored")
	}
	if store.IsEnvBacked() {
		t.Fatal("expected store to remain file-backed/empty when only CONFIG_JSON is set")
	}
	if len(store.Keys()) != 0 {
		t.Fatalf("expected ignored legacy env to leave store empty, got keys=%d", len(store.Keys()))
	}
}
