package netutil

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const AllowLocalKey contextKey = "allow_local_ssrf"

// BrowserTransport mimic standard browser to bypass Cloudflare etc protection against bots
type BrowserTransport struct {
	Transport http.RoundTripper
}

func (t *BrowserTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqClone := req.Clone(req.Context())

	ua := reqClone.Header.Get("User-Agent")
	if ua == "" || strings.Contains(ua, "Go-http-client") || strings.Contains(ua, "Gofeed") {
		reqClone.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	}

	if reqClone.Header.Get("Accept") == "" {
		reqClone.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	}
	if reqClone.Header.Get("Accept-Language") == "" {
		reqClone.Header.Set("Accept-Language", "cs,en-US;q=0.7,en;q=0.3")
	}

	return t.Transport.RoundTrip(reqClone)
}

// SafeHTTPClient create client that block private and internals IPs.
func SafeHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}

			allowLocal, _ := ctx.Value(AllowLocalKey).(bool)

			if !allowLocal {
				for _, ip := range ips {
					if ip.IP.IsPrivate() || ip.IP.IsLoopback() || ip.IP.IsUnspecified() {
						return nil, errors.New("SSRF blocked: access to internal IP address is not allowed")
					}
				}
			}

			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}

	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: &BrowserTransport{Transport: transport},
	}
}
