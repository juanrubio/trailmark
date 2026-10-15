// Command trailmark launches the recent-work TUI. Phase 1 provides
// the launch interface plus discovery and indexing; the interactive
// screens arrive in later phases.
package main

import (
	"fmt"
	"os"

	"github.com/juanrubio/trailmark/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "trailmark:", err)
		os.Exit(1)
	}
}
