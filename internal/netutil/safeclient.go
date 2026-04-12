package netutil

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

type contextKey string

const AllowLocalKey contextKey = "allow_local_ssrf"

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
		Transport: transport,
	}
}
