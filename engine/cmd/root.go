package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var jsonOutput bool

var rootCmd = &cobra.Command{
	Use:   "forksync",
	Short: "ForkSync - Auto-sync your fork repos",
	Long:  "ForkSync keeps your GitHub fork repositories up to date with their upstream sources.",
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
}

func Execute() error {
	return rootCmd.Execute()
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
		os.Exit(1)
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
