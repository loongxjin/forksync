package cmd

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/loongxjin/forksync/engine/internal/config"
	"github.com/loongxjin/forksync/engine/internal/repo"
	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/spf13/cobra"
)

var (
	postSyncName string
	postSyncCmd  string
	postSyncID   string
)

var repoPostSyncCmd = &cobra.Command{
	Use:   "post-sync",
	Short: "Manage post-sync commands for a repository",
}

var postSyncListCmd = &cobra.Command{
	Use:   "list <repo-name>",
	Short: "List post-sync commands for a repository",
	Args:  cobra.ExactArgs(1),
	RunE:  runPostSyncList,
}

var postSyncAddCmd = &cobra.Command{
	Use:   "add <repo-name>",
	Short: "Add a post-sync command to a repository",
	Args:  cobra.ExactArgs(1),
	RunE:  runPostSyncAdd,
}

var postSyncRemoveCmd = &cobra.Command{
	Use:   "remove <repo-name>",
	Short: "Remove a post-sync command from a repository",
	Args:  cobra.ExactArgs(1),
	RunE:  runPostSyncRemove,
}

func init() {
	postSyncAddCmd.Flags().StringVar(&postSyncName, "name", "", "command name/description (required)")
	postSyncAddCmd.Flags().StringVar(&postSyncCmd, "cmd", "", "shell command to execute (required)")
	_ = postSyncAddCmd.MarkFlagRequired("name")
	_ = postSyncAddCmd.MarkFlagRequired("cmd")

	postSyncRemoveCmd.Flags().StringVar(&postSyncID, "id", "", "command ID to remove (required)")
	_ = postSyncRemoveCmd.MarkFlagRequired("id")

	repoPostSyncCmd.AddCommand(postSyncListCmd)
	repoPostSyncCmd.AddCommand(postSyncAddCmd)
	repoPostSyncCmd.AddCommand(postSyncRemoveCmd)
	rootCmd.AddCommand(repoPostSyncCmd)
}

func loadRepoStore() (repo.Store, error) {
	cfgMgr := config.NewManager()
	store := repo.NewJSONStore(cfgMgr.ConfigDir())
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("load repo store: %w", err)
	}
	return store, nil
}

func runPostSyncList(cmd *cobra.Command, args []string) error {
	store, err := loadRepoStore()
	if err != nil {
		if isJSON() {
			outputJSON(types.PostSyncCommandsData{}, err)
			return nil
		}
		return err
	}

	name := args[0]
	r, ok := store.GetByName(name)
	if !ok {
		err := fmt.Errorf("repo %q not found", name)
		if isJSON() {
			outputJSON(types.PostSyncCommandsData{}, err)
			return nil
		}
		return err
	}

	data := types.PostSyncCommandsData{
		Commands: r.PostSyncCommands,
	}
	if data.Commands == nil {
		data.Commands = []types.PostSyncCommand{}
	}

	if isJSON() {
		outputJSON(data, nil)
	} else {
		if len(data.Commands) == 0 {
			outputText("No post-sync commands for %q", name)
			return nil
		}
		outputText("Post-sync commands for %q:", name)
		for _, c := range data.Commands {
			outputText("  [%s] %s: %s", c.ID, c.Name, c.Cmd)
		}
	}
	return nil
}

func runPostSyncAdd(cmd *cobra.Command, args []string) error {
	store, err := loadRepoStore()
	if err != nil {
		if isJSON() {
			outputJSON(types.PostSyncCommandsData{}, err)
			return nil
		}
		return err
	}

	name := args[0]
	r, ok := store.GetByName(name)
	if !ok {
		err := fmt.Errorf("repo %q not found", name)
		if isJSON() {
			outputJSON(types.PostSyncCommandsData{}, err)
			return nil
		}
		return err
	}

	newCmd := types.PostSyncCommand{
		ID:   uuid.New().String(),
		Name: postSyncName,
		Cmd:  postSyncCmd,
	}
	r.PostSyncCommands = append(r.PostSyncCommands, newCmd)

	if err := store.Update(r); err != nil {
		if isJSON() {
			outputJSON(types.PostSyncCommandsData{}, err)
			return nil
		}
		return fmt.Errorf("update repo: %w", err)
	}

	data := types.PostSyncCommandsData{
		Commands: r.PostSyncCommands,
	}

	if isJSON() {
		outputJSON(data, nil)
	} else {
		outputText("Added post-sync command to %q: %s (%s)", name, newCmd.Name, newCmd.Cmd)
	}
	return nil
}

func runPostSyncRemove(cmd *cobra.Command, args []string) error {
	store, err := loadRepoStore()
	if err != nil {
		if isJSON() {
			outputJSON(types.PostSyncCommandsData{}, err)
			return nil
		}
		return err
	}

	name := args[0]
	r, ok := store.GetByName(name)
	if !ok {
		err := fmt.Errorf("repo %q not found", name)
		if isJSON() {
			outputJSON(types.PostSyncCommandsData{}, err)
			return nil
		}
		return err
	}

	found := false
	filtered := make([]types.PostSyncCommand, 0, len(r.PostSyncCommands))
	for _, c := range r.PostSyncCommands {
		if c.ID == postSyncID {
			found = true
			continue
		}
		filtered = append(filtered, c)
	}

	if !found {
		err := fmt.Errorf("post-sync command with ID %q not found", postSyncID)
		if isJSON() {
			outputJSON(types.PostSyncCommandsData{}, err)
			return nil
		}
		return err
	}

	r.PostSyncCommands = filtered
	if err := store.Update(r); err != nil {
		if isJSON() {
			outputJSON(types.PostSyncCommandsData{}, err)
			return nil
		}
		return fmt.Errorf("update repo: %w", err)
	}

	data := types.PostSyncCommandsData{
		Commands: r.PostSyncCommands,
	}

	if isJSON() {
		outputJSON(data, nil)
	} else {
		outputText("Removed post-sync command from %q", name)
	}
	return nil
}
