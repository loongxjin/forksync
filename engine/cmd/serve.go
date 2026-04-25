package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/internal/repo"
	sched "github.com/loongxjin/forksync/engine/internal/scheduler"
	"github.com/spf13/cobra"
)

var serveInterval string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the ForkSync background service (scheduler)",
	Long: `Start the ForkSync background service that periodically syncs all managed repositories.
This is designed to be spawned by the Electron UI.

The service runs until interrupted (SIGINT/SIGTERM).`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().StringVar(&serveInterval, "interval", "", "sync interval (overrides config, e.g. 15m, 1h)")
	rootCmd.AddCommand(serveCmd)
}

// ServeStatus is the JSON status output for the serve command.
type ServeStatus struct {
	Running  bool   `json:"running"`
	Interval string `json:"interval"`
	Message  string `json:"message"`
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, cfgMgr := getSharedConfig()

	// Override interval from flag if provided
	// Make a copy to avoid mutating the shared config pointer.
	if serveInterval != "" && cfg != nil {
		cfgCopy := *cfg
		cfgCopy.Sync.DefaultInterval = serveInterval
		cfg = &cfgCopy
	}

	store := repo.NewJSONStore(cfgMgr.ConfigDir())
	if err := store.Load(); err != nil {
		return fmt.Errorf("load repo store: %w", err)
	}

	// Create syncer with all dependencies
	syncer, cleanup := setupSyncer(cfg, cfgMgr, store)
	defer cleanup()

	// Create and start scheduler (nil notifier — notifications handled by Electron layer)
	scheduler := sched.NewScheduler(syncer, nil, cfg)

	// Ensure logger is closed on exit
	defer logger.Close()

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// Determine interval string for output
	intervalStr := "30m"
	if cfg != nil && cfg.Sync.DefaultInterval != "" {
		intervalStr = cfg.Sync.DefaultInterval
	}

	// Output startup status
	if isJSON() {
		status := ServeStatus{
			Running:  true,
			Interval: intervalStr,
			Message:  "ForkSync service started",
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(status); err != nil {
			fmt.Fprintf(os.Stderr, "error encoding status: %v\n", err)
		}
	} else {
		outputText("🚀 ForkSync service started (interval: %s)", intervalStr)
		outputText("Press Ctrl+C to stop")
	}

	// Start scheduler (runs SyncAll immediately, then on interval)
	scheduler.Start(ctx)

	// Wait for signal
	<-sigCh
	cancel()

	outputText("Stopping ForkSync service...")
	scheduler.Stop()
	outputText("ForkSync service stopped.")

	return nil
}
