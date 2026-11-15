// Package mdcheck checks a Markdown document for problems that are common in
// books converted from print, and counts its chapters, footnotes and pages.
// See docs/architecture/manuscripts.md, sections 3 and 3a.
package mdcheck

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// StandardPageChars is the length of one standard page (normostrana).
const StandardPageChars = 1800

// Rule names a group of checks.
const (
	RuleHeadings  = "headings"
	RuleFootnotes = "footnotes"
	RulePages     = "pages"
	RuleText      = "text"
)

// Warning is one problem in a document. Warnings never block an upload.
type Warning struct {
	Line    int    `json:"line"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
	Quote   string `json:"quote"`
}

// Report is the result of Check.
type Report struct {
	Warnings []Warning `json:"warnings"`
	// Chapters counts the headings at the chapter level: "##" under a single
	// "#" title, otherwise "#".
	Chapters  int `json:"chapters"`
	Footnotes int `json:"footnotes"`
	// Characters is the number of characters (runes) of the document.
	Characters int `json:"characters"`
	// OriginalPages is the page count of the printed original from the page
	// markers. Zero means the document has no Arabic page markers.
	// A marker starts its page, so the count runs from the first Arabic
	// marker to the highest one, plus the page before the first marker when
	// body text (not only a heading) comes before it.
	OriginalPages int `json:"original_pages"`
}

// StandardPages is the length in standard pages of 1 800 characters.
func (r Report) StandardPages() float64 {
	return float64(r.Characters) / StandardPageChars
}

var (
	// A page marker is an HTML comment with a page number, optionally after a
	// page word: <!-- 15 -->, <!-- p. 15 -->, <!-- str. 15 -->, <!-- xiv -->.
	// The PDF converter (pdf-to-markdown-docling) adds the PDF page and
	// writes "?" when the printed number is not known:
	// <!-- page 41 (pdf 38) -->, <!-- page ? (pdf 12) -->.
	markerRe = regexp.MustCompile(`(?i)<!--\s*(?:(?:p\.|s\.|str\.|page)\s*)?([0-9]+|[ivxlcdm]+|\?)(?:\s*\(pdf\s*[0-9]+\))?\s*-->`)
	// A comment with a digit that is not a page marker is probably a broken one.
	nearMarkerRe = regexp.MustCompile(`<!--[^>]*?[0-9][^>]*?-->`)
	headingRe    = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?[ \t]*$`)
	footDefRe    = regexp.MustCompile(`^ {0,3}\[\^([^\]\s]+)\]:`)
	footRefRe    = regexp.MustCompile(`\[\^([^\]\s]+)\]`)
	fenceRe      = regexp.MustCompile("^ {0,3}(```|~~~)")
)

const maxHeadingRunes = 200

type heading struct {
	line, level int
}

// Check reads a Markdown document and returns its report.
func Check(src []byte) Report {
	var r Report
	warn := func(line int, rule, quote, format string, args ...any) {
		r.Warnings = append(r.Warnings, Warning{Line: line, Rule: rule, Message: fmt.Sprintf(format, args...), Quote: shorten(quote)})
	}

	rawLines := strings.Split(string(src), "\n")
	lines := make([]string, len(rawLines))
	for i, raw := range rawLines {
		if !utf8.ValidString(raw) {
			warn(i+1, RuleText, strings.ToValidUTF8(raw, "�"), "the line is not valid UTF-8")
			raw = strings.ToValidUTF8(raw, "�")
		}
		lines[i] = strings.TrimRight(raw, "\r")
	}
	body := frontMatterEnd(lines)
	for _, line := range lines[body:] {
		r.Characters += utf8.RuneCountInString(line)
	}
	if len(lines)-body > 1 {
		r.Characters += len(lines) - body - 1 // the line breaks
	}

	var headings []heading
	var lastLevel int
	footDefs := map[string]int{}
	footRefs := map[string]int{}
	var refOrder []string
	var lastRoman, lastArabic, firstArabic int
	// unknown counts the "?" markers since the last Arabic marker.
	unknown := 0
	// goingBack is set while the markers go back below the highest page, for
	// example in a block of notes at the end that the converter marks with
	// their pages. Only the first marker of such a run is reported.
	goingBack := false
	seenMarker, textBeforeMarkers, pageBeforeFirst := false, false, false
	inFence := false

	for i, line := range lines {
		n := i + 1
		if i < body {
			continue
		}
		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		isHeading := false
		if m := headingRe.FindStringSubmatch(line); m != nil {
			isHeading = true
			level, text := len(m[1]), strings.TrimSpace(strings.TrimRight(m[2], "#"))
			headings = append(headings, heading{n, level})
			switch {
			case text == "":
				warn(n, RuleHeadings, line, "the heading is empty")
			case utf8.RuneCountInString(text) > maxHeadingRunes:
				warn(n, RuleHeadings, line, "the heading is longer than %d characters; it is probably a paragraph", maxHeadingRunes)
			}
			if lastLevel > 0 && level > lastLevel+1 {
				warn(n, RuleHeadings, line, "the heading level goes from %d to %d and skips a level", lastLevel, level)
			}
			lastLevel = level
		}

		if m := footDefRe.FindStringSubmatch(line); m != nil {
			if first, ok := footDefs[m[1]]; ok {
				warn(n, RuleFootnotes, line, "footnote [^%s] is defined again; the first definition is on line %d", m[1], first)
			} else {
				footDefs[m[1]] = n
			}
		}
		refText := footDefRe.ReplaceAllString(line, "")
		for _, m := range footRefRe.FindAllStringSubmatch(refText, -1) {
			if _, ok := footRefs[m[1]]; !ok {
				footRefs[m[1]] = n
				refOrder = append(refOrder, m[1])
			}
		}

		markers := markerRe.FindAllStringSubmatchIndex(line, -1)
		if len(markers) > 0 && !seenMarker {
			// Body text before the first marker is on the page before it.
			if !isHeading && strings.TrimSpace(line[:markers[0][0]]) != "" {
				textBeforeMarkers = true
			}
		}
		for _, loc := range markers {
			value := line[loc[2]:loc[3]]
			first := !seenMarker
			seenMarker = true
			if value == "?" {
				if lastArabic > 0 && !goingBack {
					unknown++
				}
				warn(n, RulePages, line, "the printed page number is not known")
			} else if number, err := strconv.Atoi(value); err == nil {
				switch {
				case lastArabic == 0:
					firstArabic = number
					pageBeforeFirst = first && textBeforeMarkers && number > 1
				case number <= lastArabic:
					if !goingBack {
						if number == lastArabic {
							warn(n, RulePages, line, "page %d is marked again", number)
						} else {
							warn(n, RulePages, line, "page %d comes after page %d; the next markers that go back are not reported", number, lastArabic)
						}
					}
					goingBack = true
					continue
				default:
					checkGap(n, line, lastArabic, number, unknown, warn)
				}
				goingBack = false
				unknown = 0
				lastArabic = number
			} else if roman, ok := romanValue(value); ok {
				switch {
				case lastArabic > 0:
					warn(n, RulePages, line, "Roman page %s comes after the Arabic page numbers", value)
				case lastRoman > 0 && roman != lastRoman+1:
					warn(n, RulePages, line, "Roman page %s does not follow page %s", value, strings.ToLower(toRoman(lastRoman)))
				}
				lastRoman = roman
			}
		}
		if !seenMarker && !isHeading && strings.TrimSpace(line) != "" {
			textBeforeMarkers = true
		}
		for _, m := range nearMarkerRe.FindAllString(line, -1) {
			if !markerRe.MatchString(m) {
				warn(n, RulePages, line, "%s looks like a page marker but has an unknown form", m)
			}
		}

		if hyphenatedAt(lines, i) {
			warn(n, RuleText, line+" "+strings.TrimSpace(lines[i+1]), "a word is broken across lines with a hyphen")
		}
	}

	h1, h2 := 0, 0
	for _, h := range headings {
		switch h.level {
		case 1:
			h1++
		case 2:
			h2++
		}
	}
	switch {
	case h1 == 0:
		warn(body+1, RuleHeadings, firstLine(lines[body:]), "the document has no # heading")
		r.Chapters = h2
	case h1 == 1 && h2 > 0:
		r.Chapters = h2
	default:
		r.Chapters = h1
	}
	if h1 > 1 && h2 > 0 {
		second := secondHeading(headings, 1)
		warn(second, RuleHeadings, lines[second-1], "the document has more than one # title, but its chapters use ##")
	}

	for _, label := range refOrder {
		if _, ok := footDefs[label]; !ok {
			warn(footRefs[label], RuleFootnotes, lines[footRefs[label]-1], "footnote [^%s] has no definition", label)
		}
	}
	for label, line := range footDefs {
		if _, ok := footRefs[label]; !ok {
			warn(line, RuleFootnotes, lines[line-1], "footnote [^%s] is defined but never used", label)
		}
	}
	r.Footnotes = len(footDefs)

	if lastArabic > 0 {
		r.OriginalPages = lastArabic - firstArabic + 1
		if pageBeforeFirst {
			r.OriginalPages++
		}
	}

	sortWarnings(r.Warnings)
	return r
}

// checkGap reports pages missing between the Arabic pages last and number,
// when unknown markers with "?" lie between them.
func checkGap(n int, line string, last, number, unknown int, warn func(int, string, string, string, ...any)) {
	missing := number - last - 1 - unknown
	switch {
	case unknown == 0 && missing == 1:
		warn(n, RulePages, line, "page %d is missing between %d and %d", last+1, last, number)
	case unknown == 0 && missing > 1:
		warn(n, RulePages, line, "pages %d–%d are missing between %d and %d", last+1, number-1, last, number)
	case missing > 0:
		warn(n, RulePages, line, "%s missing between %d and %d, besides %s without a number", pages(missing), last, number, pages(unknown))
	case missing < 0:
		warn(n, RulePages, line, "%s without a number lie between %d and %d, but there is room for only %s", pages(unknown), last, number, pages(number-last-1))
	}
}

func pages(n int) string {
	if n == 1 {
		return "1 page"
	}
	return fmt.Sprintf("%d pages", n)
}

// frontMatterEnd returns the index of the first line after a YAML front
// matter block, or 0 when the document has none.
func frontMatterEnd(lines []string) int {
	if len(lines) == 0 || lines[0] != "---" {
		return 0
	}
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" || lines[i] == "..." {
			return i + 1
		}
	}
	return 0
}

// hyphenatedAt reports whether line i ends with a word broken by a hyphen and
// the next line continues the word in lower case.
func hyphenatedAt(lines []string, i int) bool {
	if i+1 >= len(lines) {
		return false
	}
	line := strings.TrimRight(lines[i], " \t")
	if !strings.HasSuffix(line, "-") || strings.HasSuffix(line, "--") {
		return false
	}
	before, _ := utf8.DecodeLastRuneInString(strings.TrimSuffix(line, "-"))
	after, _ := utf8.DecodeRuneInString(strings.TrimLeft(lines[i+1], " \t"))
	return unicode.IsLetter(before) && unicode.IsLower(after)
}

func secondHeading(headings []heading, level int) int {
	seen := 0
	for _, h := range headings {
		if h.level == level {
			seen++
			if seen == 2 {
				return h.line
			}
		}
	}
	return 1
}

func firstLine(lines []string) string {
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			return l
		}
	}
	return ""
}

func shorten(s string) string {
	s = strings.TrimSpace(s)
	const max = 80
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "…"
}

var romanDigits = map[rune]int{'i': 1, 'v': 5, 'x': 10, 'l': 50, 'c': 100, 'd': 500, 'm': 1000}

// romanValue converts a Roman number. It accepts only the canonical form, so
// that a word such as "mix" is not read as a number.
func romanValue(s string) (int, bool) {
	s = strings.ToLower(s)
	total := 0
	runes := []rune(s)
	for i, r := range runes {
		v := romanDigits[r]
		if i+1 < len(runes) && v < romanDigits[runes[i+1]] {
			total -= v
		} else {
			total += v
		}
	}
	if total <= 0 || strings.ToLower(toRoman(total)) != s {
		return 0, false
	}
	return total, true
}

func toRoman(n int) string {
	values := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	symbols := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var b strings.Builder
	for i, v := range values {
		for n >= v {
			b.WriteString(symbols[i])
			n -= v
		}
	}
	return b.String()
}

func sortWarnings(w []Warning) {
	// Insertion sort keeps the order of warnings on the same line.
	for i := 1; i < len(w); i++ {
		for j := i; j > 0 && w[j].Line < w[j-1].Line; j-- {
			w[j], w[j-1] = w[j-1], w[j]
		}
	}
}
