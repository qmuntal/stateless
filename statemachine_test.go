package stateless

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

const (
	stateA = "A"
	stateB = "B"
	stateC = "C"
	stateD = "D"

	triggerX = "X"
	triggerY = "Y"
	triggerZ = "Z"
)

func TestTransition_IsReentry(t *testing.T) {
	tests := []struct {
		name string
		t    *Transition
		want bool
	}{
		{"TransitionIsNotChange", &Transition{"1", "1", "0", false}, true},
		{"TransitionIsChange", &Transition{"1", "2", "0", false}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.t.IsReentry(); got != tt.want {
				t.Errorf("Transition.IsReentry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStateMachine_NewStateMachine(t *testing.T) {
	sm := NewStateMachine(stateA)
	if got := sm.MustState(); got != stateA {
		t.Errorf("MustState() = %v, want %v", got, stateA)
	}
}

func TestStateMachine_NewStateMachineWithExternalStorage(t *testing.T) {
	var state State = stateB
	sm := NewStateMachineWithExternalStorage(func(_ context.Context) (State, error) {
		return state, nil
	}, func(_ context.Context, s State) error {
		state = s
		return nil
	}, FiringImmediate)
	sm.Configure(stateB).Permit(triggerX, stateC)
	if got := sm.MustState(); got != stateB {
		t.Errorf("MustState() = %v, want %v", got, stateB)
	}
	if state != stateB {
		t.Errorf("expected state to be %v, got %v", stateB, state)
	}
	sm.Fire(triggerX)
	if got := sm.MustState(); got != stateC {
		t.Errorf("MustState() = %v, want %v", got, stateC)
	}
	if state != stateC {
		t.Errorf("expected state to be %v, got %v", stateC, state)
	}
}

func TestStateMachine_Configure_SubstateIsIncludedInCurrentState(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).SubstateOf(stateC)
	if ok, _ := sm.IsInState(stateC); !ok {
		t.Errorf("IsInState() = %v, want %v", ok, true)
	}

	if got := sm.MustState(); got != stateB {
		t.Errorf("MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_Configure_InSubstate_TriggerIgnoredInSuperstate_RemainsInSubstate(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).SubstateOf(stateC)
	sm.Configure(stateC).Ignore(triggerX)
	sm.Fire(triggerX)

	if got := sm.MustState(); got != stateB {
		t.Errorf("MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_CanFire(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).Permit(triggerX, stateA)
	if ok, _ := sm.CanFire(triggerX); !ok {
		t.Errorf("CanFire() = %v, want %v", ok, true)
	}
	if ok, _ := sm.CanFire(triggerY); ok {
		t.Errorf("CanFire() = %v, want %v", ok, false)
	}
}

func TestStateMachine_CanFire_StatusError(t *testing.T) {
	sm := NewStateMachineWithExternalStorage(func(_ context.Context) (State, error) {
		return nil, errors.New("status error")
	}, func(_ context.Context, s State) error { return nil }, FiringImmediate)

	sm.Configure(stateB).Permit(triggerX, stateA)

	ok, err := sm.CanFire(triggerX)
	if ok {
		t.Fail()
	}
	want := "status error"
	if err == nil || err.Error() != want {
		t.Errorf("CanFire() = %v, want %v", err, want)
	}
}

func TestStateMachine_IsInState_StatusError(t *testing.T) {
	sm := NewStateMachineWithExternalStorage(func(_ context.Context) (State, error) {
		return nil, errors.New("status error")
	}, func(_ context.Context, s State) error { return nil }, FiringImmediate)

	ok, err := sm.IsInState(stateA)
	if ok {
		t.Fail()
	}
	want := "status error"
	if err == nil || err.Error() != want {
		t.Errorf("IsInState() = %v, want %v", err, want)
	}
}

func TestStateMachine_Activate_StatusError(t *testing.T) {
	sm := NewStateMachineWithExternalStorage(func(_ context.Context) (State, error) {
		return nil, errors.New("status error")
	}, func(_ context.Context, s State) error { return nil }, FiringImmediate)

	want := "status error"
	if err := sm.Activate(); err == nil || err.Error() != want {
		t.Errorf("Activate() = %v, want %v", err, want)
	}
	if err := sm.Deactivate(); err == nil || err.Error() != want {
		t.Errorf("Deactivate() = %v, want %v", err, want)
	}
}

func TestStateMachine_PermittedTriggers_StatusError(t *testing.T) {
	sm := NewStateMachineWithExternalStorage(func(_ context.Context) (State, error) {
		return nil, errors.New("status error")
	}, func(_ context.Context, s State) error { return nil }, FiringImmediate)

	want := "status error"
	if _, err := sm.PermittedTriggers(); err == nil || err.Error() != want {
		t.Errorf("PermittedTriggers() = %v, want %v", err, want)
	}
}

func TestStateMachine_MustState_StatusError(t *testing.T) {
	sm := NewStateMachineWithExternalStorage(func(_ context.Context) (State, error) {
		return nil, errors.New("")
	}, func(_ context.Context, s State) error { return nil }, FiringImmediate)

	assertPanic(t, func() { sm.MustState() })
}

func TestStateMachine_Fire_StatusError(t *testing.T) {
	sm := NewStateMachineWithExternalStorage(func(_ context.Context) (State, error) {
		return nil, errors.New("status error")
	}, func(_ context.Context, s State) error { return nil }, FiringImmediate)

	want := "status error"
	if err := sm.Fire(triggerX); err == nil || err.Error() != want {
		t.Errorf("Fire() = %v, want %v", err, want)
	}
}

func TestStateMachine_Configure_PermittedTriggersIncludeSuperstatePermittedTriggers(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateA).Permit(triggerZ, stateB)
	sm.Configure(stateB).SubstateOf(stateC).Permit(triggerX, stateA)
	sm.Configure(stateC).Permit(triggerY, stateA)

	permitted, _ := sm.PermittedTriggers(context.Background())

	var hasX, hasY, hasZ bool
	for _, trigger := range permitted {
		if trigger == triggerX {
			hasX = true
		}
		if trigger == triggerY {
			hasY = true
		}
		if trigger == triggerZ {
			hasZ = true
		}
	}
	if !hasX {
		t.Errorf("expected permitted triggers to include %v", triggerX)
	}
	if !hasY {
		t.Errorf("expected permitted triggers to include %v", triggerY)
	}
	if hasZ {
		t.Errorf("expected permitted triggers to exclude %v", triggerZ)
	}
}

func TestStateMachine_PermittedTriggers_PermittedTriggersAreDistinctValues(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).SubstateOf(stateC).Permit(triggerX, stateA)
	sm.Configure(stateC).Permit(triggerX, stateB)

	permitted, _ := sm.PermittedTriggers(context.Background())

	if got := len(permitted); got != 1 {
		t.Fatalf("PermittedTriggers() = %v, want %v", got, 1)
	}
	if got := permitted[0]; got != triggerX {
		t.Errorf("PermittedTriggers() = %v, want %v", got, triggerX)
	}
}

func TestStateMachine_PermittedTriggers_AcceptedTriggersRespectGuards(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).Permit(triggerX, stateA, func(_ context.Context, _ ...any) bool {
		return false
	})

	permitted, _ := sm.PermittedTriggers(context.Background())

	if got := len(permitted); got != 0 {
		t.Errorf("PermittedTriggers() = %v, want %v", got, 0)
	}
}

func TestStateMachine_PermittedTriggers_AcceptedTriggersRespectMultipleGuards(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).Permit(triggerX, stateA, func(_ context.Context, _ ...any) bool {
		return true
	}, func(_ context.Context, _ ...any) bool {
		return false
	})

	permitted, _ := sm.PermittedTriggers(context.Background())

	if got := len(permitted); got != 0 {
		t.Errorf("PermittedTriggers() = %v, want %v", got, 0)
	}
}

func TestStateMachine_Fire_DiscriminatedByGuard_ChoosesPermitedTransition(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).
		Permit(triggerX, stateA, func(_ context.Context, _ ...any) bool {
			return false
		}).
		Permit(triggerX, stateC, func(_ context.Context, _ ...any) bool {
			return true
		})

	sm.Fire(triggerX)

	if got := sm.MustState(); got != stateC {
		t.Errorf("MustState() = %v, want %v", got, stateC)
	}
}

func TestStateMachine_Fire_SaveError(t *testing.T) {
	sm := NewStateMachineWithExternalStorage(func(_ context.Context) (State, error) {
		return stateB, nil
	}, func(_ context.Context, s State) error { return errors.New("status error") }, FiringImmediate)

	sm.Configure(stateB).
		Permit(triggerX, stateA)

	want := "status error"
	if err := sm.Fire(triggerX); err == nil || err.Error() != want {
		t.Errorf("Fire() = %v, want %v", err, want)
	}
	if sm.MustState() != stateB {
		t.Errorf("MustState() = %v, want %v", sm.MustState(), stateB)
	}
}

func TestStateMachine_Fire_TriggerIsIgnored_ActionsNotExecuted(t *testing.T) {
	fired := false
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...any) error {
			fired = true
			return nil
		}).
		Ignore(triggerX)

	sm.Fire(triggerX)

	if fired {
		t.Error("actions were executed")
	}
}

func TestStateMachine_Fire_SelfTransitionPermited_ActionsFire(t *testing.T) {
	fired := false
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...any) error {
			fired = true
			return nil
		}).
		PermitReentry(triggerX)

	sm.Fire(triggerX)
	if !fired {
		t.Error("actions did not fire")
	}
}

func TestStateMachine_Fire_ImplicitReentryIsDisallowed(t *testing.T) {
	sm := NewStateMachine(stateB)
	assertPanic(t, func() {
		sm.Configure(stateB).
			Permit(triggerX, stateB)
	})
}

func TestStateMachine_Fire_ErrorForInvalidTransition(t *testing.T) {
	sm := NewStateMachine(stateA)
	if err := sm.Fire(triggerX); err == nil {
		t.Error("error expected")
	}
}

func TestStateMachine_Fire_ErrorForInvalidTransitionMentionsGuardDescriptionIfPresent(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.Configure(stateA).Permit(triggerX, stateB, func(_ context.Context, _ ...any) bool {
		return false
	})
	if err := sm.Fire(triggerX); err == nil {
		t.Error("error expected")
	}
}

func TestStateMachine_Fire_ParametersSuppliedToFireArePassedToEntryAction(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0))
	sm.Configure(stateB).Permit(triggerX, stateC)

	var (
		entryArg1 string
		entryArg2 int
	)
	sm.Configure(stateC).OnEntryFrom(triggerX, func(_ context.Context, args ...any) error {
		entryArg1 = args[0].(string)
		entryArg2 = args[1].(int)
		return nil
	})
	suppliedArg1, suppliedArg2 := "something", 2
	sm.Fire(triggerX, suppliedArg1, suppliedArg2)

	if entryArg1 != suppliedArg1 {
		t.Errorf("entryArg1 = %v, want %v", entryArg1, suppliedArg1)
	}
	if entryArg2 != suppliedArg2 {
		t.Errorf("entryArg2 = %v, want %v", entryArg2, suppliedArg2)
	}
}

func TestStateMachine_OnUnhandledTrigger_TheProvidedHandlerIsCalledWithStateAndTrigger(t *testing.T) {
	sm := NewStateMachine(stateB)
	var (
		unhandledState   State
		unhandledTrigger Trigger
	)
	sm.OnUnhandledTrigger(func(_ context.Context, state State, trigger Trigger, unmetGuards []string) error {
		unhandledState = state
		unhandledTrigger = trigger
		return nil
	})

	sm.Fire(triggerZ)

	if stateB != unhandledState {
		t.Errorf("unhandledState = %v, want %v", unhandledState, stateB)
	}
	if triggerZ != unhandledTrigger {
		t.Errorf("unhandledTrigger = %v, want %v", unhandledTrigger, triggerZ)
	}
}

func TestStateMachine_SetTriggerParameters_TriggerParametersAreImmutableOnceSet(t *testing.T) {
	sm := NewStateMachine(stateB)

	sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0))

	assertPanic(t, func() { sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0)) })
}

func TestStateMachine_SetTriggerParameters_Interfaces(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf((*error)(nil)).Elem())

	sm.Configure(stateB).Permit(triggerX, stateA)
	defer func() {
		if r := recover(); r != nil {
			t.Error("panic not expected")
		}
	}()
	sm.Fire(triggerX, errors.New("failed"))
}

func TestStateMachine_SetTriggerParameters_Invalid(t *testing.T) {
	sm := NewStateMachine(stateB)

	sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0))
	sm.Configure(stateB).Permit(triggerX, stateA)

	assertPanic(t, func() { sm.Fire(triggerX) })
	assertPanic(t, func() { sm.Fire(triggerX, "1", "2", "3") })
	assertPanic(t, func() { sm.Fire(triggerX, "1", "2") })
}

func TestStateMachine_OnTransitioning_EventFires(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).Permit(triggerX, stateA)

	var transition Transition
	sm.OnTransitioning(func(_ context.Context, tr Transition) {
		transition = tr
	})
	sm.Fire(triggerX)

	want := Transition{
		Source:      stateB,
		Destination: stateA,
		Trigger:     triggerX,
	}
	if !reflect.DeepEqual(transition, want) {
		t.Errorf("transition = %v, want %v", transition, want)
	}
}

func TestStateMachine_OnTransitioned_EventFires(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).Permit(triggerX, stateA)

	var transition Transition
	sm.OnTransitioned(func(_ context.Context, tr Transition) {
		transition = tr
	})
	sm.Fire(triggerX)

	want := Transition{
		Source:      stateB,
		Trigger:     triggerX,
		Destination: stateA,
	}
	if !reflect.DeepEqual(transition, want) {
		t.Errorf("transition = %v, want %v", transition, want)
	}
}

func TestStateMachine_OnTransitioned_EventFiresBeforeTheOnEntryEvent(t *testing.T) {
	sm := NewStateMachine(stateB)
	expectedOrdering := []string{"OnExit", "OnTransitioning", "OnEntry", "OnTransitioned"}
	var actualOrdering []string

	sm.Configure(stateB).Permit(triggerX, stateA).OnExit(func(_ context.Context, args ...any) error {
		actualOrdering = append(actualOrdering, "OnExit")
		return nil
	}).Machine()

	var transition Transition
	sm.Configure(stateA).OnEntry(func(ctx context.Context, args ...any) error {
		actualOrdering = append(actualOrdering, "OnEntry")
		transition = GetTransition(ctx)
		return nil
	})

	sm.OnTransitioning(func(_ context.Context, tr Transition) {
		actualOrdering = append(actualOrdering, "OnTransitioning")
	})
	sm.OnTransitioned(func(_ context.Context, tr Transition) {
		actualOrdering = append(actualOrdering, "OnTransitioned")
	})

	sm.Fire(triggerX)

	if !reflect.DeepEqual(actualOrdering, expectedOrdering) {
		t.Errorf("actualOrdering = %v, want %v", actualOrdering, expectedOrdering)
	}

	want := Transition{
		Source:      stateB,
		Destination: stateA,
		Trigger:     triggerX,
	}
	if !reflect.DeepEqual(transition, want) {
		t.Errorf("transition = %v, want %v", transition, want)
	}
}

func TestStateMachine_SubstateOf_DirectCyclicConfigurationDetected(t *testing.T) {
	sm := NewStateMachine(stateA)
	assertPanic(t, func() { sm.Configure(stateA).SubstateOf(stateA) })
}

func TestStateMachine_SubstateOf_NestedCyclicConfigurationDetected(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.Configure(stateB).SubstateOf(stateA)
	assertPanic(t, func() { sm.Configure(stateA).SubstateOf(stateB) })
}

func TestStateMachine_SubstateOf_NestedTwoLevelsCyclicConfigurationDetected(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.Configure(stateB).SubstateOf(stateA)
	sm.Configure(stateC).SubstateOf(stateB)
	assertPanic(t, func() { sm.Configure(stateA).SubstateOf(stateC) })
}

func TestStateMachine_SubstateOf_DelayedNestedCyclicConfigurationDetected(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.Configure(stateB).SubstateOf(stateA)
	sm.Configure(stateC)
	sm.Configure(stateA).SubstateOf(stateC)
	assertPanic(t, func() { sm.Configure(stateC).SubstateOf(stateB) })
}

func TestStateMachine_Fire_IgnoreVsPermitReentry(t *testing.T) {
	sm := NewStateMachine(stateA)
	var calls int
	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ ...any) error {
			calls += 1
			return nil
		}).
		PermitReentry(triggerX).
		Ignore(triggerY)

	sm.Fire(triggerX)
	sm.Fire(triggerY)

	if calls != 1 {
		t.Errorf("calls = %d, want %d", calls, 1)
	}
}

func TestStateMachine_Fire_IgnoreVsPermitReentryFrom(t *testing.T) {
	sm := NewStateMachine(stateA)
	var calls int
	sm.Configure(stateA).
		OnEntryFrom(triggerX, func(_ context.Context, _ ...any) error {
			calls += 1
			return nil
		}).
		OnEntryFrom(triggerY, func(_ context.Context, _ ...any) error {
			calls += 1
			return nil
		}).
		PermitReentry(triggerX).
		Ignore(triggerY)

	sm.Fire(triggerX)
	sm.Fire(triggerY)

	if calls != 1 {
		t.Errorf("calls = %d, want %d", calls, 1)
	}
}

func TestStateMachine_Fire_IfSelfTransitionPermited_ActionsFire_InSubstate(t *testing.T) {
	sm := NewStateMachine(stateA)
	var onEntryStateBfired, onExitStateBfired, onExitStateAfired bool
	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...any) error {
			onEntryStateBfired = true
			return nil
		}).
		PermitReentry(triggerX).
		OnExit(func(_ context.Context, _ ...any) error {
			onExitStateBfired = true
			return nil
		})

	sm.Configure(stateA).
		SubstateOf(stateB).
		OnExit(func(_ context.Context, _ ...any) error {
			onExitStateAfired = true
			return nil
		})

	sm.Fire(triggerX)

	if got := sm.MustState(); got != stateB {
		t.Errorf("sm.MustState() = %v, want %v", got, stateB)
	}
	if !onEntryStateBfired {
		t.Error("OnEntryStateB was not fired")
	}
	if !onExitStateBfired {
		t.Error("OnExitStateB was not fired")
	}
	if !onExitStateAfired {
		t.Error("OnExitStateA was not fired")
	}
}

func TestStateMachine_Fire_TransitionWhenParameterizedGuardTrue(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(0))
	sm.Configure(stateA).
		Permit(triggerX, stateB, func(_ context.Context, args ...any) bool {
			return args[0].(int) == 2
		})

	sm.Fire(triggerX, 2)

	if got := sm.MustState(); got != stateB {
		t.Errorf("sm.MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_Fire_ErrorWhenParameterizedGuardFalse(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(0))
	sm.Configure(stateA).
		Permit(triggerX, stateB, func(_ context.Context, args ...any) bool {
			return args[0].(int) == 3
		})

	sm.Fire(triggerX, 2)
	if err := sm.Fire(triggerX, 2); err == nil {
		t.Error("error expected")
	}
}

func TestStateMachine_Fire_TransitionWhenBothParameterizedGuardClausesTrue(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(0))
	sm.Configure(stateA).
		Permit(triggerX, stateB, func(_ context.Context, args ...any) bool {
			return args[0].(int) == 2
		}, func(_ context.Context, args ...any) bool {
			return args[0].(int) != 3
		})

	sm.Fire(triggerX, 2)

	if got := sm.MustState(); got != stateB {
		t.Errorf("sm.MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_Fire_TransitionWhenGuardReturnsTrueOnTriggerWithMultipleParameters(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0))
	sm.Configure(stateA).
		Permit(triggerX, stateB, func(_ context.Context, args ...any) bool {
			return args[0].(string) == "3" && args[1].(int) == 2
		})

	sm.Fire(triggerX, "3", 2)

	if got := sm.MustState(); got != stateB {
		t.Errorf("sm.MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_Fire_TransitionWhenPermitDyanmicIfHasMultipleExclusiveGuards(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(0))
	sm.Configure(stateA).
		PermitDynamic(triggerX, func(_ context.Context, args ...any) (State, error) {
			if args[0].(int) == 3 {
				return stateB, nil
			}
			return stateC, nil
		}, func(_ context.Context, args ...any) bool { return args[0].(int) == 3 || args[0].(int) == 5 }).
		PermitDynamic(triggerX, func(_ context.Context, args ...any) (State, error) {
			if args[0].(int) == 2 {
				return stateC, nil
			}
			return stateD, nil
		}, func(_ context.Context, args ...any) bool { return args[0].(int) == 2 || args[0].(int) == 4 })

	sm.Fire(triggerX, 3)

	if got := sm.MustState(); got != stateB {
		t.Errorf("sm.MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_Fire_PermitDyanmic_Error(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.Configure(stateA).
		PermitDynamic(triggerX, func(_ context.Context, _ ...any) (State, error) {
			return nil, errors.New("")
		})

	if err := sm.Fire(triggerX, ""); err == nil {
		t.Error("error expected")
	}
	if got := sm.MustState(); got != stateA {
		t.Errorf("sm.MustState() = %v, want %v", got, stateA)
	}
}

func TestStateMachine_Fire_PanicsWhenPermitDyanmicIfHasMultipleNonExclusiveGuards(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(0))
	sm.Configure(stateA).
		PermitDynamic(triggerX, func(_ context.Context, args ...any) (State, error) {
			if args[0].(int) == 4 {
				return stateB, nil
			}
			return stateC, nil
		}, func(_ context.Context, args ...any) bool { return args[0].(int)%2 == 0 }).
		PermitDynamic(triggerX, func(_ context.Context, args ...any) (State, error) {
			if args[0].(int) == 2 {
				return stateC, nil
			}
			return stateD, nil
		}, func(_ context.Context, args ...any) bool { return args[0].(int) == 2 })

	assertPanic(t, func() { sm.Fire(triggerX, 2) })
}

func TestStateMachine_Fire_TransitionWhenPermitIfHasMultipleExclusiveGuardsWithSuperStateTrue(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(0))
	sm.Configure(stateA).
		Permit(triggerX, stateD, func(_ context.Context, args ...any) bool {
			return args[0].(int) == 3
		})

	sm.Configure(stateB).
		SubstateOf(stateA).
		Permit(triggerX, stateC, func(_ context.Context, args ...any) bool {
			return args[0].(int) == 2
		})

	sm.Fire(triggerX, 3)

	if got := sm.MustState(); got != stateD {
		t.Errorf("sm.MustState() = %v, want %v", got, stateD)
	}
}

func TestStateMachine_Fire_TransitionWhenPermitIfHasMultipleExclusiveGuardsWithSuperStateFalse(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(0))
	sm.Configure(stateA).
		Permit(triggerX, stateD, func(_ context.Context, args ...any) bool {
			return args[0].(int) == 3
		})

	sm.Configure(stateB).
		SubstateOf(stateA).
		Permit(triggerX, stateC, func(_ context.Context, args ...any) bool {
			return args[0].(int) == 2
		})

	sm.Fire(triggerX, 2)

	if got := sm.MustState(); got != stateC {
		t.Errorf("sm.MustState() = %v, want %v", got, stateC)
	}
}

func TestStateMachine_Fire_TransitionToSuperstateDoesNotExitSuperstate(t *testing.T) {
	sm := NewStateMachine(stateB)
	var superExit, superEntry, subExit bool
	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ ...any) error {
			superEntry = true
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			superExit = true
			return nil
		})

	sm.Configure(stateB).
		SubstateOf(stateA).
		Permit(triggerY, stateA).
		OnExit(func(_ context.Context, _ ...any) error {
			subExit = true
			return nil
		})

	sm.Fire(triggerY)

	if !subExit {
		t.Error("substate should exit")
	}
	if superEntry {
		t.Error("superstate should not enter")
	}
	if superExit {
		t.Error("superstate should not exit")
	}
}

func TestStateMachine_Fire_OnExitFiresOnlyOnceReentrySubstate(t *testing.T) {
	sm := NewStateMachine(stateA)
	var exitB, exitA, entryB, entryA int
	sm.Configure(stateA).
		SubstateOf(stateB).
		OnEntry(func(_ context.Context, _ ...any) error {
			entryA += 1
			return nil
		}).
		PermitReentry(triggerX).
		OnExit(func(_ context.Context, _ ...any) error {
			exitA += 1
			return nil
		})

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...any) error {
			entryB += 1
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			exitB += 1
			return nil
		})

	sm.Fire(triggerX)

	if entryB != 0 {
		t.Error("entryB should be 0")
	}
	if exitB != 0 {
		t.Error("exitB should be 0")
	}
	if entryA != 1 {
		t.Error("entryA should be 1")
	}
	if exitA != 1 {
		t.Error("exitA should be 1")
	}
}

func TestStateMachine_Activate(t *testing.T) {
	sm := NewStateMachine(stateA)

	expectedOrdering := []string{"ActivatedC", "ActivatedA"}
	var actualOrdering []string

	sm.Configure(stateA).
		SubstateOf(stateC).
		OnActive(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "ActivatedA")
			return nil
		})

	sm.Configure(stateC).
		OnActive(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "ActivatedC")
			return nil
		})

	// should not be called for activation
	sm.OnTransitioning(func(_ context.Context, _ Transition) {
		actualOrdering = append(actualOrdering, "OnTransitioning")
	})
	sm.OnTransitioned(func(_ context.Context, _ Transition) {
		actualOrdering = append(actualOrdering, "OnTransitioned")
	})

	sm.Activate()

	if !reflect.DeepEqual(expectedOrdering, actualOrdering) {
		t.Errorf("expectedOrdering = %v, actualOrdering = %v", expectedOrdering, actualOrdering)
	}
}

func TestStateMachine_Activate_Error(t *testing.T) {
	sm := NewStateMachine(stateA)

	var actualOrdering []string

	sm.Configure(stateA).
		SubstateOf(stateC).
		OnActive(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "ActivatedA")
			return errors.New("")
		})

	sm.Configure(stateC).
		OnActive(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "ActivatedC")
			return nil
		})

	if err := sm.Activate(); err == nil {
		t.Error("error expected")
	}
}

func TestStateMachine_Activate_Idempotent(t *testing.T) {
	sm := NewStateMachine(stateA)

	var actualOrdering []string

	sm.Configure(stateA).
		SubstateOf(stateC).
		OnActive(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "ActivatedA")
			return nil
		})

	sm.Configure(stateC).
		OnActive(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "ActivatedC")
			return nil
		})

	sm.Activate()

	if got := len(actualOrdering); got != 2 {
		t.Errorf("expected 2, got %d", got)
	}
}

func TestStateMachine_Deactivate(t *testing.T) {
	sm := NewStateMachine(stateA)

	expectedOrdering := []string{"DeactivatedA", "DeactivatedC"}
	var actualOrdering []string

	sm.Configure(stateA).
		SubstateOf(stateC).
		OnDeactivate(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "DeactivatedA")
			return nil
		})

	sm.Configure(stateC).
		OnDeactivate(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "DeactivatedC")
			return nil
		})

	// should not be called for activation
	sm.OnTransitioning(func(_ context.Context, _ Transition) {
		actualOrdering = append(actualOrdering, "OnTransitioning")
	})
	sm.OnTransitioned(func(_ context.Context, _ Transition) {
		actualOrdering = append(actualOrdering, "OnTransitioned")
	})

	sm.Activate()
	sm.Deactivate()

	if !reflect.DeepEqual(expectedOrdering, actualOrdering) {
		t.Errorf("expectedOrdering = %v, actualOrdering = %v", expectedOrdering, actualOrdering)
	}
}

func TestStateMachine_Deactivate_NoActivated(t *testing.T) {
	sm := NewStateMachine(stateA)

	var actualOrdering []string

	sm.Configure(stateA).
		SubstateOf(stateC).
		OnDeactivate(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "DeactivatedA")
			return nil
		})

	sm.Configure(stateC).
		OnDeactivate(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "DeactivatedC")
			return nil
		})

	sm.Deactivate()

	want := []string{"DeactivatedA", "DeactivatedC"}
	if !reflect.DeepEqual(want, actualOrdering) {
		t.Errorf("want = %v, actualOrdering = %v", want, actualOrdering)
	}
}

func TestStateMachine_Deactivate_Error(t *testing.T) {
	sm := NewStateMachine(stateA)

	var actualOrdering []string

	sm.Configure(stateA).
		SubstateOf(stateC).
		OnDeactivate(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "DeactivatedA")
			return errors.New("")
		})

	sm.Configure(stateC).
		OnDeactivate(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "DeactivatedC")
			return nil
		})

	sm.Activate()
	if err := sm.Deactivate(); err == nil {
		t.Error("error expected")
	}
}

func TestStateMachine_Deactivate_Idempotent(t *testing.T) {
	sm := NewStateMachine(stateA)

	var actualOrdering []string

	sm.Configure(stateA).
		SubstateOf(stateC).
		OnDeactivate(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "DeactivatedA")
			return nil
		})

	sm.Configure(stateC).
		OnDeactivate(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "DeactivatedC")
			return nil
		})

	sm.Activate()
	sm.Deactivate()
	actualOrdering = make([]string, 0)
	sm.Activate()

	if got := len(actualOrdering); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestStateMachine_Activate_Transitioning(t *testing.T) {
	sm := NewStateMachine(stateA)

	var actualOrdering []string
	expectedOrdering := []string{"ActivatedA", "ExitedA", "OnTransitioning", "EnteredB", "OnTransitioned",
		"ExitedB", "OnTransitioning", "EnteredA", "OnTransitioned"}

	sm.Configure(stateA).
		OnActive(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "ActivatedA")
			return nil
		}).
		OnDeactivate(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "DeactivatedA")
			return nil
		}).
		OnEntry(func(_ context.Context, _ ...any) error {
			actualOrdering = append(actualOrdering, "EnteredA")
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			actualOrdering = append(actualOrdering, "ExitedA")
			return nil
		}).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnActive(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "ActivatedB")
			return nil
		}).
		OnDeactivate(func(_ context.Context) error {
			actualOrdering = append(actualOrdering, "DeactivatedB")
			return nil
		}).
		OnEntry(func(_ context.Context, _ ...any) error {
			actualOrdering = append(actualOrdering, "EnteredB")
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			actualOrdering = append(actualOrdering, "ExitedB")
			return nil
		}).
		Permit(triggerY, stateA)

	sm.OnTransitioning(func(_ context.Context, _ Transition) {
		actualOrdering = append(actualOrdering, "OnTransitioning")
	})
	sm.OnTransitioned(func(_ context.Context, _ Transition) {
		actualOrdering = append(actualOrdering, "OnTransitioned")
	})

	sm.Activate()
	sm.Fire(triggerX)
	sm.Fire(triggerY)

	if !reflect.DeepEqual(expectedOrdering, actualOrdering) {
		t.Errorf("expectedOrdering = %v, actualOrdering = %v", expectedOrdering, actualOrdering)
	}
}

func TestStateMachine_Fire_ImmediateEntryAProcessedBeforeEnterB(t *testing.T) {
	sm := NewStateMachineWithMode(stateA, FiringImmediate)

	var actualOrdering []string
	expectedOrdering := []string{"ExitA", "ExitB", "EnterA", "EnterB"}

	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ ...any) error {
			actualOrdering = append(actualOrdering, "EnterA")
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			actualOrdering = append(actualOrdering, "ExitA")
			return nil
		}).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...any) error {
			sm.Fire(triggerY)
			actualOrdering = append(actualOrdering, "EnterB")
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			actualOrdering = append(actualOrdering, "ExitB")
			return nil
		}).
		Permit(triggerY, stateA)

	sm.Fire(triggerX)

	if !reflect.DeepEqual(expectedOrdering, actualOrdering) {
		t.Errorf("expectedOrdering = %v, actualOrdering = %v", expectedOrdering, actualOrdering)
	}
}

func TestStateMachine_Fire_QueuedEntryAProcessedBeforeEnterB(t *testing.T) {
	sm := NewStateMachineWithMode(stateA, FiringQueued)

	var actualOrdering []string
	expectedOrdering := []string{"ExitA", "EnterB", "ExitB", "EnterA"}

	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ ...any) error {
			actualOrdering = append(actualOrdering, "EnterA")
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			actualOrdering = append(actualOrdering, "ExitA")
			return nil
		}).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...any) error {
			sm.Fire(triggerY)
			actualOrdering = append(actualOrdering, "EnterB")
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			actualOrdering = append(actualOrdering, "ExitB")
			return nil
		}).
		Permit(triggerY, stateA)

	sm.Fire(triggerX)

	if !reflect.DeepEqual(expectedOrdering, actualOrdering) {
		t.Errorf("expectedOrdering = %v, actualOrdering = %v", expectedOrdering, actualOrdering)
	}
}

func TestStateMachine_Fire_QueuedEntryAsyncFire(t *testing.T) {
	sm := NewStateMachineWithMode(stateA, FiringQueued)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...any) error {
			go sm.Fire(triggerY)
			go sm.Fire(triggerY)
			return nil
		}).
		Permit(triggerY, stateA)

	sm.Fire(triggerX)
}

func TestStateMachine_Fire_Race(t *testing.T) {
	sm := NewStateMachineWithMode(stateA, FiringImmediate)

	var actualOrdering []string
	var mu sync.Mutex
	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ ...any) error {
			mu.Lock()
			actualOrdering = append(actualOrdering, "EnterA")
			mu.Unlock()
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			mu.Lock()
			actualOrdering = append(actualOrdering, "ExitA")
			mu.Unlock()
			return nil
		}).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...any) error {
			sm.Fire(triggerY)
			mu.Lock()
			actualOrdering = append(actualOrdering, "EnterB")
			mu.Unlock()
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			mu.Lock()
			actualOrdering = append(actualOrdering, "ExitB")
			mu.Unlock()
			return nil
		}).
		Permit(triggerY, stateA)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		sm.Fire(triggerX)
		wg.Done()
	}()
	go func() {
		sm.Fire(triggerZ)
		wg.Done()
	}()
	wg.Wait()
	if got := len(actualOrdering); got != 4 {
		t.Errorf("expected 4, got %d", got)
	}
}

func TestStateMachine_Fire_Queued_ErrorExit(t *testing.T) {
	sm := NewStateMachineWithMode(stateA, FiringQueued)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...any) error {
			sm.Fire(triggerY)
			return nil
		}).
		OnExit(func(_ context.Context, _ ...any) error {
			return errors.New("")
		}).
		Permit(triggerY, stateA)

	sm.Fire(triggerX)

	if err := sm.Fire(triggerX); err == nil {
		t.Error("expected error")
	}
}

func TestStateMachine_Fire_Queued_ErrorEnter(t *testing.T) {
	sm := NewStateMachineWithMode(stateA, FiringQueued)

	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ ...any) error {
			return errors.New("")
		}).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...any) error {
			sm.Fire(triggerY)
			return nil
		}).
		Permit(triggerY, stateA)

	sm.Fire(triggerX)

	if err := sm.Fire(triggerX); err == nil {
		t.Error("expected error")
	}
}

func TestStateMachine_InternalTransition_StayInSameStateOneState(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.Configure(stateB).
		InternalTransition(triggerX, func(_ context.Context, _ ...any) error {
			return nil
		})

	sm.Fire(triggerX)
	if got := sm.MustState(); got != stateA {
		t.Errorf("expected %v, got %v", stateA, got)
	}
}

func TestStateMachine_InternalTransition_HandledOnlyOnceInSuper(t *testing.T) {
	sm := NewStateMachine(stateA)
	handledIn := stateC
	sm.Configure(stateA).
		InternalTransition(triggerX, func(_ context.Context, _ ...any) error {
			handledIn = stateA
			return nil
		})

	sm.Configure(stateB).
		SubstateOf(stateA).
		InternalTransition(triggerX, func(_ context.Context, _ ...any) error {
			handledIn = stateB
			return nil
		})

	sm.Fire(triggerX)
	if stateA != handledIn {
		t.Errorf("expected %v, got %v", stateA, handledIn)
	}
}

func TestStateMachine_InternalTransition_HandledOnlyOnceInSub(t *testing.T) {
	sm := NewStateMachine(stateB)
	handledIn := stateC
	sm.Configure(stateA).
		InternalTransition(triggerX, func(_ context.Context, _ ...any) error {
			handledIn = stateA
			return nil
		})

	sm.Configure(stateB).
		SubstateOf(stateA).
		InternalTransition(triggerX, func(_ context.Context, _ ...any) error {
			handledIn = stateB
			return nil
		})

	sm.Fire(triggerX)
	if stateB != handledIn {
		t.Errorf("expected %v, got %v", stateB, handledIn)
	}
}

func TestStateMachine_InitialTransition_EntersSubState(t *testing.T) {
	sm := NewStateMachine(stateA)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		InitialTransition(stateC)

	sm.Configure(stateC).
		SubstateOf(stateB)

	sm.Fire(triggerX)
	if got := sm.MustState(); got != stateC {
		t.Errorf("MustState() = %v, want %v", got, stateC)
	}
}

func TestStateMachine_InitialTransition_EntersSubStateofSubstate(t *testing.T) {
	sm := NewStateMachine(stateA)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		InitialTransition(stateC)

	sm.Configure(stateC).
		InitialTransition(stateD).
		SubstateOf(stateB)

	sm.Configure(stateD).
		SubstateOf(stateC)

	sm.Fire(triggerX)
	if got := sm.MustState(); got != stateD {
		t.Errorf("MustState() = %v, want %v", got, stateD)
	}
}

func TestStateMachine_InitialTransition_Ordering(t *testing.T) {
	var actualOrdering []string
	expectedOrdering := []string{"ExitA", "OnTransitioningAB", "EnterB", "OnTransitioningBC", "EnterC", "OnTransitionedAC"}

	sm := NewStateMachine(stateA)

	sm.Configure(stateA).
		Permit(triggerX, stateB).
		OnExit(func(c context.Context, i ...any) error {
			actualOrdering = append(actualOrdering, "ExitA")
			return nil
		})

	sm.Configure(stateB).
		InitialTransition(stateC).
		OnEntry(func(c context.Context, i ...any) error {
			actualOrdering = append(actualOrdering, "EnterB")
			return nil
		})

	sm.Configure(stateC).
		SubstateOf(stateB).
		OnEntry(func(c context.Context, i ...any) error {
			actualOrdering = append(actualOrdering, "EnterC")
			return nil
		})

	sm.OnTransitioning(func(_ context.Context, tr Transition) {
		actualOrdering = append(actualOrdering, fmt.Sprintf("OnTransitioning%v%v", tr.Source, tr.Destination))
	})
	sm.OnTransitioned(func(_ context.Context, tr Transition) {
		actualOrdering = append(actualOrdering, fmt.Sprintf("OnTransitioned%v%v", tr.Source, tr.Destination))
	})

	sm.Fire(triggerX)
	if got := sm.MustState(); got != stateC {
		t.Errorf("MustState() = %v, want %v", got, stateC)
	}

	if !reflect.DeepEqual(expectedOrdering, actualOrdering) {
		t.Errorf("expected %v, got %v", expectedOrdering, actualOrdering)
	}
}

func TestStateMachine_InitialTransition_DoesNotEnterSubStateofSubstate(t *testing.T) {
	sm := NewStateMachine(stateA)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		sm.Configure(stateC).
		InitialTransition(stateD).
		SubstateOf(stateB)

	sm.Configure(stateD).
		SubstateOf(stateC)

	sm.Fire(triggerX)
	if got := sm.MustState(); got != stateB {
		t.Errorf("MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_InitialTransition_DoNotAllowTransitionToSelf(t *testing.T) {
	sm := NewStateMachine(stateA)
	assertPanic(t, func() {
		sm.Configure(stateA).
			InitialTransition(stateA)
	})
}

func TestStateMachine_InitialTransition_WithMultipleSubStates(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.Configure(stateA).Permit(triggerX, stateB)
	sm.Configure(stateB).InitialTransition(stateC)
	sm.Configure(stateC).SubstateOf(stateB)
	sm.Configure(stateD).SubstateOf(stateB)
	if err := sm.Fire(triggerX); err != nil {
		t.Error(err)
	}
}

func TestStateMachine_InitialTransition_DoNotAllowTransitionToAnotherSuperstate(t *testing.T) {
	sm := NewStateMachine(stateA)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		InitialTransition(stateA)

	assertPanic(t, func() { sm.Fire(triggerX) })
}

func TestStateMachine_InitialTransition_DoNotAllowMoreThanOneInitialTransition(t *testing.T) {
	sm := NewStateMachine(stateA)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		InitialTransition(stateC)

	assertPanic(t, func() { sm.Configure(stateB).InitialTransition(stateA) })
}

func TestStateMachine_String(t *testing.T) {
	tests := []struct {
		name string
		sm   *StateMachine
		want string
	}{
		{"noTriggers", NewStateMachine(stateA), "StateMachine {{ State = A, PermittedTriggers = [] }}"},
		{"error state", NewStateMachineWithExternalStorage(func(_ context.Context) (State, error) {
			return nil, errors.New("status error")
		}, func(_ context.Context, s State) error { return nil }, FiringImmediate), ""},
		{"triggers", NewStateMachine(stateB).Configure(stateB).Permit(triggerX, stateA).Machine(),
			"StateMachine {{ State = B, PermittedTriggers = [X] }}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sm.String(); got != tt.want {
				t.Errorf("StateMachine.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStateMachine_Firing_Queued(t *testing.T) {
	sm := NewStateMachine(stateA)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(ctx context.Context, i ...any) error {
			if !sm.Firing() {
				t.Error("expected firing to be true")
			}
			return nil
		})
	if err := sm.Fire(triggerX); err != nil {
		t.Error(err)
	}
	if sm.Firing() {
		t.Error("expected firing to be false")
	}
}

func TestStateMachine_Firing_Immediate(t *testing.T) {
	sm := NewStateMachineWithMode(stateA, FiringImmediate)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(ctx context.Context, i ...any) error {
			if !sm.Firing() {
				t.Error("expected firing to be true")
			}
			return nil
		})
	if err := sm.Fire(triggerX); err != nil {
		t.Error(err)
	}
	if sm.Firing() {
		t.Error("expected firing to be false")
	}
}

func TestStateMachine_Firing_Concurrent(t *testing.T) {
	sm := NewStateMachine(stateA)

	sm.Configure(stateA).
		PermitReentry(triggerX).
		OnEntry(func(ctx context.Context, i ...any) error {
			if !sm.Firing() {
				t.Error("expected firing to be true")
			}
			return nil
		})

	var wg sync.WaitGroup
	wg.Add(1000)
	for i := 0; i < 1000; i++ {
		go func() {
			if err := sm.Fire(triggerX); err != nil {
				t.Error(err)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	if sm.Firing() {
		t.Error("expected firing to be false")
	}
}

func TestGetTransition_ContextEmpty(t *testing.T) {
	// It should not panic
	GetTransition(context.Background())
}

func assertPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("did not panic")
		}
	}()
	f()
}
