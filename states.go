package stateless

import (
	"context"
	"fmt"
)

type actionBehaviour[S State, T Trigger, A any] struct {
	Action      ActionFunc[A]
	Description invocationInfo
	Trigger     *T
}

func (a actionBehaviour[S, T, A]) Execute(ctx context.Context, transition Transition[S, T], arg A) (err error) {
	if a.Trigger == nil || *a.Trigger == transition.Trigger {
		ctx = withTransition(ctx, transition)
		err = a.Action(ctx, arg)
	}
	return
}

type actionBehaviourSteady struct {
	Action      func(ctx context.Context) error
	Description invocationInfo
}

func (a actionBehaviourSteady) Execute(ctx context.Context) error {
	return a.Action(ctx)
}

type stateRepresentation[S State, T Trigger, A any] struct {
	State                   S
	InitialTransitionTarget S
	Superstate              *stateRepresentation[S, T, A]
	EntryActions            []actionBehaviour[S, T, A]
	ExitActions             []actionBehaviour[S, T, A]
	ActivateActions         []actionBehaviourSteady
	DeactivateActions       []actionBehaviourSteady
	Substates               []*stateRepresentation[S, T, A]
	TriggerBehaviours       map[T][]triggerBehaviour[T, A]
	HasInitialState         bool
}

func newstateRepresentation[S State, T Trigger, A any](state S) *stateRepresentation[S, T, A] {
	return &stateRepresentation[S, T, A]{
		State:             state,
		TriggerBehaviours: make(map[T][]triggerBehaviour[T, A]),
	}
}

func (sr *stateRepresentation[S, _, _]) SetInitialTransition(state S) {
	sr.InitialTransitionTarget = state
	sr.HasInitialState = true
}

func (sr *stateRepresentation[S, _, _]) state() S {
	return sr.State
}

func (sr *stateRepresentation[_, T, A]) CanHandle(ctx context.Context, trigger T, arg A) (ok bool) {
	_, ok = sr.FindHandler(ctx, trigger, arg)
	return
}

func (sr *stateRepresentation[_, T, A]) FindHandler(ctx context.Context, trigger T, arg A) (handler triggerBehaviourResult[T, A], ok bool) {
	handler, ok = sr.findHandler(ctx, trigger, arg)
	if ok || sr.Superstate == nil {
		return
	}
	handler, ok = sr.Superstate.FindHandler(ctx, trigger, arg)
	return
}

func (sr *stateRepresentation[_, T, A]) findHandler(ctx context.Context, trigger T, arg A) (result triggerBehaviourResult[T, A], ok bool) {
	possibleBehaviours, ok := sr.TriggerBehaviours[trigger]
	if !ok {
		return
	}
	var unmet []string
	for _, behaviour := range possibleBehaviours {
		unmet = behaviour.UnmetGuardConditions(ctx, unmet[:0], arg) // , arg)
		if len(unmet) == 0 {
			if result.Handler != nil && len(result.UnmetGuardConditions) == 0 {
				panic(fmt.Sprintf("stateless: Multiple permitted exit transitions are configured from state '%v' for trigger '%v'. Guard clauses must be mutually exclusive.", sr.State, trigger))
			}
			result.Handler = behaviour
			result.UnmetGuardConditions = nil
		} else if result.Handler == nil {
			result.Handler = behaviour
			result.UnmetGuardConditions = make([]string, len(unmet))
			copy(result.UnmetGuardConditions, unmet)
		}
	}
	return result, result.Handler != nil && len(result.UnmetGuardConditions) == 0
}

func (sr *stateRepresentation[S, _, _]) Activate(ctx context.Context) error {
	if sr.Superstate != nil {
		if err := sr.Superstate.Activate(ctx); err != nil {
			return err
		}
	}
	return sr.executeActivationActions(ctx)
}

func (sr *stateRepresentation[S, _, _]) Deactivate(ctx context.Context) error {
	if err := sr.executeDeactivationActions(ctx); err != nil {
		return err
	}
	if sr.Superstate != nil {
		return sr.Superstate.Deactivate(ctx)
	}
	return nil
}

func (sr *stateRepresentation[S, T, A]) Enter(ctx context.Context, transition Transition[S, T], arg A) error {
	if transition.IsReentry() {
		return sr.executeEntryActions(ctx, transition, arg)
	}
	if sr.IncludeState(transition.Source) {
		return nil
	}
	if sr.Superstate != nil && !transition.isInitial {
		if err := sr.Superstate.Enter(ctx, transition, arg); err != nil {
			return err
		}
	}
	return sr.executeEntryActions(ctx, transition, arg)
}

func (sr *stateRepresentation[S, T, A]) Exit(ctx context.Context, transition Transition[S, T], arg A) (err error) {
	isReentry := transition.IsReentry()
	if !isReentry && sr.IncludeState(transition.Destination) {
		return
	}

	err = sr.executeExitActions(ctx, transition, arg)
	// Must check if there is a superstate, and if we are leaving that superstate
	if err == nil && !isReentry && sr.Superstate != nil {
		// Check if destination is within the state list
		if sr.IsIncludedInState(transition.Destination) {
			// Destination state is within the list, exit first superstate only if it is NOT the the first
			if sr.Superstate.state() != transition.Destination {
				err = sr.Superstate.Exit(ctx, transition, arg)
			}
		} else {
			// Exit the superstate as well
			err = sr.Superstate.Exit(ctx, transition, arg)
		}
	}
	return
}

func (sr *stateRepresentation[S, T, A]) InternalAction(ctx context.Context, transition Transition[S, T], arg A) error {
	var internalTransition *internalTriggerBehaviour[S, T, A]
	var stateRep = sr
	for stateRep != nil {
		if result, ok := stateRep.findHandler(ctx, transition.Trigger, arg); ok {
			switch t := result.Handler.(type) {
			case *internalTriggerBehaviour[S, T, A]:
				internalTransition = t
			}
			break
		}
		stateRep = stateRep.Superstate
	}
	if internalTransition == nil {
		panic("stateless: The configuration is incorrect, no action assigned to this internal transition.")
	}
	return internalTransition.Execute(ctx, transition, arg)
}

func (sr *stateRepresentation[S, _, _]) IncludeState(state S) bool {
	if state == sr.State {
		return true
	}
	for _, substate := range sr.Substates {
		if substate.IncludeState(state) {
			return true
		}
	}
	return false
}

func (sr *stateRepresentation[S, _, _]) IsIncludedInState(state S) bool {
	if state == sr.State {
		return true
	}
	if sr.Superstate != nil {
		return sr.Superstate.IsIncludedInState(state)
	}
	return false
}

func (sr *stateRepresentation[_, T, A]) AddTriggerBehaviour(tb triggerBehaviour[T, A]) {
	trigger := tb.GetTrigger()
	sr.TriggerBehaviours[trigger] = append(sr.TriggerBehaviours[trigger], tb)

}

func (sr *stateRepresentation[_, T, A]) PermittedTriggers(ctx context.Context, arg A) (triggers []T) {
	var unmet []string
	for key, value := range sr.TriggerBehaviours {
		for _, tb := range value {
			if len(tb.UnmetGuardConditions(ctx, unmet[:0], arg)) == 0 {
				triggers = append(triggers, key)
				break
			}
		}
	}
	if sr.Superstate != nil {
		triggers = append(triggers, sr.Superstate.PermittedTriggers(ctx, arg)...)
		// remove duplicated
		seen := make(map[T]struct{}, len(triggers))
		j := 0
		for _, v := range triggers {
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			triggers[j] = v
			j++
		}
		triggers = triggers[:j]
	}
	return
}

func (sr *stateRepresentation[S, _, _]) executeActivationActions(ctx context.Context) error {
	for _, a := range sr.ActivateActions {
		if err := a.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (sr *stateRepresentation[S, _, _]) executeDeactivationActions(ctx context.Context) error {
	for _, a := range sr.DeactivateActions {
		if err := a.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (sr *stateRepresentation[S, T, A]) executeEntryActions(ctx context.Context, transition Transition[S, T], arg A) error {
	for _, a := range sr.EntryActions {
		if err := a.Execute(ctx, transition, arg); err != nil {
			return err
		}
	}
	return nil
}

func (sr *stateRepresentation[S, T, A]) executeExitActions(ctx context.Context, transition Transition[S, T], arg A) error {
	for _, a := range sr.ExitActions {
		if err := a.Execute(ctx, transition, arg); err != nil {
			return err
		}
	}
	return nil
}
