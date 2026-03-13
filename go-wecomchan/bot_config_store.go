package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var routeSuffixPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

var reservedRouteSuffixes = map[string]struct{}{
	"admin":       {},
	"api":         {},
	"callback":    {},
	"favicon.ico": {},
}

type BotConfig struct {
	RouteSuffix    string `json:"route_suffix"`
	DisplayName    string `json:"display_name"`
	SendKey        string `json:"sendkey"`
	WecomCID       string `json:"wecom_cid"`
	WecomSecret    string `json:"wecom_secret"`
	WecomAID       string `json:"wecom_aid"`
	WecomToUID     string `json:"wecom_touid"`
	CallbackToken  string `json:"callback_token"`
	CallbackAESKey string `json:"callback_aes_key"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type RedisConfig struct {
	Enabled  bool   `json:"enabled"`
	Addr     string `json:"addr"`
	Password string `json:"password"`
}

type botConfigFile struct {
	Redis   RedisConfig `json:"redis"`
	Configs []BotConfig `json:"configs"`
}

type legacyBotConfig struct {
	BotConfig
	RedisEnabled  bool   `json:"redis_enabled"`
	RedisAddr     string `json:"redis_addr"`
	RedisPassword string `json:"redis_password"`
}

type legacyBotConfigFile struct {
	Configs []legacyBotConfig `json:"configs"`
}

type BotConfigStore struct {
	mu      sync.RWMutex
	path    string
	redis   RedisConfig
	configs map[string]BotConfig
}

func NewBotConfigStore(path string) (*BotConfigStore, error) {
	store := &BotConfigStore{
		path:    path,
		configs: make(map[string]BotConfig),
	}

	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func normalizeRouteSuffix(value string) string {
	value = strings.TrimSpace(strings.Trim(value, "/"))
	return strings.ToLower(value)
}

func buildSeedBotConfigFromEnv() (BotConfig, bool) {
	sendKey, ok := LookupEnvTrim("SENDKEY")
	if !ok || sendKey == "" {
		return BotConfig{}, false
	}

	wecomCID, cidOK := LookupEnvTrim("WECOM_CID")
	wecomSecret, secretOK := LookupEnvTrim("WECOM_SECRET")
	wecomAID, aidOK := LookupEnvTrim("WECOM_AID")
	if !cidOK || !secretOK || !aidOK || wecomCID == "" || wecomSecret == "" || wecomAID == "" {
		return BotConfig{}, false
	}

	wecomToUID := strings.TrimSpace(GetEnvDefault("WECOM_TOUID", "@all"))
	if wecomToUID == "" {
		wecomToUID = "@all"
	}

	callbackToken, _ := LookupEnvTrim("WECOM_TOKEN")
	callbackAESKey, _ := LookupEnvTrim("WECOM_AES_KEY")

	return BotConfig{
		RouteSuffix:    DefaultRouteSuffix,
		DisplayName:    DefaultRouteSuffix,
		SendKey:        sendKey,
		WecomCID:       wecomCID,
		WecomSecret:    wecomSecret,
		WecomAID:       wecomAID,
		WecomToUID:     wecomToUID,
		CallbackToken:  callbackToken,
		CallbackAESKey: callbackAESKey,
	}, true
}

func buildSeedRedisConfigFromEnv() RedisConfig {
	redisAddr, _ := LookupEnvTrim("REDIS_ADDR")
	redisPassword, _ := LookupEnvTrim("REDIS_PASSWORD")
	return sanitizeRedisConfig(RedisConfig{
		Enabled:  strings.EqualFold(GetEnvDefault("REDIS_STAT", "OFF"), "ON"),
		Addr:     redisAddr,
		Password: redisPassword,
	})
}

func sanitizeBotConfig(cfg BotConfig) BotConfig {
	cfg.RouteSuffix = normalizeRouteSuffix(cfg.RouteSuffix)
	cfg.DisplayName = strings.TrimSpace(cfg.DisplayName)
	cfg.SendKey = strings.TrimSpace(cfg.SendKey)
	cfg.WecomCID = strings.TrimSpace(cfg.WecomCID)
	cfg.WecomSecret = strings.TrimSpace(cfg.WecomSecret)
	cfg.WecomAID = strings.TrimSpace(cfg.WecomAID)
	cfg.WecomToUID = strings.TrimSpace(cfg.WecomToUID)
	cfg.CallbackToken = strings.TrimSpace(cfg.CallbackToken)
	cfg.CallbackAESKey = strings.TrimSpace(cfg.CallbackAESKey)
	cfg.CreatedAt = strings.TrimSpace(cfg.CreatedAt)
	cfg.UpdatedAt = strings.TrimSpace(cfg.UpdatedAt)
	if cfg.DisplayName == "" {
		cfg.DisplayName = cfg.RouteSuffix
	}
	if cfg.WecomToUID == "" {
		cfg.WecomToUID = "@all"
	}
	return cfg
}

func sanitizeRedisConfig(cfg RedisConfig) RedisConfig {
	cfg.Addr = strings.TrimSpace(cfg.Addr)
	cfg.Password = strings.TrimSpace(cfg.Password)
	return cfg
}

func validateBotConfig(cfg BotConfig) error {
	if cfg.RouteSuffix == "" {
		return fmt.Errorf("route_suffix is required")
	}
	if !routeSuffixPattern.MatchString(cfg.RouteSuffix) {
		return fmt.Errorf("route_suffix must match [a-z0-9][a-z0-9_-]{0,63}")
	}
	if _, reserved := reservedRouteSuffixes[cfg.RouteSuffix]; reserved {
		return fmt.Errorf("route_suffix %q is reserved", cfg.RouteSuffix)
	}
	if cfg.SendKey == "" {
		return fmt.Errorf("sendkey is required")
	}
	if cfg.WecomCID == "" {
		return fmt.Errorf("wecom_cid is required")
	}
	if cfg.WecomSecret == "" {
		return fmt.Errorf("wecom_secret is required")
	}
	if cfg.WecomAID == "" {
		return fmt.Errorf("wecom_aid is required")
	}
	return nil
}

func validateRedisConfig(cfg RedisConfig) error {
	if cfg.Enabled && cfg.Addr == "" {
		return fmt.Errorf("redis.addr is required when redis.enabled is true")
	}
	return nil
}

func hasRedisConfig(cfg RedisConfig) bool {
	return cfg.Enabled || cfg.Addr != "" || cfg.Password != ""
}

func deriveLegacyRedisConfig(configs []legacyBotConfig) RedisConfig {
	for _, cfg := range configs {
		redisCfg := sanitizeRedisConfig(RedisConfig{
			Enabled:  cfg.RedisEnabled,
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
		})
		if hasRedisConfig(redisCfg) {
			return redisCfg
		}
	}
	return RedisConfig{}
}

func sortConfigs(configs []BotConfig) {
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].RouteSuffix < configs[j].RouteSuffix
	})
}

func (s *BotConfigStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if seed, ok := buildSeedBotConfigFromEnv(); ok {
			now := time.Now().Format(time.RFC3339)
			s.redis = buildSeedRedisConfigFromEnv()
			if err := validateRedisConfig(s.redis); err != nil {
				return err
			}
			seed = sanitizeBotConfig(seed)
			seed.CreatedAt = now
			seed.UpdatedAt = now
			s.configs[seed.RouteSuffix] = seed
			return s.saveLocked()
		}
		return nil
	}

	var file botConfigFile
	if len(data) > 0 {
		if err := json.Unmarshal(data, &file); err != nil {
			return fmt.Errorf("decode bot config file: %w", err)
		}
	}
	file.Redis = sanitizeRedisConfig(file.Redis)
	if !hasRedisConfig(file.Redis) && len(data) > 0 {
		var legacyFile legacyBotConfigFile
		if err := json.Unmarshal(data, &legacyFile); err == nil {
			file.Redis = deriveLegacyRedisConfig(legacyFile.Configs)
		}
	}
	if err := validateRedisConfig(file.Redis); err != nil {
		return err
	}
	s.redis = file.Redis

	for _, cfg := range file.Configs {
		cfg = sanitizeBotConfig(cfg)
		if err := validateBotConfig(cfg); err != nil {
			return fmt.Errorf("invalid bot config %q: %w", cfg.RouteSuffix, err)
		}
		s.configs[cfg.RouteSuffix] = cfg
	}
	return nil
}

func (s *BotConfigStore) saveLocked() error {
	configs := make([]BotConfig, 0, len(s.configs))
	for _, cfg := range s.configs {
		configs = append(configs, cfg)
	}
	sortConfigs(configs)

	payload, err := json.MarshalIndent(botConfigFile{Redis: s.redis, Configs: configs}, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		if errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.EXDEV) || errors.Is(err, syscall.EPERM) {
			if writeErr := os.WriteFile(s.path, payload, 0o600); writeErr == nil {
				_ = os.Remove(tmpPath)
				return nil
			} else {
				return fmt.Errorf("replace config file: %w (fallback write failed: %v)", err, writeErr)
			}
		}
		return err
	}
	return nil
}

func (s *BotConfigStore) List() []BotConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	configs := make([]BotConfig, 0, len(s.configs))
	for _, cfg := range s.configs {
		configs = append(configs, cfg)
	}
	sortConfigs(configs)
	return configs
}

func (s *BotConfigStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.configs)
}

func (s *BotConfigStore) RedisConfig() RedisConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.redis
}

func (s *BotConfigStore) Get(route string) (BotConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg, ok := s.configs[normalizeRouteSuffix(route)]
	return cfg, ok
}

func (s *BotConfigStore) Default() (BotConfig, bool) {
	if cfg, ok := s.Get(DefaultRouteSuffix); ok {
		return cfg, true
	}
	configs := s.List()
	if len(configs) == 0 {
		return BotConfig{}, false
	}
	return configs[0], true
}

func (s *BotConfigStore) UpdateRedisConfig(cfg RedisConfig) (RedisConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg = sanitizeRedisConfig(cfg)
	if err := validateRedisConfig(cfg); err != nil {
		return RedisConfig{}, err
	}

	s.redis = cfg
	if err := s.saveLocked(); err != nil {
		return RedisConfig{}, err
	}
	return s.redis, nil
}

func (s *BotConfigStore) Upsert(currentRoute string, cfg BotConfig) (BotConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentRoute = normalizeRouteSuffix(currentRoute)
	cfg = sanitizeBotConfig(cfg)
	if err := validateBotConfig(cfg); err != nil {
		return BotConfig{}, err
	}

	now := time.Now().Format(time.RFC3339)
	if currentRoute != "" {
		existing, ok := s.configs[currentRoute]
		if !ok {
			return BotConfig{}, fmt.Errorf("config %q not found", currentRoute)
		}
		if cfg.RouteSuffix != currentRoute {
			if _, exists := s.configs[cfg.RouteSuffix]; exists {
				return BotConfig{}, fmt.Errorf("route_suffix %q already exists", cfg.RouteSuffix)
			}
			delete(s.configs, currentRoute)
		}
		cfg.CreatedAt = existing.CreatedAt
		cfg.UpdatedAt = now
	} else {
		if _, exists := s.configs[cfg.RouteSuffix]; exists {
			return BotConfig{}, fmt.Errorf("route_suffix %q already exists", cfg.RouteSuffix)
		}
		cfg.CreatedAt = now
		cfg.UpdatedAt = now
	}

	s.configs[cfg.RouteSuffix] = cfg
	if err := s.saveLocked(); err != nil {
		return BotConfig{}, err
	}
	return cfg, nil
}

func (s *BotConfigStore) Delete(route string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	route = normalizeRouteSuffix(route)
	if route == "" {
		return fmt.Errorf("route_suffix is required")
	}
	if _, ok := s.configs[route]; !ok {
		return fmt.Errorf("config %q not found", route)
	}
	delete(s.configs, route)
	return s.saveLocked()
}
