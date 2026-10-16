package netutil

import (
	"context"
	"errors"
	"fmt"
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

// blockedNetworks lists address ranges that must never be reached from
// server-side fetches: loopback, private, link-local (incl. cloud metadata at
// 169.254.169.254), CGNAT, NAT64, documentation, benchmarking, multicast and
// reserved ranges.
var blockedNetworks = mustParseCIDRs(
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"::/128",
	"::1/128",
	"64:ff9b::/96",
	"64:ff9b:1::/48",
	"100::/64",
	"2001:db8::/32",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
)

func mustParseCIDRs(cidrs ...string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		nets = append(nets, n)
	}
	return nets
}

// IsBlockedIP reports whether ip belongs to a range that server-side fetches
// must not reach. IPv4-mapped IPv6 addresses are checked as IPv4.
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, n := range blockedNetworks {
		if n.Contains(ip) {
			return true
		}
	}
	return false
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

			if len(ips) == 0 {
				return nil, fmt.Errorf("no IP addresses found for %s", host)
			}

			allowLocal, _ := ctx.Value(AllowLocalKey).(bool)

			if !allowLocal {
				for _, ip := range ips {
					if IsBlockedIP(ip.IP) {
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
