package stateless

import (
	"context"
	"testing"
)

func TestStateMachine_Fire_TriggerHandledOnSuperStateAndSubState_UsesSubstateTransition(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		SubstateOf(stateA).
		Permit(triggerX, stateC)

	sm.Fire(triggerX)
	if got := sm.MustState(); got != stateB {
		t.Errorf("sm.MustState() = %v, want %v", got, stateB)
	}

	sm.Fire(triggerX)
	if got := sm.MustState(); got != stateC {
		t.Errorf("sm.MustState() = %v, want %v", got, stateC)
	}
}

func TestStateMachine_Fire_TriggerHandledOnSuperStateAndSubState_SubstateGuardBlocked_UsesSuperstateTransition(t *testing.T) {
	guardConditionValue := false
	sm := NewStateMachine(stateB)

	sm.Configure(stateA).
		Permit(triggerX, stateD)

	sm.Configure(stateB).
		SubstateOf(stateA).
		Permit(triggerX, stateC, func(_ context.Context, _ ...any) bool {
			return guardConditionValue
		})

	sm.Fire(triggerX)
	if got := sm.MustState(); got != stateD {
		t.Errorf("sm.MustState() = %v, want %v", got, stateD)
	}
}

func TestStateMachine_Fire_TriggerHandledOnSuperStateAndSubState_SubstateGuardOpen_UsesSubstateTransition(t *testing.T) {
	guardConditionValue := true
	sm := NewStateMachine(stateB)

	sm.Configure(stateA).
		Permit(triggerX, stateD)

	sm.Configure(stateB).
		SubstateOf(stateA).
		Permit(triggerX, stateC, func(_ context.Context, _ ...any) bool {
			return guardConditionValue
		})

	sm.Fire(triggerX)
	if got := sm.MustState(); got != stateC {
		t.Errorf("sm.MustState() = %v, want %v", got, stateC)
	}
}

func TestStateMachine_InternalTransitionIf_ExecutesOnlyFirstMatchingAction(t *testing.T) {
	sm := NewStateMachine(1)
	executed := []int{}

	sm.Configure(1).
		InternalTransition(1, func(_ context.Context, _ ...any) error {
			executed = append(executed, 1)
			return nil
		}, func(_ context.Context, _ ...any) bool {
			return true
		}).
		InternalTransition(1, func(_ context.Context, _ ...any) error {
			executed = append(executed, 2)
			return nil
		}, func(_ context.Context, _ ...any) bool {
			return false
		})

	sm.Fire(1)

	if len(executed) != 1 || executed[0] != 1 {
		t.Errorf("expected only first action to execute, got executions: %v", executed)
	}
}

func TestStateMachine_Fire_MultiLayerSubstates_ClosestAncestorTransitionUsed(t *testing.T) {
	tests := []struct {
		name                          string
		parentGuardConditionValue     bool
		childGuardConditionValue      bool
		grandchildGuardConditionValue bool
		expectedState                 string
	}{
		{"GrandchildOpen", false, false, true, "GrandchildStateTarget"},
		{"ChildOpen_GrandchildClosed", false, true, false, "ChildStateTarget"},
		{"ChildOpen_GrandchildOpen", false, true, true, "GrandchildStateTarget"},
		{"ParentOpen_ChildClosed_GrandchildClosed", true, false, false, "ParentStateTarget"},
		{"ParentOpen_ChildClosed_GrandchildOpen", true, false, true, "GrandchildStateTarget"},
		{"ParentOpen_ChildOpen_GrandchildClosed", true, true, false, "ChildStateTarget"},
		{"AllOpen", true, true, true, "GrandchildStateTarget"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewStateMachine("GrandchildState")

			sm.Configure("ParentState").
				Permit(triggerX, "ParentStateTarget", func(_ context.Context, _ ...any) bool {
					return tt.parentGuardConditionValue
				})

			sm.Configure("ChildState").
				SubstateOf("ParentState").
				Permit(triggerX, "ChildStateTarget", func(_ context.Context, _ ...any) bool {
					return tt.childGuardConditionValue
				})

			sm.Configure("GrandchildState").
				SubstateOf("ChildState").
				Permit(triggerX, "GrandchildStateTarget", func(_ context.Context, _ ...any) bool {
					return tt.grandchildGuardConditionValue
				})

			sm.Fire(triggerX)
			if got := sm.MustState(); got != tt.expectedState {
				t.Errorf("sm.MustState() = %v, want %v", got, tt.expectedState)
			}
		})
	}
}
