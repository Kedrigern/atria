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

// headingShifter je náš AST transformátor pro Goldmark
type headingShifter struct{}

// Transform projde strom Markdownu. Pokud najde H1, posune úroveň všech nadpisů.
func (s *headingShifter) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	hasH1 := false

	// 1. PRŮCHOD: Hledáme, jestli existuje nějaký nadpis úrovně 1
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
		return // Uživatel nepoužil H1, necháme nadpisy jak jsou
	}

	// 2. PRŮCHOD: Posuneme všechny nadpisy o úroveň níž
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

// RenderMarkdown vezme raw text, vyrenderuje HTML s posunutými nadpisy a vrátí metadata
func RenderMarkdown(source []byte) (string, map[string]interface{}, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM, // Podpora tabulek, task listů atd.
			meta.Meta,     // Podpora pro YAML Front Matter
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

// calloutTransformer hledá GitHub-style upozornění (> [!NOTE])
type calloutTransformer struct{}

func (s *calloutTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == ast.KindBlockquote {
			bq := n.(*ast.Blockquote)

			if bq.FirstChild() != nil && bq.FirstChild().Kind() == ast.KindParagraph {
				p := bq.FirstChild().(*ast.Paragraph)

				// 1. Čteme přímo "surové" řádky textu (ty nejsou rozsekané parserem)
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

				// 2. Našli jsme Callout! Nastavíme CSS třídu na hlavní <blockquote> tag
				cssClass := "callout callout-" + strings.ToLower(calloutType)
				bq.SetAttribute([]byte("class"), []byte(cssClass))

				// 3. Vypočítáme, kolik bajtů v původním textu zabírá značka (např. "[!WARNING]")
				exactEndIdx := strings.Index(firstLineText, "]")
				charsToRemove := exactEndIdx + 1

				// Pokud za značkou následuje mezera, schlamstneme ji taky, ať nám text nezačíná mezerou
				if charsToRemove < len(firstLineText) && (firstLineText[charsToRemove] == ' ' || firstLineText[charsToRemove] == '\n') {
					charsToRemove++
				}

				// Tento bod v původním zdrojáku si chceme nechat
				keepStart := firstLine.Start + charsToRemove

				// 4. Projdeme rozsekané AST uzly a vše, co je před `keepStart`, nekompromisně smažeme
				var next ast.Node
				for child := p.FirstChild(); child != nil; child = next {
					next = child.NextSibling()
					if textNode, ok := child.(*ast.Text); ok {
						if textNode.Segment.Stop <= keepStart {
							p.RemoveChild(p, textNode) // Uzel je celý součást značky -> smazat
						} else if textNode.Segment.Start < keepStart {
							// Uzel zasahuje do značky -> oříznout jeho začátek
							textNode.Segment = text.NewSegment(keepStart, textNode.Segment.Stop)
						} else {
							break // Už jsme za značkou v bezpečí čistého textu
						}
					}
				}

				// 5. Na začátek odstavce vložíme tučný nadpis (např. **Warning**)
				strong := ast.NewEmphasis(2) // 2 znamená tučné písmo (**)
				titleText := strings.ToUpper(calloutType[:1]) + strings.ToLower(calloutType[1:])
				strong.AppendChild(strong, ast.NewString([]byte(titleText)))
				p.InsertBefore(p, p.FirstChild(), strong)
			}
		}
		return ast.WalkContinue, nil
	})
}
