package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"atria/internal/mdcheck"
)

var checkCmd = &cobra.Command{
	Use:   "check FILE...",
	Short: "Checks Markdown files of converted books: headings, footnotes, page markers and text",
	Long: `Checks Markdown files for problems that are common in books converted from
print, without a database or a user. It reports headings that skip a level,
footnotes without a definition, gaps in the page markers of the original
(<!-- 15 -->, <!-- page 41 (pdf 38) -->, <!-- page ? (pdf 12) -->) and words
broken across lines. A page marker starts its page. Warnings do not change
the exit status; only a file that cannot be read does.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		for _, path := range args {
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			printCheckReport(out, path, mdcheck.Check(src))
		}
		return nil
	},
}

// printCheckReport prints the summary line and the warnings of a check.
func printCheckReport(out io.Writer, name string, report mdcheck.Report) {
	fmt.Fprintf(out, "%s: %d chapters, %d footnotes, %.1f standard pages, ", name, report.Chapters, report.Footnotes, report.StandardPages())
	if report.OriginalPages > 0 {
		fmt.Fprintf(out, "%d original pages, ", report.OriginalPages)
	} else {
		fmt.Fprint(out, "no page markers, ")
	}
	fmt.Fprintf(out, "%d warnings\n", len(report.Warnings))
	for _, w := range report.Warnings {
		fmt.Fprintf(out, "  line %d [%s] %s: %s\n", w.Line, w.Rule, w.Message, w.Quote)
	}
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
