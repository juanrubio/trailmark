// Command trailmark-bench generates the synthetic PRD benchmark
// corpus and measures Trailmark against the provisional targets.
// It is a development tool only; the corpus must never be committed.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/juanrubio/trailmark/internal/bench"
)

func main() {
	dir := flag.String("dir", "", "corpus directory (required)")
	sessions := flag.Int("sessions", 10000, "number of sessions")
	gib := flag.Float64("gib", 1.0, "total corpus size in GiB")
	skipGenerate := flag.Bool("skip-generate", false, "reuse an existing corpus")
	skipIndex := flag.Bool("skip-index", false, "reuse the existing index; measure warm metrics only")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "trailmark-bench: -dir is required")
		os.Exit(2)
	}
	ctx := context.Background()
	corpus := &bench.Corpus{
		Dir:        *dir,
		ClaudeRoot: *dir + "/claude-projects",
		CodexRoot:  *dir + "/codex-sessions",
		Sessions:   *sessions,
		BigFile:    *dir + "/codex-sessions/2026/10/15/rollout-bench-big.jsonl",
	}
	if !*skipGenerate {
		start := time.Now()
		c, err := bench.Generate(*dir, *sessions, int64(*gib*(1<<30)))
		if err != nil {
			fmt.Fprintln(os.Stderr, "generate:", err)
			os.Exit(1)
		}
		corpus = c
		fmt.Printf("generated %d sessions, %.2f GiB in %s\n", c.Sessions, float64(c.TotalBytes)/(1<<30), time.Since(start).Round(time.Second))
		if c.TotalBytes < int64(*gib*(1<<30)) {
			fmt.Fprintln(os.Stderr, "corpus below the requested size; refusing to measure")
			os.Exit(1)
		}
		// Re-exec in measure-only mode so peak RSS reflects the
		// indexing process, not corpus generation.
		self, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, "re-exec:", err)
			os.Exit(1)
		}
		cmd := exec.Command(self, "-dir", *dir, "-sessions", fmt.Sprint(*sessions), "-gib", fmt.Sprint(*gib), "-skip-generate")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				os.Exit(exit.ExitCode())
			}
			fmt.Fprintln(os.Stderr, "measure:", err)
			os.Exit(1)
		}
		return
	}
	indexPath := *dir + "/index.db"
	if *skipIndex {
		r, err := bench.MeasureExisting(ctx, corpus, indexPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "measure:", err)
			os.Exit(1)
		}
		fmt.Printf("\n(warm metrics only; cold indexing reported from the indexing run)\n\n| Metric | Measured | Provisional target |\n| --- | --- | --- |\n")
		fmt.Printf("| Cached list opening | %s | 2 s |\n", r.CachedList.Round(time.Millisecond))
		fmt.Printf("| Selection-to-overview p95 (100) | %s | 100 ms |\n", r.OverviewP95.Round(time.Microsecond))
		fmt.Printf("| Warm search p95 (100 queries) | %s | 200 ms |\n", r.SearchP95.Round(time.Microsecond))
		fmt.Printf("| 50 MiB transcript first viewport | %s | 1 s |\n", r.BigViewport.Round(time.Millisecond))
		fmt.Printf("\nGo: %s · OS/Arch: %s/%s · CPUs: %d\n", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
		return
	}
	r, err := bench.Measure(ctx, corpus, indexPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "measure:", err)
		os.Exit(1)
	}
	fmt.Printf("\n| Metric | Measured | Provisional target |\n| --- | --- | --- |\n")
	fmt.Printf("| Cold indexing (%d sessions) | %s | report only |\n", r.IndexedSessions, r.ColdIndex.Round(time.Millisecond))
	fmt.Printf("| Peak RSS during cold indexing | %.0f MiB | report only |\n", float64(r.PeakRSSBytes)/(1<<20))
	fmt.Printf("| Cached list opening | %s | 2 s |\n", r.CachedList.Round(time.Millisecond))
	fmt.Printf("| Selection-to-overview p95 (100) | %s | 100 ms |\n", r.OverviewP95.Round(time.Microsecond))
	fmt.Printf("| Warm search p95 (100 queries) | %s | 200 ms |\n", r.SearchP95.Round(time.Microsecond))
	fmt.Printf("| 50 MiB transcript first viewport | %s | 1 s |\n", r.BigViewport.Round(time.Millisecond))
	fmt.Printf("\nGo: %s · OS/Arch: %s/%s · CPUs: %d\n", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
}
