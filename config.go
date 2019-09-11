package stateless

import (
	"context"
	"fmt"
)

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

func (sc *StateConfiguration) enforceNotIdentityTransition(destination State) {
	if destination == sc.sr.State {
		panic("stateless: Permit() (and PermitIf()) require that the destination state is not equal to the source state. To accept a trigger without changing state, use either Ignore() or PermitReentry().")
	}
}

func (sc *StateConfiguration) permit(trigger Trigger, destinationState State) {
	sc.sr.AddTriggerBehaviour(&transitioningTriggerBehaviour{
		baseTriggerBehaviour: baseTriggerBehaviour{Trigger: trigger},
		Destination:          destinationState,
	})
}

func (sc *StateConfiguration) permitIf(trigger Trigger, destinationState State, guard transitionGuard) {
	sc.sr.AddTriggerBehaviour(&transitioningTriggerBehaviour{
		baseTriggerBehaviour: baseTriggerBehaviour{Trigger: trigger, Guard: guard},
		Destination:          destinationState,
	})
}

func (sc *StateConfiguration) permitReentryIf(trigger Trigger, destinationState State, guard transitionGuard) {
	sc.sr.AddTriggerBehaviour(&reentryTriggerBehaviour{
		baseTriggerBehaviour: baseTriggerBehaviour{Trigger: trigger, Guard: guard},
		Destination:          destinationState,
	})
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
