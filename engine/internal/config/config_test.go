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
	assert.False(t, cfg.Proxy.Enabled)

	// Verify Agent config defaults
	assert.NotEmpty(t, cfg.Agent.Priority)
	assert.Equal(t, "10m", cfg.Agent.Timeout)
	assert.Equal(t, "preserve_ours", cfg.Agent.ConflictStrategy)
	assert.True(t, cfg.Agent.ConfirmBeforeCommit)
	assert.Equal(t, "24h", cfg.Agent.SessionTTL)
}

func TestManager_SaveAndReload(t *testing.T) {
	tmpDir := t.TempDir()

	m := &Manager{
		configDir: tmpDir,
		viper:     viper.New(),
	}

	cfg := &Config{
		Sync: SyncConfig{
			DefaultInterval: "1h",
			SyncOnStartup:   false,
			AutoLaunch:      true,
		},
		Agent: AgentConfig{
			Preferred:           "claude",
			Priority:            []string{"claude", "opencode", "droid", "codex"},
			Timeout:             "10m",
			ConflictStrategy:    "preserve_ours",
			ConfirmBeforeCommit: true,
			SessionTTL:          "24h",
		},
		Notification: NotificationConfig{
			Enabled: false,
		},
		Proxy: ProxyConfig{
			Enabled: true,
			URL:     "http://proxy.example.com:8080",
		},
	}

	err := m.Save(cfg)
	require.NoError(t, err)

	configPath := filepath.Join(tmpDir, "config.yaml")
	_, err = os.Stat(configPath)
	require.NoError(t, err)

	m2 := &Manager{
		configDir: tmpDir,
		viper:     viper.New(),
	}

	loadedCfg, err := m2.Load()
	require.NoError(t, err)

	assert.Equal(t, "1h", loadedCfg.Sync.DefaultInterval)
	assert.False(t, loadedCfg.Sync.SyncOnStartup)
	assert.True(t, loadedCfg.Sync.AutoLaunch)

	assert.Equal(t, "claude", loadedCfg.Agent.Preferred)
	assert.Equal(t, []string{"claude", "opencode", "droid", "codex"}, loadedCfg.Agent.Priority)
	assert.Equal(t, "10m", loadedCfg.Agent.Timeout)
	assert.Equal(t, "preserve_ours", loadedCfg.Agent.ConflictStrategy)
	assert.True(t, loadedCfg.Agent.ConfirmBeforeCommit)
	assert.Equal(t, "24h", loadedCfg.Agent.SessionTTL)

	assert.False(t, loadedCfg.Notification.Enabled)

	assert.True(t, loadedCfg.Proxy.Enabled)
	assert.Equal(t, "http://proxy.example.com:8080", loadedCfg.Proxy.URL)
}

func TestManager_Load_MergesWithDefaults(t *testing.T) {
	tmpDir := t.TempDir()

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

	assert.Equal(t, "2h", cfg.Sync.DefaultInterval)
	assert.True(t, cfg.Sync.AutoLaunch)
	assert.True(t, cfg.Sync.SyncOnStartup)
	assert.True(t, cfg.Notification.Enabled)
	assert.False(t, cfg.Proxy.Enabled)
}

func TestManager_Save_CreatesConfigDir(t *testing.T) {
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

	err := m.Save(cfg)
	require.NoError(t, err)

	_, err = os.Stat(nestedDir)
	assert.NoError(t, err)

	configPath := filepath.Join(nestedDir, "config.yaml")
	_, err = os.Stat(configPath)
	assert.NoError(t, err)
}

func TestManager_Load_InvalidConfigFile(t *testing.T) {
	tmpDir := t.TempDir()

	configContent := `invalid: yaml: content: [:`
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	m := &Manager{
		configDir: tmpDir,
		viper:     viper.New(),
	}

	_, err = m.Load()
	assert.Error(t, err)
}

func TestNewManagerWithDir(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManagerWithDir(tmpDir)
	assert.NotNil(t, m)
	assert.Equal(t, tmpDir, m.ConfigDir())
}
