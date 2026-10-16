package core

// Column limits from the entities table.
const (
	MaxSlugLength  = 100 // well under slug VARCHAR(150)
	MaxTitleLength = 255 // title VARCHAR(255)
)

// LimitRunes shortens s to at most n characters. Unlike slicing bytes, it
// never cuts a multi-byte character in half, which PostgreSQL would reject as
// invalid UTF-8.
func LimitRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
