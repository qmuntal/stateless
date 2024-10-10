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
		t    *Transition[string, string]
		want bool
	}{
		{"TransitionIsNotChange", &Transition[string, string]{"1", "1", "0", false}, true},
		{"TransitionIsChange", &Transition[string, string]{"1", "2", "0", false}, false},
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
	sm := NewStateMachine[string, string, any](stateA)
	if got := sm.MustState(); got != stateA {
		t.Errorf("MustState() = %v, want %v", got, stateA)
	}
}

func TestStateMachine_NewStateMachineWithExternalStorage(t *testing.T) {
	state := stateB
	sm := NewStateMachineWithExternalStorage[string, string, any](func(_ context.Context) (string, error) {
		return state, nil
	}, func(_ context.Context, s string) error {
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
	sm.Fire(triggerX, nil)
	if got := sm.MustState(); got != stateC {
		t.Errorf("MustState() = %v, want %v", got, stateC)
	}
	if state != stateC {
		t.Errorf("expected state to be %v, got %v", stateC, state)
	}
}

func TestStateMachine_Configure_SubstateIsIncludedInCurrentState(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
	sm.Configure(stateB).SubstateOf(stateC)
	if ok, _ := sm.IsInState(stateC); !ok {
		t.Errorf("IsInState() = %v, want %v", ok, true)
	}

	if got := sm.MustState(); got != stateB {
		t.Errorf("MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_Configure_InSubstate_TriggerIgnoredInSuperstate_RemainsInSubstate(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
	sm.Configure(stateB).SubstateOf(stateC)
	sm.Configure(stateC).Ignore(triggerX)
	sm.Fire(triggerX, nil)

	if got := sm.MustState(); got != stateB {
		t.Errorf("MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_CanFire(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
	sm.Configure(stateB).Permit(triggerX, stateA)
	if ok, _ := sm.CanFire(triggerX, nil); !ok {
		t.Errorf("CanFire() = %v, want %v", ok, true)
	}
	if ok, _ := sm.CanFire(triggerY, nil); ok {
		t.Errorf("CanFire() = %v, want %v", ok, false)
	}
}

func TestStateMachine_CanFire_StatusError(t *testing.T) {
	sm := NewStateMachineWithExternalStorage[string, string, any](func(_ context.Context) (string, error) {
		return "", errors.New("status error")
	}, func(_ context.Context, s string) error { return nil }, FiringImmediate)

	sm.Configure(stateB).Permit(triggerX, stateA)

	ok, err := sm.CanFire(triggerX, nil)
	if ok {
		t.Fail()
	}
	want := "status error"
	if err == nil || err.Error() != want {
		t.Errorf("CanFire() = %v, want %v", err, want)
	}
}

func TestStateMachine_IsInState_StatusError(t *testing.T) {
	sm := NewStateMachineWithExternalStorage[string, string, any](func(_ context.Context) (string, error) {
		return "", errors.New("status error")
	}, func(_ context.Context, s string) error { return nil }, FiringImmediate)

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
	sm := NewStateMachineWithExternalStorage[string, string, any](func(_ context.Context) (string, error) {
		return "", errors.New("status error")
	}, func(_ context.Context, s string) error { return nil }, FiringImmediate)

	want := "status error"
	if err := sm.Activate(); err == nil || err.Error() != want {
		t.Errorf("Activate() = %v, want %v", err, want)
	}
	if err := sm.Deactivate(); err == nil || err.Error() != want {
		t.Errorf("Deactivate() = %v, want %v", err, want)
	}
}

func TestStateMachine_PermittedTriggers_StatusError(t *testing.T) {
	sm := NewStateMachineWithExternalStorage[string, string, any](func(_ context.Context) (string, error) {
		return "", errors.New("status error")
	}, func(_ context.Context, s string) error { return nil }, FiringImmediate)

	want := "status error"
	if _, err := sm.PermittedTriggers(nil); err == nil || err.Error() != want {
		t.Errorf("PermittedTriggers() = %v, want %v", err, want)
	}
}

func TestStateMachine_MustState_StatusError(t *testing.T) {
	sm := NewStateMachineWithExternalStorage[string, string, any](func(_ context.Context) (string, error) {
		return "", errors.New("")
	}, func(_ context.Context, s string) error { return nil }, FiringImmediate)

	assertPanic(t, func() { sm.MustState() })
}

func TestStateMachine_Fire_StatusError(t *testing.T) {
	sm := NewStateMachineWithExternalStorage[string, string, any](func(_ context.Context) (string, error) {
		return "", errors.New("status error")
	}, func(_ context.Context, s string) error { return nil }, FiringImmediate)

	want := "status error"
	if err := sm.Fire(triggerX, nil); err == nil || err.Error() != want {
		t.Errorf("Fire() = %v, want %v", err, want)
	}
}

func TestStateMachine_Configure_PermittedTriggersIncludeSuperstatePermittedTriggers(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
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
	sm := NewStateMachine[string, string, any](stateB)
	sm.Configure(stateB).SubstateOf(stateC).Permit(triggerX, stateA)
	sm.Configure(stateC).Permit(triggerX, stateB)

	permitted, _ := sm.PermittedTriggers(context.Background())

	want := []string{triggerX}
	if !reflect.DeepEqual(permitted, want) {
		t.Errorf("PermittedTriggers() = %v, want %v", permitted, want)
	}
}

func TestStateMachine_PermittedTriggers_AcceptedTriggersRespectGuards(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
	sm.Configure(stateB).Permit(triggerX, stateA, func(_ context.Context, _ any) bool {
		return false
	})

	permitted, _ := sm.PermittedTriggers(context.Background())

	if got := len(permitted); got != 0 {
		t.Errorf("PermittedTriggers() = %v, want %v", got, 0)
	}
}

func TestStateMachine_PermittedTriggers_AcceptedTriggersRespectMultipleGuards(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
	sm.Configure(stateB).Permit(triggerX, stateA, func(_ context.Context, _ any) bool {
		return true
	}, func(_ context.Context, _ any) bool {
		return false
	})

	permitted, _ := sm.PermittedTriggers(context.Background())

	if got := len(permitted); got != 0 {
		t.Errorf("PermittedTriggers() = %v, want %v", got, 0)
	}
}

func TestStateMachine_Fire_DiscriminatedByGuard_ChoosesPermitedTransition(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
	sm.Configure(stateB).
		Permit(triggerX, stateA, func(_ context.Context, _ any) bool {
			return false
		}).
		Permit(triggerX, stateC, func(_ context.Context, _ any) bool {
			return true
		})

	sm.Fire(triggerX, nil)

	if got := sm.MustState(); got != stateC {
		t.Errorf("MustState() = %v, want %v", got, stateC)
	}
}

func TestStateMachine_Fire_SaveError(t *testing.T) {
	sm := NewStateMachineWithExternalStorage[string, string, any](func(_ context.Context) (string, error) {
		return stateB, nil
	}, func(_ context.Context, s string) error { return errors.New("status error") }, FiringImmediate)

	sm.Configure(stateB).
		Permit(triggerX, stateA)

	want := "status error"
	if err := sm.Fire(triggerX, nil); err == nil || err.Error() != want {
		t.Errorf("Fire() = %v, want %v", err, want)
	}
	if sm.MustState() != stateB {
		t.Errorf("MustState() = %v, want %v", sm.MustState(), stateB)
	}
}

func TestStateMachine_Fire_TriggerIsIgnored_ActionsNotExecuted(t *testing.T) {
	fired := false
	sm := NewStateMachine[string, string, any](stateB)
	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ any) error {
			fired = true
			return nil
		}).
		Ignore(triggerX)

	sm.Fire(triggerX, nil)

	if fired {
		t.Error("actions were executed")
	}
}

func TestStateMachine_Fire_SelfTransitionPermited_ActionsFire(t *testing.T) {
	fired := false
	sm := NewStateMachine[string, string, any](stateB)
	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ any) error {
			fired = true
			return nil
		}).
		PermitReentry(triggerX)

	sm.Fire(triggerX, nil)
	if !fired {
		t.Error("actions did not fire")
	}
}

func TestStateMachine_Fire_ImplicitReentryIsDisallowed(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
	assertPanic(t, func() {
		sm.Configure(stateB).
			Permit(triggerX, stateB)
	})
}

func TestStateMachine_Fire_ErrorForInvalidTransition(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	if err := sm.Fire(triggerX, nil); err == nil {
		t.Error("error expected")
	}
}

func TestStateMachine_Fire_ErrorForInvalidTransitionMentionsGuardDescriptionIfPresent(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	sm.Configure(stateA).Permit(triggerX, stateB, func(_ context.Context, _ any) bool {
		return false
	})
	if err := sm.Fire(triggerX, nil); err == nil {
		t.Error("error expected")
	}
}

func TestStateMachine_Fire_ParametersSuppliedToFireArePassedToEntryAction(t *testing.T) {
	sm := NewStateMachine[string, string, Args](stateB)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0))
	sm.Configure(stateB).Permit(triggerX, stateC)

	var (
		entryArg1 string
		entryArg2 int
	)
	sm.Configure(stateC).OnEntryFrom(triggerX, func(_ context.Context, args Args) error {
		entryArg1 = args[0].(string)
		entryArg2 = args[1].(int)
		return nil
	})
	suppliedArg1, suppliedArg2 := "something", 2
	sm.Fire(triggerX, Args{suppliedArg1, suppliedArg2})

	if entryArg1 != suppliedArg1 {
		t.Errorf("entryArg1 = %v, want %v", entryArg1, suppliedArg1)
	}
	if entryArg2 != suppliedArg2 {
		t.Errorf("entryArg2 = %v, want %v", entryArg2, suppliedArg2)
	}
}

func TestStateMachine_Fire_ParametersSuppliedToFireArePassedToExitAction(t *testing.T) {
	sm := NewStateMachine[string, string, Args](stateB)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0))
	sm.Configure(stateB).Permit(triggerX, stateC)

	var (
		entryArg1 string
		entryArg2 int
	)
	sm.Configure(stateB).OnExitWith(triggerX, func(_ context.Context, args Args) error {
		entryArg1 = args[0].(string)
		entryArg2 = args[1].(int)
		return nil
	})
	suppliedArg1, suppliedArg2 := "something", 2
	sm.Fire(triggerX, Args{suppliedArg1, suppliedArg2})

	if entryArg1 != suppliedArg1 {
		t.Errorf("entryArg1 = %v, want %v", entryArg1, suppliedArg1)
	}
	if entryArg2 != suppliedArg2 {
		t.Errorf("entryArg2 = %v, want %v", entryArg2, suppliedArg2)
	}
}

func TestStateMachine_OnUnhandledTrigger_TheProvidedHandlerIsCalledWithStateAndTrigger(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
	var (
		unhandledState   string
		unhandledTrigger string
	)
	sm.OnUnhandledTrigger(func(_ context.Context, state string, trigger string, unmetGuards []string) error {
		unhandledState = state
		unhandledTrigger = trigger
		return nil
	})

	sm.Fire(triggerZ, nil)

	if stateB != unhandledState {
		t.Errorf("unhandledState = %v, want %v", unhandledState, stateB)
	}
	if triggerZ != unhandledTrigger {
		t.Errorf("unhandledTrigger = %v, want %v", unhandledTrigger, triggerZ)
	}
}

func TestStateMachine_SetTriggerParameters_TriggerParametersAreImmutableOnceSet(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)

	sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0))

	assertPanic(t, func() { sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0)) })
}

func TestStateMachine_SetTriggerParameters_Interfaces(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
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
	sm := NewStateMachine[string, string, Args](stateB)

	sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0))
	sm.Configure(stateB).Permit(triggerX, stateA)

	assertPanic(t, func() { sm.Fire(triggerX, nil) })
	assertPanic(t, func() { sm.Fire(triggerX, Args{"1", "2", "3"}) })
	assertPanic(t, func() { sm.Fire(triggerX, Args{"1", "2"}) })
}

func TestStateMachine_OnTransitioning_EventFires(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
	sm.Configure(stateB).Permit(triggerX, stateA)

	var transition Transition[string, string]
	sm.OnTransitioning(func(_ context.Context, tr Transition[string, string]) {
		transition = tr
	})
	sm.Fire(triggerX, nil)

	want := Transition[string, string]{
		Source:      stateB,
		Destination: stateA,
		Trigger:     triggerX,
	}
	if !reflect.DeepEqual(transition, want) {
		t.Errorf("transition = %v, want %v", transition, want)
	}
}

func TestStateMachine_OnTransitioned_EventFires(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
	sm.Configure(stateB).Permit(triggerX, stateA)

	var transition Transition[string, string]
	sm.OnTransitioned(func(_ context.Context, tr Transition[string, string]) {
		transition = tr
	})
	sm.Fire(triggerX, nil)

	want := Transition[string, string]{
		Source:      stateB,
		Trigger:     triggerX,
		Destination: stateA,
	}
	if !reflect.DeepEqual(transition, want) {
		t.Errorf("transition = %v, want %v", transition, want)
	}
}

func TestStateMachine_OnTransitioned_EventFiresBeforeTheOnEntryEvent(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
	expectedOrdering := []string{"OnExit", "OnTransitioning", "OnEntry", "OnTransitioned"}
	var actualOrdering []string

	sm.Configure(stateB).Permit(triggerX, stateA).OnExit(func(_ context.Context, _ any) error {
		actualOrdering = append(actualOrdering, "OnExit")
		return nil
	}).Machine()

	var transition Transition[string, string]
	sm.Configure(stateA).OnEntry(func(ctx context.Context, _ any) error {
		actualOrdering = append(actualOrdering, "OnEntry")
		transition = GetTransition[string, string](ctx)
		return nil
	})

	sm.OnTransitioning(func(_ context.Context, tr Transition[string, string]) {
		actualOrdering = append(actualOrdering, "OnTransitioning")
	})
	sm.OnTransitioned(func(_ context.Context, tr Transition[string, string]) {
		actualOrdering = append(actualOrdering, "OnTransitioned")
	})

	sm.Fire(triggerX, nil)

	if !reflect.DeepEqual(actualOrdering, expectedOrdering) {
		t.Errorf("actualOrdering = %v, want %v", actualOrdering, expectedOrdering)
	}

	want := Transition[string, string]{
		Source:      stateB,
		Destination: stateA,
		Trigger:     triggerX,
	}
	if !reflect.DeepEqual(transition, want) {
		t.Errorf("transition = %v, want %v", transition, want)
	}
}

func TestStateMachine_SubstateOf_DirectCyclicConfigurationDetected(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	assertPanic(t, func() { sm.Configure(stateA).SubstateOf(stateA) })
}

func TestStateMachine_SubstateOf_NestedCyclicConfigurationDetected(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	sm.Configure(stateB).SubstateOf(stateA)
	assertPanic(t, func() { sm.Configure(stateA).SubstateOf(stateB) })
}

func TestStateMachine_SubstateOf_NestedTwoLevelsCyclicConfigurationDetected(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	sm.Configure(stateB).SubstateOf(stateA)
	sm.Configure(stateC).SubstateOf(stateB)
	assertPanic(t, func() { sm.Configure(stateA).SubstateOf(stateC) })
}

func TestStateMachine_SubstateOf_DelayedNestedCyclicConfigurationDetected(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	sm.Configure(stateB).SubstateOf(stateA)
	sm.Configure(stateC)
	sm.Configure(stateA).SubstateOf(stateC)
	assertPanic(t, func() { sm.Configure(stateC).SubstateOf(stateB) })
}

func TestStateMachine_Fire_IgnoreVsPermitReentry(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	var calls int
	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ any) error {
			calls += 1
			return nil
		}).
		PermitReentry(triggerX).
		Ignore(triggerY)

	sm.Fire(triggerX, nil)
	sm.Fire(triggerY, nil)

	if calls != 1 {
		t.Errorf("calls = %d, want %d", calls, 1)
	}
}

func TestStateMachine_Fire_IgnoreVsPermitReentryFrom(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	var calls int
	sm.Configure(stateA).
		OnEntryFrom(triggerX, func(_ context.Context, _ any) error {
			calls += 1
			return nil
		}).
		OnEntryFrom(triggerY, func(_ context.Context, _ any) error {
			calls += 1
			return nil
		}).
		PermitReentry(triggerX).
		Ignore(triggerY)

	sm.Fire(triggerX, nil)
	sm.Fire(triggerY, nil)

	if calls != 1 {
		t.Errorf("calls = %d, want %d", calls, 1)
	}
}

func TestStateMachine_Fire_IgnoreVsPermitReentryExitWith(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	var calls int
	sm.Configure(stateA).
		OnExitWith(triggerX, func(_ context.Context, _ any) error {
			calls += 1
			return nil
		}).
		OnExitWith(triggerY, func(_ context.Context, _ any) error {
			calls += 1
			return nil
		}).
		PermitReentry(triggerX).
		Ignore(triggerY)

	sm.Fire(triggerX, nil)
	sm.Fire(triggerY, nil)

	if calls != 1 {
		t.Errorf("calls = %d, want %d", calls, 1)
	}
}

func TestStateMachine_Fire_IfSelfTransitionPermited_ActionsFire_InSubstate(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	var onEntryStateBfired, onExitStateBfired, onExitStateAfired bool
	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ any) error {
			onEntryStateBfired = true
			return nil
		}).
		PermitReentry(triggerX).
		OnExit(func(_ context.Context, _ any) error {
			onExitStateBfired = true
			return nil
		})

	sm.Configure(stateA).
		SubstateOf(stateB).
		OnExit(func(_ context.Context, _ any) error {
			onExitStateAfired = true
			return nil
		})

	sm.Fire(triggerX, nil)

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
	sm := NewStateMachine[string, string, int](stateA)
	sm.Configure(stateA).
		Permit(triggerX, stateB, func(_ context.Context, arg int) bool {
			return arg == 2
		})

	sm.Fire(triggerX, 2)

	if got := sm.MustState(); got != stateB {
		t.Errorf("sm.MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_Fire_ErrorWhenParameterizedGuardFalse(t *testing.T) {
	sm := NewStateMachine[string, string, int](stateA)
	sm.Configure(stateA).
		Permit(triggerX, stateB, func(_ context.Context, arg int) bool {
			return arg == 3
		})

	sm.Fire(triggerX, 2)
	if err := sm.Fire(triggerX, 2); err == nil {
		t.Error("error expected")
	}
}

func TestStateMachine_Fire_TransitionWhenBothParameterizedGuardClausesTrue(t *testing.T) {
	sm := NewStateMachine[string, string, int](stateA)
	sm.Configure(stateA).
		Permit(triggerX, stateB, func(_ context.Context, arg int) bool {
			return arg == 2
		}, func(_ context.Context, arg int) bool {
			return arg != 3
		})

	sm.Fire(triggerX, 2)

	if got := sm.MustState(); got != stateB {
		t.Errorf("sm.MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_Fire_TransitionWhenGuardReturnsTrueOnTriggerWithMultipleParameters(t *testing.T) {
	sm := NewStateMachine[string, string, struct {
		s string
		i int
	}](stateA)
	sm.Configure(stateA).
		Permit(triggerX, stateB, func(_ context.Context, arg struct {
			s string
			i int
		}) bool {
			return arg.s == "3" && arg.i == 2
		})

	sm.Fire(triggerX, struct {
		s string
		i int
	}{"3", 2})

	if got := sm.MustState(); got != stateB {
		t.Errorf("sm.MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_Fire_TransitionWhenPermitDyanmicIfHasMultipleExclusiveGuards(t *testing.T) {
	sm := NewStateMachine[string, string, int](stateA)
	sm.Configure(stateA).
		PermitDynamic(triggerX, func(_ context.Context, arg int) (string, error) {
			if arg == 3 {
				return stateB, nil
			}
			return stateC, nil
		}, func(_ context.Context, arg int) bool { return arg == 3 || arg == 5 }).
		PermitDynamic(triggerX, func(_ context.Context, arg int) (string, error) {
			if arg == 2 {
				return stateC, nil
			}
			return stateD, nil
		}, func(_ context.Context, arg int) bool { return arg == 2 || arg == 4 })

	sm.Fire(triggerX, 3)

	if got := sm.MustState(); got != stateB {
		t.Errorf("sm.MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_Fire_PermitDyanmic_Error(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	sm.Configure(stateA).
		PermitDynamic(triggerX, func(_ context.Context, _ any) (string, error) {
			return "", errors.New("")
		})

	if err := sm.Fire(triggerX, ""); err == nil {
		t.Error("error expected")
	}
	if got := sm.MustState(); got != stateA {
		t.Errorf("sm.MustState() = %v, want %v", got, stateA)
	}
}

func TestStateMachine_Fire_PanicsWhenPermitDyanmicIfHasMultipleNonExclusiveGuards(t *testing.T) {
	sm := NewStateMachine[string, string, int](stateA)
	sm.Configure(stateA).
		PermitDynamic(triggerX, func(_ context.Context, arg int) (string, error) {
			if arg == 4 {
				return stateB, nil
			}
			return stateC, nil
		}, func(_ context.Context, arg int) bool { return arg%2 == 0 }).
		PermitDynamic(triggerX, func(_ context.Context, arg int) (string, error) {
			if arg == 2 {
				return stateC, nil
			}
			return stateD, nil
		}, func(_ context.Context, arg int) bool { return arg == 2 })

	assertPanic(t, func() { sm.Fire(triggerX, 2) })
}

func TestStateMachine_Fire_TransitionWhenPermitIfHasMultipleExclusiveGuardsWithSuperStateTrue(t *testing.T) {
	sm := NewStateMachine[string, string, int](stateB)
	sm.Configure(stateA).
		Permit(triggerX, stateD, func(_ context.Context, arg int) bool {
			return arg == 3
		})

	sm.Configure(stateB).
		SubstateOf(stateA).
		Permit(triggerX, stateC, func(_ context.Context, arg int) bool {
			return arg == 2
		})

	sm.Fire(triggerX, 3)

	if got := sm.MustState(); got != stateD {
		t.Errorf("sm.MustState() = %v, want %v", got, stateD)
	}
}

func TestStateMachine_Fire_TransitionWhenPermitIfHasMultipleExclusiveGuardsWithSuperStateFalse(t *testing.T) {
	sm := NewStateMachine[string, string, int](stateB)
	sm.Configure(stateA).
		Permit(triggerX, stateD, func(_ context.Context, arg int) bool {
			return arg == 3
		})

	sm.Configure(stateB).
		SubstateOf(stateA).
		Permit(triggerX, stateC, func(_ context.Context, arg int) bool {
			return arg == 2
		})

	sm.Fire(triggerX, 2)

	if got := sm.MustState(); got != stateC {
		t.Errorf("sm.MustState() = %v, want %v", got, stateC)
	}
}

func TestStateMachine_Fire_TransitionToSuperstateDoesNotExitSuperstate(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
	var superExit, superEntry, subExit bool
	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ any) error {
			superEntry = true
			return nil
		}).
		OnExit(func(_ context.Context, _ any) error {
			superExit = true
			return nil
		})

	sm.Configure(stateB).
		SubstateOf(stateA).
		Permit(triggerY, stateA).
		OnExit(func(_ context.Context, _ any) error {
			subExit = true
			return nil
		})

	sm.Fire(triggerY, nil)

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
	sm := NewStateMachine[string, string, any](stateA)
	var exitB, exitA, entryB, entryA int
	sm.Configure(stateA).
		SubstateOf(stateB).
		OnEntry(func(_ context.Context, _ any) error {
			entryA += 1
			return nil
		}).
		PermitReentry(triggerX).
		OnExit(func(_ context.Context, _ any) error {
			exitA += 1
			return nil
		})

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ any) error {
			entryB += 1
			return nil
		}).
		OnExit(func(_ context.Context, _ any) error {
			exitB += 1
			return nil
		})

	sm.Fire(triggerX, nil)

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
	sm := NewStateMachine[string, string, any](stateA)

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
	sm.OnTransitioning(func(_ context.Context, _ Transition[string, string]) {
		actualOrdering = append(actualOrdering, "OnTransitioning")
	})
	sm.OnTransitioned(func(_ context.Context, _ Transition[string, string]) {
		actualOrdering = append(actualOrdering, "OnTransitioned")
	})

	sm.Activate()

	if !reflect.DeepEqual(expectedOrdering, actualOrdering) {
		t.Errorf("expectedOrdering = %v, actualOrdering = %v", expectedOrdering, actualOrdering)
	}
}

func TestStateMachine_Activate_Error(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)

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
	sm := NewStateMachine[string, string, any](stateA)

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
	sm := NewStateMachine[string, string, any](stateA)

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
	sm.OnTransitioning(func(_ context.Context, _ Transition[string, string]) {
		actualOrdering = append(actualOrdering, "OnTransitioning")
	})
	sm.OnTransitioned(func(_ context.Context, _ Transition[string, string]) {
		actualOrdering = append(actualOrdering, "OnTransitioned")
	})

	sm.Activate()
	sm.Deactivate()

	if !reflect.DeepEqual(expectedOrdering, actualOrdering) {
		t.Errorf("expectedOrdering = %v, actualOrdering = %v", expectedOrdering, actualOrdering)
	}
}

func TestStateMachine_Deactivate_NoActivated(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)

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
	sm := NewStateMachine[string, string, any](stateA)

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
	sm := NewStateMachine[string, string, any](stateA)

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
	sm := NewStateMachine[string, string, any](stateA)

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
		OnEntry(func(_ context.Context, _ any) error {
			actualOrdering = append(actualOrdering, "EnteredA")
			return nil
		}).
		OnExit(func(_ context.Context, _ any) error {
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
		OnEntry(func(_ context.Context, _ any) error {
			actualOrdering = append(actualOrdering, "EnteredB")
			return nil
		}).
		OnExit(func(_ context.Context, _ any) error {
			actualOrdering = append(actualOrdering, "ExitedB")
			return nil
		}).
		Permit(triggerY, stateA)

	sm.OnTransitioning(func(_ context.Context, _ Transition[string, string]) {
		actualOrdering = append(actualOrdering, "OnTransitioning")
	})
	sm.OnTransitioned(func(_ context.Context, _ Transition[string, string]) {
		actualOrdering = append(actualOrdering, "OnTransitioned")
	})

	sm.Activate()
	sm.Fire(triggerX, nil)
	sm.Fire(triggerY, nil)

	if !reflect.DeepEqual(expectedOrdering, actualOrdering) {
		t.Errorf("expectedOrdering = %v, actualOrdering = %v", expectedOrdering, actualOrdering)
	}
}

func TestStateMachine_Fire_ImmediateEntryAProcessedBeforeEnterB(t *testing.T) {
	sm := NewStateMachineWithMode[string, string, any](stateA, FiringImmediate)

	var actualOrdering []string
	expectedOrdering := []string{"ExitA", "ExitB", "EnterA", "EnterB"}

	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ any) error {
			actualOrdering = append(actualOrdering, "EnterA")
			return nil
		}).
		OnExit(func(_ context.Context, _ any) error {
			actualOrdering = append(actualOrdering, "ExitA")
			return nil
		}).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ any) error {
			sm.Fire(triggerY, nil)
			actualOrdering = append(actualOrdering, "EnterB")
			return nil
		}).
		OnExit(func(_ context.Context, _ any) error {
			actualOrdering = append(actualOrdering, "ExitB")
			return nil
		}).
		Permit(triggerY, stateA)

	sm.Fire(triggerX, nil)

	if !reflect.DeepEqual(expectedOrdering, actualOrdering) {
		t.Errorf("expectedOrdering = %v, actualOrdering = %v", expectedOrdering, actualOrdering)
	}
}

func TestStateMachine_Fire_QueuedEntryAProcessedBeforeEnterB(t *testing.T) {
	sm := NewStateMachineWithMode[string, string, any](stateA, FiringQueued)

	var actualOrdering []string
	expectedOrdering := []string{"ExitA", "EnterB", "ExitB", "EnterA"}

	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ any) error {
			actualOrdering = append(actualOrdering, "EnterA")
			return nil
		}).
		OnExit(func(_ context.Context, _ any) error {
			actualOrdering = append(actualOrdering, "ExitA")
			return nil
		}).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ any) error {
			sm.Fire(triggerY, nil)
			actualOrdering = append(actualOrdering, "EnterB")
			return nil
		}).
		OnExit(func(_ context.Context, _ any) error {
			actualOrdering = append(actualOrdering, "ExitB")
			return nil
		}).
		Permit(triggerY, stateA)

	sm.Fire(triggerX, nil)

	if !reflect.DeepEqual(expectedOrdering, actualOrdering) {
		t.Errorf("expectedOrdering = %v, actualOrdering = %v", expectedOrdering, actualOrdering)
	}
}

func TestStateMachine_Fire_QueuedEntryAsyncFire(t *testing.T) {
	sm := NewStateMachineWithMode[string, string, any](stateA, FiringQueued)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ any) error {
			go sm.Fire(triggerY, nil)
			go sm.Fire(triggerY, nil)
			return nil
		}).
		Permit(triggerY, stateA)

	sm.Fire(triggerX, nil)
}

func TestStateMachine_Fire_Race(t *testing.T) {
	sm := NewStateMachineWithMode[string, string, any](stateA, FiringImmediate)

	var actualOrdering []string
	var mu sync.Mutex
	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ any) error {
			mu.Lock()
			actualOrdering = append(actualOrdering, "EnterA")
			mu.Unlock()
			return nil
		}).
		OnExit(func(_ context.Context, _ any) error {
			mu.Lock()
			actualOrdering = append(actualOrdering, "ExitA")
			mu.Unlock()
			return nil
		}).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ any) error {
			sm.Fire(triggerY, nil)
			mu.Lock()
			actualOrdering = append(actualOrdering, "EnterB")
			mu.Unlock()
			return nil
		}).
		OnExit(func(_ context.Context, _ any) error {
			mu.Lock()
			actualOrdering = append(actualOrdering, "ExitB")
			mu.Unlock()
			return nil
		}).
		Permit(triggerY, stateA)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		sm.Fire(triggerX, nil)
		wg.Done()
	}()
	go func() {
		sm.Fire(triggerZ, nil)
		wg.Done()
	}()
	wg.Wait()
	if got := len(actualOrdering); got != 4 {
		t.Errorf("expected 4, got %d", got)
	}
}

func TestStateMachine_Fire_Queued_ErrorExit(t *testing.T) {
	sm := NewStateMachineWithMode[string, string, any](stateA, FiringQueued)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ any) error {
			sm.Fire(triggerY, nil)
			return nil
		}).
		OnExit(func(_ context.Context, _ any) error {
			return errors.New("")
		}).
		Permit(triggerY, stateA)

	sm.Fire(triggerX, nil)

	if err := sm.Fire(triggerX, nil); err == nil {
		t.Error("expected error")
	}
}

func TestStateMachine_Fire_Queued_ErrorEnter(t *testing.T) {
	sm := NewStateMachineWithMode[string, string, any](stateA, FiringQueued)

	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ any) error {
			return errors.New("")
		}).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ any) error {
			sm.Fire(triggerY, nil)
			return nil
		}).
		Permit(triggerY, stateA)

	sm.Fire(triggerX, nil)

	if err := sm.Fire(triggerX, nil); err == nil {
		t.Error("expected error")
	}
}

func TestStateMachine_InternalTransition_StayInSameStateOneState(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	sm.Configure(stateB).
		InternalTransition(triggerX, func(_ context.Context, _ any) error {
			return nil
		})

	sm.Fire(triggerX, nil)
	if got := sm.MustState(); got != stateA {
		t.Errorf("expected %v, got %v", stateA, got)
	}
}

func TestStateMachine_InternalTransition_HandledOnlyOnceInSuper(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	handledIn := stateC
	sm.Configure(stateA).
		InternalTransition(triggerX, func(_ context.Context, _ any) error {
			handledIn = stateA
			return nil
		})

	sm.Configure(stateB).
		SubstateOf(stateA).
		InternalTransition(triggerX, func(_ context.Context, _ any) error {
			handledIn = stateB
			return nil
		})

	sm.Fire(triggerX, nil)
	if stateA != handledIn {
		t.Errorf("expected %v, got %v", stateA, handledIn)
	}
}

func TestStateMachine_InternalTransition_HandledOnlyOnceInSub(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateB)
	handledIn := stateC
	sm.Configure(stateA).
		InternalTransition(triggerX, func(_ context.Context, _ any) error {
			handledIn = stateA
			return nil
		})

	sm.Configure(stateB).
		SubstateOf(stateA).
		InternalTransition(triggerX, func(_ context.Context, _ any) error {
			handledIn = stateB
			return nil
		})

	sm.Fire(triggerX, nil)
	if stateB != handledIn {
		t.Errorf("expected %v, got %v", stateB, handledIn)
	}
}

func TestStateMachine_InitialTransition_EntersSubState(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		InitialTransition(stateC)

	sm.Configure(stateC).
		SubstateOf(stateB)

	sm.Fire(triggerX, nil)
	if got := sm.MustState(); got != stateC {
		t.Errorf("MustState() = %v, want %v", got, stateC)
	}
}

func TestStateMachine_InitialTransition_EntersSubStateofSubstate(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		InitialTransition(stateC)

	sm.Configure(stateC).
		InitialTransition(stateD).
		SubstateOf(stateB)

	sm.Configure(stateD).
		SubstateOf(stateC)

	sm.Fire(triggerX, nil)
	if got := sm.MustState(); got != stateD {
		t.Errorf("MustState() = %v, want %v", got, stateD)
	}
}

func TestStateMachine_InitialTransition_Ordering(t *testing.T) {
	var actualOrdering []string
	expectedOrdering := []string{"ExitA", "OnTransitioningAB", "EnterB", "OnTransitioningBC", "EnterC", "OnTransitionedAC"}

	sm := NewStateMachine[string, string, any](stateA)

	sm.Configure(stateA).
		Permit(triggerX, stateB).
		OnExit(func(c context.Context, _ any) error {
			actualOrdering = append(actualOrdering, "ExitA")
			return nil
		})

	sm.Configure(stateB).
		InitialTransition(stateC).
		OnEntry(func(c context.Context, _ any) error {
			actualOrdering = append(actualOrdering, "EnterB")
			return nil
		})

	sm.Configure(stateC).
		SubstateOf(stateB).
		OnEntry(func(c context.Context, _ any) error {
			actualOrdering = append(actualOrdering, "EnterC")
			return nil
		})

	sm.OnTransitioning(func(_ context.Context, tr Transition[string, string]) {
		actualOrdering = append(actualOrdering, fmt.Sprintf("OnTransitioning%v%v", tr.Source, tr.Destination))
	})
	sm.OnTransitioned(func(_ context.Context, tr Transition[string, string]) {
		actualOrdering = append(actualOrdering, fmt.Sprintf("OnTransitioned%v%v", tr.Source, tr.Destination))
	})

	sm.Fire(triggerX, nil)
	if got := sm.MustState(); got != stateC {
		t.Errorf("MustState() = %v, want %v", got, stateC)
	}

	if !reflect.DeepEqual(expectedOrdering, actualOrdering) {
		t.Errorf("expected %v, got %v", expectedOrdering, actualOrdering)
	}
}

func TestStateMachine_InitialTransition_DoesNotEnterSubStateofSubstate(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		sm.Configure(stateC).
		InitialTransition(stateD).
		SubstateOf(stateB)

	sm.Configure(stateD).
		SubstateOf(stateC)

	sm.Fire(triggerX, nil)
	if got := sm.MustState(); got != stateB {
		t.Errorf("MustState() = %v, want %v", got, stateB)
	}
}

func TestStateMachine_InitialTransition_DoNotAllowTransitionToSelf(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	assertPanic(t, func() {
		sm.Configure(stateA).
			InitialTransition(stateA)
	})
}

func TestStateMachine_InitialTransition_WithMultipleSubStates(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	sm.Configure(stateA).Permit(triggerX, stateB)
	sm.Configure(stateB).InitialTransition(stateC)
	sm.Configure(stateC).SubstateOf(stateB)
	sm.Configure(stateD).SubstateOf(stateB)
	if err := sm.Fire(triggerX, nil); err != nil {
		t.Error(err)
	}
}

func TestStateMachine_InitialTransition_DoNotAllowTransitionToAnotherSuperstate(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		InitialTransition(stateA)

	assertPanic(t, func() { sm.Fire(triggerX, nil) })
}

func TestStateMachine_InitialTransition_DoNotAllowMoreThanOneInitialTransition(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		InitialTransition(stateC)

	assertPanic(t, func() { sm.Configure(stateB).InitialTransition(stateA) })
}

func TestStateMachine_String(t *testing.T) {
	tests := []struct {
		name string
		sm   *StateMachine[string, string, any]
		want string
	}{
		{"noTriggers", NewStateMachine[string, string, any](stateA), "StateMachine {{ State = A, PermittedTriggers = [] }}"},
		{"error state", NewStateMachineWithExternalStorage[string, string, any](func(_ context.Context) (string, error) {
			return "", errors.New("status error")
		}, func(_ context.Context, s string) error { return nil }, FiringImmediate), ""},
		{"triggers", NewStateMachine[string, string, any](stateB).Configure(stateB).Permit(triggerX, stateA).Machine(),
			"StateMachine {{ State = B, PermittedTriggers = [X] }}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sm.String(nil); got != tt.want {
				t.Errorf("StateMachine.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStateMachine_String_Concurrent(t *testing.T) {
	// Test that race mode doesn't complain about concurrent access to the state machine.
	sm := NewStateMachine[string, string, any](stateA)
	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = sm.String(nil)
		}()
	}
	wg.Wait()
}

func TestStateMachine_Firing_Queued(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(ctx context.Context, _ any) error {
			if !sm.Firing() {
				t.Error("expected firing to be true")
			}
			return nil
		})
	if err := sm.Fire(triggerX, nil); err != nil {
		t.Error(err)
	}
	if sm.Firing() {
		t.Error("expected firing to be false")
	}
}

func TestStateMachine_Firing_Immediate(t *testing.T) {
	sm := NewStateMachineWithMode[string, string, any](stateA, FiringImmediate)

	sm.Configure(stateA).
		Permit(triggerX, stateB)

	sm.Configure(stateB).
		OnEntry(func(ctx context.Context, _ any) error {
			if !sm.Firing() {
				t.Error("expected firing to be true")
			}
			return nil
		})
	if err := sm.Fire(triggerX, nil); err != nil {
		t.Error(err)
	}
	if sm.Firing() {
		t.Error("expected firing to be false")
	}
}

func TestStateMachine_Firing_Concurrent(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)

	sm.Configure(stateA).
		PermitReentry(triggerX).
		OnEntry(func(ctx context.Context, _ any) error {
			if !sm.Firing() {
				t.Error("expected firing to be true")
			}
			return nil
		})

	var wg sync.WaitGroup
	wg.Add(1000)
	for i := 0; i < 1000; i++ {
		go func() {
			if err := sm.Fire(triggerX, nil); err != nil {
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
	GetTransition[string, string](context.Background())
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

func TestStateMachineWhenInSubstate_TriggerSuperStateTwiceToSameSubstate_DoesNotReenterSubstate(t *testing.T) {
	sm := NewStateMachine[string, string, any](stateA)
	var eCount = 0

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ any) error {
			eCount++
			return nil
		}).
		SubstateOf(stateC)

	sm.Configure(stateA).
		SubstateOf(stateC)

	sm.Configure(stateC).
		Permit(triggerX, stateB)

	sm.Fire(triggerX, nil)
	sm.Fire(triggerX, nil)

	if eCount != 1 {
		t.Errorf("expected 1, got %d", eCount)
	}
}
