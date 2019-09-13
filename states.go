package stateless

import (
	"context"
	"fmt"
)

type stateReference struct {
	State State
}

type actionBehaviour struct {
	Action      func(ctx context.Context, args ...interface{}) error
	Description invocationInfo
	Trigger     *Trigger
}

func (a actionBehaviour) Execute(ctx context.Context, transition Transition, args ...interface{}) (err error) {
	if a.Trigger == nil || *a.Trigger == transition.Trigger {
		ctx = withTransition(ctx, transition)
		err = a.Action(ctx, args...)
	}
	return
}

type actionBehaviourSteady struct {
	Action func(ctx context.Context) error
}

func (a actionBehaviourSteady) Execute(ctx context.Context) error {
	return a.Action(ctx)
}

type superset interface {
	FindHandler(context.Context, Trigger, ...interface{}) (triggerBehaviourResult, bool)
	Activate(context.Context) error
	Deactivate(context.Context) error
	Enter(context.Context, Transition, ...interface{}) error
	Exit(context.Context, Transition) error
	IsIncludedInState(State) bool
	PermittedTriggers(context.Context, ...interface{}) []Trigger
	state() State
	superstate() superset
	findHandler(context.Context, Trigger, ...interface{}) (triggerBehaviourResult, bool)
}

type stateRepresentation struct {
	State                   State
	InitialTransitionTarget State
	Superstate              superset
	Active                  bool
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

func (sr *stateRepresentation) superstate() superset {
	return sr.Superstate
}

func (sr *stateRepresentation) CanHandle(ctx context.Context, trigger Trigger, args ...interface{}) (ok bool) {
	_, ok = sr.FindHandler(ctx, trigger, args...)
	return
}

func (sr *stateRepresentation) FindHandler(ctx context.Context, trigger Trigger, args ...interface{}) (handler triggerBehaviourResult, ok bool) {
	handler, ok = sr.findHandler(ctx, trigger, args...)
	if ok || sr.Superstate == nil {
		return
	}
	handler, ok = sr.Superstate.FindHandler(ctx, trigger, args...)
	return
}

func (sr *stateRepresentation) findHandler(ctx context.Context, trigger Trigger, args ...interface{}) (result triggerBehaviourResult, ok bool) {
	var (
		possibleBehaviours []triggerBehaviour
	)
	if possibleBehaviours, ok = sr.TriggerBehaviours[trigger]; !ok {
		return
	}
	allResults := make([]triggerBehaviourResult, 0, len(possibleBehaviours))
	for _, behaviour := range possibleBehaviours {
		allResults = append(allResults, triggerBehaviourResult{
			Handler:              behaviour,
			UnmetGuardConditions: behaviour.UnmetGuardConditions(ctx),
		})
	}
	metResults := make([]triggerBehaviourResult, 0, len(allResults))
	unmetResults := make([]triggerBehaviourResult, 0, len(allResults))
	for _, result := range allResults {
		if len(result.UnmetGuardConditions) == 0 {
			metResults = append(metResults, result)
		} else {
			unmetResults = append(unmetResults, result)
		}
	}
	if len(metResults) > 1 {
		panic(fmt.Sprintf("stateless: Multiple permitted exit transitions are configured from state '%s' for trigger '%s'. Guard clauses must be mutually exclusive.", sr.State, trigger))
	}
	if len(metResults) == 1 {
		result, ok = metResults[0], true
	} else if len(unmetResults) > 0 {
		result, ok = unmetResults[0], false
	}
	return
}

func (sr *stateRepresentation) Activate(ctx context.Context) (err error) {
	if sr.Superstate != nil {
		err = sr.Superstate.Activate(ctx)
	}
	if sr.Active || err != nil {
		return
	}
	err = sr.executeActivationActions(ctx)
	if err == nil {
		sr.Active = true
	}
	return
}

func (sr *stateRepresentation) Deactivate(ctx context.Context) (err error) {
	if !sr.Active {
		return
	}
	err = sr.executeDeactivationActions(ctx)
	if err != nil {
		return
	}
	sr.Active = false
	if sr.Superstate != nil {
		err = sr.Superstate.Deactivate(ctx)
	}
	return
}

func (sr *stateRepresentation) Enter(ctx context.Context, transition Transition, args ...interface{}) (err error) {
	isReentry := transition.IsReentry()
	if !isReentry && sr.IncludeState(transition.Source) {
		return
	}
	if !isReentry && sr.Superstate != nil {
		err = sr.Superstate.Enter(ctx, transition, args...)
	}
	if err == nil {
		err = sr.executeEntryActions(ctx, transition, args...)
		if err == nil {
			err = sr.executeActivationActions(ctx)
		}
	}
	return
}

func (sr *stateRepresentation) Exit(ctx context.Context, transition Transition) (err error) {
	isReentry := transition.IsReentry()
	if !isReentry && sr.IncludeState(transition.Destination) {
		return
	}

	err = sr.executeDeactivationActions(ctx)
	if err == nil {
		err = sr.executeExitActions(ctx, transition)
	}
	// Must check if there is a superstate, and if we are leaving that superstate
	if err == nil && sr.Superstate != nil {
		// Check if destination is within the state list
		if sr.IsIncludedInState(transition.Destination) {
			// Destination state is within the list, exit first superstate only if it is NOT the the first
			if sr.Superstate.state() != transition.Destination {
				err = sr.Superstate.Exit(ctx, transition)
			}
		} else {
			// Exit the superstate as well
			err = sr.Superstate.Exit(ctx, transition)
		}
	}
	return
}

func (sr *stateRepresentation) InternalAction(ctx context.Context, transition Transition, args ...interface{}) error {
	var internalTransition *internalTriggerBehaviour
	var stateRep superset = sr
	for stateRep != nil {
		if result, ok := stateRep.findHandler(ctx, transition.Trigger, args...); ok {
			switch t := result.Handler.(type) {
			case *internalTriggerBehaviour:
				internalTransition = t
				break
			}
		}
		stateRep = stateRep.superstate()
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

func (sr *stateRepresentation) PermittedTriggers(ctx context.Context, args ...interface{}) (triggers []Trigger) {
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
		seen := make(map[string]struct{}, len(triggers))
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

func (sr *stateRepresentation) executeEntryActions(ctx context.Context, transition Transition, args ...interface{}) error {
	for _, a := range sr.EntryActions {
		if err := a.Execute(ctx, transition, args...); err != nil {
			return err
		}
	}
	return nil
}

func (sr *stateRepresentation) executeExitActions(ctx context.Context, transition Transition, args ...interface{}) error {
	for _, a := range sr.ExitActions {
		if err := a.Execute(ctx, transition, args...); err != nil {
			return err
		}
	}
	return nil
}
