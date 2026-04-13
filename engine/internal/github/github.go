package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client provides GitHub API operations.
type Client struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new GitHub API client.
// token is optional (empty string for unauthenticated requests).
func NewClient(token string) *Client {
	baseURL := "https://api.github.com"
	return &Client{
		token:   token,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// RepoInfo represents the GitHub API response for a repository.
type RepoInfo struct {
	FullName    string      `json:"full_name"`
	CloneURL    string      `json:"clone_url"`
	HTMLURL     string      `json:"html_url"`
	Fork        bool        `json:"fork"`
	Source      *SourceInfo `json:"source"`
	Parent      *SourceInfo `json:"parent"`
	Description string      `json:"description"`
	Private     bool        `json:"private"`
}

// SourceInfo represents the source/parent repo info.
type SourceInfo struct {
	FullName string `json:"full_name"`
	CloneURL string `json:"clone_url"`
	HTMLURL  string `json:"html_url"`
}

// ForkResult contains fork detection results.
type ForkResult struct {
	IsFork           bool
	UpstreamURL      string
	UpstreamFullName string
}

// DetectFork checks if a repository is a fork and returns the upstream URL.
func (c *Client) DetectFork(ctx context.Context, owner, repo string) (*ForkResult, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &ForkResult{IsFork: false}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var info RepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	result := &ForkResult{IsFork: info.Fork}

	// Source is the ultimate source (for network forks)
	// Parent is the immediate parent
	if info.Source != nil {
		result.UpstreamURL = info.Source.CloneURL
		result.UpstreamFullName = info.Source.FullName
	} else if info.Parent != nil {
		result.UpstreamURL = info.Parent.CloneURL
		result.UpstreamFullName = info.Parent.FullName
	}

	return result, nil
}

// ParseRepoURL extracts owner and repo name from a git remote URL.
// Supports both HTTPS and SSH formats:
//   - https://github.com/owner/repo.git
//   - git@github.com:owner/repo.git
func ParseRepoURL(remoteURL string) (owner, repo string, err error) {
	url := remoteURL

	// Handle SSH format: git@github.com:owner/repo.git
	if strings.HasPrefix(url, "git@") {
		parts := strings.SplitN(url, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid ssh url: %s", url)
		}
		url = "https://github.com/" + parts[1]
	}

	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	// Extract owner/repo from path
	parts := strings.Split(strings.TrimPrefix(url, "https://"), "/")
	// parts should be ["github.com", "owner", "repo"] or ["owner", "repo"]
	if len(parts) >= 3 {
		return parts[1], parts[2], nil
	}
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}

	return "", "", fmt.Errorf("cannot parse repo url: %s", remoteURL)
}

// IsGitHubURL checks if a remote URL points to GitHub.
func IsGitHubURL(remoteURL string) bool {
	return strings.Contains(remoteURL, "github.com")
}
