package mail

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

const (
	maxImageSize    = 10 << 20 // 10 MB
	cacheTTL        = 1 * time.Hour
	maxCacheEntries = 500
)

type cacheEntry struct {
	data        []byte
	contentType string
	expiresAt   time.Time
}

var (
	imageCache   = make(map[string]*cacheEntry)
	imageCacheMu sync.RWMutex
)

// proxyClient is hardened against SSRF: every redirect hop is re-validated
// against the private-address check, and the transport pins the dialed IP to
// one that passed validation so a DNS-rebinding host can't swap in a private
// address between the check and the connect.
var proxyClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		DialContext:         safeDialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout:     90 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return http.ErrUseLastResponse
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("redirect to unsupported scheme %q", req.URL.Scheme)
		}
		if isPrivateHost(req.URL.Hostname()) {
			return fmt.Errorf("redirect to private address %q blocked", req.URL.Hostname())
		}
		return nil
	},
}

// checkPrivateIP is the IP-level guard shared by the pre-flight host check,
// the redirect re-validation, and the dial-time pinning. Tests override it to
// allow loopback test servers.
var checkPrivateIP = isPrivateIP

// safeDialContext resolves the target host itself, rejects the connection if
// ANY resolved address is private, and dials one of the verified IPs directly
// — so the address actually connected to is the one that was validated.
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ipAddrs) == 0 {
		return nil, fmt.Errorf("no addresses found for %q", host)
	}
	for _, ipAddr := range ipAddrs {
		if checkPrivateIP(ipAddr.IP) {
			return nil, fmt.Errorf("refusing to connect to private address %s for host %q", ipAddr.IP, host)
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var lastErr error
	for _, ipAddr := range ipAddrs {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ipAddr.IP.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	return nil, lastErr
}

func handleImageProxyRequest(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	token := re.Request.URL.Query().Get("token")
	if token == "" {
		return re.UnauthorizedError("missing token", nil)
	}

	_, err := app.FindAuthRecordByToken(token)
	if err != nil {
		return re.UnauthorizedError("invalid token", nil)
	}

	handleImageProxy(re.Response, re.Request)
	return nil
}

func handleImageProxy(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		http.Error(w, "missing url parameter", http.StatusBadRequest)
		return
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		http.Error(w, "only http/https URLs allowed", http.StatusBadRequest)
		return
	}

	if isPrivateHost(parsed.Hostname()) {
		http.Error(w, "private addresses not allowed", http.StatusForbidden)
		return
	}

	// Check cache
	imageCacheMu.RLock()
	if entry, ok := imageCache[rawURL]; ok && time.Now().Before(entry.expiresAt) {
		imageCacheMu.RUnlock()
		w.Header().Set("Content-Type", entry.contentType)
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(entry.data)
		return
	}
	imageCacheMu.RUnlock()

	resp, err := proxyClient.Get(rawURL)
	if err != nil {
		http.Error(w, "failed to fetch image", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "upstream error", resp.StatusCode)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(w, "not an image", http.StatusBadRequest)
		return
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize+1))
	if err != nil {
		http.Error(w, "failed to read image", http.StatusBadGateway)
		return
	}
	if len(data) > maxImageSize {
		http.Error(w, "image too large", http.StatusBadRequest)
		return
	}

	// Store in cache
	imageCacheMu.Lock()
	if len(imageCache) >= maxCacheEntries {
		now := time.Now()
		for k, v := range imageCache {
			if now.After(v.expiresAt) {
				delete(imageCache, k)
			}
		}
		if len(imageCache) >= maxCacheEntries {
			for k := range imageCache {
				delete(imageCache, k)
				break
			}
		}
	}
	imageCache[rawURL] = &cacheEntry{
		data:        data,
		contentType: contentType,
		expiresAt:   time.Now().Add(cacheTTL),
	}
	imageCacheMu.Unlock()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(data)
}

// isPrivateHost reports whether host (an IP literal or DNS name) resolves to
// any private address. This pre-flight check gives a clean 403 for obvious
// cases; safeDialContext is the authoritative guard (it re-resolves and pins
// the dialed IP), so a lookup failure here is allowed through to fail at dial
// time instead.
func isPrivateHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return checkPrivateIP(ip)
	}
	addrs, err := net.LookupIP(host)
	if err != nil {
		return false
	}
	for _, ip := range addrs {
		if checkPrivateIP(ip) {
			return true
		}
	}
	return false
}

// cgnatRange is 100.64.0.0/10 (RFC 6598 carrier-grade NAT), which
// net.IP.IsPrivate does not cover.
var cgnatRange = func() *net.IPNet {
	_, ipNet, _ := net.ParseCIDR("100.64.0.0/10")
	return ipNet
}()

// isPrivateIP reports whether ip must never be fetched by the proxy:
// IPv4 loopback/RFC1918/link-local/CGNAT/unspecified/multicast and
// IPv6 loopback/link-local/ULA (fc00::/7, via IsPrivate)/unspecified.
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() ||
		cgnatRange.Contains(ip)
}
