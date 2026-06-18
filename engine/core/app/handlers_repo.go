package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loongxjin/forksync/engine/core/agent"
	"github.com/loongxjin/forksync/engine/core/config"
	"github.com/loongxjin/forksync/engine/core/git"
	"github.com/loongxjin/forksync/engine/core/github"
	"github.com/loongxjin/forksync/engine/core/history"
	"github.com/loongxjin/forksync/engine/core/logger"
	syncpkg "github.com/loongxjin/forksync/engine/core/sync"
	"github.com/loongxjin/forksync/engine/pkg/types"
)

// registerRepoRoutes wires status / scan / add / remove / diff endpoints.
func (s *Server) registerRepoRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("POST /scan", s.handleScan)
	mux.HandleFunc("POST /repos", s.handleAddRepo)
	mux.HandleFunc("DELETE /repos/{name}", s.handleRemoveRepo)
	mux.HandleFunc("GET /repos/{name}/diff", s.handleRepoDiff)
}

// statusTimeout is the per-repo timeout for status operations (fetch + rev-list).
// Mirrors cmd/status.go statusTimeout.
const statusTimeout = 30 * time.Second

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	exclude := splitCSV(r.URL.Query().Get("exclude"))

	ctx, cancel := context.WithTimeout(r.Context(), statusTimeout)
	defer cancel()

	repos, err := s.deps.Store.List()
	if err != nil {
		writeErr[types.StatusData](w, fmt.Errorf("list repos: %w", err))
		return
	}

	refresher := syncpkg.NewStatusRefresher(s.deps.GitOps, s.deps.rawStore, s.deps.Cfg)
	repos, err = refresher.RefreshAll(ctx, repos, exclude)
	if err != nil {
		writeErr[types.StatusData](w, fmt.Errorf("refresh status: %w", err))
		return
	}

	// Detect installed agents (same logic as cmd/status.go).
	registry := agent.NewRegistry("")
	agents := registry.Discover()
	preferredAgent := ""
	if len(agents) > 0 {
		preferredAgent = agents[0].Name
	}

	writeOK(w, types.StatusData{
		Repos:          repos,
		Agents:         agents,
		PreferredAgent: preferredAgent,
	})
}

type scanRequest struct {
	Dir string `json:"dir"`
}

// skipScanDirs mirrors cmd/scan.go.
var skipScanDirs = []string{"node_modules", "vendor"}

func isSkipDir(name string) bool {
	for _, d := range skipScanDirs {
		if name == d {
			return true
		}
	}
	return false
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[scanRequest](w, r)
	if !ok {
		return
	}
	dir := req.Dir

	info, err := os.Stat(dir)
	if err != nil {
		writeErr[types.ScanData](w, fmt.Errorf("directory not found: %w", err))
		return
	}
	if !info.IsDir() {
		writeErr[types.ScanData](w, fmt.Errorf("%s is not a directory", dir))
		return
	}

	ghToken := ""
	if s.deps.Cfg != nil {
		ghToken = s.deps.Cfg.GitHub.Token
	}
	ghClient := github.NewClient(ghToken)
	gitOps := s.deps.GitOps

	scannedRepos := make([]types.ScannedRepo, 0)
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			logger.Warn("scan: skip path", "path", path, "error", walkErr)
			return nil
		}
		name := d.Name()
		if d.IsDir() && (len(name) == 0 || name[0] == '.' || isSkipDir(name)) {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			return nil
		}
		if !gitOps.IsGitRepo(r.Context(), path) {
			return nil
		}
		if scanned := scanRepo(r.Context(), path, gitOps, ghClient); scanned != nil {
			scannedRepos = append(scannedRepos, *scanned)
		}
		return filepath.SkipDir
	})
	if err != nil {
		writeErr[types.ScanData](w, fmt.Errorf("scan error: %w", err))
		return
	}

	writeOK(w, types.ScanData{Repos: scannedRepos})
}

// scanRepo is the per-directory scan body, ported verbatim from
// cmd/scan.go processScannedRepo.
func scanRepo(ctx context.Context, dir string, gitOps git.OperationsProvider, ghClient *github.Client) *types.ScannedRepo {
	remotes, _ := gitOps.GetRemotes(ctx, dir)
	originURL := findRemoteURL(remotes, "origin")

	scanned := types.ScannedRepo{
		Path:   dir,
		Name:   filepath.Base(dir),
		Origin: originURL,
	}

	if originURL != "" && github.IsGitHubURL(originURL) {
		owner, repoName, parseErr := github.ParseRepoURL(originURL)
		if parseErr == nil {
			result, detectErr := ghClient.DetectFork(ctx, owner, repoName)
			if detectErr == nil {
				scanned.IsFork = result.IsFork
				if result.UpstreamURL != "" {
					scanned.SuggestedUpstream = result.UpstreamURL
				}
			}
		}
	}

	scanned.LocalBranches, _ = gitOps.GetLocalBranches(ctx, dir)
	scanned.RemoteBranches, _ = gitOps.GetRemoteBranches(ctx, dir, "origin")

	for _, r := range remotes {
		if r.Name == "upstream" {
			upstreamBranches, _ := gitOps.GetRemoteBranches(ctx, dir, "upstream")
			branchMap := make(map[string]struct{})
			for _, b := range scanned.RemoteBranches {
				branchMap[b] = struct{}{}
			}
			for _, b := range upstreamBranches {
				if _, ok := branchMap[b]; !ok {
					scanned.RemoteBranches = append(scanned.RemoteBranches, b)
					branchMap[b] = struct{}{}
				}
			}
			break
		}
	}

	return &scanned
}

// findRemoteURL returns the URL of the named remote (cmd/scan.go helper).
func findRemoteURL(remotes []git.RemoteInfo, name string) string {
	for _, r := range remotes {
		if r.Name == name {
			return r.URL
		}
	}
	return ""
}

type addRepoRequest struct {
	Path          string               `json:"path"`
	Upstream      string               `json:"upstream,omitempty"`
	BranchMapping *types.BranchMapping `json:"branchMapping,omitempty"`
}

func (s *Server) handleAddRepo(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[addRepoRequest](w, r)
	if !ok {
		return
	}

	repoPath, err := filepath.Abs(req.Path)
	if err != nil {
		writeErr[types.AddData](w, fmt.Errorf("invalid path: %w", err))
		return
	}

	if !s.deps.GitOps.IsGitRepo(r.Context(), repoPath) {
		writeErr[types.AddData](w, fmt.Errorf("%s is not a git repository", repoPath))
		return
	}

	originURL := s.deps.GitOps.FindRemoteURL(r.Context(), repoPath, "origin")

	resolvedUpstream := req.Upstream
	if resolvedUpstream == "" {
		resolvedUpstream = autoDetectUpstream(r.Context(), s.deps.Cfg, originURL)
	}

	statusResult, statusErr := s.deps.GitOps.Status(r.Context(), types.Repo{Path: repoPath, Upstream: resolvedUpstream})
	if statusErr != nil {
		logger.Warn("add: failed to get status", "repo", repoPath, "error", statusErr)
	}
	branch := types.DefaultBranch
	if statusResult != nil && statusResult.Branch != "" {
		branch = statusResult.Branch
	}

	var branchMapping *types.BranchMapping
	if req.BranchMapping != nil && req.BranchMapping.LocalBranch != "" && req.BranchMapping.RemoteBranch != "" {
		bm := *req.BranchMapping
		branchMapping = &bm
	}

	repoName := filepath.Base(repoPath)
	newRepo := types.Repo{
		Name:          repoName,
		Path:          repoPath,
		Origin:        originURL,
		Upstream:      resolvedUpstream,
		Branch:        branch,
		BranchMapping: branchMapping,
		Status:        types.RepoStatusUnconfigured,
	}

	if err := s.deps.Store.Add(newRepo); err != nil {
		writeErr[types.AddData](w, fmt.Errorf("add repo: %w", err))
		return
	}

	saved, ok := s.deps.Store.GetByName(repoName)
	if !ok {
		writeErr[types.AddData](w, fmt.Errorf("add: failed to retrieve saved repo %q", repoName))
		return
	}

	writeOK(w, types.AddData{Repo: saved})
}

// autoDetectUpstream mirrors cmd/add.go autoDetectUpstream.
func autoDetectUpstream(ctx context.Context, cfg *config.Config, originURL string) string {
	if originURL == "" || !github.IsGitHubURL(originURL) {
		return ""
	}
	ghToken := ""
	if cfg != nil {
		ghToken = cfg.GitHub.Token
	}
	ghClient := github.NewClient(ghToken)
	owner, repoName, parseErr := github.ParseRepoURL(originURL)
	if parseErr != nil {
		return ""
	}
	result, detectErr := ghClient.DetectFork(ctx, owner, repoName)
	if detectErr != nil {
		return ""
	}
	if result.IsFork && result.UpstreamURL != "" {
		return result.UpstreamURL
	}
	return ""
}

// removeRepoResult mirrors the anonymous {removed string} from cmd/remove.go.
type removeRepoResult struct {
	Removed string `json:"removed"`
}

func (s *Server) handleRemoveRepo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	r2, ok := s.deps.Store.GetByName(name)
	if !ok {
		writeErr[removeRepoResult](w, fmt.Errorf("repository %q not found", name))
		return
	}

	if err := s.deps.Store.Remove(r2.ID); err != nil {
		writeErr[removeRepoResult](w, fmt.Errorf("remove repo: %w", err))
		return
	}

	// Clean up associated sync history records (best-effort).
	// Reuse the shared history store; only open a one-shot store if boot init
	// failed. Opening a fresh connection here races with concurrent syncer
	// writes on the same DB (SQLITE_BUSY).
	hStore := s.deps.HistStore
	closeAfter := false
	if hStore == nil {
		hs, err := history.NewStore(s.deps.ConfigDir())
		if err == nil {
			hStore = hs
			closeAfter = true
		}
	}
	if hStore != nil {
		if _, clearErr := hStore.ClearByRepo(r2.ID); clearErr != nil {
			logger.Warn("remove: failed to clear history", "repo", r2.Name, "error", clearErr)
		}
		if closeAfter {
			hStore.Close()
		}
	}

	writeOK(w, removeRepoResult{Removed: r2.Name})
}

// repoDiffResult mirrors the {success,diff?,error?} shape from engine.ts repoDiff.
type repoDiffResult struct {
	Success bool   `json:"success"`
	Diff    string `json:"diff,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) handleRepoDiff(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	r2, ok := s.deps.Store.GetByName(name)
	if !ok {
		writeBare(w, repoDiffResult{Success: false, Error: fmt.Sprintf("repo %q not found", name)})
		return
	}

	out, err := s.deps.GitOps.Diff(r.Context(), r2.Path)
	if err != nil {
		writeBare(w, repoDiffResult{Success: false, Error: err.Error()})
		return
	}
	writeBare(w, repoDiffResult{Success: true, Diff: string(out)})
}

// splitCSV parses a comma-separated query value into a slice, trimming spaces
// and dropping empties.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
