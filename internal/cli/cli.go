// Package cli builds Trailmark's minimal launch interface:
// trailmark, trailmark --help, and trailmark --version only.
// Every other operation lives inside the TUI in later phases.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/scan"
	"github.com/juanrubio/trailmark/internal/tui"
)

// Version is the build version reported by --version.
var Version = "dev"

// IsTerminal reports whether f is an interactive terminal. It is a
// variable so tests can simulate piped output.
var IsTerminal = func(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// NewRootCmd builds the root command. A terminal launch opens the
// recent-work TUI; the cached index paints first and a background
// scan refreshes it.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "trailmark",
		Short:         "Remember recent coding-agent work and choose how to continue it",
		Args:          cobra.NoArgs,
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !IsTerminal(os.Stdout) {
				return fmt.Errorf("needs an interactive terminal; it cannot run with piped or redirected output")
			}
			return runTUI(cmd.Context())
		},
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	return cmd
}

func runTUI(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	indexPath, err := config.IndexPath()
	if err != nil {
		return err
	}
	ix, err := index.Open(indexPath)
	if err != nil {
		return err
	}
	defer ix.Close()

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate home dir: %w", err)
	}
	scanner := &scan.Scanner{
		Index:    ix,
		Adapters: scan.DefaultAdapters(home),
		Config:   cfg,
	}
	return tui.Run(ctx, tui.Deps{Index: ix, Scanner: scanner})
}

// Execute runs the root command.
func Execute() error {
	return NewRootCmd().Execute()
}
