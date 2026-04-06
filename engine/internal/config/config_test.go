package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	assert.NotNil(t, m)
	assert.NotNil(t, m.viper)
	
	home, _ := os.UserHomeDir()
	expectedDir := filepath.Join(home, ".forksync")
	assert.Equal(t, expectedDir, m.ConfigDir())
}

func TestManager_Load_WithDefaults(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	
	m := &Manager{
		configDir: tmpDir,
		viper:     viper.New(),
	}
	
	cfg, err := m.Load()
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	
	// Verify defaults are applied when no config file exists
	assert.Equal(t, "30m", cfg.Sync.DefaultInterval)
	assert.True(t, cfg.Sync.SyncOnStartup)
	assert.False(t, cfg.Sync.AutoLaunch)
	assert.True(t, cfg.Notification.Enabled)
	assert.True(t, cfg.Notification.OnConflict)
	assert.False(t, cfg.Notification.OnSyncSuccess)
	assert.False(t, cfg.Proxy.Enabled)
	
	// Verify AI config is empty by default
	assert.Empty(t, cfg.AI.DefaultProvider)
	assert.Empty(t, cfg.AI.Providers)
}

func TestManager_SaveAndReload(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	
	m := &Manager{
		configDir: tmpDir,
		viper:     viper.New(),
	}
	
	// Create a custom config
	cfg := &Config{
		Sync: SyncConfig{
			DefaultInterval: "1h",
			SyncOnStartup:   false,
			AutoLaunch:      true,
		},
		AI: AIConfig{
			DefaultProvider: "openai",
			Providers: map[string]AIProviderConfig{
				"openai": {
					APIKey:  "test-api-key",
					Model:   "gpt-4",
					BaseURL: "https://api.openai.com",
				},
			},
		},
		Notification: NotificationConfig{
			Enabled:       false,
			OnConflict:    false,
			OnSyncSuccess: true,
		},
		Proxy: ProxyConfig{
			Enabled: true,
			URL:     "http://proxy.example.com:8080",
		},
	}
	
	// Save the config
	err := m.Save(cfg)
	require.NoError(t, err)
	
	// Verify the file was created
	configPath := filepath.Join(tmpDir, "config.yaml")
	_, err = os.Stat(configPath)
	require.NoError(t, err)
	
	// Create a new manager to reload the config
	m2 := &Manager{
		configDir: tmpDir,
		viper:     viper.New(),
	}
	
	loadedCfg, err := m2.Load()
	require.NoError(t, err)
	
	// Verify all values persisted correctly
	assert.Equal(t, "1h", loadedCfg.Sync.DefaultInterval)
	assert.False(t, loadedCfg.Sync.SyncOnStartup)
	assert.True(t, loadedCfg.Sync.AutoLaunch)
	
	assert.Equal(t, "openai", loadedCfg.AI.DefaultProvider)
	require.NotNil(t, loadedCfg.AI.Providers)
	assert.Contains(t, loadedCfg.AI.Providers, "openai")
	assert.Equal(t, "test-api-key", loadedCfg.AI.Providers["openai"].APIKey)
	assert.Equal(t, "gpt-4", loadedCfg.AI.Providers["openai"].Model)
	assert.Equal(t, "https://api.openai.com", loadedCfg.AI.Providers["openai"].BaseURL)
	
	assert.False(t, loadedCfg.Notification.Enabled)
	assert.False(t, loadedCfg.Notification.OnConflict)
	assert.True(t, loadedCfg.Notification.OnSyncSuccess)
	
	assert.True(t, loadedCfg.Proxy.Enabled)
	assert.Equal(t, "http://proxy.example.com:8080", loadedCfg.Proxy.URL)
}

func TestManager_Load_MergesWithDefaults(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	
	// Create a partial config file
	configContent := `
sync:
  default_interval: 2h
  auto_launch: true
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)
	
	m := &Manager{
		configDir: tmpDir,
		viper:     viper.New(),
	}
	
	cfg, err := m.Load()
	require.NoError(t, err)
	
	// Verify custom values are loaded
	assert.Equal(t, "2h", cfg.Sync.DefaultInterval)
	assert.True(t, cfg.Sync.AutoLaunch)
	
	// Verify defaults are still applied for missing values
	assert.True(t, cfg.Sync.SyncOnStartup) // default value
	assert.True(t, cfg.Notification.Enabled) // default value
	assert.True(t, cfg.Notification.OnConflict) // default value
	assert.False(t, cfg.Notification.OnSyncSuccess) // default value
	assert.False(t, cfg.Proxy.Enabled) // default value
}

func TestManager_Save_CreatesConfigDir(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "nested", "config")
	
	m := &Manager{
		configDir: nestedDir,
		viper:     viper.New(),
	}
	
	cfg := &Config{
		Sync: SyncConfig{
			DefaultInterval: "15m",
		},
	}
	
	// Save should create the directory
	err := m.Save(cfg)
	require.NoError(t, err)
	
	// Verify the directory was created
	_, err = os.Stat(nestedDir)
	assert.NoError(t, err)
	
	// Verify the file was created
	configPath := filepath.Join(nestedDir, "config.yaml")
	_, err = os.Stat(configPath)
	assert.NoError(t, err)
}

func TestManager_Load_InvalidConfigFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	
	// Create an invalid YAML file
	configContent := `invalid: yaml: content: [:`
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)
	
	m := &Manager{
		configDir: tmpDir,
		viper:     viper.New(),
	}
	
	// Load should return an error for invalid YAML
	_, err = m.Load()
	assert.Error(t, err)
}

func TestManager_MultipleProviders(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	
	m := &Manager{
		configDir: tmpDir,
		viper:     viper.New(),
	}
	
	// Create a config with multiple AI providers
	cfg := &Config{
		AI: AIConfig{
			DefaultProvider: "anthropic",
			Providers: map[string]AIProviderConfig{
				"openai": {
					APIKey:  "openai-key",
					Model:   "gpt-4",
					BaseURL: "https://api.openai.com",
				},
				"anthropic": {
					APIKey:  "anthropic-key",
					Model:   "claude-3",
					BaseURL: "https://api.anthropic.com",
				},
				"deepseek": {
					APIKey:  "deepseek-key",
					Model:   "deepseek-chat",
					BaseURL: "https://api.deepseek.com",
				},
			},
		},
	}
	
	// Save the config
	err := m.Save(cfg)
	require.NoError(t, err)
	
	// Reload and verify
	m2 := &Manager{
		configDir: tmpDir,
		viper:     viper.New(),
	}
	
	loadedCfg, err := m2.Load()
	require.NoError(t, err)
	
	assert.Equal(t, "anthropic", loadedCfg.AI.DefaultProvider)
	require.Len(t, loadedCfg.AI.Providers, 3)
	
	// Verify all providers
	assert.Equal(t, "openai-key", loadedCfg.AI.Providers["openai"].APIKey)
	assert.Equal(t, "gpt-4", loadedCfg.AI.Providers["openai"].Model)
	
	assert.Equal(t, "anthropic-key", loadedCfg.AI.Providers["anthropic"].APIKey)
	assert.Equal(t, "claude-3", loadedCfg.AI.Providers["anthropic"].Model)
	
	assert.Equal(t, "deepseek-key", loadedCfg.AI.Providers["deepseek"].APIKey)
	assert.Equal(t, "deepseek-chat", loadedCfg.AI.Providers["deepseek"].Model)
}
