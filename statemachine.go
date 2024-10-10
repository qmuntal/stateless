package stateless

import (
	"context"
	"fmt"
	"reflect"
	"sync"
)

// State is used to to represent the possible machine states.
type State interface {
	comparable
}

// Trigger is used to represent the triggers that cause state transitions.
type Trigger interface {
	comparable
}

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
type Transition[S State, T Trigger] struct {
	Source      S
	Destination S
	Trigger     T

	isInitial bool
}

// IsReentry returns true if the transition is a re-entry,
// i.e. the identity transition.
func (t *Transition[_, _]) IsReentry() bool {
	return t.Source == t.Destination
}

type TransitionFunc[S State, T Trigger] func(context.Context, Transition[S, T])

// UnhandledTriggerActionFunc defines a function that will be called when a trigger is not handled.
type UnhandledTriggerActionFunc[S State, T Trigger] func(ctx context.Context, state S, trigger T, unmetGuards []string) error

// DefaultUnhandledTriggerAction is the default unhandled trigger action.
func DefaultUnhandledTriggerAction[S State, T Trigger](_ context.Context, state S, trigger T, unmetGuards []string) error {
	if len(unmetGuards) != 0 {
		return fmt.Errorf("stateless: Trigger '%v' is valid for transition from state '%v' but a guard conditions are not met. Guard descriptions: '%v", trigger, state, unmetGuards)
	}
	return fmt.Errorf("stateless: No valid leaving transitions are permitted from state '%v' for trigger '%v', consider ignoring the trigger", state, trigger)
}

func callEvents[S State, T Trigger](events []TransitionFunc[S, T], ctx context.Context, transition Transition[S, T]) {
	for _, e := range events {
		e(ctx, transition)
	}
}

// A StateMachine is an abstract machine that can be in exactly one of a finite number of states at any given time.
// It is safe to use the StateMachine concurrently, but non of the callbacks (state manipulation, actions, events, ...) are guarded,
// so it is up to the client to protect them against race conditions.
type StateMachine[S State, T Trigger, A any] struct {
	stateConfig            map[S]*stateRepresentation[S, T, A]
	triggerConfig          map[T]triggerWithParameters[T]
	stateAccessor          func(context.Context) (S, error)
	stateMutator           func(context.Context, S) error
	unhandledTriggerAction UnhandledTriggerActionFunc[S, T]
	onTransitioningEvents  []TransitionFunc[S, T]
	onTransitionedEvents   []TransitionFunc[S, T]
	stateMutex             sync.RWMutex
	mode                   fireMode[T, A]
}

func newStateMachine[S State, T Trigger, A any](firingMode FiringMode) *StateMachine[S, T, A] {
	sm := &StateMachine[S, T, A]{
		stateConfig:            make(map[S]*stateRepresentation[S, T, A]),
		triggerConfig:          make(map[T]triggerWithParameters[T]),
		unhandledTriggerAction: UnhandledTriggerActionFunc[S, T](DefaultUnhandledTriggerAction[S, T]),
	}
	if firingMode == FiringImmediate {
		sm.mode = &fireModeImmediate[S, T, A]{sm: sm}
	} else {
		sm.mode = &fireModeQueued[S, T, A]{sm: sm}
	}
	return sm
}

// NewStateMachine returns a queued state machine.
func NewStateMachine[S State, T Trigger, A any](initialState S) *StateMachine[S, T, A] {
	return NewStateMachineWithMode[S, T, A](initialState, FiringQueued)
}

// NewStateMachineWithMode returns a state machine with the desired firing mode
func NewStateMachineWithMode[S State, T Trigger, A any](initialState S, firingMode FiringMode) *StateMachine[S, T, A] {
	var stateMutex sync.Mutex
	sm := newStateMachine[S, T, A](firingMode)
	reference := &struct {
		State S
	}{State: initialState}
	sm.stateAccessor = func(_ context.Context) (S, error) {
		stateMutex.Lock()
		defer stateMutex.Unlock()
		return reference.State, nil
	}
	sm.stateMutator = func(_ context.Context, state S) error {
		stateMutex.Lock()
		defer stateMutex.Unlock()
		reference.State = state
		return nil
	}
	return sm
}

// NewStateMachineWithExternalStorage returns a state machine with external state storage.
func NewStateMachineWithExternalStorage[S State, T Trigger, A any](stateAccessor func(context.Context) (S, error), stateMutator func(context.Context, S) error, firingMode FiringMode) *StateMachine[S, T, A] {
	sm := newStateMachine[S, T, A](firingMode)
	sm.stateAccessor = stateAccessor
	sm.stateMutator = stateMutator
	return sm
}

// ToGraph returns the DOT representation of the state machine.
// It is not guaranteed that the returned string will be the same in different executions.
func (sm *StateMachine[S, T, A]) ToGraph() string {
	return new(graph[S, T, A]).formatStateMachine(sm)
}

// State returns the current state.
func (sm *StateMachine[S, _, _]) State(ctx context.Context) (S, error) {
	return sm.stateAccessor(ctx)
}

// MustState returns the current state without the error.
// It is safe to use this method when used together with NewStateMachine
// or when using NewStateMachineWithExternalStorage with an state accessor that
// does not return an error.
func (sm *StateMachine[S, _, _]) MustState() S {
	st, err := sm.State(context.Background())
	if err != nil {
		panic(err)
	}
	return st
}

// PermittedTriggers see PermittedTriggersCtx.
func (sm *StateMachine[_, T, A]) PermittedTriggers(arg A) ([]T, error) {
	return sm.PermittedTriggersCtx(context.Background(), arg)
}

// PermittedTriggersCtx returns the currently-permissible trigger values.
func (sm *StateMachine[_, T, A]) PermittedTriggersCtx(ctx context.Context, arg A) ([]T, error) {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return nil, err
	}
	return sr.PermittedTriggers(ctx, arg), nil
}

// Activate see ActivateCtx.
func (sm *StateMachine[S, T, _]) Activate() error {
	return sm.ActivateCtx(context.Background())
}

// ActivateCtx activates current state. Actions associated with activating the current state will be invoked.
// The activation is idempotent and subsequent activation of the same current state
// will not lead to re-execution of activation callbacks.
func (sm *StateMachine[S, T, _]) ActivateCtx(ctx context.Context) error {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return err
	}
	return sr.Activate(ctx)
}

// Deactivate see DeactivateCtx.
func (sm *StateMachine[S, T, _]) Deactivate() error {
	return sm.DeactivateCtx(context.Background())
}

// DeactivateCtx deactivates current state. Actions associated with deactivating the current state will be invoked.
// The deactivation is idempotent and subsequent deactivation of the same current state
// will not lead to re-execution of deactivation callbacks.
func (sm *StateMachine[S, T, _]) DeactivateCtx(ctx context.Context) error {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return err
	}
	return sr.Deactivate(ctx)
}

// IsInState see IsInStateCtx.
func (sm *StateMachine[S, T, _]) IsInState(state S) (bool, error) {
	return sm.IsInStateCtx(context.Background(), state)
}

// IsInStateCtx determine if the state machine is in the supplied state.
// Returns true if the current state is equal to, or a substate of, the supplied state.
func (sm *StateMachine[S, T, _]) IsInStateCtx(ctx context.Context, state S) (bool, error) {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return false, err
	}
	return sr.IsIncludedInState(state), nil
}

// CanFire see CanFireCtx.
func (sm *StateMachine[S, T, A]) CanFire(trigger T, arg A) (bool, error) {
	return sm.CanFireCtx(context.Background(), trigger, arg)
}

// CanFireCtx returns true if the trigger can be fired in the current state.
func (sm *StateMachine[S, T, A]) CanFireCtx(ctx context.Context, trigger T, arg A) (bool, error) {
	sr, err := sm.currentState(ctx)
	if err != nil {
		return false, err
	}
	return sr.CanHandle(ctx, trigger, arg), nil
}

// SetTriggerParameters specify the arguments that must be supplied when a specific trigger is fired.
func (sm *StateMachine[S, T, A]) SetTriggerParameters(trigger T, argumentTypes ...reflect.Type) {
	config := triggerWithParameters[T]{Trigger: trigger, ArgumentTypes: argumentTypes}
	if _, ok := sm.triggerConfig[config.Trigger]; ok {
		panic(fmt.Sprintf("stateless: Parameters for the trigger '%v' have already been configured.", trigger))
	}
	sm.triggerConfig[trigger] = config
}

// Fire see FireCtx
func (sm *StateMachine[S, T, A]) Fire(trigger T, arg A) error {
	return sm.FireCtx(context.Background(), trigger, arg)
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
func (sm *StateMachine[S, T, A]) FireCtx(ctx context.Context, trigger T, arg A) error {
	return sm.internalFire(ctx, trigger, arg)
}

// OnTransitioned registers a callback that will be invoked every time the state machine
// successfully finishes a transitions from one state into another.
func (sm *StateMachine[S, T, _]) OnTransitioned(fn ...TransitionFunc[S, T]) {
	sm.onTransitionedEvents = append(sm.onTransitionedEvents, fn...)
}

// OnTransitioning registers a callback that will be invoked every time the state machine
// starts a transitions from one state into another.
func (sm *StateMachine[S, T, _]) OnTransitioning(fn ...TransitionFunc[S, T]) {
	sm.onTransitioningEvents = append(sm.onTransitioningEvents, fn...)
}

// OnUnhandledTrigger override the default behaviour of returning an error when an unhandled trigger.
func (sm *StateMachine[S, T, _]) OnUnhandledTrigger(fn UnhandledTriggerActionFunc[S, T]) {
	sm.unhandledTriggerAction = fn
}

// Configure begin configuration of the entry/exit actions and allowed transitions
// when the state machine is in a particular state.
func (sm *StateMachine[S, T, A]) Configure(state S) *StateConfiguration[S, T, A] {
	return &StateConfiguration[S, T, A]{sm: sm, sr: sm.stateRepresentation(state), lookup: sm.stateRepresentation}
}

// Firing returns true when the state machine is processing a trigger.
func (sm *StateMachine[_, _, _]) Firing() bool {
	return sm.mode.Firing()
}

// String returns a human-readable representation of the state machine.
// It is not guaranteed that the order of the PermittedTriggers is the same in consecutive executions.
func (sm *StateMachine[_, T, A]) String(arg A) string {
	state, err := sm.State(context.Background())
	if err != nil {
		return ""
	}

	// PermittedTriggers only returns an error if state accessor returns one, and it has already been checked.
	triggers, _ := sm.PermittedTriggers(arg)
	return fmt.Sprintf("StateMachine {{ State = %v, PermittedTriggers = %v }}", state, triggers)
}

func (sm *StateMachine[S, T, _]) setState(ctx context.Context, state S) error {
	return sm.stateMutator(ctx, state)
}

func (sm *StateMachine[S, T, A]) currentState(ctx context.Context) (*stateRepresentation[S, T, A], error) {
	state, err := sm.State(ctx)
	if err != nil {
		return nil, err
	}
	return sm.stateRepresentation(state), nil
}

func (sm *StateMachine[S, T, A]) stateRepresentation(state S) *stateRepresentation[S, T, A] {
	sm.stateMutex.RLock()
	sr, ok := sm.stateConfig[state]
	sm.stateMutex.RUnlock()
	if !ok {
		sm.stateMutex.Lock()
		defer sm.stateMutex.Unlock()
		// Check again, since another goroutine may have added it while we were waiting for the lock.
		if sr, ok = sm.stateConfig[state]; !ok {
			sr = newstateRepresentation[S, T, A](state)
			sm.stateConfig[state] = sr
		}
	}
	return sr
}

func (sm *StateMachine[S, T, A]) internalFire(ctx context.Context, trigger T, arg A) error {
	return sm.mode.Fire(ctx, trigger, arg)
}

func (sm *StateMachine[S, T, A]) internalFireOne(ctx context.Context, trigger T, arg A) error {
	var (
		val Validatable
		ok  bool
	)
	if val, ok = any(arg).(Validatable); ok {
		var config triggerWithParameters[T]
		if config, ok = sm.triggerConfig[trigger]; ok {
			config.validateParameters(val)
		}
	}
	source, err := sm.State(ctx)
	if err != nil {
		return err
	}
	representativeState := sm.stateRepresentation(source)
	var result triggerBehaviourResult[T, A]
	if result, ok = representativeState.FindHandler(ctx, trigger, arg); !ok {
		return sm.unhandledTriggerAction(ctx, representativeState.State, trigger, result.UnmetGuardConditions)
	}
	switch t := result.Handler.(type) {
	case *ignoredTriggerBehaviour[T, A]:
		// ignored
	case *reentryTriggerBehaviour[S, T, A]:
		transition := Transition[S, T]{Source: source, Destination: t.Destination, Trigger: trigger}
		err = sm.handleReentryTrigger(ctx, representativeState, transition, arg)
	case *dynamicTriggerBehaviour[S, T, A]:
		var destination S
		destination, err = t.Destination(ctx, arg)
		if err == nil {
			transition := Transition[S, T]{Source: source, Destination: destination, Trigger: trigger}
			err = sm.handleTransitioningTrigger(ctx, representativeState, transition, arg)
		}
	case *transitioningTriggerBehaviour[S, T, A]:
		if source == t.Destination {
			// If a trigger was found on a superstate that would cause unintended reentry, don't trigger.
			break
		}
		transition := Transition[S, T]{Source: source, Destination: t.Destination, Trigger: trigger}
		err = sm.handleTransitioningTrigger(ctx, representativeState, transition, arg)
	case *internalTriggerBehaviour[S, T, A]:
		var sr *stateRepresentation[S, T, A]
		sr, err = sm.currentState(ctx)
		if err == nil {
			transition := Transition[S, T]{Source: source, Destination: source, Trigger: trigger}
			err = sr.InternalAction(ctx, transition, arg)
		}
	}
	return err
}

func (sm *StateMachine[S, T, A]) handleReentryTrigger(ctx context.Context, sr *stateRepresentation[S, T, A], transition Transition[S, T], arg A) error {
	if err := sr.Exit(ctx, transition, arg); err != nil {
		return err
	}
	newSr := sm.stateRepresentation(transition.Destination)
	if !transition.IsReentry() {
		transition = Transition[S, T]{Source: transition.Destination, Destination: transition.Destination, Trigger: transition.Trigger}
		if err := newSr.Exit(ctx, transition, arg); err != nil {
			return err
		}
	}
	callEvents(sm.onTransitioningEvents, ctx, transition)
	rep, err := sm.enterState(ctx, newSr, transition, arg)
	if err != nil {
		return err
	}
	if err := sm.setState(ctx, rep.State); err != nil {
		return err
	}
	callEvents(sm.onTransitionedEvents, ctx, transition)
	return nil
}

func (sm *StateMachine[S, T, A]) handleTransitioningTrigger(ctx context.Context, sr *stateRepresentation[S, T, A], transition Transition[S, T], arg A) error {
	if err := sr.Exit(ctx, transition, arg); err != nil {
		return err
	}
	callEvents(sm.onTransitioningEvents, ctx, transition)
	if err := sm.setState(ctx, transition.Destination); err != nil {
		return err
	}
	newSr := sm.stateRepresentation(transition.Destination)
	rep, err := sm.enterState(ctx, newSr, transition, arg)
	if err != nil {
		return err
	}
	// Check if state has changed by entering new state (by firing triggers in OnEntry or such)
	if rep.State != newSr.State {
		if err := sm.setState(ctx, rep.State); err != nil {
			return err
		}
	}
	callEvents(sm.onTransitionedEvents, ctx, Transition[S, T]{transition.Source, rep.State, transition.Trigger, false})
	return nil
}

func (sm *StateMachine[S, T, A]) enterState(ctx context.Context, sr *stateRepresentation[S, T, A], transition Transition[S, T], arg A) (*stateRepresentation[S, T, A], error) {
	// Enter the new state
	err := sr.Enter(ctx, transition, arg)
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
		initialTranslation := Transition[S, T]{Source: transition.Source, Destination: sr.InitialTransitionTarget, Trigger: transition.Trigger, isInitial: true}
		sr = sm.stateRepresentation(sr.InitialTransitionTarget)
		callEvents(sm.onTransitioningEvents, ctx, Transition[S, T]{transition.Destination, initialTranslation.Destination, transition.Trigger, false})
		sr, err = sm.enterState(ctx, sr, initialTranslation, arg)
	}
	return sr, err
}
