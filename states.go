package stateless

import (
	"context"
	"fmt"
)

type stateReference struct {
	State State
}

type Transition struct {
	Source      State
	Destination State
	Trigger     State
}

// IsReentry returns true if the transition is a re-entry,
// i.e. the identity transition.
func (t *Transition) IsReentry() bool {
	return t.Source == t.Destination
}

type actionBehaviour struct {
	Action  func(ctx context.Context, transition Transition, args ...interface{}) error
	Trigger *Trigger
}

func (a actionBehaviour) Execute(ctx context.Context, transition Transition, args ...interface{}) error {
	if a.Trigger == nil || *a.Trigger == transition.Trigger {
		return a.Action(ctx, transition, args)
	}
	return nil
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
	Exit(context.Context, Transition) (Transition, error)
	IsIncludedInState(State) bool
	PermittedTriggers(context.Context, ...interface{}) []Trigger
	state() State
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
	Substates               []stateRepresentation
	TriggerBehaviours       map[Trigger][]triggerBehaviour
	HasInitialState         bool
}

func newstateRepresentation(state State) *stateRepresentation {
	return &stateRepresentation{
		State:             state,
		TriggerBehaviours: make(map[Trigger][]triggerBehaviour),
	}
}

func (r *stateRepresentation) SetInitialState(state State) {
	r.State = state
	r.HasInitialState = true
}

func (r *stateRepresentation) state() State {
	return r.State
}

func (r *stateRepresentation) CanHandle(ctx context.Context, trigger Trigger, args ...interface{}) (ok bool) {
	_, ok = r.FindHandler(ctx, trigger, args)
	return
}

func (r *stateRepresentation) FindHandler(ctx context.Context, trigger Trigger, args ...interface{}) (handler triggerBehaviourResult, ok bool) {
	handler, ok = r.findHandler(ctx, trigger, args)
	if ok || r.Superstate == nil {
		return
	}
	handler, ok = r.Superstate.FindHandler(ctx, trigger, args)
	return
}

func (r *stateRepresentation) findHandler(ctx context.Context, trigger Trigger, args ...interface{}) (result triggerBehaviourResult, ok bool) {
	var (
		possibleBehaviours []triggerBehaviour
	)
	if possibleBehaviours, ok = r.TriggerBehaviours[trigger]; !ok {
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
		panic(fmt.Sprintf("stateless: Multiple permitted exit transitions are configured from state '%d' for trigger '%d'. Guard clauses must be mutually exclusive.", r.State, trigger))
	}
	if len(metResults) == 1 {
		result, ok = metResults[0], true
	} else if len(unmetResults) > 0 {
		result, ok = unmetResults[0], true
	}
	return
}

func (r *stateRepresentation) Activate(ctx context.Context) (err error) {
	if r.Superstate != nil {
		err = r.Superstate.Activate(ctx)
	}
	if r.Active || err != nil {
		return
	}
	err = r.executeActivationActions(ctx)
	if err != nil {
		r.Active = true
	}
	return
}

func (r *stateRepresentation) Deactivate(ctx context.Context) (err error) {
	if !r.Active {
		return
	}
	err = r.executeDeactivationActions(ctx)
	if err != nil {
		return
	}
	r.Active = false
	if r.Superstate != nil {
		err = r.Superstate.Deactivate(ctx)
	}
	return
}

func (r *stateRepresentation) Enter(ctx context.Context, transition Transition, args ...interface{}) (err error) {
	if transition.IsReentry() {
		err = r.executeEntryActions(ctx, transition, args)
		if err != nil {
			err = r.executeActivationActions(ctx)
		}
	} else if !r.IncludeState(transition.Source) {
		if r.Superstate != nil {
			err = r.Superstate.Enter(ctx, transition, args)
		}
		if err != nil {
			err = r.executeEntryActions(ctx, transition, args)
			if err != nil {
				err = r.executeActivationActions(ctx)
			}
		}
	}
	return
}

func (r *stateRepresentation) Exit(ctx context.Context, transition Transition) (newTransition Transition, err error) {
	newTransition = transition
	if transition.IsReentry() {
		err = r.executeDeactivationActions(ctx)
		if err != nil {
			err = r.executeExitActions(ctx, transition)
		}
	} else if !r.IncludeState(transition.Destination) {
		err = r.executeDeactivationActions(ctx)
		if err != nil {
			err = r.executeExitActions(ctx, transition)
		}
		if err != nil && r.Superstate != nil {
			if r.IsIncludedInState(transition.Destination) {
				if r.Superstate.state() != transition.Destination {
					newTransition, err = r.Superstate.Exit(ctx, transition)
				}
			} else {
				newTransition, err = r.Superstate.Exit(ctx, transition)
			}
		}
	}
	return
}

func (r *stateRepresentation) InternalAction(ctx context.Context, transition Transition, args ...interface{}) error {
	var internalTransition *internalTriggerBehaviour
	var stateRep superset = r
	for stateRep != nil {
		if result, ok := stateRep.findHandler(ctx, transition.Trigger, args); ok {
			switch t := result.Handler.(type) {
			case *internalTriggerBehaviour:
				internalTransition = t
				break
			}
		}
		stateRep = r.Superstate
	}
	if internalTransition == nil {
		panic("stateless: The configuration is incorrect, no action assigned to this internal transition.")
	}
	return internalTransition.Execute(ctx, transition, args)
}

func (r *stateRepresentation) IncludeState(state State) bool {
	if state == r.State {
		return true
	}
	for _, substate := range r.Substates {
		if substate.IncludeState(state) {
			return true
		}
	}
	return false
}

func (r *stateRepresentation) IsIncludedInState(state State) bool {
	if state == r.State {
		return true
	}
	if r.Superstate != nil {
		return r.Superstate.IsIncludedInState(state)
	}
	return false
}

func (r *stateRepresentation) AddTriggerBehaviour(tb triggerBehaviour) {
	var (
		allowed []triggerBehaviour
		ok      bool
	)
	trigger := tb.GetTrigger()
	if allowed, ok = r.TriggerBehaviours[trigger]; !ok {
		allowed = []triggerBehaviour{tb}
		r.TriggerBehaviours[trigger] = allowed
	}
	allowed = append(allowed, tb)

}

func (r *stateRepresentation) PermittedTriggers(ctx context.Context, args ...interface{}) (triggers []Trigger) {
	for key, value := range r.TriggerBehaviours {
		for _, tb := range value {
			if len(tb.UnmetGuardConditions(ctx, args)) == 0 {
				triggers = append(triggers, key)
			}
		}
	}
	if r.Superstate != nil {
		triggers = append(triggers, r.Superstate.PermittedTriggers(ctx, args)...)
	}
	return
}

func (r *stateRepresentation) executeActivationActions(ctx context.Context) error {
	for _, a := range r.ActivateActions {
		if err := a.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *stateRepresentation) executeDeactivationActions(ctx context.Context) error {
	for _, a := range r.DeactivateActions {
		if err := a.Execute(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *stateRepresentation) executeEntryActions(ctx context.Context, transition Transition, args ...interface{}) error {
	for _, a := range r.EntryActions {
		if err := a.Execute(ctx, transition, args); err != nil {
			return err
		}
	}
	return nil
}

func (r *stateRepresentation) executeExitActions(ctx context.Context, transition Transition, args ...interface{}) error {
	for _, a := range r.ExitActions {
		if err := a.Execute(ctx, transition, args); err != nil {
			return err
		}
	}
	return nil
}
