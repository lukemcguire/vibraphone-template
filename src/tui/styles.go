package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/lukemcguire/zombiecrawl/result"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true)
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	errorStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	dimStyle     = lipgloss.NewStyle().Faint(true)
)

// RenderSummary produces a Lip Gloss styled summary of crawl results.
func RenderSummary(res *result.Result) string {
	if res == nil {
		return errorStyle.Render("No results available.")
	}

	var builder strings.Builder

	if len(res.BrokenLinks) == 0 {
		builder.WriteString(successStyle.Render("No broken links found!"))
		builder.WriteString("\n")
		builder.WriteString(dimStyle.Render(fmt.Sprintf(
			"Checked %d URLs in %s",
			res.Stats.TotalChecked,
			res.Stats.Duration.Round(1_000_000), // round to ms
		)))
		builder.WriteString("\n")
		return builder.String()
	}

	// Build table rows.
	rows := make([][]string, 0, len(res.BrokenLinks))
	for _, link := range res.BrokenLinks {
		status := fmt.Sprintf("%d", link.StatusCode)
		if link.Error != "" {
			status = link.Error
		}
		rows = append(rows, []string{link.URL, status, link.SourcePage})
	}

	resultsTable := table.New().
		Border(lipgloss.RoundedBorder()).
		Headers("URL", "Status", "Found On").
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return errorStyle
		}).
		Rows(rows...)

	builder.WriteString(resultsTable.Render())
	builder.WriteString("\n\n")
	builder.WriteString(titleStyle.Render(fmt.Sprintf(
		"Found %d broken links out of %d URLs checked (%s)",
		res.Stats.BrokenCount,
		res.Stats.TotalChecked,
		res.Stats.Duration.Round(1_000_000),
	)))
	builder.WriteString("\n")

	return builder.String()
}
