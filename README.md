# stateless

[![Documentation](https://godoc.org/github.com/qmuntal/stateless?status.svg)](https://godoc.org/github.com/qmuntal/stateless)
[![Build Status](https://travis-ci.com/qmuntal/stateless.svg?branch=master)](https://travis-ci.com/qmuntal/stateless)
[![Go Report Card](https://goreportcard.com/badge/github.com/qmuntal/stateless)](https://goreportcard.com/report/github.com/qmuntal/stateless)
[![codecov](https://coveralls.io/repos/github/qmuntal/stateless/badge.svg)](https://coveralls.io/github/qmuntal/stateless?branch=master)
[![codeclimate](https://codeclimate.com/github/qmuntal/stateless/badges/gpa.svg)](https://codeclimate.com/github/qmuntal/stateless)
[![License](https://img.shields.io/badge/License-BSD%202--Clause-orange.svg)](https://opensource.org/licenses/MIT)

**Create *state machines* and lightweight *state machine-based workflows* directly in Go code:**

```go
phoneCall := stateless.NewStateMachine(stateOffHook)

phoneCall.Configure(stateOffHook).Permit(triggerCallDialed, stateRinging)

phoneCall.Configure(stateRinging).
    OnEntryFrom(triggerCallDialed, func(_ context.Context, args ...interface{}) error {
        onDialed(args[0].(string))
        return nil
    }).
    Permit(triggerCallConnected, stateConnected)

phoneCall.Configure(stateConnected).
    OnEntry(func(_ context.Context, _ ...interface{}) error {
        startCallTimer()
        return nil
    }).
    OnExit(func(_ context.Context, _ ...interface{}) error {
        stopCallTimer()
        return nil
    }).
    Permit(triggerLeftMessage, stateOffHook).
    Permit(triggerPlacedOnHold, stateOnHold)

// .. 

phoneCall.Fire(ctx, triggerCallDialed, "qmuntal")
```

This project, as well as the example above, is a almost direct port of [dotnet-state-machine/stateless](https://github.com/dotnet-state-machine/stateless), which is written in C#.

## Features

Most standard state machine constructs are supported:

 * Hierarchical states
 * Entry/exit events for states
 * Guard clauses to support conditional transitions
 * Introspection

Some useful extensions are also provided:

 * Ability to store state externally (for example, in a property tracked by an ORM)
 * Parameterised triggers
 * Reentrant states

### Hierarchical States

In the example below, the `OnHold` state is a substate of the `Connected` state. This means that an `OnHold` call is still connected.

```go
phoneCall.Configure(stateOnHold).SubstateOf(stateConnected).
    Permit(triggerTakenOffHold, stateConnected).
    Permit(triggerPhoneHurledAgainstWall, statePhoneDestroyed)
```

In addition to the `StateMachine.State` property, which will report the precise current state, an `IsInState(State)` method is provided. `IsInState(State)` will take substates into account, so that if the example above was in the `OnHold` state, `IsInState(State.Connected)` would also evaluate to `true`.

### Entry/Exit Events

In the example, the `StartCallTimer()` method will be executed when a call is connected. The `StopCallTimer()` will be executed when call completes (by either hanging up or hurling the phone against the wall.)

The call can move between the `Connected` and `OnHold` states without the `StartCallTimer()` and `StopCallTimer()` methods being called repeatedly because the `OnHold` state is a substate of the `Connected` state.

Entry/Exit event handlers can be supplied with a parameter of type `Transition` that describes the trigger, source and destination states.

### External State Storage

Stateless is designed to be embedded in various application models. For example, some ORMs place requirements upon where mapped data may be stored, and UI frameworks often require state to be stored in special "bindable" properties. To this end, the `StateMachine` constructor can accept function arguments that will be used to read and write the state values:

```go
stateMachine := stateless.NewStateMachineWithExternalStorage(func(_ context.Context) (stateless.State, error) {
    return myState.Value, nil
}, func(_ context.Context, state stateless.State) error {
    myState.Value  = state
    return nil
}, stateless.FiringQueued)
```

In this example the state machine will use the `myState` object for state storage.

### Introspection

The state machine can provide a list of the triggers that can be successfully fired within the current state via the `StateMachine.PermittedTriggers` property.

### Guard Clauses

The state machine will choose between multiple transitions based on guard clauses, e.g.:

```go
phoneCall.Configure(stateOffHook).
    Permit(triggerCallDialled, stateRinging, func(_ context.Context, _ ...interface{}) bool {return IsValidNumber()}).
    Permit(triggerCallDialled, stateBeeping, func(_ context.Context, _ ...interface{}) bool {return !IsValidNumber()})
```

Guard clauses within a state must be mutually exclusive (multiple guard clauses cannot be valid at the same time.) Substates can override transitions by respecifying them, however substates cannot disallow transitions that are allowed by the superstate.

The guard clauses will be evaluated whenever a trigger is fired. Guards should therefor be made side effect free.

### Parameterised Triggers

Strongly-typed parameters can be assigned to triggers:

```go
stateMachine.SetTriggerParameters(triggerCallDialed, reflect.TypeOf(""))

stateMachine.Configure(stateRinging).OnEntryFrom(triggerCallDialed, func(_ context.Context, args ...interface{}) error {
    fmt.Println(args[0].(string))
    return nil
})

stateMachine.Fire(triggerCallDialed, "qmuntal")
```

It is runtime safe to cast parameters to the ones specified in `SetTriggerParameters`. If the parameters passed in `Fire` do not match the ones specified, a panic will be thrown

Trigger parameters can be used to dynamically select the destination state using the `PermitDynamic()` configuration method.

### Ignored Transitions and Reentrant States

Firing a trigger that does not have an allowed transition associated with it will cause a panic to be thrown.

To ignore triggers within certain states, use the `Ignore(Trigger)` directive:

```go
phoneCall.Configure(stateConnected).
    Ignore(triggerCallDialled)
```

Alternatively, a state can be marked reentrant so its entry and exit events will fire even when transitioning from/to itself:

```go
stateMachine.Configure(stateAssigned).
    PermitReentry(triggerAssigned).
    OnEntry(func(_ context.Context, _ ...interface{}) error {
        startCallTimer()
        return nil
    })
```

By default, triggers must be ignored explicitly. To override Stateless's default behaviour of throwing a panic when an unhandled trigger is fired, configure the state machine using the `OnUnhandledTrigger` method:

```go
stateMachine.OnUnhandledTrigger( func (_ context.Context, state State, _ Trigger, _ []string) {})
```
