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
)

// Version is the build version reported by --version.
var Version = "dev"

// IsTerminal reports whether f is an interactive terminal. It is a
// variable so tests can simulate piped output.
var IsTerminal = func(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// NewRootCmd builds the root command. In Phase 1 a terminal launch
// performs discovery and indexing and reports coverage; the
// interactive TUI itself arrives in Phase 2.
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
			return runScan(cmd.Context(), cmd)
		},
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	return cmd
}

func runScan(ctx context.Context, cmd *cobra.Command) error {
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
	stats, err := scanner.Scan(ctx)
	if err != nil {
		return err
	}
	counts, err := ix.Stats(ctx)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	for _, failed := range stats.FailedRoots {
		fmt.Fprintf(out, "Warning: could not scan %s; its previously indexed sessions were kept.\n", failed)
	}
	fmt.Fprintf(out, "Indexed %d session(s): %d discovered, %d updated, %d unchanged, %d partial, %d failed, %d excluded.\n",
		counts.Sessions, stats.Discovered, stats.Indexed, stats.Skipped, stats.Partial, stats.Failed, stats.Excluded)
	fmt.Fprintf(out, "Index: %s\n", indexPath)
	fmt.Fprintln(out, "The interactive TUI arrives in a later phase; the local index is ready and source logs were left unchanged.")
	return nil
}

// Execute runs the root command.
func Execute() error {
	return NewRootCmd().Execute()
}
