package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Sync         SyncConfig         `mapstructure:"sync" yaml:"sync"`
	AI           AIConfig           `mapstructure:"ai" yaml:"ai"`
	GitHub       GitHubConfig       `mapstructure:"github" yaml:"github"`
	Notification NotificationConfig `mapstructure:"notification" yaml:"notification"`
	Proxy        ProxyConfig        `mapstructure:"proxy" yaml:"proxy"`
}

type SyncConfig struct {
	DefaultInterval string `mapstructure:"default_interval" yaml:"default_interval"`
	SyncOnStartup   bool   `mapstructure:"sync_on_startup" yaml:"sync_on_startup"`
	AutoLaunch      bool   `mapstructure:"auto_launch" yaml:"auto_launch"`
}

type AIConfig struct {
	DefaultProvider string                      `mapstructure:"default_provider" yaml:"default_provider"`
	Providers       map[string]AIProviderConfig `mapstructure:"providers" yaml:"providers"`
}

type AIProviderConfig struct {
	APIKey  string `mapstructure:"api_key" yaml:"api_key"`
	Model   string `mapstructure:"model" yaml:"model"`
	BaseURL string `mapstructure:"base_url" yaml:"base_url"`
}

type GitHubConfig struct {
	Token string `mapstructure:"token" yaml:"token"`
}

type NotificationConfig struct {
	Enabled       bool `mapstructure:"enabled" yaml:"enabled"`
	OnConflict    bool `mapstructure:"on_conflict" yaml:"on_conflict"`
	OnSyncSuccess bool `mapstructure:"on_sync_success" yaml:"on_sync_success"`
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
	m.viper.SetDefault("notification.enabled", true)
	m.viper.SetDefault("notification.on_conflict", true)
	m.viper.SetDefault("notification.on_sync_success", false)
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
	// Ensure the config directory exists
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
