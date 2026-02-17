package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/lukemcguire/zombiecrawl/result"
)

var (
	titleStyle       = lipgloss.NewStyle().Bold(true)
	successStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	errorStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
	headerStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	categoryStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	dimStyle         = lipgloss.NewStyle().Faint(true)
	urlStyle         = lipgloss.NewStyle()
	statusErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

// categoryOrder defines the display order for error categories (most to least actionable).
var categoryOrder = []result.ErrorCategory{
	result.Category4xx,
	result.Category5xx,
	result.CategoryTimeout,
	result.CategoryDNSFailure,
	result.CategoryConnectionRefused,
	result.CategoryRedirectLoop,
	result.CategoryUnknown,
}

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

	// Group broken links by error category
	grouped := make(map[result.ErrorCategory][]result.LinkResult)
	for _, link := range res.BrokenLinks {
		cat := link.ErrorCategory
		if cat == "" {
			cat = result.CategoryUnknown
		}
		grouped[cat] = append(grouped[cat], link)
	}

	// Display each category in order
	for _, cat := range categoryOrder {
		links, exists := grouped[cat]
		if !exists || len(links) == 0 {
			continue
		}

		// Category header
		builder.WriteString(categoryStyle.Render(fmt.Sprintf("## %s (%d)", result.FormatCategory(cat), len(links))))
		builder.WriteString("\n")

		// Build table for this category
		rows := make([][]string, 0, len(links))
		for _, link := range links {
			status := fmt.Sprintf("%d", link.StatusCode)
			if link.Error != "" {
				status = link.Error
			}
			rows = append(rows, []string{link.URL, status, link.SourcePage})
		}

		catTable := table.New().
			Border(lipgloss.RoundedBorder()).
			Headers("URL", "Status", "Found On").
			StyleFunc(func(row, col int) lipgloss.Style {
				if row == table.HeaderRow {
					return headerStyle
				}
				if col == 1 { // Status column
					return statusErrorStyle
				}
				return urlStyle
			}).
			Rows(rows...)

		builder.WriteString(catTable.Render())
		builder.WriteString("\n\n")
	}

	// Summary stats
	builder.WriteString(titleStyle.Render(fmt.Sprintf(
		"Found %d broken links out of %d URLs checked (%s)",
		res.Stats.BrokenCount,
		res.Stats.TotalChecked,
		res.Stats.Duration.Round(1_000_000),
	)))
	builder.WriteString("\n")

	return builder.String()
}
