package git

import (
	"testing"
	"time"

	"github.com/loongxjin/forksync/engine/pkg/types"
	"github.com/stretchr/testify/assert"
)

// TestFetchGateSuppressesRapidRefetch verifies that within the TTL window,
// repeated Fetch calls for the same repo do not re-fetch.
func TestFetchGateSuppressesRapidRefetch(t *testing.T) {
	o := NewOperationsWithTTL(60 * time.Second)

	// First call: not fetched recently → should fetch.
	assert.True(t, o.shouldFetch("/repo/a"), "first call should allow fetch")
	o.markFetched("/repo/a", time.Now())

	// Second call within TTL: should be suppressed.
	assert.False(t, o.shouldFetch("/repo/a"), "second call within TTL should be suppressed")
}

// TestFetchGateExpiresAfterTTL verifies that after the TTL elapses, fetch is
// allowed again.
func TestFetchGateExpiresAfterTTL(t *testing.T) {
	ttl := 60 * time.Second
	o := NewOperationsWithTTL(ttl)
	now := time.Now()

	// Seed a fetch in the past beyond the TTL window.
	o.markFetched("/repo/b", now.Add(-(ttl + time.Second)))

	assert.True(t, o.shouldFetch("/repo/b"), "call after TTL should allow fetch")
}

// TestFetchGateIsPerRepo verifies that the gate is keyed per-repo-path.
func TestFetchGateIsPerRepo(t *testing.T) {
	o := NewOperationsWithTTL(60 * time.Second)

	o.markFetched("/repo/a", time.Now())

	// /repo/a suppressed, /repo/c allowed.
	assert.False(t, o.shouldFetch("/repo/a"), "/repo/a should be suppressed")
	assert.True(t, o.shouldFetch("/repo/c"), "/repo/c should be allowed")
}

// TestFetchGateConcurrentSafe verifies that concurrent shouldFetch/markFetched
// calls do not race (guarded by go test -race).
func TestFetchGateConcurrentSafe(t *testing.T) {
	o := NewOperationsWithTTL(60 * time.Second)
	done := make(chan struct{})

	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_ = o.shouldFetch("/repo/x")
			o.markFetched("/repo/x", time.Now())
		}
	}()

	for i := 0; i < 200; i++ {
		_ = o.shouldFetch("/repo/y")
		o.markFetched("/repo/y", time.Now())
	}
	<-done
}

// TestFetchGateRepoKeyFromRepo verifies the helper that derives a cache key
// from a types.Repo (path is the natural identity).
func TestFetchGateRepoKeyFromRepo(t *testing.T) {
	r := types.Repo{Path: "/some/path", Name: "foo"}
	assert.Equal(t, "/some/path", fetchKey(r))
}
