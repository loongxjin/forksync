package cmd

import (
	"fmt"

	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove <repo-name>",
	Short: "Remove a repository from ForkSync management",
	Long: `Remove a repository from ForkSync management.
This only removes it from tracking; the local repository is not deleted.`,
	Args: cobra.ExactArgs(1),
	RunE: runRemove,
}

func init() {
	rootCmd.AddCommand(removeCmd)
}

func runRemove(cmd *cobra.Command, args []string) error {
	cfgMgr := config.NewManager()
	_, _ = cfgMgr.Load()

	store := repo.NewJSONStore(cfgMgr.ConfigDir())
	if err := store.Load(); err != nil {
		return fmt.Errorf("load repo store: %w", err)
	}

	r, ok := store.GetByName(args[0])
	if !ok {
		return fmt.Errorf("repository %q not found", args[0])
	}

	if err := store.Remove(r.ID); err != nil {
		return fmt.Errorf("remove repo: %w", err)
	}

	if isJSON() {
		outputJSON(types.ApiResponse[struct {
			Removed string `json:"removed"`
		}]{
			Success: true,
			Data: struct {
				Removed string `json:"removed"`
			}{Removed: r.Name},
		}, nil)
	} else {
		outputText("✅ Removed %s from ForkSync", r.Name)
	}

	return nil
}
