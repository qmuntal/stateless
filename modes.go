package stateless

import (
	"context"
	"sync"
	"sync/atomic"
)

type fireMode interface {
	Fire(ctx context.Context, trigger Trigger, args ...any) error
	Firing() bool
}

type fireModeImmediate struct {
	ops atomic.Uint64
	sm  *StateMachine
}

func (f *fireModeImmediate) Firing() bool {
	return f.ops.Load() > 0
}

func (f *fireModeImmediate) Fire(ctx context.Context, trigger Trigger, args ...any) error {
	f.ops.Add(1)
	defer f.ops.Add(^uint64(0))
	return f.sm.internalFireOne(ctx, trigger, args...)
}

type queuedTrigger struct {
	Context context.Context
	Trigger Trigger
	Args    []any
}

type fireModeQueued struct {
	firing atomic.Bool
	sm     *StateMachine

	triggers []queuedTrigger
	mu       sync.Mutex // guards triggers
}

func (f *fireModeQueued) Firing() bool {
	return f.firing.Load()
}

func (f *fireModeQueued) Fire(ctx context.Context, trigger Trigger, args ...any) error {
	f.enqueue(ctx, trigger, args...)
	for {
		et, ok := f.fetch()
		if !ok {
			break
		}
		err := f.execute(et)
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *fireModeQueued) enqueue(ctx context.Context, trigger Trigger, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.triggers = append(f.triggers, queuedTrigger{Context: ctx, Trigger: trigger, Args: args})
}

func (f *fireModeQueued) fetch() (et queuedTrigger, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.triggers) == 0 {
		return queuedTrigger{}, false
	}

	if !f.firing.CompareAndSwap(false, true) {
		return queuedTrigger{}, false
	}

	et, f.triggers = f.triggers[0], f.triggers[1:]
	return et, true
}

func (f *fireModeQueued) execute(et queuedTrigger) error {
	defer f.firing.Swap(false)
	return f.sm.internalFireOne(et.Context, et.Trigger, et.Args...)
}
