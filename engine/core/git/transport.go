package git

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/loongxjin/forksync/engine/core/logger"
	"golang.org/x/net/proxy"
)

// initOnce guards the one-time go-git protocol-transport install. go-git's
// client.InstallProtocol mutates a global registry, so the socks5 transport
// must be installed exactly once per process.
var initOnce sync.Once

// InitTransport configures go-git's HTTP(S) transport for the given proxy URL.
//
// go-git's native ProxyOptions only understands HTTP CONNECT proxies: handing
// it a socks5:// URL makes go-git attempt an HTTP CONNECT handshake against a
// SOCKS server, which closes the connection ("fetch: ... EOF"). To support
// socks5 we install a custom http.Transport whose DialContext routes through
// a socks5 dialer (golang.org/x/net/proxy). http/https proxies are left to
// go-git's native handling, which already works.
//
// This only affects the go-git (in-process) path; the CLI fallback path sets
// HTTP_PROXY/HTTPS_PROXY env vars and works for both schemes via libcurl.
func InitTransport(proxyURL string) {
	if !isSocksProxy(proxyURL) {
		// http(s) proxy or none: go-git's native ProxyOptions handles it
		// (see Operations.proxyOptions / FetchOptions).
		return
	}
	initOnce.Do(func() {
		dialer, err := newSocks5Dialer(proxyURL)
		if err != nil {
			logger.Warn("git: failed to build socks5 dialer, go-git will fall back to CLI", "proxy", proxyURL, "error", err)
			return
		}
		transport := &http.Transport{
			DialContext:       dialer.DialContext,
			ForceAttemptHTTP2: true,
		}
		httpClient := &http.Client{Transport: transport}
		// Install for both http and https endpoints.
		client.InstallProtocol("http", githttp.NewClient(httpClient))
		client.InstallProtocol("https", githttp.NewClient(httpClient))
		logger.Info("git: installed socks5 transport for go-git", "proxy", proxyURL)
	})
}

// isSocksProxy reports whether the URL scheme is a SOCKS proxy that go-git's
// native ProxyOptions cannot handle.
func isSocksProxy(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	s := strings.ToLower(rawURL)
	return strings.HasPrefix(s, "socks5://") ||
		strings.HasPrefix(s, "socks5h://") ||
		strings.HasPrefix(s, "socks4://") ||
		strings.HasPrefix(s, "socks4a://")
}

// parseSocksAddress extracts the host:port from a socks proxy URL.
func parseSocksAddress(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse socks url: %w", err)
	}
	host := u.Hostname()
	port := u.Port()
	if host == "" || port == "" {
		return "", fmt.Errorf("socks url %q missing host or port", rawURL)
	}
	return net.JoinHostPort(host, port), nil
}

// newSocks5Dialer builds a socks5 dialer from a proxy URL. Supports socks5 and
// socks5h (hostname resolution delegated to the proxy). The returned dialer is
// context-aware so transport dials honour deadlines and cancellation.
func newSocks5Dialer(rawURL string) (proxy.ContextDialer, error) {
	addr, err := parseSocksAddress(rawURL)
	if err != nil {
		return nil, err
	}
	dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("create socks5 dialer for %s: %w", rawURL, err)
	}
	// proxy.SOCKS5 returns a dialer that also implements ContextDialer.
	cd, ok := dialer.(proxy.ContextDialer)
	if !ok {
		// Fallback: wrap the plain Dialer so DialContext still works.
		return &dialContextWrapper{d: dialer}, nil
	}
	return cd, nil
}

// dialContextWrapper adapts a proxy.Dialer (Dial only) to proxy.ContextDialer
// by ignoring the context — used only if the underlying dialer unexpectedly
// lacks DialContext. The socks5 dialer always has it, so this is defensive.
type dialContextWrapper struct{ d proxy.Dialer }

func (w *dialContextWrapper) Dial(network, addr string) (net.Conn, error) {
	return w.d.Dial(network, addr)
}
func (w *dialContextWrapper) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if ctx.Done() != nil {
		// Best-effort: spawn the dial and race it against ctx cancellation.
		type result struct {
			c   net.Conn
			err error
		}
		ch := make(chan result, 1)
		go func() {
			c, err := w.d.Dial(network, addr)
			ch <- result{c, err}
		}()
		select {
		case r := <-ch:
			return r.c, r.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return w.d.Dial(network, addr)
}
