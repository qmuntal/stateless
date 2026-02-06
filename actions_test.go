package stateless

import (
	"context"
	"testing"
)

func TestStateMachine_Fire_IgnoredTriggerMustBeIgnoredInSubstate(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateA).
		Permit(triggerX, stateC)

	sm.Configure(stateB).
		SubstateOf(stateA).
		Ignore(triggerX)

	sm.Fire(triggerX)

	if got := sm.MustState(); got != stateB {
		t.Errorf("sm.MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_Fire_IgnoreIfTrue_TriggerMustBeIgnored(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateA).
		Permit(triggerX, stateC)

	sm.Configure(stateB).
		SubstateOf(stateA).
		Ignore(triggerX, func(_ context.Context, _ ...any) bool {
			return true
		})

	sm.Fire(triggerX)

	if got := sm.MustState(); got != stateB {
		t.Errorf("sm.MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_Fire_IgnoreIfFalse_TriggerMustNotBeIgnored(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateA).
		Permit(triggerX, stateC)

	sm.Configure(stateB).
		SubstateOf(stateA).
		Ignore(triggerX, func(_ context.Context, _ ...any) bool {
			return false
		})

	sm.Fire(triggerX)

	if got := sm.MustState(); got != stateC {
		t.Errorf("sm.MustState() = %v, want %v", got, stateC)
	}
}

func TestStateMachine_Fire_SuperStateShouldNotExitOnSubStateTransition(t *testing.T) {
	sm := NewStateMachine(stateA)
	record := []string{}

	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ ...any) error {
			record = append(record, "Entered state A")
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			record = append(record, "Exited state A")
			return nil
		}).
		Permit(triggerX, stateB)

	sm.Configure(stateB). // Our super state
				InitialTransition(stateC).
				OnEntry(func(_ context.Context, _ ...any) error {
			record = append(record, "Entered super state B")
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			record = append(record, "Exited super state B")
			return nil
		})

	sm.Configure(stateC). // Our first sub state
				SubstateOf(stateB).
				OnEntry(func(_ context.Context, _ ...any) error {
			record = append(record, "Entered sub state C")
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			record = append(record, "Exited sub state C")
			return nil
		}).
		Permit(triggerY, stateD)

	sm.Configure(stateD). // Our second sub state
				SubstateOf(stateB).
				OnEntry(func(_ context.Context, _ ...any) error {
			record = append(record, "Entered sub state D")
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			record = append(record, "Exited sub state D")
			return nil
		})

	sm.Fire(triggerX)
	sm.Fire(triggerY)

	expected := []string{
		"Exited state A",
		"Entered super state B",
		"Entered sub state C",
		"Exited sub state C",
		"Entered sub state D",
	}

	if len(record) != len(expected) {
		t.Errorf("record length = %v, want %v", len(record), len(expected))
		return
	}

	for i, v := range expected {
		if record[i] != v {
			t.Errorf("record[%d] = %v, want %v", i, record[i], v)
		}
	}
}
