package stateless

import (
	"context"
	"fmt"
)

type actionBehaviour[T any] struct {
	Action      ActionFunc[T]
	Description invocationInfo
	Trigger     *Trigger
}

func (a actionBehaviour[T]) Execute(ctx context.Context, transition Transition, extendedState T, args ...interface{}) (err error) {
	if a.Trigger == nil || *a.Trigger == transition.Trigger {
		ctx = withTransition(ctx, transition)
		err = a.Action(ctx, extendedState, args...)
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

type stateRepresentation[T any] struct {
	State                   State
	InitialTransitionTarget State
	Superstate              *stateRepresentation[T]
	EntryActions            []actionBehaviour[T]
	ExitActions             []actionBehaviour[T]
	ActivateActions         []actionBehaviourSteady
	DeactivateActions       []actionBehaviourSteady
	Substates               []*stateRepresentation[T]
	TriggerBehaviours       map[Trigger][]triggerBehaviour[T]
	HasInitialState         bool
}

func newstateRepresentation[T any](state State) *stateRepresentation[T] {
	return &stateRepresentation[T]{
		State:             state,
		TriggerBehaviours: make(map[Trigger][]triggerBehaviour[T]),
	}
}

func (sr *stateRepresentation[T]) SetInitialTransition(state State) {
	sr.InitialTransitionTarget = state
	sr.HasInitialState = true
}

func (sr *stateRepresentation[T]) state() State {
	return sr.State
}

func (sr *stateRepresentation[T]) CanHandle(ctx context.Context, trigger Trigger, extendedState T, args ...interface{}) (ok bool) {
	_, ok = sr.FindHandler(ctx, trigger, extendedState, args...)
	return
}

func (sr *stateRepresentation[T]) FindHandler(ctx context.Context, trigger Trigger, extendedState T, args ...interface{}) (handler triggerBehaviourResult[T], ok bool) {
	handler, ok = sr.findHandler(ctx, trigger, extendedState, args...)
	if ok || sr.Superstate == nil {
		return
	}
	handler, ok = sr.Superstate.FindHandler(ctx, trigger, extendedState, args...)
	return
}

func (sr *stateRepresentation[T]) findHandler(ctx context.Context, trigger Trigger, extendedState T, args ...interface{}) (result triggerBehaviourResult[T], ok bool) {
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
			UnmetGuardConditions: behaviour.UnmetGuardConditions(ctx, extendedState, args...),
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

func (sr *stateRepresentation[T]) Activate(ctx context.Context) error {
	if sr.Superstate != nil {
		if err := sr.Superstate.Activate(ctx); err != nil {
			return err
		}
	}
	return sr.executeActivationActions(ctx)
}

func (sr *stateRepresentation[T]) Deactivate(ctx context.Context) error {
	if err := sr.executeDeactivationActions(ctx); err != nil {
		return err
	}
	if sr.Superstate != nil {
		return sr.Superstate.Deactivate(ctx)
	}
	return nil
}

func (sr *stateRepresentation[T]) Enter(ctx context.Context, transition Transition, extendedState T, args ...interface{}) error {
	if transition.IsReentry() {
		return sr.executeEntryActions(ctx, transition, extendedState, args...)
	}
	if sr.IncludeState(transition.Source) {
		return nil
	}
	if sr.Superstate != nil && !transition.isInitial {
		if err := sr.Superstate.Enter(ctx, transition, extendedState, args...); err != nil {
			return err
		}
	}
	return sr.executeEntryActions(ctx, transition, extendedState, args...)
}

func (sr *stateRepresentation[T]) Exit(ctx context.Context, transition Transition, extendedState T, args ...interface{}) (err error) {
	isReentry := transition.IsReentry()
	if !isReentry && sr.IncludeState(transition.Destination) {
		return
	}

	err = sr.executeExitActions(ctx, transition, extendedState, args...)
	// Must check if there is a superstate, and if we are leaving that superstate
	if err == nil && !isReentry && sr.Superstate != nil {
		// Check if destination is within the state list
		if sr.IsIncludedInState(transition.Destination) {
			// Destination state is within the list, exit first superstate only if it is NOT the the first
			if sr.Superstate.state() != transition.Destination {
				err = sr.Superstate.Exit(ctx, transition, extendedState, args...)
			}
		} else {
			// Exit the superstate as well
			err = sr.Superstate.Exit(ctx, transition, extendedState, args...)
		}
	}
	return
}

func (sr *stateRepresentation[T]) InternalAction(ctx context.Context, transition Transition, extendedState T, args ...interface{}) error {
	var internalTransition *internalTriggerBehaviour[T]
	var stateRep *stateRepresentation[T] = sr
	for stateRep != nil {
		if result, ok := stateRep.findHandler(ctx, transition.Trigger, extendedState, args...); ok {
			switch t := result.Handler.(type) {
			case *internalTriggerBehaviour[T]:
				internalTransition = t
			}
			break
		}
		stateRep = stateRep.Superstate
	}
	if internalTransition == nil {
		panic("stateless: The configuration is incorrect, no action assigned to this internal transition.")
	}
	return internalTransition.Execute(ctx, transition, extendedState, args...)
}

func (sr *stateRepresentation[T]) IncludeState(state State) bool {
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

func (sr *stateRepresentation[T]) IsIncludedInState(state State) bool {
	if state == sr.State {
		return true
	}
	if sr.Superstate != nil {
		return sr.Superstate.IsIncludedInState(state)
	}
	return false
}

func (sr *stateRepresentation[T]) AddTriggerBehaviour(tb triggerBehaviour[T]) {
	trigger := tb.GetTrigger()
	sr.TriggerBehaviours[trigger] = append(sr.TriggerBehaviours[trigger], tb)

}

func (sr *stateRepresentation[T]) PermittedTriggers(ctx context.Context, extendedState T, args ...interface{}) (triggers []Trigger) {
	for key, value := range sr.TriggerBehaviours {
		for _, tb := range value {
			if len(tb.UnmetGuardConditions(ctx, extendedState, args...)) == 0 {
				triggers = append(triggers, key)
				break
			}
		}
	}
	if sr.Superstate != nil {
		triggers = append(triggers, sr.Superstate.PermittedTriggers(ctx, extendedState, args...)...)
		// remove duplicated
		seen := make(map[Trigger]struct{}, len(triggers))
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

func (sr *stateRepresentation[T]) executeActivationActions(ctx context.Context) error {
	for _, a := range sr.ActivateActions {
		if err := a.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (sr *stateRepresentation[T]) executeDeactivationActions(ctx context.Context) error {
	for _, a := range sr.DeactivateActions {
		if err := a.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (sr *stateRepresentation[T]) executeEntryActions(ctx context.Context, transition Transition, extendedState T, args ...interface{}) error {
	for _, a := range sr.EntryActions {
		if err := a.Execute(ctx, transition, extendedState, args...); err != nil {
			return err
		}
	}
	return nil
}

func (sr *stateRepresentation[T]) executeExitActions(ctx context.Context, transition Transition, extendedState T, args ...interface{}) error {
	for _, a := range sr.ExitActions {
		if err := a.Execute(ctx, transition, extendedState, args...); err != nil {
			return err
		}
	}
	return nil
}
