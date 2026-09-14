package cloudmcp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 4 << 20

var blockedNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"),
}

func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range blockedNetworks {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func ValidateEndpoint(raw string) (string, error) {
	if len(raw) > 2048 {
		return "", errors.New("MCP endpoint is too long")
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || u.Opaque != "" || u.Scheme != "https" {
		return "", errors.New("cloud MCP requires an absolute HTTPS endpoint")
	}
	// Query credentials cannot be safely distinguished from routing parameters.
	if u.User != nil || u.Fragment != "" || u.RawQuery != "" || u.ForceQuery {
		return "", errors.New("MCP endpoint must not contain credentials, query parameters or fragments")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.Contains(host, "%") {
		return "", errors.New("cloud MCP cannot reach local addresses")
	}
	if ip, err := netip.ParseAddr(host); err == nil && !publicAddress(ip) {
		return "", errors.New("cloud MCP cannot reach non-public addresses")
	}
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

type publicDialer struct {
	lookup func(context.Context, string) ([]netip.Addr, error)
	dial   func(context.Context, string, string) (net.Conn, error)
}

func (d publicDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errors.New("invalid MCP network address")
	}
	ips, err := d.lookup(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, errors.New("MCP DNS lookup failed")
	}
	for _, ip := range ips {
		if !publicAddress(ip) {
			return nil, errors.New("MCP DNS resolved a non-public address")
		}
	}
	for _, ip := range ips {
		// Dial the validated address, never resolve the hostname a second time.
		conn, err := d.dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil, errors.New("MCP endpoint is unreachable")
}

type boundedBody struct {
	io.ReadCloser
	remaining int64
}

func (b *boundedBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, errors.New("MCP response exceeds size limit")
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	return n, err
}

type guardedTransport struct {
	base   http.RoundTripper
	origin string
	bearer string
}

func (t *guardedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, err := ValidateEndpoint(req.URL.String()); err != nil {
		return nil, err
	}
	if t.origin != "" && req.URL.Scheme+"://"+req.URL.Host != t.origin {
		return nil, errors.New("MCP cross-origin requests are forbidden")
	}
	out := req.Clone(req.Context())
	out.Header.Del("Authorization")
	out.Header.Del("Cookie")
	if t.bearer != "" {
		out.Header.Set("Authorization", "Bearer "+t.bearer)
	}
	res, err := t.base.RoundTrip(out)
	if err != nil {
		return nil, errors.New("MCP transport unavailable")
	}
	if res.ContentLength > maxResponseBytes {
		res.Body.Close()
		return nil, errors.New("MCP response exceeds size limit")
	}
	res.Body = &boundedBody{ReadCloser: res.Body, remaining: maxResponseBytes + 1}
	return res, nil
}

func newHTTPClient(endpoint, bearer string) (*http.Client, func()) {
	d := publicDialer{
		lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
		dial: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	}
	base := &http.Transport{Proxy: nil, DialContext: d.DialContext, TLSHandshakeTimeout: 10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second, MaxResponseHeaderBytes: 32 << 10, DisableCompression: true, DisableKeepAlives: true}
	u, _ := url.Parse(endpoint)
	origin := ""
	if u != nil && u.Host != "" {
		origin = u.Scheme + "://" + u.Host
	}
	client := &http.Client{Timeout: 45 * time.Second, Transport: &guardedTransport{base: base, origin: origin, bearer: bearer},
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("MCP redirects are forbidden") }}
	return client, base.CloseIdleConnections
}

// NewMetadataClient applies the same egress policy without any account token.
func NewMetadataClient() (*http.Client, func()) { return newHTTPClient("", "") }
