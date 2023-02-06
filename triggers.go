package stateless

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

type invocationInfo struct {
	Method string
}

func newinvocationInfo(method interface{}) invocationInfo {
	funcName := runtime.FuncForPC(reflect.ValueOf(method).Pointer()).Name()
	nameParts := strings.Split(funcName, ".")
	var name string
	if len(nameParts) != 0 {
		name = nameParts[len(nameParts)-1]
	}
	return invocationInfo{
		Method: name,
	}
}

func (inv invocationInfo) String() string {
	if inv.Method != "" {
		return inv.Method
	}
	return "<nil>"
}

type guardCondition[T any] struct {
	Guard       GuardFunc[T]
	Description invocationInfo
}

type transitionGuard[T any] struct {
	Guards []guardCondition[T]
}

func newtransitionGuard[T any](guards ...GuardFunc[T]) transitionGuard[T] {
	tg := transitionGuard[T]{Guards: make([]guardCondition[T], len(guards))}
	for i, guard := range guards {
		tg.Guards[i] = guardCondition[T]{
			Guard:       guard,
			Description: newinvocationInfo(guard),
		}
	}
	return tg
}

// GuardConditionsMet is true if all of the guard functions return true.
func (t transitionGuard[T]) GuardConditionMet(ctx context.Context, extendedState T, args ...interface{}) bool {
	for _, guard := range t.Guards {
		if !guard.Guard(ctx, extendedState, args...) {
			return false
		}
	}
	return true
}

func (t transitionGuard[T]) UnmetGuardConditions(ctx context.Context, extendedState T, args ...interface{}) []string {
	unmet := make([]string, 0, len(t.Guards))
	for _, guard := range t.Guards {
		if !guard.Guard(ctx, extendedState, args...) {
			unmet = append(unmet, guard.Description.String())
		}
	}
	return unmet
}

type triggerBehaviour[T any] interface {
	GuardConditionMet(context.Context, T, ...interface{}) bool
	UnmetGuardConditions(context.Context, T, ...interface{}) []string
	GetTrigger() Trigger
}

type baseTriggerBehaviour[T any] struct {
	Guard   transitionGuard[T]
	Trigger Trigger
}

func (t *baseTriggerBehaviour[T]) GetTrigger() Trigger {
	return t.Trigger
}

func (t *baseTriggerBehaviour[T]) GuardConditionMet(ctx context.Context, extendedState T, args ...interface{}) bool {
	return t.Guard.GuardConditionMet(ctx, extendedState, args...)
}

func (t *baseTriggerBehaviour[T]) UnmetGuardConditions(ctx context.Context, extendedState T, args ...interface{}) []string {
	return t.Guard.UnmetGuardConditions(ctx, extendedState, args...)
}

type ignoredTriggerBehaviour[T any] struct {
	baseTriggerBehaviour[T]
}

type reentryTriggerBehaviour[T any] struct {
	baseTriggerBehaviour[T]
	Destination State
}

type transitioningTriggerBehaviour[T any] struct {
	baseTriggerBehaviour[T]
	Destination State
}

type dynamicTriggerBehaviour[T any] struct {
	baseTriggerBehaviour[T]
	Destination func(context.Context, ...interface{}) (State, error)
}

type internalTriggerBehaviour[T any] struct {
	baseTriggerBehaviour[T]
	Action ActionFunc[T]
}

func (t *internalTriggerBehaviour[T]) Execute(ctx context.Context, transition Transition, extendedState T, args ...interface{}) error {
	ctx = withTransition(ctx, transition)
	return t.Action(ctx, extendedState, args...)
}

type triggerBehaviourResult[T any] struct {
	Handler              triggerBehaviour[T]
	UnmetGuardConditions []string
}

// triggerWithParameters associates configured parameters with an underlying trigger value.
type triggerWithParameters struct {
	Trigger       Trigger
	ArgumentTypes []reflect.Type
}

func (t triggerWithParameters) validateParameters(args ...interface{}) {
	if len(args) != len(t.ArgumentTypes) {
		panic(fmt.Sprintf("stateless: Too many parameters have been supplied. Expecting '%d' but got '%d'.", len(t.ArgumentTypes), len(args)))
	}
	for i := range t.ArgumentTypes {
		tp := reflect.TypeOf(args[i])
		want := t.ArgumentTypes[i]
		if !tp.ConvertibleTo(want) {
			panic(fmt.Sprintf("stateless: The argument in position '%d' is of type '%v' but must be convertible to '%v'.", i, tp, want))
		}
	}
}
