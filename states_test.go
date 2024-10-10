package stateless

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func createSuperSubstatePair() (*stateRepresentation[string, string, any], *stateRepresentation[string, string, any]) {
	super := newstateRepresentation[string, string, any](stateA)
	sub := newstateRepresentation[string, string, any](stateB)
	super.Substates = append(super.Substates, sub)
	sub.Superstate = super
	return super, sub
}

func Test_stateRepresentation_Includes_SameState(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	if !sr.IncludeState(stateB) {
		t.Fail()
	}
}

func Test_stateRepresentation_Includes_Substate(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	sr.Substates = append(sr.Substates, newstateRepresentation[string, string, any](stateC))
	if !sr.IncludeState(stateC) {
		t.Fail()
	}
}

func Test_stateRepresentation_Includes_UnrelatedState(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	if sr.IncludeState(stateC) {
		t.Fail()
	}
}

func Test_stateRepresentation_Includes_Superstate(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	sr.Superstate = newstateRepresentation[string, string, any](stateC)
	if sr.IncludeState(stateC) {
		t.Fail()
	}
}

func Test_stateRepresentation_IsIncludedInState_SameState(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	if !sr.IsIncludedInState(stateB) {
		t.Fail()
	}
}

func Test_stateRepresentation_IsIncludedInState_Substate(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	sr.Substates = append(sr.Substates, newstateRepresentation[string, string, any](stateC))
	if sr.IsIncludedInState(stateC) {
		t.Fail()
	}
}

func Test_stateRepresentation_IsIncludedInState_UnrelatedState(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	if sr.IsIncludedInState(stateC) {
		t.Fail()
	}
}

func Test_stateRepresentation_IsIncludedInState_Superstate(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	if sr.IsIncludedInState(stateC) {
		t.Fail()
	}
}

func Test_stateRepresentation_CanHandle_TransitionExists_TriggerCannotBeFired(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	if sr.CanHandle(context.Background(), triggerX, nil) {
		t.Fail()
	}
}

func Test_stateRepresentation_CanHandle_TransitionDoesNotExist_TriggerCanBeFired(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	sr.AddTriggerBehaviour(&ignoredTriggerBehaviour[string, any]{baseTriggerBehaviour: baseTriggerBehaviour[string, any]{Trigger: triggerX}})
	if !sr.CanHandle(context.Background(), triggerX, nil) {
		t.Fail()
	}
}

func Test_stateRepresentation_CanHandle_TransitionExistsInSupersate_TriggerCanBeFired(t *testing.T) {
	super, sub := createSuperSubstatePair()
	super.AddTriggerBehaviour(&ignoredTriggerBehaviour[string, any]{baseTriggerBehaviour: baseTriggerBehaviour[string, any]{Trigger: triggerX}})
	if !sub.CanHandle(context.Background(), triggerX, nil) {
		t.Fail()
	}
}

func Test_stateRepresentation_CanHandle_TransitionUnmetGuardConditions_TriggerCannotBeFired(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	sr.AddTriggerBehaviour(&transitioningTriggerBehaviour[string, string, any]{baseTriggerBehaviour: baseTriggerBehaviour[string, any]{
		Trigger: triggerX,
		Guard: newtransitionGuard(func(_ context.Context, _ any) bool {
			return true
		}, func(_ context.Context, _ any) bool {
			return false
		}),
	}, Destination: stateC})
	if sr.CanHandle(context.Background(), triggerX, nil) {
		t.Fail()
	}
}

func Test_stateRepresentation_CanHandle_TransitionGuardConditionsMet_TriggerCanBeFired(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	sr.AddTriggerBehaviour(&transitioningTriggerBehaviour[string, string, any]{baseTriggerBehaviour: baseTriggerBehaviour[string, any]{
		Trigger: triggerX,
		Guard: newtransitionGuard(func(_ context.Context, _ any) bool {
			return true
		}, func(_ context.Context, _ any) bool {
			return true
		}),
	}, Destination: stateC})
	if !sr.CanHandle(context.Background(), triggerX, nil) {
		t.Fail()
	}
}

func Test_stateRepresentation_FindHandler_TransitionExistAndSuperstateUnmetGuardConditions_FireNotPossible(t *testing.T) {
	super, sub := createSuperSubstatePair()
	super.AddTriggerBehaviour(&transitioningTriggerBehaviour[string, string, any]{baseTriggerBehaviour: baseTriggerBehaviour[string, any]{
		Trigger: triggerX,
		Guard: newtransitionGuard(func(_ context.Context, _ any) bool {
			return true
		}, func(_ context.Context, _ any) bool {
			return false
		}),
	}, Destination: stateC})
	handler, ok := sub.FindHandler(context.Background(), triggerX, nil)
	if ok {
		t.Fail()
	}
	if sub.CanHandle(context.Background(), triggerX, nil) {
		t.Fail()
	}
	if super.CanHandle(context.Background(), triggerX, nil) {
		t.Fail()
	}
	if handler.Handler.GuardConditionMet(context.Background(), nil) {
		t.Fail()
	}
}

func Test_stateRepresentation_FindHandler_TransitionExistSuperstateMetGuardConditions_CanBeFired(t *testing.T) {
	super, sub := createSuperSubstatePair()
	super.AddTriggerBehaviour(&transitioningTriggerBehaviour[string, string, any]{baseTriggerBehaviour: baseTriggerBehaviour[string, any]{
		Trigger: triggerX,
		Guard: newtransitionGuard(func(_ context.Context, _ any) bool {
			return true
		}, func(_ context.Context, _ any) bool {
			return true
		}),
	}, Destination: stateC})
	handler, ok := sub.FindHandler(context.Background(), triggerX, nil)
	if !ok {
		t.Fail()
	}
	if !sub.CanHandle(context.Background(), triggerX, nil) {
		t.Fail()
	}
	if !super.CanHandle(context.Background(), triggerX, nil) {
		t.Fail()
	}
	if !handler.Handler.GuardConditionMet(context.Background(), nil) {
		t.Error("expected guard condition to be met")
	}
	if len(handler.UnmetGuardConditions) != 0 {
		t.Error("expected no unmet guard conditions")
	}
}

func Test_stateRepresentation_Enter_EnteringActionsExecuted(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	transition := Transition[string, string]{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition[string, string]
	sr.EntryActions = append(sr.EntryActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			actualTransition = transition
			return nil
		},
	})
	if err := sr.Enter(context.Background(), transition, nil); err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(transition, actualTransition) {
		t.Error("expected transition to be passed to action")
	}
}

func Test_stateRepresentation_Enter_EnteringActionsExecuted_Error(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	transition := Transition[string, string]{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition[string, string]
	sr.EntryActions = append(sr.EntryActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			return errors.New("")
		},
	})
	if err := sr.Enter(context.Background(), transition, nil); err == nil {
		t.Error("error expected")
	}
	if reflect.DeepEqual(transition, actualTransition) {
		t.Error("transition should not be passed to action")
	}
}

func Test_stateRepresentation_Enter_LeavingActionsNotExecuted(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateA)
	transition := Transition[string, string]{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition[string, string]
	sr.ExitActions = append(sr.ExitActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			actualTransition = transition
			return nil
		},
	})
	sr.Enter(context.Background(), transition, nil)
	if actualTransition != (Transition[string, string]{}) {
		t.Error("expected transition to not be passed to action")
	}
}

func Test_stateRepresentation_Enter_FromSubToSuperstate_SubstateEntryActionsExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	sub.EntryActions = append(sub.EntryActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			executed = true
			return nil
		},
	})
	transition := Transition[string, string]{Source: super.State, Destination: sub.State, Trigger: triggerX}
	sub.Enter(context.Background(), transition, nil)
	if !executed {
		t.Error("expected substate entry actions to be executed")
	}
}

func Test_stateRepresentation_Enter_SuperFromSubstate_SuperEntryActionsNotExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	super.EntryActions = append(super.EntryActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			executed = true
			return nil
		},
	})
	transition := Transition[string, string]{Source: super.State, Destination: sub.State, Trigger: triggerX}
	sub.Enter(context.Background(), transition, nil)
	if executed {
		t.Error("expected superstate entry actions not to be executed")
	}
}

func Test_stateRepresentation_Enter_Substate_SuperEntryActionsExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	super.EntryActions = append(super.EntryActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			executed = true
			return nil
		},
	})
	transition := Transition[string, string]{Source: stateC, Destination: sub.State, Trigger: triggerX}
	sub.Enter(context.Background(), transition, nil)
	if !executed {
		t.Error("expected superstate entry actions to be executed")
	}
}

func Test_stateRepresentation_Enter_ActionsExecuteInOrder(t *testing.T) {
	var actual []int
	sr := newstateRepresentation[string, string, any](stateB)
	sr.EntryActions = append(sr.EntryActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			actual = append(actual, 0)
			return nil
		},
	})
	sr.EntryActions = append(sr.EntryActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			actual = append(actual, 1)
			return nil
		},
	})
	transition := Transition[string, string]{Source: stateA, Destination: stateB, Trigger: triggerX}
	sr.Enter(context.Background(), transition, nil)
	want := []int{0, 1}
	if !reflect.DeepEqual(actual, want) {
		t.Errorf("expected %v, got %v", want, actual)
	}
}

func Test_stateRepresentation_Enter_Substate_SuperstateEntryActionsExecuteBeforeSubstate(t *testing.T) {
	super, sub := createSuperSubstatePair()
	var order, subOrder, superOrder int
	super.EntryActions = append(super.EntryActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			order += 1
			superOrder = order
			return nil
		},
	})
	sub.EntryActions = append(sub.EntryActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			order += 1
			subOrder = order
			return nil
		},
	})
	transition := Transition[string, string]{Source: stateC, Destination: sub.State, Trigger: triggerX}
	sub.Enter(context.Background(), transition, nil)
	if superOrder >= subOrder {
		t.Error("expected superstate entry actions to execute before substate entry actions")
	}
}

func Test_stateRepresentation_Exit_EnteringActionsNotExecuted(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateB)
	transition := Transition[string, string]{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition[string, string]
	sr.EntryActions = append(sr.EntryActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			actualTransition = transition
			return nil
		},
	})
	sr.Exit(context.Background(), transition, nil)
	if actualTransition != (Transition[string, string]{}) {
		t.Error("expected transition to not be passed to action")
	}
}

func Test_stateRepresentation_Exit_LeavingActionsExecuted(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateA)
	transition := Transition[string, string]{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition[string, string]
	sr.ExitActions = append(sr.ExitActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			actualTransition = transition
			return nil
		},
	})
	if err := sr.Exit(context.Background(), transition, nil); err != nil {
		t.Error(err)
	}
	if actualTransition != transition {
		t.Error("expected transition to be passed to leaving actions")
	}
}

func Test_stateRepresentation_Exit_LeavingActionsExecuted_Error(t *testing.T) {
	sr := newstateRepresentation[string, string, any](stateA)
	transition := Transition[string, string]{Source: stateA, Destination: stateB, Trigger: triggerX}
	var actualTransition Transition[string, string]
	sr.ExitActions = append(sr.ExitActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			return errors.New("")
		},
	})
	if err := sr.Exit(context.Background(), transition, nil); err == nil {
		t.Error("expected error")
	}
	if actualTransition == transition {
		t.Error("expected transition to not be passed to leaving actions")
	}
}

func Test_stateRepresentation_Exit_FromSubToSuperstate_SubstateExitActionsExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	sub.ExitActions = append(sub.ExitActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			executed = true
			return nil
		},
	})
	transition := Transition[string, string]{Source: sub.State, Destination: super.State, Trigger: triggerX}
	sub.Exit(context.Background(), transition, nil)
	if !executed {
		t.Error("expected substate exit actions to be executed")
	}
}

func Test_stateRepresentation_Exit_FromSubToOther_SuperstateExitActionsExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	supersuper := newstateRepresentation[string, string, any](stateC)
	super.Superstate = supersuper
	supersuper.Superstate = newstateRepresentation[string, string, any](stateD)
	executed := false
	super.ExitActions = append(super.ExitActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			executed = true
			return nil
		},
	})
	transition := Transition[string, string]{Source: sub.State, Destination: stateD, Trigger: triggerX}
	sub.Exit(context.Background(), transition, nil)
	if !executed {
		t.Error("expected superstate exit actions to be executed")
	}
}

func Test_stateRepresentation_Exit_FromSuperToSubstate_SuperExitActionsNotExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	super.ExitActions = append(super.ExitActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			executed = true
			return nil
		},
	})
	transition := Transition[string, string]{Source: super.State, Destination: sub.State, Trigger: triggerX}
	sub.Exit(context.Background(), transition, nil)
	if executed {
		t.Error("expected superstate exit actions to not be executed")
	}
}

func Test_stateRepresentation_Exit_Substate_SuperExitActionsExecuted(t *testing.T) {
	super, sub := createSuperSubstatePair()
	executed := false
	super.ExitActions = append(super.ExitActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			executed = true
			return nil
		},
	})
	transition := Transition[string, string]{Source: sub.State, Destination: stateC, Trigger: triggerX}
	sub.Exit(context.Background(), transition, nil)
	if !executed {
		t.Error("expected superstate exit actions to be executed")
	}
}

func Test_stateRepresentation_Exit_ActionsExecuteInOrder(t *testing.T) {
	var actual []int
	sr := newstateRepresentation[string, string, any](stateB)
	sr.ExitActions = append(sr.ExitActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			actual = append(actual, 0)
			return nil
		},
	})
	sr.ExitActions = append(sr.ExitActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			actual = append(actual, 1)
			return nil
		},
	})
	transition := Transition[string, string]{Source: stateB, Destination: stateC, Trigger: triggerX}
	sr.Exit(context.Background(), transition, nil)
	want := []int{0, 1}
	if !reflect.DeepEqual(actual, want) {
		t.Errorf("expected %v, got %v", want, actual)
	}
}

func Test_stateRepresentation_Exit_Substate_SubstateEntryActionsExecuteBeforeSuperstate(t *testing.T) {
	super, sub := createSuperSubstatePair()
	var order, subOrder, superOrder int
	super.ExitActions = append(super.ExitActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			order += 1
			superOrder = order
			return nil
		},
	})
	sub.ExitActions = append(sub.ExitActions, actionBehaviour[string, string, any]{
		Action: func(_ context.Context, _ any) error {
			order += 1
			subOrder = order
			return nil
		},
	})
	transition := Transition[string, string]{Source: sub.State, Destination: stateC, Trigger: triggerX}
	sub.Exit(context.Background(), transition, nil)
	if subOrder >= superOrder {
		t.Error("expected substate exit actions to execute before superstate")
	}
}
