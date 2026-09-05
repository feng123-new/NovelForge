package autopilot_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/autopilot"
	repository "github.com/voocel/ainovel-cli/internal/server/repository"
)

func testStore(t *testing.T) *autopilot.Store {
	t.Helper()
	p, err := repository.Initialize(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &autopilot.Store{Path: p.Database}
}
func input() autopilot.Input {
	return autopilot.Input{Idea: "A controlled short story", StartChapter: 1, TargetChapter: 2, ReviewEvery: 1, MaxRewrites: 2, MaxRetries: 0}
}
func TestAutopilotDurabilityControlAndExclusion(t *testing.T) {
	ctx := t.Context()
	s := testStore(t)
	j, err := s.Enqueue(ctx, "p", "start", input())
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Enqueue(ctx, "p", "start", input())
	if err != nil || replay.ID != j.ID {
		t.Fatal("enqueue replay", err)
	}
	if _, err = s.Enqueue(ctx, "p", "different", input()); !errors.Is(err, autopilot.ErrConflict) {
		t.Fatal("parallel job was allowed", err)
	}
	running, err := s.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	paused, err := s.Control(ctx, j.ID, "pause")
	if err != nil || paused.State != autopilot.Running || paused.Control != "pause" {
		t.Fatal("pause acknowledged before writer quiescence", paused, err)
	}
	running.Stage = "plan"
	after, err := s.Finish(ctx, j, running, nil)
	if err != nil || after.State != autopilot.Paused || after.Stage != "plan" {
		t.Fatal("pause checkpoint", after, err)
	}
	if _, err = s.Control(ctx, j.ID, "resume"); err != nil {
		t.Fatal(err)
	}
	claimed, err := s.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// A new Store instance sees persisted progress; only the sole worker may
	// call Recover after acquiring its OS lease.
	reopened := autopilot.Store{Path: s.Path}
	if err = reopened.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get(ctx, j.ID)
	if err != nil || got.State != autopilot.Pending || got.Stage != claimed.Stage {
		t.Fatal("recovery lost progress", got, err)
	}
	claimed, err = reopened.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := reopened.Finish(ctx, claimed, claimed, autopilot.Retry("TEST_RETRY"))
	if err != nil || failed.State != autopilot.Failed {
		t.Fatal("retry limit not enforced", failed, err)
	}
	if _, err = reopened.Control(ctx, j.ID, "stop"); err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.Control(ctx, j.ID, "resume"); !errors.Is(err, autopilot.ErrConflict) {
		t.Fatal("terminal stop resumed", err)
	}
	if _, err = reopened.Enqueue(ctx, "p", "new", input()); err != nil {
		t.Fatal("terminal job retained project slot", err)
	}
}

type nopLease struct{}

func (nopLease) Close() error { return nil }

type blockingEngine struct {
	entered chan struct{}
	once    sync.Once
}

func (e *blockingEngine) Step(ctx context.Context, j autopilot.Job) (autopilot.Job, error) {
	e.once.Do(func() { close(e.entered) })
	<-ctx.Done()
	return j, ctx.Err()
}
func TestAutopilotWorkerLeaseAndShutdown(t *testing.T) {
	s := testStore(t)
	j, err := s.Enqueue(t.Context(), "p", "start", input())
	if err != nil {
		t.Fatal(err)
	}
	engine := &blockingEngine{entered: make(chan struct{})}
	acquire := func(context.Context, string) (io.Closer, error) { return nopLease{}, nil }
	r, err := autopilot.Start(s, engine, acquire)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	select {
	case <-engine.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not call engine")
	}
	if second, err := autopilot.Start(s, engine, acquire); err == nil {
		second.Close()
		t.Fatal("second worker acquired same workspace")
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(t.Context(), j.ID)
	if err != nil || got.State != autopilot.Pending || got.Stage != "foundation" {
		t.Fatal("shutdown lost durable cursor", got, err)
	}
	if _, err = s.Control(t.Context(), j.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	r2, err := autopilot.Start(s, engine, acquire)
	if err != nil {
		t.Fatal(err)
	}
	r2.Close()
}
