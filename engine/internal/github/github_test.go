package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	t.Run("without token", func(t *testing.T) {
		client := NewClient("")
		assert.Equal(t, "", client.token)
		assert.Equal(t, "https://api.github.com", client.baseURL)
		assert.NotNil(t, client.httpClient)
		assert.Equal(t, 30*time.Second, client.httpClient.Timeout)
	})

	t.Run("with token", func(t *testing.T) {
		client := NewClient("ghp_test123")
		assert.Equal(t, "ghp_test123", client.token)
		assert.Equal(t, "https://api.github.com", client.baseURL)
	})
}

func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "HTTPS with .git suffix",
			input:     "https://github.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "HTTPS without .git suffix",
			input:     "https://github.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "SSH format with .git suffix",
			input:     "git@github.com:owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "SSH format without .git suffix",
			input:     "git@github.com:owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "nested org name",
			input:     "https://github.com/my-org/my-project.git",
			wantOwner: "my-org",
			wantRepo:  "my-project",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid SSH URL missing colon",
			input:   "git@github.com/owner/repo.git",
			wantErr: true,
		},
		{
			name:    "just a hostname",
			input:   "https://github.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := ParseRepoURL(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
		})
	}
}

func TestIsGitHubURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "HTTPS GitHub URL",
			input: "https://github.com/owner/repo.git",
			want:  true,
		},
		{
			name:  "SSH GitHub URL",
			input: "git@github.com:owner/repo.git",
			want:  true,
		},
		{
			name:  "Gitee URL",
			input: "https://gitee.com/owner/repo.git",
			want:  false,
		},
		{
			name:  "GitLab URL",
			input: "https://gitlab.com/owner/repo.git",
			want:  false,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsGitHubURL(tt.input))
		})
	}
}

func TestDetectFork(t *testing.T) {
	t.Run("fork repo with source", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/repos/user/fork-repo", r.URL.Path)
			assert.Equal(t, "application/vnd.github.v3+json", r.Header.Get("Accept"))

			resp := RepoInfo{
				FullName: "user/fork-repo",
				CloneURL: "https://github.com/user/fork-repo.git",
				Fork:     true,
				Source: &SourceInfo{
					FullName: "original/repo",
					CloneURL: "https://github.com/original/repo.git",
					HTMLURL:  "https://github.com/original/repo",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewClient("")
		client.baseURL = server.URL
		client.httpClient = server.Client()

		result, err := client.DetectFork(context.Background(), "user", "fork-repo")
		require.NoError(t, err)
		assert.True(t, result.IsFork)
		assert.Equal(t, "https://github.com/original/repo.git", result.UpstreamURL)
		assert.Equal(t, "original/repo", result.UpstreamFullName)
	})

	t.Run("fork repo with parent but no source", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := RepoInfo{
				FullName: "user/fork-repo",
				CloneURL: "https://github.com/user/fork-repo.git",
				Fork:     true,
				Parent: &SourceInfo{
					FullName: "parent/repo",
					CloneURL: "https://github.com/parent/repo.git",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewClient("")
		client.baseURL = server.URL
		client.httpClient = server.Client()

		result, err := client.DetectFork(context.Background(), "user", "fork-repo")
		require.NoError(t, err)
		assert.True(t, result.IsFork)
		assert.Equal(t, "https://github.com/parent/repo.git", result.UpstreamURL)
		assert.Equal(t, "parent/repo", result.UpstreamFullName)
	})

	t.Run("non-fork repo", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := RepoInfo{
				FullName: "golang/go",
				CloneURL: "https://github.com/golang/go.git",
				Fork:     false,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewClient("")
		client.baseURL = server.URL
		client.httpClient = server.Client()

		result, err := client.DetectFork(context.Background(), "golang", "go")
		require.NoError(t, err)
		assert.False(t, result.IsFork)
		assert.Empty(t, result.UpstreamURL)
		assert.Empty(t, result.UpstreamFullName)
	})

	t.Run("404 not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient("")
		client.baseURL = server.URL
		client.httpClient = server.Client()

		result, err := client.DetectFork(context.Background(), "nonexistent", "repo")
		require.NoError(t, err)
		assert.False(t, result.IsFork)
	})

	t.Run("500 server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := NewClient("")
		client.baseURL = server.URL
		client.httpClient = server.Client()

		result, err := client.DetectFork(context.Background(), "owner", "repo")
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "status 500")
	})

	t.Run("with token sets authorization header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer ghp_secret123", r.Header.Get("Authorization"))

			resp := RepoInfo{
				FullName: "user/repo",
				CloneURL: "https://github.com/user/repo.git",
				Fork:     false,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewClient("ghp_secret123")
		client.baseURL = server.URL
		client.httpClient = server.Client()

		result, err := client.DetectFork(context.Background(), "user", "repo")
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use select to respect context from the client side.
			// The server itself doesn't see the client's context,
			// but we keep the delay short enough for fast tests.
			time.Sleep(500 * time.Millisecond)
		}))
		defer server.Close()

		client := NewClient("")
		client.baseURL = server.URL
		client.httpClient = server.Client()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		result, err := client.DetectFork(ctx, "owner", "repo")
		elapsed := time.Since(start)

		assert.Error(t, err)
		assert.Nil(t, result)
		// Verify the call returned quickly (within 200ms), not after the full 500ms sleep
		assert.Less(t, elapsed, 200*time.Millisecond, "context cancellation should abort quickly")
		assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	})
}
