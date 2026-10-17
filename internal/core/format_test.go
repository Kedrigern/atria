package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatBytes(t *testing.T) {
	cases := map[int64]string{
		0:               "0 B",
		1023:            "1023 B",
		1024:            "1.0 KiB",
		1536:            "1.5 KiB",
		5 * 1024 * 1024: "5.0 MiB",
		3 << 30:         "3.0 GiB",
		1<<40 + 1<<39:   "1.5 TiB",
	}
	for n, want := range cases {
		assert.Equal(t, want, FormatBytes(n), "FormatBytes(%d)", n)
	}
}
