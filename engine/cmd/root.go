package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/loongxjin/forksync/engine/internal/logger"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/loongxjin/forksync/engine/pkg/version"
	"github.com/spf13/cobra"
)

var jsonOutput bool

var rootCmd = &cobra.Command{
	Use:   "forksync",
	Short: "ForkSync - Auto-sync your fork repos",
	Long:  "ForkSync keeps your GitHub fork repositories up to date with their upstream sources.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip logger init for version command
		if cmd.Name() == "version" {
			return nil
		}
		_, cfgMgr := getSharedConfig()
		logDir := filepath.Join(cfgMgr.ConfigDir(), "logs")
		return logger.Init(logDir)
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	rootCmd.AddCommand(versionCmd)
}

func Execute() error {
	return rootCmd.Execute()
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print forksync version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("forksync %s\n", version.Version)
		fmt.Printf("commit:  %s\n", version.Commit)
		fmt.Printf("built:   %s\n", version.BuildDate)
	},
}

// outputJSON writes a JSON ApiResponse to stdout.
func outputJSON[T any](data T, err error) {
	resp := types.ApiResponse[T]{}
	if err != nil {
		resp.Success = false
		resp.Error = err.Error()
	} else {
		resp.Success = true
		resp.Data = data
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if encodeErr := enc.Encode(resp); encodeErr != nil {
		fmt.Fprintf(os.Stderr, "error encoding json: %v\n", encodeErr)
	}
}

// outputText writes a human-readable message to stdout.
func outputText(format string, args ...interface{}) {
	fmt.Printf(format+"\n", args...)
}

// isJSON returns whether JSON output mode is enabled.
func isJSON() bool {
	return jsonOutput
}
