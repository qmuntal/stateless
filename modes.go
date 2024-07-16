package stateless

import (
	"context"
	"sync"
	"sync/atomic"
)

type fireMode[T Trigger] interface {
	Fire(ctx context.Context, trigger T, args ...any) error
	Firing() bool
}

type fireModeImmediate[S State, T Trigger] struct {
	ops atomic.Uint64
	sm  *StateMachine[S, T]
}

func (f *fireModeImmediate[_, _]) Firing() bool {
	return f.ops.Load() > 0
}

func (f *fireModeImmediate[_, T]) Fire(ctx context.Context, trigger T, args ...any) error {
	f.ops.Add(1)
	defer f.ops.Add(^uint64(0))
	return f.sm.internalFireOne(ctx, trigger, args...)
}

type queuedTrigger[T Trigger] struct {
	Context context.Context
	Trigger T
	Args    []any
}

type fireModeQueued[S State, T Trigger] struct {
	firing atomic.Bool
	sm     *StateMachine[S, T]

	triggers []queuedTrigger[T]
	mu       sync.Mutex // guards triggers
}

func (f *fireModeQueued[_, _]) Firing() bool {
	return f.firing.Load()
}

func (f *fireModeQueued[_, T]) Fire(ctx context.Context, trigger T, args ...any) error {
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

func (f *fireModeQueued[_, T]) enqueue(ctx context.Context, trigger T, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.triggers = append(f.triggers, queuedTrigger[T]{Context: ctx, Trigger: trigger, Args: args})
}

func (f *fireModeQueued[S, T]) fetch() (et queuedTrigger[T], ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.triggers) == 0 {
		return queuedTrigger[T]{}, false
	}

	if !f.firing.CompareAndSwap(false, true) {
		return queuedTrigger[T]{}, false
	}

	et, f.triggers = f.triggers[0], f.triggers[1:]
	return et, true
}

func (f *fireModeQueued[S, T]) execute(et queuedTrigger[T]) error {
	defer f.firing.Swap(false)
	return f.sm.internalFireOne(et.Context, et.Trigger, et.Args...)
}
