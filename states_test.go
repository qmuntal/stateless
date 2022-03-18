package stateless

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createSuperSubstatePair() (*stateRepresentation[string, string], *stateRepresentation[string, string]) {
	super := newstateRepresentation[string, string](stateA)
	sub := newstateRepresentation[string, string](stateB)
	super.Substates = append(super.Substates, sub)
	sub.Superstate = super
	return super, sub
}

func Test_stateRepresentation_Includes_SameState(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	assert.True(t, sr.IncludeState(stateB))
}

func Test_stateRepresentation_Includes_Substate(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	sr.Substates = append(sr.Substates, newstateRepresentation[string, string](stateC))
	assert.True(t, sr.IncludeState(stateC))
}

func Test_stateRepresentation_Includes_UnrelatedState(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	assert.False(t, sr.IncludeState(stateC))
}

func Test_stateRepresentation_Includes_Superstate(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	sr.Superstate = newstateRepresentation[string, string](stateC)
	assert.False(t, sr.IncludeState(stateC))
}

func Test_stateRepresentation_IsIncludedInState_SameState(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	assert.True(t, sr.IsIncludedInState(stateB))
}

func Test_stateRepresentation_IsIncludedInState_Substate(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	sr.Substates = append(sr.Substates, newstateRepresentation[string, string](stateC))
	assert.False(t, sr.IsIncludedInState(stateC))
}

func Test_stateRepresentation_IsIncludedInState_UnrelatedState(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	assert.False(t, sr.IsIncludedInState(stateC))
}

func Test_stateRepresentation_IsIncludedInState_Superstate(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	assert.False(t, sr.IsIncludedInState(stateC))
}

func Test_stateRepresentation_CanHandle_TransitionExists_TriggerCannotBeFired(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	assert.False(t, sr.CanHandle(context.Background(), triggerX))
}

func Test_stateRepresentation_CanHandle_TransitionDoesNotExist_TriggerCanBeFired(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	sr.AddTriggerBehaviour(&ignoredTriggerBehaviour[string]{baseTriggerBehaviour: baseTriggerBehaviour[string]{Trigger: triggerX}})
	assert.True(t, sr.CanHandle(context.Background(), triggerX))
}

func Test_stateRepresentation_CanHandle_TransitionExistsInSupersate_TriggerCanBeFired(t *testing.T) {
	super, sub := createSuperSubstatePair()
	super.AddTriggerBehaviour(&ignoredTriggerBehaviour[string]{baseTriggerBehaviour: baseTriggerBehaviour[string]{Trigger: triggerX}})
	assert.True(t, sub.CanHandle(context.Background(), triggerX))
}

func Test_stateRepresentation_CanHandle_TransitionUnmetGuardConditions_TriggerCannotBeFired(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	sr.AddTriggerBehaviour(&transitioningTriggerBehaviour[string, string]{baseTriggerBehaviour: baseTriggerBehaviour[string]{
		Trigger: triggerX,
		Guard: newtransitionGuard(func(_ context.Context, _ ...interface{}) bool {
			return true
		}, func(_ context.Context, _ ...interface{}) bool {
			return false
		}),
	}, Destination: stateC})
	assert.False(t, sr.CanHandle(context.Background(), triggerX))
}

func Test_stateRepresentation_CanHandle_TransitionGuardConditionsMet_TriggerCanBeFired(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	sr.AddTriggerBehaviour(&transitioningTriggerBehaviour[string, string]{baseTriggerBehaviour: baseTriggerBehaviour[string]{
		Trigger: triggerX,
		Guard: newtransitionGuard(func(_ context.Context, _ ...interface{}) bool {
			return true
		}, func(_ context.Context, _ ...interface{}) bool {
			return true
		}),
	}, Destination: stateC})
	assert.True(t, sr.CanHandle(context.Background(), triggerX))
}

func Test_stateRepresentation_FindHandler_TransitionExistAndSuperstateUnmetGuardConditions_FireNotPossible(t *testing.T) {
	super, sub := createSuperSubstatePair()
	super.AddTriggerBehaviour(&transitioningTriggerBehaviour[string, string]{baseTriggerBehaviour: baseTriggerBehaviour[string]{
		Trigger: triggerX,
		Guard: newtransitionGuard(func(_ context.Context, _ ...interface{}) bool {
			return true
		}, func(_ context.Context, _ ...interface{}) bool {
			return false
		}),
	}, Destination: stateC})
	handler, ok := sub.FindHandler(context.Background(), triggerX)
	assert.False(t, ok)
	assert.NotNil(t, handler)
	assert.False(t, sub.CanHandle(context.Background(), triggerX))
	assert.False(t, super.CanHandle(context.Background(), triggerX))
	assert.False(t, handler.Handler.GuardConditionMet(context.Background()))
}

func Test_stateRepresentation_FindHandler_TransitionExistSuperstateMetGuardConditions_CanBeFired(t *testing.T) {
	super, sub := createSuperSubstatePair()
	super.AddTriggerBehaviour(&transitioningTriggerBehaviour[string, string]{baseTriggerBehaviour: baseTriggerBehaviour[string]{
		Trigger: triggerX,
		Guard: newtransitionGuard(func(_ context.Context, _ ...interface{}) bool {
			return true
		}, func(_ context.Context, _ ...interface{}) bool {
			return true
		}),
	}, Destination: stateC})
	handler, ok := sub.FindHandler(context.Background(), triggerX)
	assert.True(t, ok)
	assert.NotNil(t, handler)
	assert.True(t, sub.CanHandle(context.Background(), triggerX))
	assert.True(t, super.CanHandle(context.Background(), triggerX))
	assert.True(t, handler.Handler.GuardConditionMet(context.Background()))
	assert.Empty(t, handler.UnmetGuardConditions)
}

func Test_stateRepresentation_Enter_EnteringActionsExecuted(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	transition := Transition[string, string]{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition[string, string]
	sr.EntryActions = append(sr.EntryActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			actualTransition = transition
			return nil
		},
	})
	err := sr.Enter(context.Background(), transition)
	assert.Equal(t, transition, actualTransition)
	assert.NoError(t, err)
}

func Test_stateRepresentation_Enter_EnteringActionsExecuted_Error(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	transition := Transition[string, string]{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition[string, string]
	sr.EntryActions = append(sr.EntryActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			return errors.New("")
		},
	})
	err := sr.Enter(context.Background(), transition)
	assert.NotEqual(t, transition, actualTransition)
	assert.Error(t, err)
}

func Test_stateRepresentation_Enter_LeavingActionsNotExecuted(t *testing.T) {
	sr := newstateRepresentation[string, string](stateA)
	transition := Transition[string, string]{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition[string, string]
	sr.ExitActions = append(sr.ExitActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			actualTransition = transition
			return nil
		},
	})
	sr.Enter(context.Background(), transition)
	assert.Zero(t, actualTransition)
}

func Test_stateRepresentation_Enter_FromSubToSuperstate_SubstateEntryActionsExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	sub.EntryActions = append(sub.EntryActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			executed = true
			return nil
		},
	})
	transition := Transition[string, string]{Source: super.State, Destination: sub.State, Trigger: triggerX}
	sub.Enter(context.Background(), transition)
	assert.True(t, executed)
}

func Test_stateRepresentation_Enter_SuperFromSubstate_SuperEntryActionsNotExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	super.EntryActions = append(super.EntryActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			executed = true
			return nil
		},
	})
	transition := Transition[string, string]{Source: super.State, Destination: sub.State, Trigger: triggerX}
	sub.Enter(context.Background(), transition)
	assert.False(t, executed)
}

func Test_stateRepresentation_Enter_Substate_SuperEntryActionsExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	super.EntryActions = append(super.EntryActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			executed = true
			return nil
		},
	})
	transition := Transition[string, string]{Source: stateC, Destination: sub.State, Trigger: triggerX}
	sub.Enter(context.Background(), transition)
	assert.True(t, executed)
}

func Test_stateRepresentation_Enter_ActionsExecuteInOrder(t *testing.T) {
	var actual []int
	sr := newstateRepresentation[string, string](stateB)
	sr.EntryActions = append(sr.EntryActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			actual = append(actual, 0)
			return nil
		},
	})
	sr.EntryActions = append(sr.EntryActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			actual = append(actual, 1)
			return nil
		},
	})
	transition := Transition[string, string]{Source: stateA, Destination: stateB, Trigger: triggerX}
	sr.Enter(context.Background(), transition)
	assert.Equal(t, 2, len(actual))
	assert.Equal(t, 0, actual[0])
	assert.Equal(t, 1, actual[1])
}

func Test_stateRepresentation_Enter_Substate_SuperstateEntryActionsExecuteBeforeSubstate(t *testing.T) {
	super, sub := createSuperSubstatePair()
	var order, subOrder, superOrder int
	super.EntryActions = append(super.EntryActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			order += 1
			superOrder = order
			return nil
		},
	})
	sub.EntryActions = append(sub.EntryActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			order += 1
			subOrder = order
			return nil
		},
	})
	transition := Transition[string, string]{Source: stateC, Destination: sub.State, Trigger: triggerX}
	sub.Enter(context.Background(), transition)
	assert.True(t, superOrder < subOrder)
}

func Test_stateRepresentation_Exit_EnteringActionsNotExecuted(t *testing.T) {
	sr := newstateRepresentation[string, string](stateB)
	transition := Transition[string, string]{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition[string, string]
	sr.EntryActions = append(sr.EntryActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			actualTransition = transition
			return nil
		},
	})
	sr.Exit(context.Background(), transition)
	assert.Zero(t, actualTransition)
}

func Test_stateRepresentation_Exit_LeavingActionsExecuted(t *testing.T) {
	sr := newstateRepresentation[string, string](stateA)
	transition := Transition[string, string]{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition[string, string]
	sr.ExitActions = append(sr.ExitActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			actualTransition = transition
			return nil
		},
	})
	err := sr.Exit(context.Background(), transition)
	assert.Equal(t, transition, actualTransition)
	assert.NoError(t, err)
}

func Test_stateRepresentation_Exit_LeavingActionsExecuted_Error(t *testing.T) {
	sr := newstateRepresentation[string, string](stateA)
	transition := Transition[string, string]{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition[string, string]
	sr.ExitActions = append(sr.ExitActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			return errors.New("")
		},
	})
	err := sr.Exit(context.Background(), transition)
	assert.NotEqual(t, transition, actualTransition)
	assert.Error(t, err)
}

func Test_stateRepresentation_Exit_FromSubToSuperstate_SubstateExitActionsExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	sub.ExitActions = append(sub.ExitActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			executed = true
			return nil
		},
	})
	transition := Transition[string, string]{Source: sub.State, Destination: super.State, Trigger: triggerX}
	sub.Exit(context.Background(), transition)
	assert.True(t, executed)
}

func Test_stateRepresentation_Exit_FromSubToOther_SuperstateExitActionsExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	supersuper := newstateRepresentation[string, string](stateC)
	super.Superstate = supersuper
	supersuper.Superstate = newstateRepresentation[string, string](stateD)
	executed := false
	super.ExitActions = append(super.ExitActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			executed = true
			return nil
		},
	})
	transition := Transition[string, string]{Source: sub.State, Destination: stateD, Trigger: triggerX}
	sub.Exit(context.Background(), transition)
	assert.True(t, executed)
}

func Test_stateRepresentation_Exit_FromSuperToSubstate_SuperExitActionsNotExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	super.ExitActions = append(super.ExitActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			executed = true
			return nil
		},
	})
	transition := Transition[string, string]{Source: super.State, Destination: sub.State, Trigger: triggerX}
	sub.Exit(context.Background(), transition)
	assert.False(t, executed)
}

func Test_stateRepresentation_Exit_Substate_SuperExitActionsExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	super.ExitActions = append(super.ExitActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			executed = true
			return nil
		},
	})
	transition := Transition[string, string]{Source: sub.State, Destination: stateC, Trigger: triggerX}
	sub.Exit(context.Background(), transition)
	assert.True(t, executed)
}

func Test_stateRepresentation_Exit_ActionsExecuteInOrder(t *testing.T) {
	var actual []int
	sr := newstateRepresentation[string, string](stateB)
	sr.ExitActions = append(sr.ExitActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			actual = append(actual, 0)
			return nil
		},
	})
	sr.ExitActions = append(sr.ExitActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			actual = append(actual, 1)
			return nil
		},
	})
	transition := Transition[string, string]{Source: stateB, Destination: stateC, Trigger: triggerX}
	sr.Exit(context.Background(), transition)
	assert.Equal(t, 2, len(actual))
	assert.Equal(t, 0, actual[0])
	assert.Equal(t, 1, actual[1])
}

func Test_stateRepresentation_Exit_Substate_SubstateEntryActionsExecuteBeforeSuperstate(t *testing.T) {
	super, sub := createSuperSubstatePair()
	var order, subOrder, superOrder int
	super.ExitActions = append(super.ExitActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			order += 1
			superOrder = order
			return nil
		},
	})
	sub.ExitActions = append(sub.ExitActions, actionBehaviour[string, string]{
		Action: func(_ context.Context, _ ...interface{}) error {
			order += 1
			subOrder = order
			return nil
		},
	})
	transition := Transition[string, string]{Source: sub.State, Destination: stateC, Trigger: triggerX}
	sub.Exit(context.Background(), transition)
	assert.True(t, subOrder < superOrder)
}
