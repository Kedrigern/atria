package core

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// headingShifter is an AST transformer for Goldmark.
type headingShifter struct{}

// Transform walks the Markdown AST. If an H1 exists, all headings are shifted down one level.
func (s *headingShifter) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	hasH1 := false

	// Pass 1: Check whether any H1 heading exists.
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == ast.KindHeading {
			heading := n.(*ast.Heading)
			if heading.Level == 1 {
				hasH1 = true
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})

	if !hasH1 {
		return // No H1 found; leave headings unchanged.
	}

	// Pass 2: Shift all headings down one level.
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == ast.KindHeading {
			heading := n.(*ast.Heading)
			if heading.Level < 6 {
				heading.Level++
			}
		}
		return ast.WalkContinue, nil
	})
}

// RenderMarkdown parses raw Markdown, renders HTML with shifted headings, and returns front-matter metadata.
func RenderMarkdown(source []byte) (string, map[string]interface{}, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM, // Tables, task lists, etc.
			meta.Meta,     // YAML front matter support.
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(
				util.Prioritized(&headingShifter{}, 100),
				util.Prioritized(&calloutTransformer{}, 200),
			),
		),
	)

	context := parser.NewContext()
	var buf bytes.Buffer

	if err := md.Convert(source, &buf, parser.WithContext(context)); err != nil {
		return "", nil, err
	}

	metaData := meta.Get(context)
	return buf.String(), metaData, nil
}

// calloutTransformer detects GitHub-style callouts (> [!NOTE]) and converts them to styled blockquotes.
type calloutTransformer struct{}

func (s *calloutTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == ast.KindBlockquote {
			bq := n.(*ast.Blockquote)

			if bq.FirstChild() != nil && bq.FirstChild().Kind() == ast.KindParagraph {
				p := bq.FirstChild().(*ast.Paragraph)

				// 1. Read the raw source lines directly (not yet split by the parser).
				lines := p.Lines()
				if lines.Len() == 0 {
					return ast.WalkContinue, nil
				}

				firstLine := lines.At(0)
				firstLineText := string(firstLine.Value(reader.Source()))

				trimmed := strings.TrimLeft(firstLineText, " \t")
				if !strings.HasPrefix(trimmed, "[!") {
					return ast.WalkContinue, nil
				}

				endIdx := strings.Index(trimmed, "]")
				if endIdx <= 0 {
					return ast.WalkContinue, nil
				}

				calloutType := trimmed[2:endIdx]
				if len(calloutType) == 0 {
					return ast.WalkContinue, nil
				}

				// 2. Callout found — set the CSS class on the enclosing <blockquote>.
				cssClass := "callout callout-" + strings.ToLower(calloutType)
				bq.SetAttribute([]byte("class"), []byte(cssClass))

				// 3. Calculate how many bytes the marker (e.g. "[!WARNING]") occupies in the source.
				exactEndIdx := strings.Index(firstLineText, "]")
				charsToRemove := exactEndIdx + 1

				// Also consume a trailing space or newline so the content doesn't start with whitespace.
				if charsToRemove < len(firstLineText) && (firstLineText[charsToRemove] == ' ' || firstLineText[charsToRemove] == '\n') {
					charsToRemove++
				}

				// This is the byte offset in the source where the actual content begins.
				keepStart := firstLine.Start + charsToRemove

				// 4. Walk the AST text nodes and remove or trim everything before keepStart.
				var next ast.Node
				for child := p.FirstChild(); child != nil; child = next {
					next = child.NextSibling()
					if textNode, ok := child.(*ast.Text); ok {
						if textNode.Segment.Stop <= keepStart {
							p.RemoveChild(p, textNode) // Entire node is part of the marker — remove it.
						} else if textNode.Segment.Start < keepStart {
							// Node spans the marker boundary — trim its start.
							textNode.Segment = text.NewSegment(keepStart, textNode.Segment.Stop)
						} else {
							break // Past the marker; remaining nodes are clean content.
						}
					}
				}

				// 5. Prepend a bold title (e.g. **Warning**) to the paragraph.
				strong := ast.NewEmphasis(2) // 2 = bold (**)
				titleText := strings.ToUpper(calloutType[:1]) + strings.ToLower(calloutType[1:])
				strong.AppendChild(strong, ast.NewString([]byte(titleText)))
				p.InsertBefore(p, p.FirstChild(), strong)
			}
		}
		return ast.WalkContinue, nil
	})
}
