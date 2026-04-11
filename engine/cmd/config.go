package cmd

import (
	"fmt"
	"sort"

	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage ForkSync configuration",
	Long:  "Read and write ForkSync configuration values. All keys use dot-notation (e.g. 'agent.preferred').",
}

var configGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get all configuration values",
	Long:  "Display all configuration values as JSON (with --json) or in human-readable format.",
	RunE:  runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value by key. Keys use dot-notation.

Examples:
  forksync config set agent.preferred claude
  forksync config set sync.default_interval 1h
  forksync config set notification.enabled false
  forksync config set agent.priority '["claude","opencode"]'`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configKeysCmd = &cobra.Command{
	Use:   "keys",
	Short: "List all configuration keys",
	Long:  "Display all supported configuration keys and their types.",
	RunE:  runConfigKeys,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configKeysCmd)
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	mgr := config.NewManager()
	cfg, err := mgr.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if isJSON() {
		outputJSON(cfg, nil)
	} else {
		outputText("ForkSync Configuration:")
		outputText("")
		outputText("Sync:")
		outputText("  default_interval: %s", cfg.Sync.DefaultInterval)
		outputText("  sync_on_startup:  %t", cfg.Sync.SyncOnStartup)
		outputText("  auto_launch:      %t", cfg.Sync.AutoLaunch)
		outputText("")
		outputText("Agent:")
		outputText("  preferred:             %s", cfg.Agent.Preferred)
		outputText("  priority:              %v", cfg.Agent.Priority)
		outputText("  timeout:               %s", cfg.Agent.Timeout)
		outputText("  conflict_strategy:     %s", cfg.Agent.ConflictStrategy)
		outputText("  confirm_before_commit: %t", cfg.Agent.ConfirmBeforeCommit)
		outputText("  session_ttl:           %s", cfg.Agent.SessionTTL)
		outputText("")
		outputText("GitHub:")
		if cfg.GitHub.Token != "" {
			outputText("  token: (set)")
		} else {
			outputText("  token: (not set)")
		}
		outputText("")
		outputText("Notification:")
		outputText("  enabled:         %t", cfg.Notification.Enabled)
		outputText("  on_conflict:     %t", cfg.Notification.OnConflict)
		outputText("  on_sync_success: %t", cfg.Notification.OnSyncSuccess)
		outputText("")
		outputText("Proxy:")
		outputText("  enabled: %t", cfg.Proxy.Enabled)
		outputText("  url:     %s", cfg.Proxy.URL)
	}

	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	mgr := config.NewManager()
	if err := mgr.Set(key, value); err != nil {
		return err
	}

	// Read back to confirm
	newValue, err := mgr.Get(key)
	if err != nil {
		return err
	}

	if isJSON() {
		outputJSON(map[string]interface{}{
			"key":   key,
			"value": newValue,
		}, nil)
	} else {
		outputText("Config updated: %s = %v", key, newValue)
	}

	return nil
}

func runConfigKeys(cmd *cobra.Command, args []string) error {
	keys := config.ValidConfigKeys()
	sort.Strings(keys)

	if isJSON() {
		result := make([]map[string]string, 0, len(keys))
		for _, k := range keys {
			result = append(result, map[string]string{
				"key":  k,
				"type": config.GetKeyType(k),
			})
		}
		outputJSON(result, nil)
	} else {
		outputText("Available config keys:")
		outputText("")
		for _, k := range keys {
			outputText("  %-35s (%s)", k, config.GetKeyType(k))
		}
		outputText("")
		outputText("Usage: forksync config set <key> <value>")
		outputText("Note: For '[]string' type, use JSON array format: '[\"a\",\"b\"]'")
		outputText("Note: For 'bool' type, use: true/false")
	}

	return nil
}
