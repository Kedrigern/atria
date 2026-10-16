package netutil

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1",
		"169.254.169.254", // cloud metadata
		"100.64.0.1",      // CGNAT
		"0.0.0.0", "224.0.0.1", "255.255.255.255",
		"::1", "::", "fe80::1", "fd00::1", "ff02::1",
		"64:ff9b::a9fe:a9fe", // NAT64 of 169.254.169.254
		"::ffff:10.0.0.1",    // IPv4-mapped private address
	}
	for _, s := range blocked {
		if !IsBlockedIP(net.ParseIP(s)) {
			t.Errorf("expected %s to be blocked", s)
		}
	}

	allowed := []string{"1.1.1.1", "8.8.8.8", "140.82.112.3", "2606:4700:4700::1111"}
	for _, s := range allowed {
		if IsBlockedIP(net.ParseIP(s)) {
			t.Errorf("expected %s to be allowed", s)
		}
	}
}

func TestSafeHTTPClientBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	client := SafeHTTPClient()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	if _, err := client.Do(req); err == nil {
		t.Fatal("expected request to loopback to be blocked")
	}

	ctx := context.WithValue(context.Background(), AllowLocalKey, true)
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("expected request with AllowLocalKey to succeed: %v", err)
	}
	resp.Body.Close()
}
