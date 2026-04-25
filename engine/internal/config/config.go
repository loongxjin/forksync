package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"

	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Sync         SyncConfig         `mapstructure:"sync" yaml:"sync"`
	Agent        AgentConfig        `mapstructure:"agent" yaml:"agent"`
	GitHub       GitHubConfig       `mapstructure:"github" yaml:"github"`
	Notification NotificationConfig `mapstructure:"notification" yaml:"notification"`
	Proxy        ProxyConfig        `mapstructure:"proxy" yaml:"proxy"`
}

type SyncConfig struct {
	DefaultInterval string `mapstructure:"default_interval" yaml:"default_interval"`
	SyncOnStartup   bool   `mapstructure:"sync_on_startup" yaml:"sync_on_startup"`
	AutoLaunch      bool   `mapstructure:"auto_launch" yaml:"auto_launch"`
	AutoSummary     bool   `mapstructure:"auto_summary" yaml:"auto_summary"`
	SummaryAgent    string `mapstructure:"summary_agent" yaml:"summary_agent"`
	SummaryLanguage string `mapstructure:"summary_language" yaml:"summary_language"`
	SummaryTimeout  string `mapstructure:"summary_timeout" yaml:"summary_timeout"`
}

type AgentConfig struct {
	Preferred           string   `mapstructure:"preferred" yaml:"preferred"`
	Priority            []string `mapstructure:"priority" yaml:"priority"`
	Timeout             string   `mapstructure:"timeout" yaml:"timeout"`
	ConflictStrategy    string   `mapstructure:"conflict_strategy" yaml:"conflict_strategy"`
	ResolveStrategy     string   `mapstructure:"resolve_strategy" yaml:"resolve_strategy"`
	ConfirmBeforeCommit bool     `mapstructure:"confirm_before_commit" yaml:"confirm_before_commit"`
	SessionTTL          string   `mapstructure:"session_ttl" yaml:"session_ttl"`
}

type GitHubConfig struct {
	Token string `mapstructure:"token" yaml:"token"`
}

type NotificationConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
}

type ProxyConfig struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled"`
	URL     string `mapstructure:"url" yaml:"url"`
}

type Manager struct {
	configDir string
	viper     *viper.Viper
}

func NewManager() *Manager {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return &Manager{
		configDir: filepath.Join(home, ".forksync"),
		viper:     viper.New(),
	}
}

// NewManagerWithDir creates a Manager with a custom config directory (for testing).
func NewManagerWithDir(dir string) *Manager {
	return &Manager{
		configDir: dir,
		viper:     viper.New(),
	}
}

func (m *Manager) ConfigDir() string {
	return m.configDir
}

func (m *Manager) Load() (*Config, error) {
	if err := os.MkdirAll(m.configDir, 0755); err != nil {
		return nil, err
	}

	m.viper.SetConfigName("config")
	m.viper.SetConfigType("yaml")
	m.viper.AddConfigPath(m.configDir)

	// Defaults
	m.viper.SetDefault("sync.default_interval", "30m")
	m.viper.SetDefault("sync.sync_on_startup", true)
	m.viper.SetDefault("sync.auto_launch", false)
	m.viper.SetDefault("sync.auto_summary", false)
	m.viper.SetDefault("sync.summary_agent", "")
	m.viper.SetDefault("sync.summary_language", "zh")
	m.viper.SetDefault("sync.summary_timeout", "3m")
	m.viper.SetDefault("agent.priority", []string{"claude", "opencode", "droid", "codex"})
	m.viper.SetDefault("agent.timeout", "10m")
	m.viper.SetDefault("agent.conflict_strategy", "agent_resolve")
	m.viper.SetDefault("agent.resolve_strategy", "preserve_ours")
	m.viper.SetDefault("agent.confirm_before_commit", true)
	m.viper.SetDefault("agent.session_ttl", "24h")
	m.viper.SetDefault("notification.enabled", true)
	m.viper.SetDefault("proxy.enabled", false)

	if err := m.viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := m.viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Backward compatibility: if conflict_strategy contains a resolve strategy
	// value (preserve_ours, preserve_theirs, balanced), migrate it to the new
	// two-field model: conflict_strategy=agent_resolve + resolve_strategy=<value>.
	resolveStrategies := map[string]bool{types.ResolveStrategyPreserveOurs: true, types.ResolveStrategyPreserveTheirs: true, types.ResolveStrategyBalanced: true}
	if resolveStrategies[cfg.Agent.ConflictStrategy] {
		cfg.Agent.ResolveStrategy = cfg.Agent.ConflictStrategy
		cfg.Agent.ConflictStrategy = types.StrategyAgentResolve
		// Persist migration
		if err := m.Save(&cfg); err != nil {
			// Non-fatal: migration was applied in memory; save failure means
			// it will be re-applied on next load.
		}
	}

	return &cfg, nil
}

func (m *Manager) Save(cfg *Config) error {
	if err := os.MkdirAll(m.configDir, 0755); err != nil {
		return err
	}

	configPath := filepath.Join(m.configDir, "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0600)
}

// validConfigKeys defines all supported dot-notation config keys and their types.
var validConfigKeys = map[string]string{
	// sync
	"sync.default_interval": "string",
	"sync.sync_on_startup":  "bool",
	"sync.auto_launch":      "bool",
	"sync.auto_summary":     "bool",
	"sync.summary_agent":    "string",
	"sync.summary_language": "string",
	"sync.summary_timeout":  "string",
	// agent
	"agent.preferred":             "string",
	"agent.priority":              "[]string",
	"agent.timeout":               "string",
	"agent.conflict_strategy":     "string",
	"agent.resolve_strategy":      "string",
	"agent.confirm_before_commit": "bool",
	"agent.session_ttl":           "string",
	// github
	"github.token": "string",
	// notification
	"notification.enabled": "bool",
	// proxy
	"proxy.enabled": "bool",
	"proxy.url":     "string",
}

// GetKeyType returns the type of a config key (e.g. "string", "bool", "[]string").
// Returns empty string if the key is not recognized.
func GetKeyType(key string) string {
	return validConfigKeys[key]
}

// ValidConfigKeys returns all supported config keys.
func ValidConfigKeys() []string {
	keys := make([]string, 0, len(validConfigKeys))
	for k := range validConfigKeys {
		keys = append(keys, k)
	}
	return keys
}

// configFieldPaths maps each config key to the struct field index path within Config.
// For example, "agent.preferred" -> {1, 0} means Config.Agent.Preferred.
var configFieldPaths = map[string][]int{
	// sync
	"sync.default_interval": {0, 0},
	"sync.sync_on_startup":  {0, 1},
	"sync.auto_launch":      {0, 2},
	"sync.auto_summary":     {0, 3},
	"sync.summary_agent":    {0, 4},
	"sync.summary_language": {0, 5},
	"sync.summary_timeout":  {0, 6},
	// agent
	"agent.preferred":             {1, 0},
	"agent.priority":              {1, 1},
	"agent.timeout":               {1, 2},
	"agent.conflict_strategy":     {1, 3},
	"agent.resolve_strategy":      {1, 4},
	"agent.confirm_before_commit": {1, 5},
	"agent.session_ttl":           {1, 6},
	// github
	"github.token": {2, 0},
	// notification
	"notification.enabled": {3, 0},
	// proxy
	"proxy.enabled": {4, 0},
	"proxy.url":     {4, 1},
}

// configField returns the reflect.Value for the config key's target field.
func configField(cfg *Config, key string) (reflect.Value, error) {
	path, ok := configFieldPaths[key]
	if !ok {
		return reflect.Value{}, fmt.Errorf("unknown config key: %q", key)
	}

	v := reflect.ValueOf(cfg).Elem()
	for _, idx := range path {
		v = v.Field(idx)
	}
	return v, nil
}

// Get returns the value of a config key using dot-notation (e.g. "agent.preferred").
func (m *Manager) Get(key string) (interface{}, error) {
	if _, ok := validConfigKeys[key]; !ok {
		return nil, fmt.Errorf("unknown config key: %q", key)
	}

	cfg, err := m.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	field, err := configField(cfg, key)
	if err != nil {
		return nil, err
	}
	return field.Interface(), nil
}

// Set updates a single config key using dot-notation and saves the full config.
// For "[]string" type keys, value should be a JSON-encoded string array like `["a","b"]`.
// ResolveStrategyOrDefault returns the resolve strategy from config, or the default.
func ResolveStrategyOrDefault(cfg *Config) string {
	if cfg != nil && cfg.Agent.ResolveStrategy != "" {
		return cfg.Agent.ResolveStrategy
	}
	return types.ResolveStrategyPreserveOurs
}

func (m *Manager) Set(key string, value string) error {
	keyType, ok := validConfigKeys[key]
	if !ok {
		return fmt.Errorf("unknown config key: %q. Valid keys: %v", key, ValidConfigKeys())
	}

	cfg, err := m.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	field, err := configField(cfg, key)
	if err != nil {
		return err
	}

	switch keyType {
	case "bool":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid bool value %q for %s: %w", value, key, err)
		}
		field.SetBool(v)
	case "[]string":
		var arr []string
		if err := json.Unmarshal([]byte(value), &arr); err != nil {
			return fmt.Errorf("invalid JSON array %q for %s: %w", value, key, err)
		}
		field.Set(reflect.ValueOf(arr))
	case "string":
		field.SetString(value)
	default:
		return fmt.Errorf("unsupported config type %q for key %q", keyType, key)
	}

	return m.Save(cfg)
}
