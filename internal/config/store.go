package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
)

type Store struct {
	mu      sync.RWMutex
	cfg     Config
	path    string
	fromEnv bool
	keyMap  map[string]struct{} // O(1) API key lookup index
}

func LoadStore() *Store {
	store, err := loadStore()
	if err != nil {
		Logger.Warn("[config] load failed", "error", err)
	}
	if len(store.cfg.Keys) == 0 {
		Logger.Warn("[config] empty config loaded")
	}
	store.rebuildIndexes()
	return store
}

func LoadStoreWithError() (*Store, error) {
	store, err := loadStore()
	if err != nil {
		return nil, err
	}
	store.rebuildIndexes()
	return store, nil
}

func loadStore() (*Store, error) {
	cfg, fromEnv, err := loadConfig()
	cfg.NormalizeCredentials()
	if validateErr := ValidateConfig(cfg); validateErr != nil {
		err = errors.Join(err, validateErr)
	}
	return &Store{cfg: cfg, path: ConfigPath(), fromEnv: fromEnv}, err
}

func loadConfig() (Config, bool, error) {
	rawCfg := strings.TrimSpace(os.Getenv("TOOL_GATEWAY_CONFIG_JSON"))
	path := ConfigPath()
	if rawCfg != "" {
		cfg, err := parseConfigString(rawCfg)
		if err != nil {
			if !IsVercel() && envWritebackEnabled() {
				if fileCfg, fileErr := loadConfigFromFile(path); fileErr == nil {
					return fileCfg, false, nil
				}
			}
			return cfg, true, err
		}
		if IsVercel() || !envWritebackEnabled() {
			return cfg, true, err
		}
		content, fileErr := os.ReadFile(path)
		if fileErr == nil {
			var fileCfg Config
			if unmarshalErr := json.Unmarshal(content, &fileCfg); unmarshalErr == nil {
				return fileCfg, false, err
			}
		}
		if errors.Is(fileErr, os.ErrNotExist) {
			if validateErr := ValidateConfig(cfg); validateErr != nil {
				return cfg, true, validateErr
			}
			if writeErr := writeConfigFile(path, cfg.Clone()); writeErr == nil {
				return cfg, false, err
			} else {
				Logger.Warn("[config] env writeback bootstrap failed", "error", writeErr)
			}
		}
		return cfg, true, err
	}
	cfg, err := loadConfigFromFile(path)
	if err != nil {
		if shouldTryLegacyContainerConfigPath() {
			legacyPath := legacyContainerConfigPath()
			if legacyCfg, legacyErr := loadConfigFromFile(legacyPath); legacyErr == nil {
				Logger.Info("[config] loaded legacy container config path", "path", legacyPath)
				return legacyCfg, false, nil
			}
		}
		if IsVercel() {
			// Vercel may start without writable/present config; keep in-memory bootstrap config.
			return Config{}, true, nil
		}
		if shouldBootstrapMissingConfigFile(err) {
			Logger.Warn("[config] config file missing; starting with empty file-backed config", "path", path)
			return Config{}, false, nil
		}
		return Config{}, false, err
	}
	if IsVercel() {
		// Vercel filesystem is ephemeral/read-only for runtime writes; avoid save errors.
		return cfg, true, nil
	}
	return cfg, false, nil
}

func shouldBootstrapMissingConfigFile(err error) bool {
	return errors.Is(err, os.ErrNotExist) && strings.TrimSpace(os.Getenv("TOOL_GATEWAY_CONFIG_PATH")) != ""
}

func loadConfigFromFile(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(content, &cfg); err != nil {
		return Config{}, err
	}
	cfg.NormalizeCredentials()
	return cfg, nil
}

func (s *Store) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Clone()
}

func (s *Store) HasAPIKey(k string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.keyMap[k]
	return ok
}

func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cfg.Keys)
}

func (s *Store) Replace(cfg Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg.NormalizeCredentials()
	s.cfg = cfg.Clone()
	s.rebuildIndexes()
	return s.saveLocked()
}

func (s *Store) Update(mutator func(*Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	base := s.cfg.Clone()
	cfg := base.Clone()
	if err := mutator(&cfg); err != nil {
		return err
	}
	cfg.ReconcileCredentials(base)
	cfg.NormalizeCredentials()
	s.cfg = cfg
	s.rebuildIndexes()
	return s.saveLocked()
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fromEnv && (IsVercel() || !envWritebackEnabled()) {
		Logger.Info("[save_config] source from env, skip write")
		return nil
	}
	persistCfg := s.cfg.Clone()
	b, err := json.MarshalIndent(persistCfg, "", "  ")
	if err != nil {
		return err
	}
	if err := writeConfigBytes(s.path, b); err != nil {
		return err
	}
	s.fromEnv = false
	return nil
}

func (s *Store) saveLocked() error {
	if s.fromEnv && (IsVercel() || !envWritebackEnabled()) {
		Logger.Info("[save_config] source from env, skip write")
		return nil
	}
	persistCfg := s.cfg.Clone()
	b, err := json.MarshalIndent(persistCfg, "", "  ")
	if err != nil {
		return err
	}
	if err := writeConfigBytes(s.path, b); err != nil {
		return err
	}
	s.fromEnv = false
	return nil
}

func (s *Store) IsEnvBacked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fromEnv
}

func (s *Store) SetVercelSync(hash string, ts int64) error {
	return s.Update(func(c *Config) error {
		c.VercelSyncHash = hash
		c.VercelSyncTime = ts
		return nil
	})
}

func (s *Store) ExportJSONAndBase64() (string, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	exportCfg := s.cfg.Clone()
	b, err := json.Marshal(exportCfg)
	if err != nil {
		return "", "", err
	}
	return string(b), base64.StdEncoding.EncodeToString(b), nil
}
