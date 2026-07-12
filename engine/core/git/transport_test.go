package git

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsSocksProxy verifies detection of socks proxy schemes. go-git's native
// ProxyOptions only understands HTTP CONNECT proxies; socks5 must be handled by
// injecting a custom transport with a socks5 dialer.
func TestIsSocksProxy(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"socks5://127.0.0.1:1080", true},
		{"socks5h://127.0.0.1:1080", true},
		{"socks4://127.0.0.1:1080", true},
		{"http://127.0.0.1:7890", false},
		{"https://proxy.example.com:443", false},
		{"", false},
		{"not a url", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.want, isSocksProxy(tt.url))
		})
	}
}

// TestParseSocksAddress verifies the socks5 host:port parsing used to build
// the dialer. It must accept the full proxy URL and yield address suitable for
// proxy.SOCKS5.
func TestParseSocksAddress(t *testing.T) {
	addr, err := parseSocksAddress("socks5://127.0.0.1:1080")
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:1080", addr)

	// socks5h (hostname resolved by proxy) parses the same way.
	addr, err = parseSocksAddress("socks5h://example.com:1080")
	require.NoError(t, err)
	assert.Equal(t, "example.com:1080", addr)
}

// TestParseSocksAddressInvalid verifies malformed socks URLs are rejected.
func TestParseSocksAddressInvalid(t *testing.T) {
	_, err := parseSocksAddress("socks5://127.0.0.1") // no port
	assert.Error(t, err)

	_, err = parseSocksAddress("socks5://") // no host
	assert.Error(t, err)
}

// TestSocks5DialContextTimeout verifies the dialer respects a context
// deadline, so a dead/unreachable socks proxy does not hang fetch forever.
// This is the transport-level counterpart of the fetch sub-timeout.
func TestSocks5DialContextTimeout(t *testing.T) {
	// Dial into a black-hole port (nothing listening) with a short ctx. The
	// dialer must return within the deadline, not block for the OS default.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	// occupy the port so connect refuses fast, then close to free it.
	freeAddr := ln.Addr().String()
	_ = ln.Close()

	d, err := newSocks5Dialer("socks5://" + freeAddr)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, derr := d.DialContext(ctx, "tcp", "github.com:443")
	elapsed := time.Since(start)

	assert.Error(t, derr, "dial to dead proxy must fail")
	assert.Less(t, elapsed, 2*time.Second, "must respect the context deadline")
}
