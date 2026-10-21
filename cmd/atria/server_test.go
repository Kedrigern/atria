package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListenAddress(t *testing.T) {
	cases := []struct {
		name, envHost, envPort, flagHost, flagPort, want string
		wantErr                                          bool
	}{
		{name: "defaults: all interfaces", want: ":8080"},
		{name: "env port", envPort: "8089", want: ":8089"},
		{name: "env host", envHost: "127.0.0.1", envPort: "8089", want: "127.0.0.1:8089"},
		{name: "ipv6", envHost: "::1", envPort: "8089", want: "[::1]:8089"},
		{name: "localhost", envHost: "localhost", want: "localhost:8080"},
		{name: "flags override env", envHost: "0.0.0.0", envPort: "1", flagHost: "127.0.0.1", flagPort: "9000", want: "127.0.0.1:9000"},
		{name: "hostname rejected", envHost: "example.com", wantErr: true},
		{name: "host with port rejected", envHost: "127.0.0.1:8089", wantErr: true},
		{name: "bad port", envPort: "http", wantErr: true},
		{name: "port out of range", envPort: "70000", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("ATRIA_HOST", c.envHost)
			t.Setenv("PORT", c.envPort)
			got, err := listenAddress(c.flagHost, c.flagPort)
			if c.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}
