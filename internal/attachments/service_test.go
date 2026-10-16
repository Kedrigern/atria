package attachments

import "testing"

func TestStorageExtension(t *testing.T) {
	cases := []struct {
		original, local, detected, want string
	}{
		{"photo.JPG", "/tmp/atria-upload-123", ".jpg", ".jpg"},
		{"no-extension", "/tmp/cover.png", ".png", ".png"},
		{"weird.ex$t", "/tmp/atria-upload-123", ".txt", ".txt"},
		{"../../etc/passwd", "/tmp/atria-upload-123", ".txt", ".txt"},
		{"archive.tar.gz", "", ".gz", ".gz"},
	}
	for _, c := range cases {
		if got := storageExtension(c.original, c.local, c.detected); got != c.want {
			t.Errorf("storageExtension(%q, %q, %q) = %q, want %q", c.original, c.local, c.detected, got, c.want)
		}
	}
}
