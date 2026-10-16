// Package bench generates a synthetic session corpus and measures
// Trailmark against the PRD section 9 performance targets. The
// corpus is generated locally and must never be committed.
package bench

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/juanrubio/trailmark/internal/config"
	"github.com/juanrubio/trailmark/internal/index"
	"github.com/juanrubio/trailmark/internal/model"
	"github.com/juanrubio/trailmark/internal/scan"
)

// Corpus describes a generated benchmark corpus.
type Corpus struct {
	Dir        string
	ClaudeRoot string
	CodexRoot  string
	Sessions   int
	TotalBytes int64
	BigFile    string // the single ~50 MiB transcript
}

// filler is deterministic synthetic message text with searchable terms.
func filler(seed int, words int) string {
	vocab := benchVocab
	var b strings.Builder
	x := uint32(seed)*2654435761 + 97
	for i := 0; i < words; i++ {
		x = x*1664525 + 1013904223
		b.WriteString(vocab[(x>>16)%uint32(len(vocab))])
		b.WriteByte(' ')
	}
	return b.String()
}

// benchVocab mixes domain terms with generated words so match
// density resembles real logs instead of every message matching
// every query.
var benchVocab = func() []string {
	roots := []string{"upload", "retry", "deployment", "worker", "session", "search", "index", "transcript", "project", "review", "cache", "settings", "benchmark", "fixture", "message", "context", "server", "client", "request", "response", "error", "timeout", "config", "database", "query", "render", "scroll", "filter", "export", "import", "token", "model"}
	suffixes := []string{"", "s", "ing", "ed", "er", "ly", "ment", "tion"}
	var out []string
	for _, r := range roots {
		for _, suf := range suffixes {
			out = append(out, r+suf)
		}
	}
	return out
}()

// Generate writes a synthetic corpus: sessions split across both
// providers, sized to reach totalBytes, with one big transcript.
func Generate(dir string, sessions int, totalBytes int64) (*Corpus, error) {
	claudeRoot := filepath.Join(dir, "claude-projects")
	codexRoot := filepath.Join(dir, "codex-sessions")
	for _, d := range []string{claudeRoot, codexRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("create corpus dir: %w", err)
		}
	}
	const bigSize = 50 << 20
	remaining := totalBytes - bigSize
	if remaining < 0 {
		remaining = 0
	}
	perSession := int64(0)
	if sessions > 1 {
		perSession = remaining / int64(sessions-1)
	}
	c := &Corpus{Dir: dir, ClaudeRoot: claudeRoot, CodexRoot: codexRoot, Sessions: sessions}
	base := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	writeSession := func(path string, codex bool, id string, dayOffset, sizeWords int, day time.Time) (int64, error) {
		f, err := os.Create(path)
		if err != nil {
			return 0, err
		}
		defer f.Close()
		w := bufio.NewWriterSize(f, 1<<20)
		defer w.Flush()
		var written int64
		ts := day.Format(time.RFC3339)
		if codex {
			line := fmt.Sprintf("{\"timestamp\":%q,\"type\":\"session_meta\",\"payload\":{\"id\":%q,\"timestamp\":%q,\"cwd\":\"/bench/project-%d\",\"originator\":\"bench\",\"cli_version\":\"0\",\"source\":\"cli\",\"model_provider\":\"bench\"}}\n", ts, id, ts, dayOffset%40)
			w.WriteString(line)
			written += int64(len(line))
		}
		msgs := 24
		for i := 0; i < msgs; i++ {
			text := filler(dayOffset*100+i, sizeWords)
			var line string
			if codex {
				role := "user"
				kind := "input_text"
				if i%2 == 1 {
					role, kind = "assistant", "output_text"
				}
				line = fmt.Sprintf("{\"timestamp\":%q,\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"role\":%q,\"content\":[{\"type\":%q,\"text\":%q}]}}\n", ts, role, kind, text)
			} else {
				typ := "user"
				if i%2 == 1 {
					typ = "assistant"
				}
				line = fmt.Sprintf("{\"type\":%q,\"sessionId\":%q,\"timestamp\":%q,\"cwd\":\"/bench/project-%d\",\"uuid\":\"u-%d\",\"message\":{\"role\":%q,\"content\":%q}}\n", typ, id, ts, dayOffset%40, i, typ, text)
			}
			w.WriteString(line)
			written += int64(len(line))
		}
		return written, w.Flush()
	}
	// 24 messages per session, roughly 7.5 bytes per filler word
	// plus per-record JSON overhead: solve words-per-message from
	// the per-session byte target so the corpus reaches totalBytes.
	wordsPerSession := int((perSession/24 - 200) / 8)
	if wordsPerSession < 40 {
		wordsPerSession = 40
	}
	for i := 0; i < sessions-1; i++ {
		day := base.AddDate(0, 0, -(i % 30))
		codex := i%2 == 1
		id := fmt.Sprintf("bench-%06d", i)
		var path string
		if codex {
			dir2 := filepath.Join(codexRoot, day.Format("2006"), day.Format("01"), day.Format("02"))
			os.MkdirAll(dir2, 0o755)
			path = filepath.Join(dir2, "rollout-"+id+".jsonl")
		} else {
			dir2 := filepath.Join(claudeRoot, fmt.Sprintf("proj-%d", i%40))
			os.MkdirAll(dir2, 0o755)
			path = filepath.Join(dir2, id+".jsonl")
		}
		n, err := writeSession(path, codex, id, i, wordsPerSession, day)
		if err != nil {
			return nil, err
		}
		c.TotalBytes += n
	}
	// One big transcript of roughly 50 MiB.
	bigPath := filepath.Join(codexRoot, base.Format("2006"), base.Format("01"), base.Format("02"), "rollout-bench-big.jsonl")
	os.MkdirAll(filepath.Dir(bigPath), 0o755)
	n, err := writeSession(bigPath, true, "bench-big", 0, int(bigSize/2048/1), base)
	_ = n
	if err != nil {
		return nil, err
	}
	// writeSession writes a fixed message count; grow the big file
	// with additional filler records until it reaches ~50 MiB.
	f, err := os.OpenFile(bigPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	info, _ := os.Stat(bigPath)
	size := info.Size()
	chunk := filler(42, 2000)
	line := fmt.Sprintf("{\"timestamp\":%q,\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":%q}]}}\n", base.Format(time.RFC3339), chunk)
	for size < bigSize {
		w.WriteString(line)
		size += int64(len(line))
	}
	w.Flush()
	f.Close()
	info, _ = os.Stat(bigPath)
	c.TotalBytes += info.Size()
	c.BigFile = bigPath
	// Top up the big transcript until the corpus reaches the
	// requested size; never silently record a sub-threshold run.
	if c.TotalBytes < totalBytes {
		f, err := os.OpenFile(bigPath, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		w := bufio.NewWriterSize(f, 1<<20)
		for c.TotalBytes < totalBytes {
			w.WriteString(line)
			c.TotalBytes += int64(len(line))
		}
		w.Flush()
		f.Close()
	}
	if c.TotalBytes < totalBytes {
		return nil, fmt.Errorf("corpus reached %d bytes, below the requested %d", c.TotalBytes, totalBytes)
	}
	return c, nil
}

// Results holds one benchmark run's measurements.
type Results struct {
	ColdIndex       time.Duration
	PeakRSSBytes    int64
	CachedList      time.Duration
	OverviewP95     time.Duration
	SearchP95       time.Duration
	BigViewport     time.Duration
	IndexedSessions int
}

// Measure indexes the corpus cold and measures the PRD targets.
func Measure(ctx context.Context, c *Corpus, indexPath string) (*Results, error) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		os.Remove(indexPath + suffix)
	}
	ix, err := index.Open(indexPath)
	if err != nil {
		return nil, err
	}
	cfg := &config.Config{Roots: []config.Root{
		{Provider: model.ProviderClaudeCode, Path: c.ClaudeRoot},
		{Provider: model.ProviderCodex, Path: c.CodexRoot},
	}}
	scanner := &scan.Scanner{Index: ix, Adapters: scan.DefaultAdapters(os.TempDir()), Config: cfg}
	// DefaultAdapters resolves thread names from a home dir; the
	// adapters themselves do not depend on it for bench fixtures.
	start := time.Now()
	if _, err := scanner.Scan(ctx); err != nil {
		return nil, err
	}
	r := &Results{ColdIndex: time.Since(start)}
	r.PeakRSSBytes = peakRSS()
	ix.Close()

	// Cached opening is measured on a freshly reopened index in
	// this process; OS page cache remains warm, which the report
	// records honestly.
	ix, err = index.Open(indexPath)
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	listStart := time.Now()
	sessions, err := ix.ListSessions(ctx, 100000)
	if err != nil {
		return nil, err
	}
	r.CachedList = time.Since(listStart)
	r.IndexedSessions = len(sessions)

	// Selection-to-overview latency over 100 changes.
	var overviewLat []time.Duration
	for i := 0; i < 100 && i < len(sessions); i++ {
		t0 := time.Now()
		if _, err := ix.Overview(ctx, sessions[i].SourcePath); err != nil {
			return nil, err
		}
		overviewLat = append(overviewLat, time.Since(t0))
	}
	r.OverviewP95 = p95(overviewLat)

	paths := make([]string, 0, len(sessions))
	for _, s := range sessions {
		paths = append(paths, s.SourcePath)
	}
	terms := []string{"upload", "retry", "deployment worker", "\"search index\"", "internal/worker", "benchmark", "cache settings", "transcript", "project review", "fixture"}
	var searchLat []time.Duration
	for i := 0; i < 100; i++ {
		t0 := time.Now()
		if _, err := ix.SearchContentScoped(ctx, terms[i%len(terms)], paths, 50); err != nil {
			return nil, err
		}
		searchLat = append(searchLat, time.Since(t0))
	}
	r.SearchP95 = p95(searchLat)

	t0 := time.Now()
	if _, _, err := ix.TranscriptPage(ctx, c.BigFile, 0, 200); err != nil {
		return nil, err
	}
	r.BigViewport = time.Since(t0)
	return r, nil
}

func p95(lat []time.Duration) time.Duration {
	if len(lat) == 0 {
		return 0
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	idx := (95 * len(lat)) / 100
	if idx >= len(lat) {
		idx = len(lat) - 1
	}
	return lat[idx]
}

func peakRSS() int64 {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmHWM:") {
			var kb int64
			fmt.Sscanf(line, "VmHWM: %d kB", &kb)
			return kb * 1024
		}
	}
	return 0
}

// MeasureExisting measures the warm metrics against an already
// built index without deleting or rescanning it. Cold fields stay
// zero and must be reported from the run that built the index.
func MeasureExisting(ctx context.Context, c *Corpus, indexPath string) (*Results, error) {
	ix, err := index.Open(indexPath)
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	r := &Results{}
	listStart := time.Now()
	sessions, err := ix.ListSessions(ctx, 100000)
	if err != nil {
		return nil, err
	}
	r.CachedList = time.Since(listStart)
	r.IndexedSessions = len(sessions)
	var overviewLat []time.Duration
	for i := 0; i < 100 && i < len(sessions); i++ {
		t0 := time.Now()
		if _, err := ix.Overview(ctx, sessions[i].SourcePath); err != nil {
			return nil, err
		}
		overviewLat = append(overviewLat, time.Since(t0))
	}
	r.OverviewP95 = p95(overviewLat)
	paths := make([]string, 0, len(sessions))
	for _, s := range sessions {
		paths = append(paths, s.SourcePath)
	}
	terms := []string{"upload", "retry", "deployment worker", "\"search index\"", "internal/worker", "benchmark", "cache settings", "transcript", "project review", "fixture"}
	var searchLat []time.Duration
	for i := 0; i < 100; i++ {
		t0 := time.Now()
		if _, err := ix.SearchContentScoped(ctx, terms[i%len(terms)], paths, 50); err != nil {
			return nil, err
		}
		searchLat = append(searchLat, time.Since(t0))
		if (i+1)%20 == 0 {
			fmt.Fprintf(os.Stderr, "search %d/100\n", i+1)
		}
	}
	r.SearchP95 = p95(searchLat)
	t0 := time.Now()
	if _, _, err := ix.TranscriptPage(ctx, c.BigFile, 0, 200); err != nil {
		return nil, err
	}
	r.BigViewport = time.Since(t0)
	r.PeakRSSBytes = peakRSS()
	return r, nil
}
