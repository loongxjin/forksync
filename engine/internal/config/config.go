package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

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
}

type AgentConfig struct {
	Preferred           string   `mapstructure:"preferred" yaml:"preferred"`
	Priority            []string `mapstructure:"priority" yaml:"priority"`
	Timeout             string   `mapstructure:"timeout" yaml:"timeout"`
	ConflictStrategy    string   `mapstructure:"conflict_strategy" yaml:"conflict_strategy"`
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
	home, _ := os.UserHomeDir()
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
	m.viper.SetDefault("agent.priority", []string{"claude", "opencode", "droid", "codex"})
	m.viper.SetDefault("agent.timeout", "10m")
	m.viper.SetDefault("agent.conflict_strategy", "preserve_ours")
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

	return os.WriteFile(configPath, data, 0644)
}

// validConfigKeys defines all supported dot-notation config keys and their types.
var validConfigKeys = map[string]string{
	// sync
	"sync.default_interval": "string",
	"sync.sync_on_startup":  "bool",
	"sync.auto_launch":     "bool",
	// agent
	"agent.preferred":             "string",
	"agent.priority":              "[]string",
	"agent.timeout":               "string",
	"agent.conflict_strategy":     "string",
	"agent.confirm_before_commit": "bool",
	"agent.session_ttl":           "string",
	// github
	"github.token": "string",
	// notification
	"notification.enabled":        "bool",
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

// Get returns the value of a config key using dot-notation (e.g. "agent.preferred").
func (m *Manager) Get(key string) (interface{}, error) {
	if _, ok := validConfigKeys[key]; !ok {
		return nil, fmt.Errorf("unknown config key: %q", key)
	}

	cfg, err := m.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	switch key {
	// sync
	case "sync.default_interval":
		return cfg.Sync.DefaultInterval, nil
	case "sync.sync_on_startup":
		return cfg.Sync.SyncOnStartup, nil
	case "sync.auto_launch":
		return cfg.Sync.AutoLaunch, nil
	// agent
	case "agent.preferred":
		return cfg.Agent.Preferred, nil
	case "agent.priority":
		return cfg.Agent.Priority, nil
	case "agent.timeout":
		return cfg.Agent.Timeout, nil
	case "agent.conflict_strategy":
		return cfg.Agent.ConflictStrategy, nil
	case "agent.confirm_before_commit":
		return cfg.Agent.ConfirmBeforeCommit, nil
	case "agent.session_ttl":
		return cfg.Agent.SessionTTL, nil
	// github
	case "github.token":
		return cfg.GitHub.Token, nil
	// notification
	case "notification.enabled":
		return cfg.Notification.Enabled, nil
	// proxy
	case "proxy.enabled":
		return cfg.Proxy.Enabled, nil
	case "proxy.url":
		return cfg.Proxy.URL, nil
	default:
		return nil, fmt.Errorf("unknown config key: %q", key)
	}
}

// Set updates a single config key using dot-notation and saves the full config.
// For "[]string" type keys, value should be a JSON-encoded string array like `["a","b"]`.
func (m *Manager) Set(key string, value string) error {
	if _, ok := validConfigKeys[key]; !ok {
		return fmt.Errorf("unknown config key: %q. Valid keys: %v", key, ValidConfigKeys())
	}

	cfg, err := m.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	switch key {
	// sync
	case "sync.default_interval":
		cfg.Sync.DefaultInterval = value
	case "sync.sync_on_startup":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid bool value %q for %s: %w", value, key, err)
		}
		cfg.Sync.SyncOnStartup = v
	case "sync.auto_launch":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid bool value %q for %s: %w", value, key, err)
		}
		cfg.Sync.AutoLaunch = v
	// agent
	case "agent.preferred":
		cfg.Agent.Preferred = value
	case "agent.priority":
		var arr []string
		if err := json.Unmarshal([]byte(value), &arr); err != nil {
			return fmt.Errorf("invalid JSON array %q for %s: %w", value, key, err)
		}
		cfg.Agent.Priority = arr
	case "agent.timeout":
		cfg.Agent.Timeout = value
	case "agent.conflict_strategy":
		cfg.Agent.ConflictStrategy = value
	case "agent.confirm_before_commit":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid bool value %q for %s: %w", value, key, err)
		}
		cfg.Agent.ConfirmBeforeCommit = v
	case "agent.session_ttl":
		cfg.Agent.SessionTTL = value
	// github
	case "github.token":
		cfg.GitHub.Token = value
	// notification
	case "notification.enabled":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid bool value %q for %s: %w", value, key, err)
		}
		cfg.Notification.Enabled = v
	// proxy
	case "proxy.enabled":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid bool value %q for %s: %w", value, key, err)
		}
		cfg.Proxy.Enabled = v
	case "proxy.url":
		cfg.Proxy.URL = value
	default:
		return fmt.Errorf("unknown config key: %q", key)
	}

	return m.Save(cfg)
}
