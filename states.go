package stateless

import (
	"context"
	"fmt"
)

type actionBehaviour[S State, T Trigger] struct {
	Action      ActionFunc
	Description invocationInfo
	Trigger     *T
}

func (a actionBehaviour[S, T]) Execute(ctx context.Context, transition Transition[S, T], args ...interface{}) (err error) {
	if a.Trigger == nil || *a.Trigger == transition.Trigger {
		ctx = withTransition(ctx, transition)
		err = a.Action(ctx, args...)
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

type stateRepresentation[S State, T Trigger] struct {
	State                   S
	InitialTransitionTarget S
	Superstate              *stateRepresentation[S, T]
	EntryActions            []actionBehaviour[S, T]
	ExitActions             []actionBehaviour[S, T]
	ActivateActions         []actionBehaviourSteady
	DeactivateActions       []actionBehaviourSteady
	Substates               []*stateRepresentation[S, T]
	TriggerBehaviours       map[T][]triggerBehaviour[T]
	HasInitialState         bool
}

func newstateRepresentation[S State, T Trigger](state S) *stateRepresentation[S, T] {
	return &stateRepresentation[S, T]{
		State:             state,
		TriggerBehaviours: make(map[T][]triggerBehaviour[T]),
	}
}

func (sr *stateRepresentation[S, T]) SetInitialTransition(state S) {
	sr.InitialTransitionTarget = state
	sr.HasInitialState = true
}

func (sr *stateRepresentation[S, T]) state() S {
	return sr.State
}

func (sr *stateRepresentation[S, T]) CanHandle(ctx context.Context, trigger T, args ...interface{}) (ok bool) {
	_, ok = sr.FindHandler(ctx, trigger, args...)
	return
}

func (sr *stateRepresentation[S, T]) FindHandler(ctx context.Context, trigger T, args ...interface{}) (handler triggerBehaviourResult[T], ok bool) {
	handler, ok = sr.findHandler(ctx, trigger, args...)
	if ok || sr.Superstate == nil {
		return
	}
	handler, ok = sr.Superstate.FindHandler(ctx, trigger, args...)
	return
}

func (sr *stateRepresentation[S, T]) findHandler(ctx context.Context, trigger T, args ...interface{}) (result triggerBehaviourResult[T], ok bool) {
	var (
		possibleBehaviours []triggerBehaviour[T]
	)
	if possibleBehaviours, ok = sr.TriggerBehaviours[trigger]; !ok {
		return
	}
	allResults := make([]triggerBehaviourResult[T], 0, len(possibleBehaviours))
	for _, behaviour := range possibleBehaviours {
		allResults = append(allResults, triggerBehaviourResult[T]{
			Handler:              behaviour,
			UnmetGuardConditions: behaviour.UnmetGuardConditions(ctx, args...),
		})
	}
	metResults := make([]triggerBehaviourResult[T], 0, len(allResults))
	unmetResults := make([]triggerBehaviourResult[T], 0, len(allResults))
	for _, result := range allResults {
		if len(result.UnmetGuardConditions) == 0 {
			metResults = append(metResults, result)
		} else {
			unmetResults = append(unmetResults, result)
		}
	}
	if len(metResults) > 1 {
		panic(fmt.Sprintf("stateless: Multiple permitted exit transitions are configured from state '%v' for trigger '%v'. Guard clauses must be mutually exclusive.", sr.State, trigger))
	}
	if len(metResults) == 1 {
		result, ok = metResults[0], true
	} else if len(unmetResults) > 0 {
		result, ok = unmetResults[0], false
	}
	return
}

func (sr *stateRepresentation[S, T]) Activate(ctx context.Context) error {
	if sr.Superstate != nil {
		if err := sr.Superstate.Activate(ctx); err != nil {
			return err
		}
	}
	return sr.executeActivationActions(ctx)
}

func (sr *stateRepresentation[S, T]) Deactivate(ctx context.Context) error {
	if err := sr.executeDeactivationActions(ctx); err != nil {
		return err
	}
	if sr.Superstate != nil {
		return sr.Superstate.Deactivate(ctx)
	}
	return nil
}

func (sr *stateRepresentation[S, T]) Enter(ctx context.Context, transition Transition[S, T], args ...interface{}) error {
	if transition.IsReentry() {
		return sr.executeEntryActions(ctx, transition, args...)
	}
	if sr.IncludeState(transition.Source) {
		return nil
	}
	if sr.Superstate != nil && !transition.isInitial {
		if err := sr.Superstate.Enter(ctx, transition, args...); err != nil {
			return err
		}
	}
	return sr.executeEntryActions(ctx, transition, args...)
}

func (sr *stateRepresentation[S, T]) Exit(ctx context.Context, transition Transition[S, T], args ...interface{}) (err error) {
	isReentry := transition.IsReentry()
	if !isReentry && sr.IncludeState(transition.Destination) {
		return
	}

	err = sr.executeExitActions(ctx, transition, args...)
	// Must check if there is a superstate, and if we are leaving that superstate
	if err == nil && !isReentry && sr.Superstate != nil {
		// Check if destination is within the state list
		if sr.IsIncludedInState(transition.Destination) {
			// Destination state is within the list, exit first superstate only if it is NOT the the first
			if sr.Superstate.state() != transition.Destination {
				err = sr.Superstate.Exit(ctx, transition, args...)
			}
		} else {
			// Exit the superstate as well
			err = sr.Superstate.Exit(ctx, transition, args...)
		}
	}
	return
}

func (sr *stateRepresentation[S, T]) InternalAction(ctx context.Context, transition Transition[S, T], args ...interface{}) error {
	var internalTransition *internalTriggerBehaviour[S, T]
	var stateRep *stateRepresentation[S, T] = sr
	for stateRep != nil {
		if result, ok := stateRep.findHandler(ctx, transition.Trigger, args...); ok {
			switch t := result.Handler.(type) {
			case *internalTriggerBehaviour[S, T]:
				internalTransition = t
			}
			break
		}
		stateRep = stateRep.Superstate
	}
	if internalTransition == nil {
		panic("stateless: The configuration is incorrect, no action assigned to this internal transition.")
	}
	return internalTransition.Execute(ctx, transition, args...)
}

func (sr *stateRepresentation[S, T]) IncludeState(state S) bool {
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

func (sr *stateRepresentation[S, T]) IsIncludedInState(state S) bool {
	if state == sr.State {
		return true
	}
	if sr.Superstate != nil {
		return sr.Superstate.IsIncludedInState(state)
	}
	return false
}

func (sr *stateRepresentation[S, T]) AddTriggerBehaviour(tb triggerBehaviour[T]) {
	trigger := tb.GetTrigger()
	sr.TriggerBehaviours[trigger] = append(sr.TriggerBehaviours[trigger], tb)

}

func (sr *stateRepresentation[S, T]) PermittedTriggers(ctx context.Context, args ...interface{}) (triggers []T) {
	for key, value := range sr.TriggerBehaviours {
		for _, tb := range value {
			if len(tb.UnmetGuardConditions(ctx, args...)) == 0 {
				triggers = append(triggers, key)
				break
			}
		}
	}
	if sr.Superstate != nil {
		triggers = append(triggers, sr.Superstate.PermittedTriggers(ctx, args...)...)
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

func (sr *stateRepresentation[S, T]) executeActivationActions(ctx context.Context) error {
	for _, a := range sr.ActivateActions {
		if err := a.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (sr *stateRepresentation[S, T]) executeDeactivationActions(ctx context.Context) error {
	for _, a := range sr.DeactivateActions {
		if err := a.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (sr *stateRepresentation[S, T]) executeEntryActions(ctx context.Context, transition Transition[S, T], args ...interface{}) error {
	for _, a := range sr.EntryActions {
		if err := a.Execute(ctx, transition, args...); err != nil {
			return err
		}
	}
	return nil
}

func (sr *stateRepresentation[S, T]) executeExitActions(ctx context.Context, transition Transition[S, T], args ...interface{}) error {
	for _, a := range sr.ExitActions {
		if err := a.Execute(ctx, transition, args...); err != nil {
			return err
		}
	}
	return nil
}
