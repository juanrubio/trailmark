package tui

import (
	"fmt"
	"strings"

	"github.com/juanrubio/trailmark/internal/index"
)

// copyItem is one entry in the copy menu with its exact payload.
type copyItem struct {
	label     string
	text      string
	available bool
}

// copyItems builds the copy menu entries for the selected session.
// Payloads show provenance before anything is copied.
func (m *appModel) buildCopyItems(row index.SessionRow, ov *index.Overview) []copyItem {
	items := []copyItem{{label: "Copy directory", text: row.CWD, available: row.CWD != ""}}
	if ov == nil {
		items = append(items,
			copyItem{label: "Copy initial request (unavailable)"},
			copyItem{label: "Copy latest exchange (unavailable)"})
		return items
	}
	header := provenanceHeader(row)
	if ov.Initial.Text != "" {
		items = append(items, copyItem{
			label:     "Copy initial request",
			text:      header + "\n\n" + excerptBlock("Initial request", ov.Initial),
			available: true,
		})
	} else {
		items = append(items, copyItem{label: "Copy initial request (unavailable)"})
	}
	var blocks []string
	if ov.LatestUserEx.Text != "" {
		blocks = append(blocks, excerptBlock("Latest user message", ov.LatestUserEx))
	}
	if ov.LatestAsstEx.Text != "" {
		blocks = append(blocks, excerptBlock("Latest agent reply", ov.LatestAsstEx))
	}
	if len(blocks) > 0 {
		items = append(items, copyItem{
			label:     "Copy latest exchange",
			text:      header + "\n\n" + strings.Join(blocks, "\n\n"),
			available: true,
		})
	} else {
		items = append(items, copyItem{label: "Copy latest exchange (unavailable)"})
	}
	return items
}

func provenanceHeader(row index.SessionRow) string {
	id := row.ID
	if id == "" {
		id = "unavailable"
	}
	return fmt.Sprintf("Provider: %s\nSession: %s\nSource: %s", providerLabel(row.Provider), id, row.SourcePath)
}

func excerptBlock(label string, ex index.Excerpt) string {
	loc := fmt.Sprintf("line %d", ex.Line)
	if ex.RecordID != "" {
		loc += ", record " + ex.RecordID
	}
	if ex.Timestamp != nil {
		loc += ", " + ex.Timestamp.Format("2006-01-02 15:04")
	}
	return fmt.Sprintf("%s (%s):\n%s", label, loc, ex.Text)
}
