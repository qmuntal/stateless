package stateless

import (
	"context"
	"fmt"
)

type actionBehaviour struct {
	Action      ActionFunc
	Description invocationInfo
	Trigger     *Trigger
}

func (a actionBehaviour) Execute(ctx context.Context, transition Transition, args ...any) (err error) {
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

type stateRepresentation struct {
	State                   State
	InitialTransitionTarget State
	Superstate              *stateRepresentation
	EntryActions            []actionBehaviour
	ExitActions             []actionBehaviour
	ActivateActions         []actionBehaviourSteady
	DeactivateActions       []actionBehaviourSteady
	Substates               []*stateRepresentation
	TriggerBehaviours       map[Trigger][]triggerBehaviour
	HasInitialState         bool
}

func newstateRepresentation(state State) *stateRepresentation {
	return &stateRepresentation{
		State:             state,
		TriggerBehaviours: make(map[Trigger][]triggerBehaviour),
	}
}

func (sr *stateRepresentation) SetInitialTransition(state State) {
	sr.InitialTransitionTarget = state
	sr.HasInitialState = true
}

func (sr *stateRepresentation) state() State {
	return sr.State
}

func (sr *stateRepresentation) CanHandle(ctx context.Context, trigger Trigger, args ...any) (ok bool) {
	_, ok = sr.FindHandler(ctx, trigger, args...)
	return
}

func (sr *stateRepresentation) FindHandler(ctx context.Context, trigger Trigger, args ...any) (handler triggerBehaviourResult, ok bool) {
	handler, ok = sr.findHandler(ctx, trigger, args...)
	if ok || sr.Superstate == nil {
		return
	}
	handler, ok = sr.Superstate.FindHandler(ctx, trigger, args...)
	return
}

func (sr *stateRepresentation) findHandler(ctx context.Context, trigger Trigger, args ...any) (result triggerBehaviourResult, ok bool) {
	possibleBehaviours, ok := sr.TriggerBehaviours[trigger]
	if !ok {
		return
	}
	var unmet []string
	for _, behaviour := range possibleBehaviours {
		unmet = behaviour.UnmetGuardConditions(ctx, unmet[:0], args...)
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

func (sr *stateRepresentation) Activate(ctx context.Context) error {
	if sr.Superstate != nil {
		if err := sr.Superstate.Activate(ctx); err != nil {
			return err
		}
	}
	return sr.executeActivationActions(ctx)
}

func (sr *stateRepresentation) Deactivate(ctx context.Context) error {
	if err := sr.executeDeactivationActions(ctx); err != nil {
		return err
	}
	if sr.Superstate != nil {
		return sr.Superstate.Deactivate(ctx)
	}
	return nil
}

func (sr *stateRepresentation) Enter(ctx context.Context, transition Transition, args ...any) error {
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

func (sr *stateRepresentation) Exit(ctx context.Context, transition Transition, args ...any) (err error) {
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

func (sr *stateRepresentation) InternalAction(ctx context.Context, transition Transition, args ...any) error {
	var internalTransition *internalTriggerBehaviour
	var stateRep *stateRepresentation = sr
	for stateRep != nil {
		if result, ok := stateRep.findHandler(ctx, transition.Trigger, args...); ok {
			switch t := result.Handler.(type) {
			case *internalTriggerBehaviour:
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

func (sr *stateRepresentation) IncludeState(state State) bool {
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

func (sr *stateRepresentation) IsIncludedInState(state State) bool {
	if state == sr.State {
		return true
	}
	if sr.Superstate != nil {
		return sr.Superstate.IsIncludedInState(state)
	}
	return false
}

func (sr *stateRepresentation) AddTriggerBehaviour(tb triggerBehaviour) {
	trigger := tb.GetTrigger()
	sr.TriggerBehaviours[trigger] = append(sr.TriggerBehaviours[trigger], tb)

}

func (sr *stateRepresentation) PermittedTriggers(ctx context.Context, args ...any) (triggers []Trigger) {
	var unmet []string
	for key, value := range sr.TriggerBehaviours {
		for _, tb := range value {
			if len(tb.UnmetGuardConditions(ctx, unmet[:0], args...)) == 0 {
				triggers = append(triggers, key)
				break
			}
		}
	}
	if sr.Superstate != nil {
		triggers = append(triggers, sr.Superstate.PermittedTriggers(ctx, args...)...)
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

func (sr *stateRepresentation) executeActivationActions(ctx context.Context) error {
	for _, a := range sr.ActivateActions {
		if err := a.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (sr *stateRepresentation) executeDeactivationActions(ctx context.Context) error {
	for _, a := range sr.DeactivateActions {
		if err := a.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (sr *stateRepresentation) executeEntryActions(ctx context.Context, transition Transition, args ...any) error {
	for _, a := range sr.EntryActions {
		if err := a.Execute(ctx, transition, args...); err != nil {
			return err
		}
	}
	return nil
}

func (sr *stateRepresentation) executeExitActions(ctx context.Context, transition Transition, args ...any) error {
	for _, a := range sr.ExitActions {
		if err := a.Execute(ctx, transition, args...); err != nil {
			return err
		}
	}
	return nil
}
