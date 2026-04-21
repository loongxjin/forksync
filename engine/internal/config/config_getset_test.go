package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_Get_StringKey(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{configDir: tmpDir, viper: newViper()}

	// Default value
	val, err := m.Get("agent.preferred")
	require.NoError(t, err)
	assert.Equal(t, "", val) // default is empty string

	// After set
	err = m.Set("agent.preferred", "claude")
	require.NoError(t, err)
	val, err = m.Get("agent.preferred")
	require.NoError(t, err)
	assert.Equal(t, "claude", val)
}

func TestManager_Get_BoolKey(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{configDir: tmpDir, viper: newViper()}

	// Default value
	val, err := m.Get("sync.sync_on_startup")
	require.NoError(t, err)
	assert.Equal(t, true, val)

	// After set
	err = m.Set("sync.sync_on_startup", "false")
	require.NoError(t, err)
	val, err = m.Get("sync.sync_on_startup")
	require.NoError(t, err)
	assert.Equal(t, false, val)
}

func TestManager_Get_SliceKey(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{configDir: tmpDir, viper: newViper()}

	// Default value
	val, err := m.Get("agent.priority")
	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "opencode", "droid", "codex"}, val)

	// After set
	err = m.Set("agent.priority", `["opencode","claude"]`)
	require.NoError(t, err)
	val, err = m.Get("agent.priority")
	require.NoError(t, err)
	assert.Equal(t, []string{"opencode", "claude"}, val)
}

func TestManager_Get_UnknownKey(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{configDir: tmpDir, viper: newViper()}

	_, err := m.Get("nonexistent.key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown config key")
}

func TestManager_Set_UnknownKey(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{configDir: tmpDir, viper: newViper()}

	err := m.Set("nonexistent.key", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown config key")
}

func TestManager_Set_InvalidBool(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{configDir: tmpDir, viper: newViper()}

	err := m.Set("notification.enabled", "notabool")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid bool value")
}

func TestManager_Set_InvalidSlice(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{configDir: tmpDir, viper: newViper()}

	err := m.Set("agent.priority", "not-an-array")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON array")
}

func TestManager_Get_AllKeys(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{configDir: tmpDir, viper: newViper()}

	// Test that every valid key can be read
	for _, key := range ValidConfigKeys() {
		_, err := m.Get(key)
		assert.NoError(t, err, "Get(%q) should not error", key)
	}
}

func TestManager_Set_AllStringKeys(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{configDir: tmpDir, viper: newViper()}

	stringKeys := []string{
		"sync.default_interval",
		"agent.preferred",
		"agent.timeout",
		"agent.conflict_strategy",
		"agent.resolve_strategy",
		"agent.session_ttl",
		"github.token",
		"proxy.url",
	}

	for _, key := range stringKeys {
		err := m.Set(key, "test-value")
		require.NoError(t, err, "Set(%q) should not error", key)

		val, err := m.Get(key)
		require.NoError(t, err)
		assert.Equal(t, "test-value", val, "Get(%q) after Set", key)
	}
}

func TestManager_Set_AllBoolKeys(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{configDir: tmpDir, viper: newViper()}

	boolKeys := []string{
		"sync.sync_on_startup",
		"sync.auto_launch",
		"agent.confirm_before_commit",
		"notification.enabled",
		"proxy.enabled",
	}

	for _, key := range boolKeys {
		err := m.Set(key, "true")
		require.NoError(t, err, "Set(%q, true) should not error", key)

		val, err := m.Get(key)
		require.NoError(t, err)
		assert.Equal(t, true, val, "Get(%q) after Set true", key)

		err = m.Set(key, "false")
		require.NoError(t, err, "Set(%q, false) should not error", key)

		val, err = m.Get(key)
		require.NoError(t, err)
		assert.Equal(t, false, val, "Get(%q) after Set false", key)
	}
}

func TestManager_Set_PersistsToDisk(t *testing.T) {
	tmpDir := t.TempDir()
	m1 := &Manager{configDir: tmpDir, viper: newViper()}

	err := m1.Set("sync.default_interval", "2h")
	require.NoError(t, err)

	// Create a new manager reading from the same dir
	m2 := &Manager{configDir: tmpDir, viper: newViper()}
	val, err := m2.Get("sync.default_interval")
	require.NoError(t, err)
	assert.Equal(t, "2h", val)
}

func TestGetKeyType(t *testing.T) {
	assert.Equal(t, "string", GetKeyType("agent.preferred"))
	assert.Equal(t, "bool", GetKeyType("notification.enabled"))
	assert.Equal(t, "[]string", GetKeyType("agent.priority"))
	assert.Equal(t, "", GetKeyType("nonexistent"))
}

func TestValidConfigKeys(t *testing.T) {
	keys := ValidConfigKeys()
	assert.NotEmpty(t, keys)
	// Should have at least 14 keys (sync:3 + agent:7 + github:1 + notification:1 + proxy:2 = 15)
	assert.GreaterOrEqual(t, len(keys), 14)
}

func newViper() *viper.Viper {
	return viper.New()
}
