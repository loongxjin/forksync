package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// IDEInfo mirrors frontend/src/shared/types/ide.ts IDEInfo.
type IDEInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CLIICommand string `json:"cliCommand"`
	AppName     string `json:"appName"`
	Installed   bool   `json:"installed"`
	OpenMethod  string `json:"openMethod"`
	IsCustom    bool   `json:"isCustom,omitempty"`
}

type CustomIDE struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CLIICommand string `json:"cliCommand"`
}

type IDEConfig struct {
	DefaultIDE   string      `json:"defaultIDE"`
	DetectedIDEs []IDEInfo   `json:"detectedIDEs"`
	CustomIDEs   []CustomIDE `json:"customIDEs"`
}

type IDEOpenResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

var builtinIDEs = []struct {
	ID, Name, CLI, AppName string
}{
	{"vscode", "VS Code", "code", "Visual Studio Code"},
	{"cursor", "Cursor", "cursor", "Cursor"},
	{"trae", "Trae", "trae", "Trae"},
}

const ideConfigPath = ".forksync/ide-config.json"

type persistedIDEConfig struct {
	DefaultIDE string      `json:"defaultIDE"`
	CustomIDEs []CustomIDE `json:"customIDEs"`
}

func ideConfigFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ideConfigPath)
}

func loadIDEConfig() persistedIDEConfig {
	data, err := os.ReadFile(ideConfigFile())
	if err != nil {
		return persistedIDEConfig{CustomIDEs: []CustomIDE{}}
	}
	var c persistedIDEConfig
	if json.Unmarshal(data, &c) != nil {
		return persistedIDEConfig{CustomIDEs: []CustomIDE{}}
	}
	if c.CustomIDEs == nil {
		c.CustomIDEs = []CustomIDE{}
	}
	return c
}

func saveIDEConfig(c persistedIDEConfig) {
	dir := filepath.Dir(ideConfigFile())
	os.MkdirAll(dir, 0o755)
	data, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(ideConfigFile(), data, 0o644)
}

func whichCommand() string {
	if goruntime.GOOS == "windows" {
		return "where"
	}
	return "which"
}

func execWithTimeout(cmd string, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, cmd, args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func detectSingleIDE(id, name, cli, appName string) IDEInfo {
	// Try CLI
	if _, err := execWithTimeout(whichCommand(), []string{cli}); err == nil {
		return IDEInfo{ID: id, Name: name, CLIICommand: cli, AppName: appName, Installed: true, OpenMethod: "cli"}
	}
	// Platform app detection
	switch goruntime.GOOS {
	case "darwin":
		if _, err := os.Stat(fmt.Sprintf("/Applications/%s.app", appName)); err == nil {
			return IDEInfo{ID: id, Name: name, CLIICommand: cli, AppName: appName, Installed: true, OpenMethod: "app"}
		}
	case "linux":
		snap := fmt.Sprintf("/snap/bin/%s", cli)
		if _, err := os.Stat(snap); err == nil {
			return IDEInfo{ID: id, Name: name, CLIICommand: snap, AppName: appName, Installed: true, OpenMethod: "cli"}
		}
	case "windows":
		localApp := os.Getenv("LOCALAPPDATA")
		progFiles := os.Getenv("PROGRAMFILES")
		paths := []string{}
		switch id {
		case "vscode":
			paths = []string{
				filepath.Join(localApp, "Programs", "Microsoft VS Code", "bin", "code.cmd"),
				filepath.Join(progFiles, "Microsoft VS Code", "bin", "code.cmd"),
			}
		case "cursor":
			paths = []string{
				filepath.Join(localApp, "Programs", "cursor", "Cursor.exe"),
				filepath.Join(progFiles, "Cursor", "Cursor.exe"),
			}
		case "trae":
			paths = []string{
				filepath.Join(localApp, "Programs", "Trae", "Trae.exe"),
				filepath.Join(progFiles, "Trae", "Trae.exe"),
			}
		}
		for _, p := range paths {
			if p != "" {
				if _, err := os.Stat(p); err == nil {
					return IDEInfo{ID: id, Name: name, CLIICommand: p, AppName: appName, Installed: true, OpenMethod: "cli"}
				}
			}
		}
	}
	return IDEInfo{ID: id, Name: name, CLIICommand: cli, AppName: appName, Installed: false, OpenMethod: "cli"}
}

// IDEDetect returns all detected IDEs (builtin + custom).
func (a *App) IDEDetect() ([]IDEInfo, error) {
	cfg := loadIDEConfig()
	result := make([]IDEInfo, 0, len(builtinIDEs)+len(cfg.CustomIDEs))
	for _, ide := range builtinIDEs {
		result = append(result, detectSingleIDE(ide.ID, ide.Name, ide.CLI, ide.AppName))
	}
	for _, c := range cfg.CustomIDEs {
		_, err := execWithTimeout(whichCommand(), []string{c.CLIICommand})
		result = append(result, IDEInfo{
			ID: c.ID, Name: c.Name, CLIICommand: c.CLIICommand, AppName: c.Name,
			Installed: err == nil, OpenMethod: "cli", IsCustom: true,
		})
	}
	return result, nil
}

// IDEGetConfig returns the IDE configuration.
func (a *App) IDEGetConfig() (IDEConfig, error) {
	ides, _ := a.IDEDetect()
	cfg := loadIDEConfig()
	def := cfg.DefaultIDE
	if def != "" {
		found := false
		for _, i := range ides {
			if i.ID == def && i.Installed {
				found = true
				break
			}
		}
		if !found {
			def = ""
			cfg.DefaultIDE = ""
			saveIDEConfig(cfg)
		}
	}
	return IDEConfig{DefaultIDE: def, DetectedIDEs: ides, CustomIDEs: cfg.CustomIDEs}, nil
}

// IDESetDefault sets the default IDE.
func (a *App) IDESetDefault(ideID string) (map[string]bool, error) {
	cfg := loadIDEConfig()
	cfg.DefaultIDE = ideID
	saveIDEConfig(cfg)
	return map[string]bool{"success": true}, nil
}

// IDEAddCustom adds a custom IDE.
func (a *App) IDEAddCustom(name, cliCommand string) (map[string]interface{}, error) {
	cfg := loadIDEConfig()
	cfg.CustomIDEs = append(cfg.CustomIDEs, CustomIDE{
		ID: fmt.Sprintf("custom-%d", time.Now().UnixNano()), Name: name, CLIICommand: cliCommand,
	})
	saveIDEConfig(cfg)
	return map[string]interface{}{"success": true}, nil
}

// IDERemoveCustom removes a custom IDE.
func (a *App) IDERemoveCustom(ideID string) (map[string]bool, error) {
	cfg := loadIDEConfig()
	filtered := make([]CustomIDE, 0, len(cfg.CustomIDEs))
	for _, c := range cfg.CustomIDEs {
		if c.ID != ideID {
			filtered = append(filtered, c)
		}
	}
	cfg.CustomIDEs = filtered
	if cfg.DefaultIDE == ideID {
		cfg.DefaultIDE = ""
	}
	saveIDEConfig(cfg)
	return map[string]bool{"success": true}, nil
}

// IDEOpen opens a repo path in the specified IDE.
func (a *App) IDEOpen(repoPath, ideID string) (IDEOpenResult, error) {
	if _, err := os.Stat(repoPath); err != nil {
		return IDEOpenResult{Success: false, Error: fmt.Sprintf("path does not exist: %s", repoPath)}, nil
	}
	ides, _ := a.IDEDetect()
	cfg := loadIDEConfig()
	resolvedID := ideID
	if ideID == "default" || ideID == "" {
		if cfg.DefaultIDE != "" {
			resolvedID = cfg.DefaultIDE
		} else {
			for _, i := range ides {
				if i.Installed {
					resolvedID = i.ID
					break
				}
			}
		}
	}
	var ide *IDEInfo
	for i := range ides {
		if ides[i].ID == resolvedID {
			ide = &ides[i]
			break
		}
	}
	if ide == nil {
		return IDEOpenResult{Success: false, Error: fmt.Sprintf("IDE %q not found", resolvedID)}, nil
	}
	if !ide.Installed {
		return IDEOpenResult{Success: false, Error: fmt.Sprintf("%s is not installed", ide.Name)}, nil
	}
	// Launch in background
	var cmd *exec.Cmd
	switch {
	case ide.OpenMethod == "cli":
		cmd = exec.Command(ide.CLIICommand, repoPath)
	case goruntime.GOOS == "darwin":
		cmd = exec.Command("open", "-a", ide.AppName, repoPath)
	case goruntime.GOOS == "windows":
		cmd = exec.Command("powershell", "-Command", "Start-Process", ide.AppName, repoPath)
	default:
		return IDEOpenResult{Success: false, Error: "unsupported open method"}, nil
	}
	if err := cmd.Start(); err != nil {
		return IDEOpenResult{Success: false, Error: err.Error()}, nil
	}
	cmd.Process.Release()
	return IDEOpenResult{Success: true}, nil
}

// OpenDirectoryDialog shows a native directory picker.
func (a *App) OpenDirectoryDialog() (map[string]interface{}, error) {
	dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select Repository Directory",
	})
	if err != nil {
		return map[string]interface{}{"canceled": true, "error": err.Error()}, nil
	}
	if dir == "" {
		return map[string]interface{}{"canceled": true}, nil
	}
	return map[string]interface{}{"canceled": false, "filePaths": []string{dir}}, nil
}

// IsGitRepo checks if a directory is a git repository.
func (a *App) IsGitRepo(dirPath string) (bool, error) {
	if a.deps == nil {
		return false, nil
	}
	return a.deps.GitOps.IsGitRepo(a.ctx, dirPath), nil
}

// SetLocale persists the UI locale preference.
func (a *App) SetLocale(locale string) (map[string]bool, error) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".forksync", "locale.txt")
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(locale), 0o644); err != nil {
		return map[string]bool{"success": false}, nil
	}
	return map[string]bool{"success": true}, nil
}

// SetAutoLaunch enables or disables launch on system startup.
func (a *App) SetAutoLaunch(enabled bool) (map[string]interface{}, error) {
	switch goruntime.GOOS {
	case "linux":
		return setAutoLaunchLinux(enabled)
	case "darwin":
		return setAutoLaunchDarwin(enabled)
	default:
		return map[string]interface{}{"success": true}, nil
	}
}

func setAutoLaunchLinux(enabled bool) (map[string]interface{}, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, _ := os.UserHomeDir()
		configHome = filepath.Join(home, ".config")
	}
	autoStartDir := filepath.Join(configHome, "autostart")
	desktopFile := filepath.Join(autoStartDir, "forksync.desktop")
	if enabled {
		os.MkdirAll(autoStartDir, 0o755)
		exe, _ := os.Executable()
		content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=ForkSync
Comment=Fork Repository Sync Tool
Exec=%s
Icon=forksync
Categories=Development;
Terminal=false
Hidden=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
`, exe)
		os.WriteFile(desktopFile, []byte(content), 0o644)
	} else {
		os.Remove(desktopFile)
	}
	return map[string]interface{}{"success": true}, nil
}

func setAutoLaunchDarwin(enabled bool) (map[string]interface{}, error) {
	home, _ := os.UserHomeDir()
	launchDir := filepath.Join(home, "Library", "LaunchAgents")
	plistFile := filepath.Join(launchDir, "com.forksync.app.plist")
	if enabled {
		os.MkdirAll(launchDir, 0o755)
		exe, _ := os.Executable()
		content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.forksync.app</string>
<key>ProgramArguments</key><array><string>%s</string></array>
<key>RunAtLoad</key><true/>
</dict></plist>`, exe)
		os.WriteFile(plistFile, []byte(content), 0o644)
	} else {
		os.Remove(plistFile)
	}
	return map[string]interface{}{"success": true}, nil
}
