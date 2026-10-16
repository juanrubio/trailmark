package tui

import (
	"context"
	"testing"
	"time"

	"github.com/juanrubio/trailmark/internal/scan"
)

func TestScanRuntimeShutdownWaitsAndRejectsQueuedWork(t *testing.T) {
	r := &scanRuntime{}
	ctx, cancel := context.WithCancel(context.Background())
	if !r.start() {
		t.Fatal("initial admission rejected")
	}
	closed := make(chan struct{})
	go func() { r.close(cancel); close(closed) }()
	<-ctx.Done()
	if r.start() {
		t.Fatal("queued command admitted during shutdown")
	}
	select {
	case <-closed:
		t.Fatal("shutdown returned before worker finished")
	default:
	}
	r.finish()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not complete")
	}
}

func TestScanRuntimeSnapshotsOwnTheirSlices(t *testing.T) {
	r := &scanRuntime{}
	p := scan.Progress{Stats: scan.Stats{FailedRoots: []string{"root"}}}
	r.setProgress(p)
	p.Stats.FailedRoots[0] = "changed"
	got := r.progress()
	if got.Stats.FailedRoots[0] != "root" {
		t.Fatal("input snapshot aliased")
	}
	got.Stats.FailedRoots[0] = "changed again"
	if r.progress().Stats.FailedRoots[0] != "root" {
		t.Fatal("output snapshot aliased")
	}
}

func TestRescanCancellationDoesNotCancelParentOrNextScan(t *testing.T) {
	r := &scanRuntime{}
	parent := context.Background()
	first, finishFirst := r.rescanContext(parent)
	r.cancelRescans()
	if first.Err() != context.Canceled {
		t.Fatal("active rescan not canceled")
	}
	if parent.Err() != nil {
		t.Fatal("parent canceled")
	}
	finishFirst()
	next, finishNext := r.rescanContext(parent)
	defer finishNext()
	if next.Err() != nil {
		t.Fatal("new rescan inherited prior cancellation")
	}
}
func TestRescanEarlyCancellationIsConsumedOnce(t *testing.T) {
	r := &scanRuntime{}
	r.cancelRescans()
	first, finishFirst := r.rescanContext(context.Background())
	if first.Err() != context.Canceled {
		t.Fatal("early cancellation was lost")
	}
	finishFirst()
	next, finishNext := r.rescanContext(context.Background())
	defer finishNext()
	if next.Err() != nil {
		t.Fatal("early cancellation poisoned future scan")
	}
}
func TestRescanCancellationCoversQueuedInvocations(t *testing.T) {
	r := &scanRuntime{}
	first, finishFirst := r.rescanContext(context.Background())
	defer finishFirst()
	queued, finishQueued := r.rescanContext(context.Background())
	defer finishQueued()
	r.cancelRescans()
	if first.Err() != context.Canceled || queued.Err() != context.Canceled {
		t.Fatal("admitted or queued invocation escaped cancellation")
	}
}
