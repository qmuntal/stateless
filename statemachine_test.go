package stateless

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransition_IsReentry(t *testing.T) {
	tests := []struct {
		name string
		t    *Transition
		want bool
	}{
		{"TransitionIsNotChange", &Transition{"1", "1", "0"}, true},
		{"TransitionIsChange", &Transition{"1", "2", "0"}, false},
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
	assert.Equal(t, stateA, sm.MustState())
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
	assert.Equal(t, stateB, sm.MustState())
	assert.Equal(t, stateB, state)
	sm.Fire(triggerX)
	assert.Equal(t, stateC, sm.MustState())
	assert.Equal(t, stateC, state)
}

func TestStateMachine_Configure_SubstateIsIncludedInCurrentState(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).SubstateOf(stateC)
	ok, _ := sm.IsInState(stateC)

	assert.Equal(t, stateB, sm.MustState())
	assert.True(t, ok)
}

func TestStateMachine_Configure_InSubstate_TriggerIgnoredInSuperstate_RemainsInSubstate(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).SubstateOf(stateC)
	sm.Configure(stateC).Ignore(triggerX)
	sm.Fire(triggerX)

	assert.Equal(t, stateB, sm.MustState())
}

func TestStateMachine_Configure_PermittedTriggersIncludeSuperstatePermittedTriggers(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateA).Permit(triggerZ, stateB)
	sm.Configure(stateB).SubstateOf(stateC).Permit(triggerX, stateA)
	sm.Configure(stateC).Permit(triggerY, stateA)

	permitted, _ := sm.PermittedTriggers(context.Background())

	assert.Contains(t, permitted, triggerX)
	assert.Contains(t, permitted, triggerY)
	assert.NotContains(t, permitted, triggerZ)
}

func TestStateMachine_PermittedTriggers_PermittedTriggersAreDistinctValues(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).SubstateOf(stateC).Permit(triggerX, stateA)
	sm.Configure(stateC).Permit(triggerX, stateB)

	permitted, _ := sm.PermittedTriggers(context.Background())

	assert.Len(t, permitted, 1)
	assert.Equal(t, permitted[0], triggerX)
}

func TestStateMachine_PermittedTriggers_AcceptedTriggersRespectGuards(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).Permit(triggerX, stateA, func(_ context.Context, _ ...interface{}) bool {
		return false
	})

	permitted, _ := sm.PermittedTriggers(context.Background())

	assert.Len(t, permitted, 0)
}

func TestStateMachine_PermittedTriggers_AcceptedTriggersRespectMultipleGuards(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).Permit(triggerX, stateA, func(_ context.Context, _ ...interface{}) bool {
		return true
	}, func(_ context.Context, _ ...interface{}) bool {
		return false
	})

	permitted, _ := sm.PermittedTriggers(context.Background())

	assert.Len(t, permitted, 0)
}

func TestStateMachine_Fire_DiscriminatedByGuard_ChoosesPermitedTransition(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).
		Permit(triggerX, stateA, func(_ context.Context, _ ...interface{}) bool {
			return false
		}).
		Permit(triggerX, stateC, func(_ context.Context, _ ...interface{}) bool {
			return true
		})

	sm.Fire(triggerX)

	assert.Equal(t, stateC, sm.MustState())
}

func TestStateMachine_Fire_TriggerIsIgnored_ActionsNotExecuted(t *testing.T) {
	fired := false
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...interface{}) error {
			fired = true
			return nil
		}).
		Ignore(triggerX)

	sm.Fire(triggerX)

	assert.False(t, fired)
}

func TestStateMachine_Fire_SelfTransitionPermited_ActionsFire(t *testing.T) {
	fired := false
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...interface{}) error {
			fired = true
			return nil
		}).
		PermitReentry(triggerX)

	sm.Fire(triggerX)

	assert.True(t, fired)
}

func TestStateMachine_Fire_ImplicitReentryIsDisallowed(t *testing.T) {
	sm := NewStateMachine(stateB)
	assert.Panics(t, func() {
		sm.Configure(stateB).
			Permit(triggerX, stateB)
	})
}

func TestStateMachine_Fire_ErrorForInvalidTransition(t *testing.T) {
	sm := NewStateMachine(stateA)
	assert.Error(t, sm.Fire(triggerX))
}

func TestStateMachine_Fire_ErrorForInvalidTransitionMentionsGuardDescriptionIfPresent(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.Configure(stateA).Permit(triggerX, stateB, func(_ context.Context, _ ...interface{}) bool {
		return false
	})
	assert.Error(t, sm.Fire(triggerX))
}

func TestStateMachine_Fire_ParametersSuppliedToFireArePassedToEntryAction(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0))
	sm.Configure(stateB).Permit(triggerX, stateC)

	var (
		entryArg1 string
		entryArg2 int
	)
	sm.Configure(stateC).OnEntryFrom(triggerX, func(_ context.Context, args ...interface{}) error {
		entryArg1 = args[0].(string)
		entryArg2 = args[1].(int)
		return nil
	})
	suppliedArg1, suppliedArg2 := "something", 2
	sm.Fire(triggerX, suppliedArg1, suppliedArg2)

	assert.Equal(t, suppliedArg1, entryArg1)
	assert.Equal(t, suppliedArg2, entryArg2)
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

	assert.Equal(t, stateB, unhandledState)
	assert.Equal(t, triggerZ, unhandledTrigger)
}

func TestStateMachine_SetTriggerParameters_TriggerParametersAreImmutableOnceSet(t *testing.T) {
	sm := NewStateMachine(stateB)

	sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0))

	assert.Panics(t, func() { sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0)) })
}

func TestStateMachine_OnTransitioned_EventFires(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).Permit(triggerX, stateA)

	var transition Transition
	sm.OnTransitioned(func(_ context.Context, tr Transition) {
		transition = tr
	})
	sm.Fire(triggerX)

	assert.NotZero(t, transition)
	assert.Equal(t, triggerX, transition.Trigger)
	assert.Equal(t, stateB, transition.Source)
	assert.Equal(t, stateA, transition.Destination)
}

func TestStateMachine_OnTransitioned_EventFiresBeforeTheOnEntryEvent(t *testing.T) {
	sm := NewStateMachine(stateB)
	expectedOrdering := []string{"OnExit", "OnTransitioned", "OnEntry"}
	var actualOrdering []string

	sm.Configure(stateB).Permit(triggerX, stateA).OnExit(func(_ context.Context, args ...interface{}) error {
		actualOrdering = append(actualOrdering, "OnExit")
		return nil
	})

	sm.Configure(stateA).OnEntry(func(_ context.Context, args ...interface{}) error {
		actualOrdering = append(actualOrdering, "OnEntry")
		return nil
	})

	sm.OnTransitioned(func(_ context.Context, tr Transition) {
		actualOrdering = append(actualOrdering, "OnTransitioned")
	})

	sm.Fire(triggerX)

	assert.Equal(t, expectedOrdering, actualOrdering)
}

func TestStateMachine_SubstateOf_DirectCyclicConfigurationDetected(t *testing.T) {
	sm := NewStateMachine(stateA)
	assert.Panics(t, func() { sm.Configure(stateA).SubstateOf(stateA) })
}

func TestStateMachine_SubstateOf_NestedCyclicConfigurationDetected(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.Configure(stateB).SubstateOf(stateA)
	assert.Panics(t, func() { sm.Configure(stateA).SubstateOf(stateB) })
}

func TestStateMachine_SubstateOf_NestedTwoLevelsCyclicConfigurationDetected(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.Configure(stateB).SubstateOf(stateA)
	sm.Configure(stateC).SubstateOf(stateB)
	assert.Panics(t, func() { sm.Configure(stateA).SubstateOf(stateC) })
}

func TestStateMachine_SubstateOf_DelayedNestedCyclicConfigurationDetected(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.Configure(stateB).SubstateOf(stateA)
	sm.Configure(stateC)
	sm.Configure(stateA).SubstateOf(stateC)
	assert.Panics(t, func() { sm.Configure(stateC).SubstateOf(stateB) })
}

func TestStateMachine_Fire_IgnoreVsPermitReentry(t *testing.T) {
	sm := NewStateMachine(stateA)
	var calls int
	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ ...interface{}) error {
			calls += 1
			return nil
		}).
		PermitReentry(triggerX).
		Ignore(triggerY)

	sm.Fire(triggerX)
	sm.Fire(triggerY)

	assert.Equal(t, calls, 1)
}

func TestStateMachine_Fire_IgnoreVsPermitReentryFrom(t *testing.T) {
	sm := NewStateMachine(stateA)
	var calls int
	sm.Configure(stateA).
		OnEntryFrom(triggerX, func(_ context.Context, _ ...interface{}) error {
			calls += 1
			return nil
		}).
		OnEntryFrom(triggerY, func(_ context.Context, _ ...interface{}) error {
			calls += 1
			return nil
		}).
		PermitReentry(triggerX).
		Ignore(triggerY)

	sm.Fire(triggerX)
	sm.Fire(triggerY)

	assert.Equal(t, calls, 1)
}

func TestStateMachine_Fire_IfSelfTransitionPermited_ActionsFire_InSubstate(t *testing.T) {
	sm := NewStateMachine(stateA)
	var onEntryStateBfired, onExitStateBfired, onExitStateAfired bool
	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...interface{}) error {
			onEntryStateBfired = true
			return nil
		}).
		PermitReentry(triggerX).
		OnExit(func(_ context.Context, _ ...interface{}) error {
			onExitStateBfired = true
			return nil
		})

	sm.Configure(stateA).
		SubstateOf(stateB).
		OnExit(func(_ context.Context, _ ...interface{}) error {
			onExitStateAfired = true
			return nil
		})

	sm.Fire(triggerX)

	assert.Equal(t, stateB, sm.MustState())
	assert.True(t, onEntryStateBfired)
	assert.True(t, onExitStateBfired)
	assert.True(t, onExitStateAfired)
}

func TestStateMachine_Fire_TransitionWhenParameterizedGuardTrue(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(0))
	sm.Configure(stateA).
		Permit(triggerX, stateB, func(_ context.Context, args ...interface{}) bool {
			return args[0].(int) == 2
		})

	sm.Fire(triggerX, 2)

	assert.Equal(t, stateB, sm.MustState())
}

func TestStateMachine_Fire_ErrorWhenParameterizedGuardFalse(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(0))
	sm.Configure(stateA).
		Permit(triggerX, stateB, func(_ context.Context, args ...interface{}) bool {
			return args[0].(int) == 3
		})

	sm.Fire(triggerX, 2)

	assert.Error(t, sm.Fire(triggerX, 2))
}

func TestStateMachine_Fire_TransitionWhenBothParameterizedGuardClausesTrue(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(0))
	sm.Configure(stateA).
		Permit(triggerX, stateB, func(_ context.Context, args ...interface{}) bool {
			return args[0].(int) == 2
		}, func(_ context.Context, args ...interface{}) bool {
			return args[0].(int) != 3
		})

	sm.Fire(triggerX, 2)

	assert.Equal(t, stateB, sm.MustState())
}

func TestStateMachine_Fire_TransitionWhenGuardReturnsTrueOnTriggerWithMultipleParameters(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0))
	sm.Configure(stateA).
		Permit(triggerX, stateB, func(_ context.Context, args ...interface{}) bool {
			return args[0].(string) == "3" && args[1].(int) == 2
		})

	sm.Fire(triggerX, "3", 2)

	assert.Equal(t, stateB, sm.MustState())
}

func TestStateMachine_Fire_TransitionWhenPermitDyanmicIfHasMultipleExclusiveGuards(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(0))
	sm.Configure(stateA).
		PermitDynamic(triggerX, func(_ context.Context, args ...interface{}) (State, error) {
			if args[0].(int) == 3 {
				return stateB, nil
			}
			return stateC, nil
		}, func(_ context.Context, args ...interface{}) bool { return args[0].(int) == 3 || args[0].(int) == 5 }).
		PermitDynamic(triggerX, func(_ context.Context, args ...interface{}) (State, error) {
			if args[0].(int) == 2 {
				return stateC, nil
			}
			return stateD, nil
		}, func(_ context.Context, args ...interface{}) bool { return args[0].(int) == 2 || args[0].(int) == 4 })

	sm.Fire(triggerX, 3)

	assert.Equal(t, stateB, sm.MustState())
}

func TestStateMachine_Fire_PanicsWhenPermitDyanmicIfHasMultipleNonExclusiveGuards(t *testing.T) {
	sm := NewStateMachine(stateA)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(0))
	sm.Configure(stateA).
		PermitDynamic(triggerX, func(_ context.Context, args ...interface{}) (State, error) {
			if args[0].(int) == 4 {
				return stateB, nil
			}
			return stateC, nil
		}, func(_ context.Context, args ...interface{}) bool { return args[0].(int)%2 == 0 }).
		PermitDynamic(triggerX, func(_ context.Context, args ...interface{}) (State, error) {
			if args[0].(int) == 2 {
				return stateC, nil
			}
			return stateD, nil
		}, func(_ context.Context, args ...interface{}) bool { return args[0].(int) == 2 })

	assert.Panics(t, func() { sm.Fire(triggerX, 2) })
}

func TestStateMachine_Fire_TransitionWhenPermitIfHasMultipleExclusiveGuardsWithSuperStateTrue(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(0))
	sm.Configure(stateA).
		Permit(triggerX, stateD, func(_ context.Context, args ...interface{}) bool {
			return args[0].(int) == 3
		})

	sm.Configure(stateB).
		SubstateOf(stateA).
		Permit(triggerX, stateC, func(_ context.Context, args ...interface{}) bool {
			return args[0].(int) == 2
		})

	sm.Fire(triggerX, 3)

	assert.Equal(t, stateD, sm.MustState())
}

func TestStateMachine_Fire_TransitionWhenPermitIfHasMultipleExclusiveGuardsWithSuperStateFalse(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.SetTriggerParameters(triggerX, reflect.TypeOf(0))
	sm.Configure(stateA).
		Permit(triggerX, stateD, func(_ context.Context, args ...interface{}) bool {
			return args[0].(int) == 3
		})

	sm.Configure(stateB).
		SubstateOf(stateA).
		Permit(triggerX, stateC, func(_ context.Context, args ...interface{}) bool {
			return args[0].(int) == 2
		})

	sm.Fire(triggerX, 2)

	assert.Equal(t, stateC, sm.MustState())
}

func TestStateMachine_Fire_TransitionToSuperstateDoesNotExitSuperstate(t *testing.T) {
	sm := NewStateMachine(stateB)
	var superExit, superEntry, subExit bool
	sm.Configure(stateA).
		OnEntry(func(_ context.Context, _ ...interface{}) error {
			superEntry = true
			return nil
		}).
		OnExit(func(_ context.Context, _ ...interface{}) error {
			superExit = true
			return nil
		})

	sm.Configure(stateB).
		SubstateOf(stateA).
		Permit(triggerY, stateA).
		OnExit(func(_ context.Context, _ ...interface{}) error {
			subExit = true
			return nil
		})

	sm.Fire(triggerY)

	assert.True(t, subExit)
	assert.False(t, superEntry)
	assert.False(t, superExit)
}

func TestStateMachine_Fire_OnExitFiresOnlyOnceReentrySubstate(t *testing.T) {
	sm := NewStateMachine(stateA)
	var exitB, exitA, entryB, entryA int
	sm.Configure(stateA).
		SubstateOf(stateB).
		OnEntry(func(_ context.Context, _ ...interface{}) error {
			entryA += 1
			return nil
		}).
		PermitReentry(triggerX).
		OnExit(func(_ context.Context, _ ...interface{}) error {
			exitA += 1
			return nil
		})

	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...interface{}) error {
			entryB += 1
			return nil
		}).
		OnExit(func(_ context.Context, _ ...interface{}) error {
			exitB += 1
			return nil
		})

	sm.Fire(triggerX)

	// assert.Equal(t, 0, exitB)
	// assert.Equal(t, 0, entryB)
	// assert.Equal(t, 1, exitA)
	// assert.Equal(t, 1, entryA)
}
