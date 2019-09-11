package stateless

import (
	"context"
	"fmt"
)

// Guard defines a function that controls the transition from one state to another.
type Guard struct {
	Func GuardFunc
	Desc string
}

// StateConfiguration is the configuration for a single state value.
type StateConfiguration struct {
	sm     *StateMachine
	sr     *stateRepresentation
	lookup func(State) *stateRepresentation
}

// The State is configured with this configuration.
func (sc *StateConfiguration) State() State {
	return sc.sr.State
}

// The Machine that is configured with this configuration.
func (sc *StateConfiguration) Machine() *StateMachine {
	return sc.sm
}

// InitialTransition adds internal transition to this state.
// When entering the current state the state machine will look for an initial transition,
// and enter the target state.
func (sc *StateConfiguration) InitialTransition(targetState State) *StateConfiguration {
	if sc.sr.HasInitialState {
		panic(fmt.Sprintf("stateless: This state has already been configured with an inital transition (%d).", sc.sr.InitialTransitionTarget))
	}
	if targetState == sc.State() {
		panic("stateless: Setting the current state as the target destination state is not allowed.")
	}
	sc.sr.SetInitialTransition(targetState)
	return sc
}

// Permit accept the specified trigger and transition to the destination state if the guard conditions are met (if any).
func (sc *StateConfiguration) Permit(trigger Trigger, destinationState State, guards ...Guard) *StateConfiguration {
	if destinationState == sc.sr.State {
		panic("stateless: Permit() (and PermitIf()) require that the destination state is not equal to the source state. To accept a trigger without changing state, use either Ignore() or PermitReentry().")
	}
	sc.sr.AddTriggerBehaviour(&transitioningTriggerBehaviour{
		baseTriggerBehaviour: baseTriggerBehaviour{Trigger: trigger, Guard: newtransitionGuard(guards...)},
		Destination:          destinationState,
	})
	return sc
}

// InternalTransition add an internal transition to the state machine.
// An internal action does not cause the Exit and Entry actions to be triggered, and does not change the state of the state machine
func (sc *StateConfiguration) InternalTransition(trigger Trigger, action func(context.Context, Transition, ...interface{}) error, guards ...Guard) *StateConfiguration {
	sc.sr.AddTriggerBehaviour(&internalTriggerBehaviour{
		baseTriggerBehaviour: baseTriggerBehaviour{Trigger: trigger, Guard: newtransitionGuard(guards...)},
		Action:               action,
	})
	return sc
}

// PermitReentry accept the specified trigger, execute exit actions and re-execute entry actions.
// Reentry behaves as though the configured state transitions to an identical sibling state.
// Applies to the current state only. Will not re-execute superstate actions, or
// cause actions to execute transitioning between super- and sub-states.
func (sc *StateConfiguration) PermitReentry(trigger Trigger, guards ...Guard) *StateConfiguration {
	sc.sr.AddTriggerBehaviour(&reentryTriggerBehaviour{
		baseTriggerBehaviour: baseTriggerBehaviour{Trigger: trigger, Guard: newtransitionGuard(guards...)},
		Destination:          sc.sr.State,
	})
	return sc
}

// Ignore the specified trigger when in the configured state, if the guards return true.
func (sc *StateConfiguration) Ignore(trigger Trigger, guards ...Guard) *StateConfiguration {
	sc.sr.AddTriggerBehaviour(&ignoredTriggerBehaviour{
		baseTriggerBehaviour: baseTriggerBehaviour{Trigger: trigger, Guard: newtransitionGuard(guards...)},
	})
	return sc
}

// PermitDynamic accept the specified trigger and transition to the destination state, calculated dynamically by the supplied function.
func (sc *StateConfiguration) PermitDynamic(trigger Trigger, destinationSelector func(context.Context, ...interface{}) (State, error),
	destinationSelectorDesc string, possibleStates []DynamicStateInfo, guards ...Guard) *StateConfiguration {
	guardDescriptors := make([]InvocationInfo, len(guards))
	for i, guard := range guards {
		guardDescriptors[i] = newInvocationInfo(guard, guard.Desc, false)
	}
	sc.sr.AddTriggerBehaviour(&dynamicTriggerBehaviour{
		baseTriggerBehaviour: baseTriggerBehaviour{Trigger: trigger, Guard: newtransitionGuard(guards...)},
		Destination:          destinationSelector,
		TransitionInfo: DynamicTransitionInfo{
			TransitionInfo: TransitionInfo{
				Trigger:           TriggerInfo(trigger),
				GuardDescriptions: guardDescriptors,
			},
			DestinationStateSelectorDescription: newInvocationInfo(destinationSelector, destinationSelectorDesc, false),
			PossibleDestinationStates:           possibleStates,
		},
	})
	return sc
}

func (sc *StateConfiguration) permitDynamicIf(trigger Trigger, destinationSelector func(context.Context, ...interface{}) (State, error), destinationSelectorDesc string, guard transitionGuard, possibleStates []DynamicStateInfo) {
	guardDescriptors := make([]InvocationInfo, len(guard.Guards))
	for i := range guard.Guards {
		guardDescriptors[i] = guard.Guards[i].Description
	}
	sc.sr.AddTriggerBehaviour(&dynamicTriggerBehaviour{
		baseTriggerBehaviour: baseTriggerBehaviour{Trigger: trigger, Guard: guard},
		Destination:          destinationSelector,
		TransitionInfo: DynamicTransitionInfo{
			TransitionInfo: TransitionInfo{
				Trigger:           TriggerInfo(trigger),
				GuardDescriptions: guardDescriptors,
			},
			DestinationStateSelectorDescription: newInvocationInfo(destinationSelector, destinationSelectorDesc, false),
			PossibleDestinationStates:           possibleStates,
		},
	})
}
