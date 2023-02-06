package stateless

import (
	"container/list"
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
)

// State is used to to represent the possible machine states.
type State = interface{}

// Trigger is used to represent the triggers that cause state transitions.
type Trigger = interface{}

// FiringMode enumerate the different modes used when Fire-ing a trigger.
type FiringMode uint8

const (
	// FiringQueued mode shoud be used when run-to-completion is required. This is the recommended mode.
	FiringQueued FiringMode = iota
	// FiringImmediate should be used when the queing of trigger events are not needed.
	// Care must be taken when using this mode, as there is no run-to-completion guaranteed.
	FiringImmediate
)

// Transition describes a state transition.
type Transition struct {
	Source      State
	Destination State
	Trigger     Trigger

	isInitial bool
}

// IsReentry returns true if the transition is a re-entry,
// i.e. the identity transition.
func (t *Transition) IsReentry() bool {
	return t.Source == t.Destination
}

type TransitionFunc = func(context.Context, Transition)

// UnhandledTriggerActionFunc defines a function that will be called when a trigger is not handled.
type UnhandledTriggerActionFunc = func(ctx context.Context, state State, trigger Trigger, unmetGuards []string) error

// DefaultUnhandledTriggerAction is the default unhandled trigger action.
func DefaultUnhandledTriggerAction(_ context.Context, state State, trigger Trigger, unmetGuards []string) error {
	if len(unmetGuards) != 0 {
		return fmt.Errorf("stateless: Trigger '%v' is valid for transition from state '%v' but a guard conditions are not met. Guard descriptions: '%v", trigger, state, unmetGuards)
	}
	return fmt.Errorf("stateless: No valid leaving transitions are permitted from state '%v' for trigger '%v', consider ignoring the trigger", state, trigger)
}

func callEvents(events []TransitionFunc, ctx context.Context, transition Transition) {
	for _, e := range events {
		e(ctx, transition)
	}
}

// A StateMachine is an abstract machine that can be in exactly one of a finite number of states at any given time.
// It is safe to use the StateMachine concurrently, but non of the callbacks (state manipulation, actions, events, ...) are guarded,
// so it is up to the client to protect them against race conditions.
type StateMachine[T any] struct {
	ops                    atomic.Uint64
	stateConfig            map[State]*stateRepresentation[T]
	triggerConfig          map[Trigger]triggerWithParameters
	stateAccessor          func(context.Context) (State, error)
	stateMutator           func(context.Context, State) error
	unhandledTriggerAction UnhandledTriggerActionFunc
	onTransitioningEvents  []TransitionFunc
	onTransitionedEvents   []TransitionFunc
	eventQueue             list.List
	firingMode             FiringMode
	firingMutex            sync.Mutex
	extendedState          T
}

func newStateMachine[T any](extendedState T) *StateMachine[T] {
	return &StateMachine[T]{
		stateConfig:            make(map[State]*stateRepresentation[T]),
		triggerConfig:          make(map[Trigger]triggerWithParameters),
		unhandledTriggerAction: UnhandledTriggerActionFunc(DefaultUnhandledTriggerAction),
		extendedState:          extendedState,
	}
}

// NewStateMachine returns a queued state machine.
func NewStateMachine[T any](initialState State, extendedState T) *StateMachine[T] {
	return NewStateMachineWithMode(initialState, extendedState, FiringQueued)
}

// NewStateMachineWithMode returns a state machine with the desired firing mode
func NewStateMachineWithMode[T any](initialState State, extendedState T, firingMode FiringMode) *StateMachine[T] {
	var stateMutex sync.Mutex
	sm := newStateMachine(extendedState)
	reference := &struct {
		State State
	}{State: initialState}
	sm.stateAccessor = func(_ context.Context) (State, error) {
		stateMutex.Lock()
		defer stateMutex.Unlock()
		return reference.State, nil
	}
	sm.stateMutator = func(_ context.Context, state State) error {
		stateMutex.Lock()
		defer stateMutex.Unlock()
		reference.State = state
		return nil
	}
	sm.firingMode = firingMode
	return sm
}

// NewStateMachineWithExternalStorage returns a state machine with external state storage.
func NewStateMachineWithExternalStorage[T any](extendedState T, stateAccessor func(context.Context) (State, error), stateMutator func(context.Context, State) error, firingMode FiringMode) *StateMachine[T] {
	sm := newStateMachine(extendedState)
	sm.stateAccessor = stateAccessor
	sm.stateMutator = stateMutator
	sm.firingMode = firingMode
	return sm
}

// ToGraph returns the DOT representation of the state machine.
// It is not guaranteed that the returned string will be the same in different executions.
func (sm *StateMachine[T]) ToGraph() string {
	return new(graph[T]).formatStateMachine(sm)
}

// State returns the current state.
func (sm *StateMachine[T]) State(ctx context.Context) (State, error) {
	return sm.stateAccessor(ctx)
}

// ExtendedState returns the extended state.
func (sm *StateMachine[T]) ExtendedState() T {
	return sm.extendedState
}

// MustState returns the current state without the error.
// It is safe to use this method when used together with NewStateMachine
// or when using NewStateMachineWithExternalStorage with an state accessor that
// does not return an error.
func (sm *StateMachine[T]) MustState() State {
	st, err := sm.State(context.Background())
	if err != nil {
		panic(err)
	}
	return st
}

// PermittedTriggers see PermittedTriggersCtx.
func (sm *StateMachine[T]) PermittedTriggers(args ...interface{}) ([]Trigger, error) {
	return sm.PermittedTriggersCtx(context.Background(), args...)
}

// PermittedTriggersCtx returns the currently-permissible trigger values.
func (sm *StateMachine[T]) PermittedTriggersCtx(ctx context.Context, args ...interface{}) ([]Trigger, error) {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return nil, err
	}
	return sr.PermittedTriggers(ctx, sm.extendedState, args...), nil
}

// Activate see ActivateCtx.
func (sm *StateMachine[T]) Activate() error {
	return sm.ActivateCtx(context.Background())
}

// ActivateCtx activates current state. Actions associated with activating the current state will be invoked.
// The activation is idempotent and subsequent activation of the same current state
// will not lead to re-execution of activation callbacks.
func (sm *StateMachine[T]) ActivateCtx(ctx context.Context) error {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return err
	}
	return sr.Activate(ctx)
}

// Deactivate see DeactivateCtx.
func (sm *StateMachine[T]) Deactivate() error {
	return sm.DeactivateCtx(context.Background())
}

// DeactivateCtx deactivates current state. Actions associated with deactivating the current state will be invoked.
// The deactivation is idempotent and subsequent deactivation of the same current state
// will not lead to re-execution of deactivation callbacks.
func (sm *StateMachine[T]) DeactivateCtx(ctx context.Context) error {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return err
	}
	return sr.Deactivate(ctx)
}

// IsInState see IsInStateCtx.
func (sm *StateMachine[T]) IsInState(state State) (bool, error) {
	return sm.IsInStateCtx(context.Background(), state)
}

// IsInStateCtx determine if the state machine is in the supplied state.
// Returns true if the current state is equal to, or a substate of, the supplied state.
func (sm *StateMachine[T]) IsInStateCtx(ctx context.Context, state State) (bool, error) {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return false, err
	}
	return sr.IsIncludedInState(state), nil
}

// CanFire see CanFireCtx.
func (sm *StateMachine[T]) CanFire(trigger Trigger, args ...interface{}) (bool, error) {
	return sm.CanFireCtx(context.Background(), trigger, args...)
}

// CanFireCtx returns true if the trigger can be fired in the current state.
func (sm *StateMachine[T]) CanFireCtx(ctx context.Context, trigger Trigger, args ...interface{}) (bool, error) {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return false, err
	}
	return sr.CanHandle(ctx, trigger, sm.extendedState, args...), nil
}

// SetTriggerParameters specify the arguments that must be supplied when a specific trigger is fired.
func (sm *StateMachine[T]) SetTriggerParameters(trigger Trigger, argumentTypes ...reflect.Type) {
	config := triggerWithParameters{Trigger: trigger, ArgumentTypes: argumentTypes}
	if _, ok := sm.triggerConfig[config.Trigger]; ok {
		panic(fmt.Sprintf("stateless: Parameters for the trigger '%v' have already been configured.", trigger))
	}
	sm.triggerConfig[trigger] = config
}

// Fire see FireCtx
func (sm *StateMachine[T]) Fire(trigger Trigger, args ...interface{}) error {
	return sm.FireCtx(context.Background(), trigger, args...)
}

// FireCtx transition from the current state via the specified trigger.
// The target state is determined by the configuration of the current state.
// Actions associated with leaving the current state and entering the new one will be invoked.
//
// An error is returned if any of the state machine actions or the state callbacks return an error
// without wrapping. It can also return an error if the trigger is not mapped to any state change,
// being this error the one returned by `OnUnhandledTrigger` func.
//
// There is no rollback mechanism in case there is an action error after the state has been changed.
// Guard clauses or error states can be used gracefully handle this situations.
//
// The context is passed down to all actions and callbacks called within the scope of this method.
// There is no context error checking, although it may be implemented in future releases.
func (sm *StateMachine[T]) FireCtx(ctx context.Context, trigger Trigger, args ...interface{}) error {
	return sm.internalFire(ctx, trigger, args...)
}

// OnTransitioned registers a callback that will be invoked every time the state machine
// successfully finishes a transitions from one state into another.
func (sm *StateMachine[T]) OnTransitioned(fn ...TransitionFunc) {
	sm.onTransitionedEvents = append(sm.onTransitionedEvents, fn...)
}

// OnTransitioning registers a callback that will be invoked every time the state machine
// starts a transitions from one state into another.
func (sm *StateMachine[T]) OnTransitioning(fn ...TransitionFunc) {
	sm.onTransitioningEvents = append(sm.onTransitioningEvents, fn...)
}

// OnUnhandledTrigger override the default behaviour of returning an error when an unhandled trigger.
func (sm *StateMachine[T]) OnUnhandledTrigger(fn UnhandledTriggerActionFunc) {
	sm.unhandledTriggerAction = fn
}

// Configure begin configuration of the entry/exit actions and allowed transitions
// when the state machine is in a particular state.
func (sm *StateMachine[T]) Configure(state State) *StateConfiguration[T] {
	return &StateConfiguration[T]{sm: sm, sr: sm.stateRepresentation(state), lookup: sm.stateRepresentation}
}

// Firing returns true when the state machine is processing a trigger.
func (sm *StateMachine[T]) Firing() bool {
	return sm.ops.Load() != 0
}

// String returns a human-readable representation of the state machine.
// It is not guaranteed that the order of the PermittedTriggers is the same in consecutive executions.
func (sm *StateMachine[T]) String() string {
	state, err := sm.State(context.Background())
	if err != nil {
		return ""
	}

	// PermittedTriggers only returns an error if state accessor returns one, and it has already been checked.
	triggers, _ := sm.PermittedTriggers()
	return fmt.Sprintf("StateMachine {{ State = %v, PermittedTriggers = %v }}", state, triggers)
}

func (sm *StateMachine[T]) setState(ctx context.Context, state State) error {
	return sm.stateMutator(ctx, state)
}

func (sm *StateMachine[T]) currentState(ctx context.Context) (sr *stateRepresentation[T], err error) {
	var state State
	state, err = sm.State(ctx)
	if err == nil {
		sr = sm.stateRepresentation(state)
	}
	return
}

func (sm *StateMachine[T]) stateRepresentation(state State) (sr *stateRepresentation[T]) {
	var ok bool
	if sr, ok = sm.stateConfig[state]; !ok {
		sr = newstateRepresentation[T](state)
		sm.stateConfig[state] = sr
	}
	return
}

func (sm *StateMachine[T]) internalFire(ctx context.Context, trigger Trigger, args ...interface{}) error {
	switch sm.firingMode {
	case FiringImmediate:
		return sm.internalFireOne(ctx, trigger, args...)
	case FiringQueued:
		fallthrough
	default:
		return sm.internalFireQueued(ctx, trigger, args...)
	}
}

type queuedTrigger struct {
	Context context.Context
	Trigger Trigger
	Args    []interface{}
}

func (sm *StateMachine[T]) internalFireQueued(ctx context.Context, trigger Trigger, args ...interface{}) error {
	sm.firingMutex.Lock()
	sm.eventQueue.PushBack(queuedTrigger{Context: ctx, Trigger: trigger, Args: args})
	sm.firingMutex.Unlock()
	if sm.Firing() {
		return nil
	}

	for {
		sm.firingMutex.Lock()
		e := sm.eventQueue.Front()
		if e == nil {
			sm.firingMutex.Unlock()
			break
		}
		et := sm.eventQueue.Remove(e).(queuedTrigger)
		sm.firingMutex.Unlock()
		if err := sm.internalFireOne(et.Context, et.Trigger, et.Args...); err != nil {
			return err
		}
	}
	return nil
}

func (sm *StateMachine[T]) internalFireOne(ctx context.Context, trigger Trigger, args ...interface{}) (err error) {
	sm.ops.Add(1)
	defer sm.ops.Add(^uint64(0))
	var (
		config triggerWithParameters
		ok     bool
	)
	if config, ok = sm.triggerConfig[trigger]; ok {
		config.validateParameters(args...)
	}
	source, err := sm.State(ctx)
	if err != nil {
		return
	}
	representativeState := sm.stateRepresentation(source)
	var result triggerBehaviourResult[T]
	if result, ok = representativeState.FindHandler(ctx, trigger, sm.extendedState, args...); !ok {
		return sm.unhandledTriggerAction(ctx, representativeState.State, trigger, result.UnmetGuardConditions)
	}
	switch t := result.Handler.(type) {
	case *ignoredTriggerBehaviour[T]:
		// ignored
	case *reentryTriggerBehaviour[T]:
		transition := Transition{Source: source, Destination: t.Destination, Trigger: trigger}
		err = sm.handleReentryTrigger(ctx, representativeState, transition, args...)
	case *dynamicTriggerBehaviour[T]:
		var destination interface{}
		destination, err = t.Destination(ctx, args...)
		if err == nil {
			transition := Transition{Source: source, Destination: destination, Trigger: trigger}
			err = sm.handleTransitioningTrigger(ctx, representativeState, transition, args...)
		}
	case *transitioningTriggerBehaviour[T]:
		transition := Transition{Source: source, Destination: t.Destination, Trigger: trigger}
		err = sm.handleTransitioningTrigger(ctx, representativeState, transition, args...)
	case *internalTriggerBehaviour[T]:
		var sr *stateRepresentation[T]
		sr, err = sm.currentState(ctx)
		if err == nil {
			transition := Transition{Source: source, Destination: source, Trigger: trigger}
			err = sr.InternalAction(ctx, transition, sm.extendedState, args...)
		}
	}
	return
}

func (sm *StateMachine[T]) handleReentryTrigger(ctx context.Context, sr *stateRepresentation[T], transition Transition, args ...interface{}) error {
	if err := sr.Exit(ctx, transition, sm.extendedState, args...); err != nil {
		return err
	}
	newSr := sm.stateRepresentation(transition.Destination)
	if !transition.IsReentry() {
		transition = Transition{Source: transition.Destination, Destination: transition.Destination, Trigger: transition.Trigger}
		if err := newSr.Exit(ctx, transition, sm.extendedState, args...); err != nil {
			return err
		}
	}
	callEvents(sm.onTransitioningEvents, ctx, transition)
	rep, err := sm.enterState(ctx, newSr, transition, args...)
	if err != nil {
		return err
	}
	if err := sm.setState(ctx, rep.State); err != nil {
		return err
	}
	callEvents(sm.onTransitionedEvents, ctx, transition)
	return nil
}

func (sm *StateMachine[T]) handleTransitioningTrigger(ctx context.Context, sr *stateRepresentation[T], transition Transition, args ...interface{}) error {
	if err := sr.Exit(ctx, transition, sm.extendedState, args...); err != nil {
		return err
	}
	callEvents(sm.onTransitioningEvents, ctx, transition)
	if err := sm.setState(ctx, transition.Destination); err != nil {
		return err
	}
	newSr := sm.stateRepresentation(transition.Destination)
	rep, err := sm.enterState(ctx, newSr, transition, args...)
	if err != nil {
		return err
	}
	// Check if state has changed by entering new state (by firing triggers in OnEntry or such)
	if rep.State != newSr.State {
		if err := sm.setState(ctx, rep.State); err != nil {
			return err
		}
	}
	callEvents(sm.onTransitionedEvents, ctx, Transition{transition.Source, rep.State, transition.Trigger, false})
	return nil
}

func (sm *StateMachine[T]) enterState(ctx context.Context, sr *stateRepresentation[T], transition Transition, args ...interface{}) (*stateRepresentation[T], error) {
	// Enter the new state
	err := sr.Enter(ctx, transition, sm.extendedState, args...)
	if err != nil {
		return nil, err
	}
	// Recursively enter substates that have an initial transition
	if sr.HasInitialState {
		isValidForInitialState := false
		for _, substate := range sr.Substates {
			// Verify that the target state is a substate
			// Check if state has substate(s), and if an initial transition(s) has been set up.
			if substate.State == sr.InitialTransitionTarget {
				isValidForInitialState = true
				break
			}
		}
		if !isValidForInitialState {
			panic(fmt.Sprintf("stateless: The target (%v) for the initial transition is not a substate.", sr.InitialTransitionTarget))
		}
		initialTranslation := Transition{Source: transition.Source, Destination: sr.InitialTransitionTarget, Trigger: transition.Trigger, isInitial: true}
		sr = sm.stateRepresentation(sr.InitialTransitionTarget)
		callEvents(sm.onTransitioningEvents, ctx, Transition{transition.Destination, initialTranslation.Destination, transition.Trigger, false})
		sr, err = sm.enterState(ctx, sr, initialTranslation, args...)
	}
	return sr, err
}
