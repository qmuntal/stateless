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

func newinvocationInfo(method any) invocationInfo {
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

type guardCondition struct {
	Guard       GuardFunc
	Description invocationInfo
}

type transitionGuard struct {
	Guards []guardCondition
}

func newtransitionGuard(guards ...GuardFunc) transitionGuard {
	tg := transitionGuard{Guards: make([]guardCondition, len(guards))}
	for i, guard := range guards {
		tg.Guards[i] = guardCondition{
			Guard:       guard,
			Description: newinvocationInfo(guard),
		}
	}
	return tg
}

// GuardConditionsMet is true if all of the guard functions return true.
func (t transitionGuard) GuardConditionMet(ctx context.Context, args ...any) bool {
	for _, guard := range t.Guards {
		if !guard.Guard(ctx, args...) {
			return false
		}
	}
	return true
}

func (t transitionGuard) UnmetGuardConditions(ctx context.Context, buf []string, args ...any) []string {
	if cap(buf) < len(t.Guards) {
		buf = make([]string, 0, len(t.Guards))
	}
	buf = buf[:0]
	for _, guard := range t.Guards {
		if !guard.Guard(ctx, args...) {
			buf = append(buf, guard.Description.String())
		}
	}
	return buf
}

type triggerBehaviour[T Trigger] interface {
	GuardConditionMet(context.Context, ...any) bool
	UnmetGuardConditions(context.Context, []string, ...any) []string
	GetTrigger() T
}

type baseTriggerBehaviour[T Trigger] struct {
	Guard   transitionGuard
	Trigger T
}

func (t *baseTriggerBehaviour[T]) GetTrigger() T {
	return t.Trigger
}

func (t *baseTriggerBehaviour[T]) GuardConditionMet(ctx context.Context, args ...any) bool {
	return t.Guard.GuardConditionMet(ctx, args...)
}

func (t *baseTriggerBehaviour[T]) UnmetGuardConditions(ctx context.Context, buf []string, args ...any) []string {
	return t.Guard.UnmetGuardConditions(ctx, buf, args...)
}

type ignoredTriggerBehaviour[T Trigger] struct {
	baseTriggerBehaviour[T]
}

type reentryTriggerBehaviour[S State, T Trigger] struct {
	baseTriggerBehaviour[T]
	Destination S
}

type transitioningTriggerBehaviour[S State, T Trigger] struct {
	baseTriggerBehaviour[T]
	Destination S
}

type dynamicTriggerBehaviour[S State, T Trigger] struct {
	baseTriggerBehaviour[T]
	Destination func(context.Context, ...any) (S, error)
}

type internalTriggerBehaviour[S State, T Trigger] struct {
	baseTriggerBehaviour[T]
	Action ActionFunc
}

func (t *internalTriggerBehaviour[S, T]) Execute(ctx context.Context, transition Transition[S, T], args ...any) error {
	ctx = withTransition(ctx, transition)
	return t.Action(ctx, args...)
}

type triggerBehaviourResult[T Trigger] struct {
	Handler              triggerBehaviour[T]
	UnmetGuardConditions []string
}

// triggerWithParameters associates configured parameters with an underlying trigger value.
type triggerWithParameters[T Trigger] struct {
	Trigger       T
	ArgumentTypes []reflect.Type
}

func (t triggerWithParameters[T]) validateParameters(args ...any) {
	if len(args) != len(t.ArgumentTypes) {
		panic(fmt.Sprintf("stateless: An unexpected amount of parameters have been supplied. Expecting '%d' but got '%d'.", len(t.ArgumentTypes), len(args)))
	}
	for i := range t.ArgumentTypes {
		tp := reflect.TypeOf(args[i])
		want := t.ArgumentTypes[i]
		if !tp.ConvertibleTo(want) {
			panic(fmt.Sprintf("stateless: The argument in position '%d' is of type '%v' but must be convertible to '%v'.", i, tp, want))
		}
	}
}
