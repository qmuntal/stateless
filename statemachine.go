package stateless

import (
	"container/list"
	"context"
	"fmt"
	"reflect"
)

// State is used to to represent the possible machine states.
type State = int

// Trigger is used to represent the triggers that cause state transitions.
type Trigger = int

// FiringMode enumerate the different modes used when Fire-ing a trigger.
type FiringMode uint8

const (
	// FiringQueued mode shoud be used when run-to-completion is required. This is the recommended mode.
	FiringQueued FiringMode = iota
	// FiringImmediate should be used when the queing of trigger events are not needed.
	// Care must be taken when using this mode, as there is no run-to-completion guaranteed.
	FiringImmediate
)

// UnhandledTriggerActionFunc defines a function that will be called when a trigger is not handled.
type UnhandledTriggerActionFunc func(ctx context.Context, state State, trigger Trigger, unmetGuards []string)

// DefaultUnhandledTriggerAction is the default unhandled trigger action.
func DefaultUnhandledTriggerAction(_ context.Context, state State, trigger Trigger, unmetGuards []string) {
	if len(unmetGuards) != 0 {
		panic(fmt.Sprintf("stateless: Trigger '%d' is valid for transition from state '%d' but a guard conditions are not met. Guard descriptions: '%v", trigger, state, unmetGuards))
	}
	panic(fmt.Sprintf("stateless: No valid leaving transitions are permitted from state '%d' for trigger '%d'. Consider ignoring the trigger.", state, trigger))
}

// A StateMachine is an abstract machine that can be in exactly one of a finite number of states at any given time.
//
// stateAccessor will be called to read the current state value.
// stateMutator will be called to write new state values.
type StateMachine struct {
	stateConfig            map[State]*stateRepresentation
	triggerConfig          map[Trigger]TriggerWithParameters
	stateAccessor          func(context.Context) (State, error)
	stateMutator           func(context.Context, State) error
	UnhandledTriggerAction UnhandledTriggerActionFunc
	onTransitionEvents     onTransitionEvents
	eventQueue             *list.List
	firingMode             FiringMode
	firing                 bool
}

func newStateMachine() *StateMachine {
	return &StateMachine{
		stateConfig:            make(map[State]*stateRepresentation),
		triggerConfig:          make(map[Trigger]TriggerWithParameters),
		UnhandledTriggerAction: UnhandledTriggerActionFunc(DefaultUnhandledTriggerAction),
		eventQueue:             list.New(),
	}
}

// NewStateMachine returns a queued state machine in state 0.
func NewStateMachine() *StateMachine {
	return NewStateMachineWithState(0, FiringQueued)
}

// NewStateMachineWithState returns a state machine.
func NewStateMachineWithState(initialState State, firingMode FiringMode) *StateMachine {
	sm := newStateMachine()
	reference := &stateReference{State: initialState}
	sm.stateAccessor = func(_ context.Context) (State, error) { return reference.State, nil }
	sm.stateMutator = func(_ context.Context, state State) error {
		reference.State = state
		return nil
	}
	sm.firingMode = firingMode
	return sm
}

// NewStateMachineWithExternalStorage returns a state machine with external state storage.
func NewStateMachineWithExternalStorage(stateAccessor func(context.Context) (State, error), stateMutator func(context.Context, State) error, firingMode FiringMode) *StateMachine {
	sm := newStateMachine()
	sm.stateAccessor = stateAccessor
	sm.stateMutator = stateMutator
	sm.firingMode = firingMode
	return sm
}

// State returns the current state.
func (sm *StateMachine) State(ctx context.Context) (State, error) {
	return sm.stateAccessor(ctx)
}

// PermittedTriggers returns the currently-permissible trigger values.
func (sm *StateMachine) PermittedTriggers(ctx context.Context, args ...interface{}) ([]Trigger, error) {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return nil, err
	}
	return sr.PermittedTriggers(ctx, args), nil
}

// Activate activates current state. Actions associated with activating the currrent state will be invoked.
// The activation is idempotent and subsequent activation of the same current state
// will not lead to re-execution of activation callbacks.
func (sm *StateMachine) Activate(ctx context.Context) error {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return err
	}
	return sr.Activate(ctx)
}

// Deactivate deactivates current state. Actions associated with deactivating the currrent state will be invoked.
// The deactivation is idempotent and subsequent deactivation of the same current state
// will not lead to re-execution of deactivation callbacks.
func (sm *StateMachine) Deactivate(ctx context.Context) error {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return err
	}
	return sr.Deactivate(ctx)
}

// IsInState determine if the state machine is in the supplied state.
// Returns true if the current state is equal to, or a substate of, the supplied state.
func (sm *StateMachine) IsInState(ctx context.Context, state State) (bool, error) {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return false, err
	}
	return sr.IsIncludedInState(state), nil
}

// CanFire returns true if the trigger can be fired in the current state.
func (sm *StateMachine) CanFire(ctx context.Context, trigger Trigger) (bool, error) {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return false, err
	}
	return sr.CanHandle(ctx, trigger), nil
}

// SetTriggerParameters specify the arguments that must be supplied when a specific trigger is fired.
// The returned object can be passed to the Fire() method in order to fire the parameterised trigger.
func (sm *StateMachine) SetTriggerParameters(trigger Trigger, argumentTypes []reflect.Type) TriggerWithParameters {
	config := TriggerWithParameters{Trigger: trigger, ArgumentTypes: argumentTypes}
	if _, ok := sm.triggerConfig[config.Trigger]; ok {
		panic(fmt.Sprintf("stateless: Parameters for the trigger '%d' have already been configured.", trigger))
	}
	sm.triggerConfig[trigger] = config
	return config
}

// Fire Transition from the current state via the specified trigger.
// The target state is determined by the configuration of the current state.
// Actions associated with leaving the current state and entering the new one
// will be invoked.
func (sm *StateMachine) Fire(ctx context.Context, trigger Trigger) error {
	return sm.internalFire(ctx, trigger)
}

// OnTransitioned registers a callback that will be invoked every time the statemachine
// transitions from one state into another.
func (sm *StateMachine) OnTransitioned(onTransitionAction func(context.Context, Transition)) {
	sm.onTransitionEvents = append(sm.onTransitionEvents, onTransitionAction)
}

func (sm *StateMachine) setState(ctx context.Context, state State) error {
	return sm.stateMutator(ctx, state)
}

func (sm *StateMachine) currentState(ctx context.Context) (sr *stateRepresentation, err error) {
	var state State
	state, err = sm.State(ctx)
	if err == nil {
		sr = sm.stateRepresentation(state)
	}
	return
}

func (sm *StateMachine) stateRepresentation(state State) (sr *stateRepresentation) {
	var ok bool
	if sr, ok = sm.stateConfig[state]; !ok {
		sr = newstateRepresentation(state)
		sm.stateConfig[state] = sr
	}
	return
}

func (sm *StateMachine) internalFire(ctx context.Context, trigger Trigger, args ...interface{}) error {
	switch sm.firingMode {
	case FiringImmediate:
		return sm.internalFireOne(ctx, trigger, args)
	case FiringQueued:
		fallthrough
	default:
		return sm.internalFireQueued(ctx, trigger, args)
	}
}

func (sm *StateMachine) internalFireQueued(ctx context.Context, trigger Trigger, args ...interface{}) (err error) {
	if sm.firing {
		sm.eventQueue.PushBack(queuedTrigger{Trigger: trigger, Args: args})
	}
	sm.firing = true
	defer func() { sm.firing = false }()
	err = sm.internalFireOne(ctx, trigger, args)
	if err != nil {
		return
	}
	for sm.eventQueue.Len() != 0 {
		e := sm.eventQueue.Front()
		et := e.Value.(queuedTrigger)
		err = sm.internalFireOne(ctx, et.Trigger, et.Args)
		if err != nil {
			break
		}
		sm.eventQueue.Remove(e)
	}
	return
}

func (sm *StateMachine) internalFireOne(ctx context.Context, trigger Trigger, args ...interface{}) (err error) {
	var (
		config TriggerWithParameters
		ok     bool
	)
	if config, ok = sm.triggerConfig[trigger]; ok {
		config.validateParameters(args)
	}
	source, err := sm.State(ctx)
	if err != nil {
		return
	}
	representativeState := sm.stateRepresentation(source)
	var result triggerBehaviourResult
	if result, ok = representativeState.findHandler(ctx, trigger, args); !ok {
		sm.UnhandledTriggerAction(ctx, representativeState.State, trigger, result.UnmetGuardConditions)
		return nil
	}
	switch t := result.Handler.(type) {
	case *ignoredTriggerBehaviour:
		// ignored
	case *reentryTriggerBehaviour:
		transition := Transition{Source: source, Destination: t.Destination, Trigger: trigger}
		err = sm.handleReentryTrigger(ctx, representativeState, transition, args)
	case *dynamicTriggerBehaviour:
		destination, ok := result.Handler.ResultsInTransitionFrom(ctx, source, args)
		if !ok {
			err = fmt.Errorf("stateless: Dynamic handler for trigger %d in state %d has failed", trigger, source)
		} else {
			transition := Transition{Source: source, Destination: destination, Trigger: trigger}
			err = sm.handleTransitioningTrigger(ctx, representativeState, transition, args)
		}
	case *transitioningTriggerBehaviour:
		destination, ok := result.Handler.ResultsInTransitionFrom(ctx, source, args)
		if !ok {
			err = fmt.Errorf("stateless: Transition handler for trigger %d in state %d has failed", trigger, source)
		} else {
			transition := Transition{Source: source, Destination: destination, Trigger: trigger}
			err = sm.handleTransitioningTrigger(ctx, representativeState, transition, args)
		}
	case *internalTriggerBehaviour:
		var sr *stateRepresentation
		sr, err = sm.currentState(ctx)
		if err == nil {
			transition := Transition{Source: source, Destination: source, Trigger: trigger}
			err = sr.InternalAction(ctx, transition, args)
		}
	default:
		panic("stateless: State machine configuration incorrect, no handler for trigger.")
	}
	return
}

func (sm *StateMachine) handleReentryTrigger(ctx context.Context, sr *stateRepresentation, transition Transition, args ...interface{}) (err error) {
	transition, err = sr.Exit(ctx, transition)
	if err != nil {
		return
	}
	representation := &stateRepresentation{}
	newSr := sm.stateRepresentation(transition.Destination)
	if transition.Source != transition.Destination {
		transition = Transition{Source: transition.Destination, Destination: transition.Destination, Trigger: transition.Trigger}
		_, err = newSr.Exit(ctx, transition)
		if err != nil {
			return
		}
	}
	sm.onTransitionEvents.Invoke(ctx, transition)
	representation, err = sm.enterState(ctx, newSr, transition, args)
	return sm.setState(ctx, representation.State)
}

func (sm *StateMachine) handleTransitioningTrigger(ctx context.Context, sr *stateRepresentation, transition Transition, args ...interface{}) (err error) {
	transition, err = sr.Exit(ctx, transition)
	if err != nil {
		return
	}
	sm.setState(ctx, transition.Destination)
	newSr := sm.stateRepresentation(transition.Destination)

	//Alert all listeners of state transition
	sm.onTransitionEvents.Invoke(ctx, transition)
	newSr, err = sm.enterState(ctx, newSr, transition, args)
	if err != nil {
		return
	}
	return sm.setState(ctx, newSr.State)
}

func (sm *StateMachine) enterState(ctx context.Context, sr *stateRepresentation, transition Transition, args ...interface{}) (*stateRepresentation, error) {
	// Enter the new state
	err := sr.Enter(ctx, transition, args)
	if err != nil {
		return nil, err
	}
	// Recursively enter substates that have an initial transition
	if sr.HasInitialState {
		for _, substate := range sr.Substates {
			// Verify that the target state is a substate
			// Check if state has substate(s), and if an initial transition(s) has been set up.
			if substate.State == sr.InitialTransitionTarget {
				panic(fmt.Sprintf("stateless: The target (%d) for the initial transition is not a substate.", sr.InitialTransitionTarget))
			}
		}
		initialTranslation := Transition{Source: transition.Source, Destination: sr.InitialTransitionTarget, Trigger: transition.Trigger}
		sr = sm.stateRepresentation(sr.InitialTransitionTarget)
		sr, err = sm.enterState(ctx, sr, initialTranslation, args)
	}
	return sr, err
}
