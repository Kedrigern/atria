package testutil

import "testing"

func TestValidateTestDatabaseURL(t *testing.T) {
	const app = "postgres://u:p@localhost:5432/atria_dev?sslmode=disable"
	cases := []struct {
		dsn     string
		wantErr bool
	}{
		{"postgres://u:p@localhost:5432/atria_test?sslmode=disable", false},
		{"postgres://u:p@localhost:5432/TEST_atria", false},
		{"postgres://u:p@localhost:5432/atria_dev?sslmode=disable", true},
		{"postgres://u:p@localhost:5432/atria", true},
		{"postgres://u:p@localhost:5432/", true},
		{app, true},
		{"://not a url", true},
	}
	for _, c := range cases {
		err := ValidateTestDatabaseURL(c.dsn, app)
		if (err != nil) != c.wantErr {
			t.Errorf("ValidateTestDatabaseURL(%q) error = %v, wantErr %v", c.dsn, err, c.wantErr)
		}
	}
}
