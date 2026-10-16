package tui

import (
	"context"
	"sync"

	"github.com/juanrubio/trailmark/internal/scan"
)

// scanRuntime admits commands until shutdown, then waits for admitted work.
// Admission and shutdown share a mutex so a late queued command cannot race
// Wait with an Add or access the index after Run returns.
type scanRuntime struct {
	mu            sync.Mutex
	closing       bool
	workers       sync.WaitGroup
	latest        scan.Progress
	rescans       map[uint64]context.CancelFunc
	nextRescan    uint64
	cancelPending bool
}

func (r *scanRuntime) start() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing {
		return false
	}
	r.workers.Add(1)
	return true
}
func (r *scanRuntime) finish() { r.workers.Done() }
func (r *scanRuntime) close(cancel context.CancelFunc) {
	r.mu.Lock()
	r.closing = true
	cancel()
	r.mu.Unlock()
	r.workers.Wait()
}
func (r *scanRuntime) setProgress(p scan.Progress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.Stats.FailedRoots = append([]string(nil), p.Stats.FailedRoots...)
	r.latest = p
}
func (r *scanRuntime) progress() scan.Progress {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.latest
	p.Stats.FailedRoots = append([]string(nil), p.Stats.FailedRoots...)
	return p
}

// rescanContext registers cancellation before scanner admission. An early Esc
// can arrive while Bubble Tea is still scheduling the foreground command.
func (r *scanRuntime) rescanContext(parent context.Context) (context.Context, func()) {
	child, cancel := context.WithCancel(parent)
	r.mu.Lock()
	r.nextRescan++
	id := r.nextRescan
	if r.rescans == nil {
		r.rescans = make(map[uint64]context.CancelFunc)
	}
	r.rescans[id] = cancel
	if r.closing || r.cancelPending {
		cancel()
		r.cancelPending = false
	}
	r.mu.Unlock()
	return child, func() {
		cancel()
		r.mu.Lock()
		delete(r.rescans, id)
		r.mu.Unlock()
	}
}
func (r *scanRuntime) cancelRescans() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.rescans) == 0 {
		r.cancelPending = true
	}
	for _, cancel := range r.rescans {
		cancel()
	}
}
