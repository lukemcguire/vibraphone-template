package result

import (
	"fmt"
	"io"
)

// PrintResults writes broken link details and a summary to w.
func PrintResults(w io.Writer, res *Result) {
	writef := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

	if len(res.BrokenLinks) == 0 {
		writef("No broken links found!\n")
	} else {
		writef("Broken Links:\n")
		for i, link := range res.BrokenLinks {
			writef("  URL: %s\n", link.URL)
			if link.Error != "" {
				writef("  Error: %s\n", link.Error)
			} else {
				writef("  Status: %d\n", link.StatusCode)
			}
			writef("  Found on: %s\n", link.SourcePage)
			if i < len(res.BrokenLinks)-1 {
				writef("\n")
			}
		}
	}
	writef("Checked %d URLs, found %d broken links\n", res.Stats.TotalChecked, res.Stats.BrokenCount)
}
