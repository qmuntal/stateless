package stateless

import (
	"context"
	"sync"
	"sync/atomic"
)

type fireMode[T Trigger, A any] interface {
	Fire(ctx context.Context, trigger T, arg A) error
	Firing() bool
}

type fireModeImmediate[S State, T Trigger, A any] struct {
	ops atomic.Uint64
	sm  *StateMachine[S, T, A]
}

func (f *fireModeImmediate[_, _, _]) Firing() bool {
	return f.ops.Load() > 0
}

func (f *fireModeImmediate[_, T, A]) Fire(ctx context.Context, trigger T, arg A) error {
	f.ops.Add(1)
	defer f.ops.Add(^uint64(0))
	return f.sm.internalFireOne(ctx, trigger, arg)
}

type queuedTrigger[T Trigger, A any] struct {
	Context context.Context
	Trigger T
	Arg     A
}

type fireModeQueued[S State, T Trigger, A any] struct {
	firing atomic.Bool
	sm     *StateMachine[S, T, A]

	triggers []queuedTrigger[T, A]
	mu       sync.Mutex // guards triggers
}

func (f *fireModeQueued[_, _, _]) Firing() bool {
	return f.firing.Load()
}

func (f *fireModeQueued[_, T, A]) Fire(ctx context.Context, trigger T, arg A) error {
	f.enqueue(ctx, trigger, arg)
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

func (f *fireModeQueued[_, T, A]) enqueue(ctx context.Context, trigger T, arg A) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.triggers = append(f.triggers, queuedTrigger[T, A]{Context: ctx, Trigger: trigger, Arg: arg})
}

func (f *fireModeQueued[S, T, A]) fetch() (et queuedTrigger[T, A], ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.triggers) == 0 {
		return queuedTrigger[T, A]{}, false
	}

	if !f.firing.CompareAndSwap(false, true) {
		return queuedTrigger[T, A]{}, false
	}

	et, f.triggers = f.triggers[0], f.triggers[1:]
	return et, true
}

func (f *fireModeQueued[S, T, A]) execute(et queuedTrigger[T, A]) error {
	defer f.firing.Swap(false)
	return f.sm.internalFireOne(et.Context, et.Trigger, et.Arg)
}
