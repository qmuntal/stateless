package stateless

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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

func createSuperSubstatePair() (*stateRepresentation, *stateRepresentation) {
	super := newstateRepresentation(stateA)
	sub := newstateRepresentation(stateB)
	super.Substates = append(super.Substates, sub)
	sub.Superstate = super
	return super, sub
}

func Test_stateRepresentation_Includes_SameState(t *testing.T) {
	sr := newstateRepresentation(stateB)
	assert.True(t, sr.IncludeState(stateB))
}

func Test_stateRepresentation_Includes_Substate(t *testing.T) {
	sr := newstateRepresentation(stateB)
	sr.Substates = append(sr.Substates, newstateRepresentation(stateC))
	assert.True(t, sr.IncludeState(stateC))
}

func Test_stateRepresentation_Includes_UnrelatedState(t *testing.T) {
	sr := newstateRepresentation(stateB)
	assert.False(t, sr.IncludeState(stateC))
}

func Test_stateRepresentation_Includes_Superstate(t *testing.T) {
	sr := newstateRepresentation(stateB)
	sr.Superstate = newstateRepresentation(stateC)
	assert.False(t, sr.IncludeState(stateC))
}

func Test_stateRepresentation_IsIncludedInState_SameState(t *testing.T) {
	sr := newstateRepresentation(stateB)
	assert.True(t, sr.IsIncludedInState(stateB))
}

func Test_stateRepresentation_IsIncludedInState_Substate(t *testing.T) {
	sr := newstateRepresentation(stateB)
	sr.Substates = append(sr.Substates, newstateRepresentation(stateC))
	assert.False(t, sr.IsIncludedInState(stateC))
}

func Test_stateRepresentation_IsIncludedInState_UnrelatedState(t *testing.T) {
	sr := newstateRepresentation(stateB)
	assert.False(t, sr.IsIncludedInState(stateC))
}

func Test_stateRepresentation_IsIncludedInState_Superstate(t *testing.T) {
	sr := newstateRepresentation(stateB)
	assert.False(t, sr.IsIncludedInState(stateC))
}

func Test_stateRepresentation_CanHandle_TransitionExists_TriggerCannotBeFired(t *testing.T) {
	sr := newstateRepresentation(stateB)
	assert.False(t, sr.CanHandle(context.Background(), triggerX))
}

func Test_stateRepresentation_CanHandle_TransitionDoesNotExist_TriggerCanBeFired(t *testing.T) {
	sr := newstateRepresentation(stateB)
	sr.AddTriggerBehaviour(&ignoredTriggerBehaviour{baseTriggerBehaviour: baseTriggerBehaviour{Trigger: triggerX}})
	assert.True(t, sr.CanHandle(context.Background(), triggerX))
}

func Test_stateRepresentation_CanHandle_TransitionExistsInSupersate_TriggerCanBeFired(t *testing.T) {
	super, sub := createSuperSubstatePair()
	super.AddTriggerBehaviour(&ignoredTriggerBehaviour{baseTriggerBehaviour: baseTriggerBehaviour{Trigger: triggerX}})
	assert.True(t, sub.CanHandle(context.Background(), triggerX))
}

func Test_stateRepresentation_CanHandle_TransitionUnmetGuardConditions_TriggerCannotBeFired(t *testing.T) {
	sr := newstateRepresentation(stateB)
	sr.AddTriggerBehaviour(&transitioningTriggerBehaviour{baseTriggerBehaviour: baseTriggerBehaviour{
		Trigger: triggerX,
		Guard: newtransitionGuard(func(ctx context.Context, args ...interface{}) bool {
			return true
		}, func(ctx context.Context, args ...interface{}) bool {
			return false
		}),
	}, Destination: stateC})
	assert.False(t, sr.CanHandle(context.Background(), triggerX))
}

func Test_stateRepresentation_CanHandle_TransitionGuardConditionsMet_TriggerCanBeFired(t *testing.T) {
	sr := newstateRepresentation(stateB)
	sr.AddTriggerBehaviour(&transitioningTriggerBehaviour{baseTriggerBehaviour: baseTriggerBehaviour{
		Trigger: triggerX,
		Guard: newtransitionGuard(func(ctx context.Context, args ...interface{}) bool {
			return true
		}, func(ctx context.Context, args ...interface{}) bool {
			return true
		}),
	}, Destination: stateC})
	assert.True(t, sr.CanHandle(context.Background(), triggerX))
}

func Test_stateRepresentation_FindHandler_TransitionExistAndSuperstateUnmetGuardConditions_FireNotPossible(t *testing.T) {
	super, sub := createSuperSubstatePair()
	super.AddTriggerBehaviour(&transitioningTriggerBehaviour{baseTriggerBehaviour: baseTriggerBehaviour{
		Trigger: triggerX,
		Guard: newtransitionGuard(func(ctx context.Context, args ...interface{}) bool {
			return true
		}, func(ctx context.Context, args ...interface{}) bool {
			return false
		}),
	}, Destination: stateC})
	handler, ok := sub.FindHandler(context.Background(), triggerX)
	assert.False(t, ok)
	assert.NotNil(t, handler)
	assert.False(t, sub.CanHandle(context.Background(), triggerX))
	assert.False(t, super.CanHandle(context.Background(), triggerX))
}

func Test_stateRepresentation_FindHandler_TransitionExistSuperstateMetGuardConditions_CanBeFired(t *testing.T) {
	super, sub := createSuperSubstatePair()
	super.AddTriggerBehaviour(&transitioningTriggerBehaviour{baseTriggerBehaviour: baseTriggerBehaviour{
		Trigger: triggerX,
		Guard: newtransitionGuard(func(ctx context.Context, args ...interface{}) bool {
			return true
		}, func(ctx context.Context, args ...interface{}) bool {
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
	sr := newstateRepresentation(stateB)
	transition := Transition{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition
	sr.EntryActions = append(sr.EntryActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			actualTransition = transition
			return nil
		},
	})
	sr.Enter(context.Background(), transition)
	assert.Equal(t, transition, actualTransition)
}

func Test_stateRepresentation_Enter_LeavingActionsNotExecuted(t *testing.T) {
	sr := newstateRepresentation(stateA)
	transition := Transition{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition
	sr.ExitActions = append(sr.ExitActions, actionBehaviour{
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
	sub.EntryActions = append(sub.EntryActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			executed = true
			return nil
		},
	})
	transition := Transition{Source: super.State, Destination: sub.State, Trigger: triggerX}
	sub.Enter(context.Background(), transition)
	assert.True(t, executed)
}

func Test_stateRepresentation_Enter_SuperFromSubstate_SuperEntryActionsNotExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	super.EntryActions = append(super.EntryActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			executed = true
			return nil
		},
	})
	transition := Transition{Source: super.State, Destination: sub.State, Trigger: triggerX}
	sub.Enter(context.Background(), transition)
	assert.False(t, executed)
}

func Test_stateRepresentation_Enter_Substate_SuperEntryActionsExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	super.EntryActions = append(super.EntryActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			executed = true
			return nil
		},
	})
	transition := Transition{Source: stateC, Destination: sub.State, Trigger: triggerX}
	sub.Enter(context.Background(), transition)
	assert.True(t, executed)
}

func Test_stateRepresentation_Enter_ActionsExecuteInOrder(t *testing.T) {
	var actual []int
	sr := newstateRepresentation(stateB)
	sr.EntryActions = append(sr.EntryActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			actual = append(actual, 0)
			return nil
		},
	})
	sr.EntryActions = append(sr.EntryActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			actual = append(actual, 1)
			return nil
		},
	})
	transition := Transition{Source: stateA, Destination: stateB, Trigger: triggerX}
	sr.Enter(context.Background(), transition)
	assert.Equal(t, 2, len(actual))
	assert.Equal(t, 0, actual[0])
	assert.Equal(t, 1, actual[1])
}

func Test_stateRepresentation_Enter_Substate_SuperstateEntryActionsExecuteBeforeSubstate(t *testing.T) {
	super, sub := createSuperSubstatePair()
	var order, subOrder, superOrder int
	super.EntryActions = append(super.EntryActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			order += 1
			superOrder = order
			return nil
		},
	})
	sub.EntryActions = append(sub.EntryActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			order += 1
			subOrder = order
			return nil
		},
	})
	transition := Transition{Source: stateC, Destination: sub.State, Trigger: triggerX}
	sub.Enter(context.Background(), transition)
	assert.True(t, superOrder < subOrder)
}

func Test_stateRepresentation_Exit_EnteringActionsNotExecuted(t *testing.T) {
	sr := newstateRepresentation(stateB)
	transition := Transition{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition
	sr.EntryActions = append(sr.EntryActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			actualTransition = transition
			return nil
		},
	})
	sr.Exit(context.Background(), transition)
	assert.Zero(t, actualTransition)
}

func Test_stateRepresentation_Exit_LeavingActionsExecuted(t *testing.T) {
	sr := newstateRepresentation(stateA)
	transition := Transition{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition
	sr.ExitActions = append(sr.ExitActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			actualTransition = transition
			return nil
		},
	})
	sr.Exit(context.Background(), transition)
	assert.Equal(t, transition, actualTransition)
}

func Test_stateRepresentation_Exit_FromSubToSuperstate_SubstateExitActionsExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	sub.ExitActions = append(sub.ExitActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			executed = true
			return nil
		},
	})
	transition := Transition{Source: sub.State, Destination: super.State, Trigger: triggerX}
	sub.Exit(context.Background(), transition)
	assert.True(t, executed)
}

func Test_stateRepresentation_Exit_FromSuperToSubstate_SuperExitActionsNotExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	super.ExitActions = append(super.ExitActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			executed = true
			return nil
		},
	})
	transition := Transition{Source: super.State, Destination: sub.State, Trigger: triggerX}
	sub.Exit(context.Background(), transition)
	assert.False(t, executed)
}

func Test_stateRepresentation_Exit_Substate_SuperExitActionsExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	super.ExitActions = append(super.ExitActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			executed = true
			return nil
		},
	})
	transition := Transition{Source: sub.State, Destination: stateC, Trigger: triggerX}
	sub.Exit(context.Background(), transition)
	assert.True(t, executed)
}

func Test_stateRepresentation_Exit_ActionsExecuteInOrder(t *testing.T) {
	var actual []int
	sr := newstateRepresentation(stateB)
	sr.ExitActions = append(sr.ExitActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			actual = append(actual, 0)
			return nil
		},
	})
	sr.ExitActions = append(sr.ExitActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			actual = append(actual, 1)
			return nil
		},
	})
	transition := Transition{Source: stateB, Destination: stateC, Trigger: triggerX}
	sr.Exit(context.Background(), transition)
	assert.Equal(t, 2, len(actual))
	assert.Equal(t, 0, actual[0])
	assert.Equal(t, 1, actual[1])
}

func Test_stateRepresentation_Exit_Substate_SubstateEntryActionsExecuteBeforeSuperstate(t *testing.T) {
	super, sub := createSuperSubstatePair()
	var order, subOrder, superOrder int
	super.ExitActions = append(super.ExitActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			order += 1
			superOrder = order
			return nil
		},
	})
	sub.ExitActions = append(sub.ExitActions, actionBehaviour{
		Action: func(_ context.Context, _ ...interface{}) error {
			order += 1
			subOrder = order
			return nil
		},
	})
	transition := Transition{Source: sub.State, Destination: stateC, Trigger: triggerX}
	sub.Exit(context.Background(), transition)
	assert.True(t, subOrder < superOrder)
}
