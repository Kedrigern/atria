package web

import "testing"

func TestValidateTrustedProxies(t *testing.T) {
	cases := map[string]bool{
		"":                       true,
		"127.0.0.1,::1":          true,
		"127.0.0.1, ::1":         true, // spaces used to make gin trust every proxy
		"10.0.0.0/8, 172.17.0.1": true,
		"localhost":              false,
		"127.0.0.1,not-an-ip":    false,
		"10.0.0.0/33":            false,
	}
	for value, valid := range cases {
		t.Setenv("TRUSTED_PROXIES", value)
		if err := validateTrustedProxies(); (err == nil) != valid {
			t.Errorf("TRUSTED_PROXIES=%q: err=%v, want valid=%v", value, err, valid)
		}
	}
}
