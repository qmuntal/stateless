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
func (t transitionGuard) GuardConditionMet(ctx context.Context, args ...interface{}) bool {
	for _, guard := range t.Guards {
		if !guard.Guard(ctx, args...) {
			return false
		}
	}
	return true
}

func (t transitionGuard) UnmetGuardConditions(ctx context.Context, args ...interface{}) []string {
	unmet := make([]string, 0, len(t.Guards))
	for _, guard := range t.Guards {
		if !guard.Guard(ctx, args...) {
			unmet = append(unmet, guard.Description.String())
		}
	}
	return unmet
}

type triggerBehaviour[T Trigger] interface {
	GuardConditionMet(context.Context, ...interface{}) bool
	UnmetGuardConditions(context.Context, ...interface{}) []string
	GetTrigger() T
}

type baseTriggerBehaviour[T Trigger] struct {
	Guard   transitionGuard
	Trigger T
}

func (t *baseTriggerBehaviour[T]) GetTrigger() T {
	return t.Trigger
}

func (t *baseTriggerBehaviour[_]) GuardConditionMet(ctx context.Context, args ...interface{}) bool {
	return t.Guard.GuardConditionMet(ctx, args...)
}

func (t *baseTriggerBehaviour[_]) UnmetGuardConditions(ctx context.Context, args ...interface{}) []string {
	return t.Guard.UnmetGuardConditions(ctx, args...)
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
	Destination func(context.Context, ...interface{}) (S, error)
}

func (t *dynamicTriggerBehaviour[S, _]) ResultsInTransitionFrom(ctx context.Context, _ S, args ...interface{}) (st S, ok bool) {
	var err error
	st, err = t.Destination(ctx, args...)
	if err == nil {
		ok = true
	}
	return
}

type internalTriggerBehaviour[S State, T Trigger] struct {
	baseTriggerBehaviour[T]
	Action ActionFunc
}

func (t *internalTriggerBehaviour[S, T]) Execute(ctx context.Context, transition Transition[S, T], args ...interface{}) error {
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

func (t triggerWithParameters[_]) validateParameters(args ...interface{}) {
	if len(args) != len(t.ArgumentTypes) {
		panic(fmt.Sprintf("stateless: Too many parameters have been supplied. Expecting '%d' but got '%d'.", len(t.ArgumentTypes), len(args)))
	}
	for i := range t.ArgumentTypes {
		if t.ArgumentTypes[i] != reflect.TypeOf(args[i]) {
			panic(fmt.Sprintf("stateless: The argument in position '%d' is of type '%v' but must be of type '%v'.", i, reflect.TypeOf(args[i]), t.ArgumentTypes[i]))
		}
	}
}
