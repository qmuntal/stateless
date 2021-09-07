package stateless

import (
	"context"
	"fmt"
)

type transitionKey struct{}

func withTransition[S State, T Trigger](ctx context.Context, transition Transition[S, T]) context.Context {
	return context.WithValue(ctx, transitionKey{}, transition)
}

// GetTransition returns the transition from the context.
// If there is no transition the returned value is empty.
func GetTransition[S State, T Trigger](ctx context.Context) Transition[S, T] {
	return ctx.Value(transitionKey{}).(Transition[S, T])
}

// ActionFunc describes a generic action function.
// The context will always contain Transition information.
type ActionFunc = func(ctx context.Context, args ...interface{}) error

// GuardFunc defines a generic guard function.
type GuardFunc = func(ctx context.Context, args ...interface{}) bool

// DestinationSelectorFunc defines a functions that is called to select a dynamic destination.
type DestinationSelectorFunc[S State] func(ctx context.Context, args ...interface{}) (S, error)

// StateConfiguration is the configuration for a single state value.
type StateConfiguration[S State, T Trigger] struct {
	sm     *StateMachine[S, T]
	sr     *stateRepresentation[S, T]
	lookup func(S) *stateRepresentation[S, T]
}

// State is configured with this configuration.
func (sc *StateConfiguration[S, T]) State() S {
	return sc.sr.State
}

// Machine that is configured with this configuration.
func (sc *StateConfiguration[S, T]) Machine() *StateMachine[S, T] {
	return sc.sm
}

// InitialTransition adds internal transition to this state.
// When entering the current state the state machine will look for an initial transition,
// and enter the target state.
func (sc *StateConfiguration[S, T]) InitialTransition(targetState S) *StateConfiguration[S, T] {
	if sc.sr.HasInitialState {
		panic(fmt.Sprintf("stateless: This state has already been configured with an initial transition (%v).", sc.sr.InitialTransitionTarget))
	}
	if targetState == sc.State() {
		panic("stateless: Setting the current state as the target destination state is not allowed.")
	}
	sc.sr.SetInitialTransition(targetState)
	return sc
}

// Permit accept the specified trigger and transition to the destination state if the guard conditions are met (if any).
func (sc *StateConfiguration[S, T]) Permit(trigger T, destinationState S, guards ...GuardFunc) *StateConfiguration[S, T] {
	if destinationState == sc.sr.State {
		panic("stateless: Permit() require that the destination state is not equal to the source state. To accept a trigger without changing state, use either Ignore() or PermitReentry().")
	}
	sc.sr.AddTriggerBehaviour(&transitioningTriggerBehaviour[S, T]{
		baseTriggerBehaviour: baseTriggerBehaviour[T]{Trigger: trigger, Guard: newtransitionGuard(guards...)},
		Destination:          destinationState,
	})
	return sc
}

// InternalTransition add an internal transition to the state machine.
// An internal action does not cause the Exit and Entry actions to be triggered, and does not change the state of the state machine.
func (sc *StateConfiguration[S, T]) InternalTransition(trigger T, action ActionFunc, guards ...GuardFunc) *StateConfiguration[S, T] {
	sc.sr.AddTriggerBehaviour(&internalTriggerBehaviour[S, T]{
		baseTriggerBehaviour: baseTriggerBehaviour[T]{Trigger: trigger, Guard: newtransitionGuard(guards...)},
		Action:               action,
	})
	return sc
}

// PermitReentry accept the specified trigger, execute exit actions and re-execute entry actions.
// Reentry behaves as though the configured state transitions to an identical sibling state.
// Applies to the current state only. Will not re-execute superstate actions, or
// cause actions to execute transitioning between super- and sub-states.
func (sc *StateConfiguration[S, T]) PermitReentry(trigger T, guards ...GuardFunc) *StateConfiguration[S, T] {
	sc.sr.AddTriggerBehaviour(&reentryTriggerBehaviour[S, T]{
		baseTriggerBehaviour: baseTriggerBehaviour[T]{Trigger: trigger, Guard: newtransitionGuard(guards...)},
		Destination:          sc.sr.State,
	})
	return sc
}

// Ignore the specified trigger when in the configured state, if the guards return true.
func (sc *StateConfiguration[S, T]) Ignore(trigger T, guards ...GuardFunc) *StateConfiguration[S, T] {
	sc.sr.AddTriggerBehaviour(&ignoredTriggerBehaviour[T]{
		baseTriggerBehaviour: baseTriggerBehaviour[T]{Trigger: trigger, Guard: newtransitionGuard(guards...)},
	})
	return sc
}

// PermitDynamic accept the specified trigger and transition to the destination state, calculated dynamically by the supplied function.
func (sc *StateConfiguration[S, T]) PermitDynamic(trigger T, selector DestinationSelectorFunc[S], guards ...GuardFunc) *StateConfiguration[S, T] {
	guardDescriptors := make([]invocationInfo, len(guards))
	for i, guard := range guards {
		guardDescriptors[i] = newinvocationInfo(guard)
	}
	sc.sr.AddTriggerBehaviour(&dynamicTriggerBehaviour[S, T]{
		baseTriggerBehaviour: baseTriggerBehaviour[T]{Trigger: trigger, Guard: newtransitionGuard(guards...)},
		Destination:          selector,
	})
	return sc
}

// OnActive specify an action that will execute when activating the configured state.
func (sc *StateConfiguration[S, T]) OnActive(action func(context.Context) error) *StateConfiguration[S, T] {
	sc.sr.ActivateActions = append(sc.sr.ActivateActions, actionBehaviourSteady{
		Action:      action,
		Description: newinvocationInfo(action),
	})
	return sc
}

// OnDeactivate specify an action that will execute when deactivating the configured state.
func (sc *StateConfiguration[S, T]) OnDeactivate(action func(context.Context) error) *StateConfiguration[S, T] {
	sc.sr.DeactivateActions = append(sc.sr.DeactivateActions, actionBehaviourSteady{
		Action:      action,
		Description: newinvocationInfo(action),
	})
	return sc
}

// OnEntry specify an action that will execute when transitioning into the configured state.
func (sc *StateConfiguration[S, T]) OnEntry(action ActionFunc) *StateConfiguration[S, T] {
	sc.sr.EntryActions = append(sc.sr.EntryActions, actionBehaviour[S, T]{
		Action:      action,
		Description: newinvocationInfo(action),
	})
	return sc
}

// OnEntryFrom Specify an action that will execute when transitioning into the configured state from a specific trigger.
func (sc *StateConfiguration[S, T]) OnEntryFrom(trigger T, action ActionFunc) *StateConfiguration[S, T] {
	sc.sr.EntryActions = append(sc.sr.EntryActions, actionBehaviour[S, T]{
		Action:      action,
		Description: newinvocationInfo(action),
		Trigger:     &trigger,
	})
	return sc
}

// OnExit specify an action that will execute when transitioning from the configured state.
func (sc *StateConfiguration[S, T]) OnExit(action ActionFunc) *StateConfiguration[S, T] {
	sc.sr.ExitActions = append(sc.sr.ExitActions, actionBehaviour[S, T]{
		Action:      action,
		Description: newinvocationInfo(action),
	})
	return sc
}

// SubstateOf sets the superstate that the configured state is a substate of.
// Substates inherit the allowed transitions of their superstate.
// When entering directly into a substate from outside of the superstate,
// entry actions for the superstate are executed.
// Likewise when leaving from the substate to outside the supserstate,
// exit actions for the superstate will execute.
func (sc *StateConfiguration[S, T]) SubstateOf(superstate S) *StateConfiguration[S, T] {
	state := sc.sr.State
	// Check for accidental identical cyclic configuration
	if state == superstate {
		panic(fmt.Sprintf("stateless: Configuring %v as a substate of %v creates an illegal cyclic configuration.", state, superstate))
	}

	// Check for accidental identical nested cyclic configuration
	var empty struct{}
	supersets := map[S]struct{}{state: empty}
	// Build list of super states and check for

	activeSc := sc.lookup(superstate)
	for activeSc.Superstate != nil {
		// Check if superstate is already added to hashset
		if _, ok := supersets[activeSc.Superstate.state()]; ok {
			panic(fmt.Sprintf("stateless: Configuring %v as a substate of %v creates an illegal nested cyclic configuration.", state, supersets))
		}
		supersets[activeSc.Superstate.state()] = empty
		activeSc = sc.lookup(activeSc.Superstate.state())
	}

	// The check was OK, we can add this
	superRepresentation := sc.lookup(superstate)
	sc.sr.Superstate = superRepresentation
	superRepresentation.Substates = append(superRepresentation.Substates, sc.sr)
	return sc
}
