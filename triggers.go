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

type guardCondition[A any] struct {
	Guard       GuardFunc[A]
	Description invocationInfo
}

type transitionGuard[A any] struct {
	Guards []guardCondition[A]
}

func newtransitionGuard[A any](guards ...GuardFunc[A]) transitionGuard[A] {
	tg := transitionGuard[A]{Guards: make([]guardCondition[A], len(guards))}
	for i, guard := range guards {
		tg.Guards[i] = guardCondition[A]{
			Guard:       guard,
			Description: newinvocationInfo(guard),
		}
	}
	return tg
}

// GuardConditionsMet is true if all of the guard functions return true.
func (t transitionGuard[A]) GuardConditionMet(ctx context.Context, arg A) bool {
	for _, guard := range t.Guards {
		if !guard.Guard(ctx, arg) {
			return false
		}
	}
	return true
}

func (t transitionGuard[A]) UnmetGuardConditions(ctx context.Context, buf []string, arg A) []string {
	if cap(buf) < len(t.Guards) {
		buf = make([]string, 0, len(t.Guards))
	}
	buf = buf[:0]
	for _, guard := range t.Guards {
		if !guard.Guard(ctx, arg) {
			buf = append(buf, guard.Description.String())
		}
	}
	return buf
}

type triggerBehaviour[T Trigger, A any] interface {
	GuardConditionMet(context.Context, A) bool
	UnmetGuardConditions(context.Context, []string, A) []string
	GetTrigger() T
}

type baseTriggerBehaviour[T Trigger, A any] struct {
	Guard   transitionGuard[A]
	Trigger T
}

func (t *baseTriggerBehaviour[T, A]) GetTrigger() T {
	return t.Trigger
}

func (t *baseTriggerBehaviour[T, A]) GuardConditionMet(ctx context.Context, arg A) bool {
	return t.Guard.GuardConditionMet(ctx, arg)
}

func (t *baseTriggerBehaviour[T, A]) UnmetGuardConditions(ctx context.Context, buf []string, arg A) []string {
	return t.Guard.UnmetGuardConditions(ctx, buf, arg)
}

type ignoredTriggerBehaviour[T Trigger, A any] struct {
	baseTriggerBehaviour[T, A]
}

type reentryTriggerBehaviour[S State, T Trigger, A any] struct {
	baseTriggerBehaviour[T, A]
	Destination S
}

type transitioningTriggerBehaviour[S State, T Trigger, A any] struct {
	baseTriggerBehaviour[T, A]
	Destination S
}

type dynamicTriggerBehaviour[S State, T Trigger, A any] struct {
	baseTriggerBehaviour[T, A]
	Destination func(context.Context, A) (S, error)
}

type internalTriggerBehaviour[S State, T Trigger, A any] struct {
	baseTriggerBehaviour[T, A]
	Action ActionFunc[A]
}

func (t *internalTriggerBehaviour[S, T, A]) Execute(ctx context.Context, transition Transition[S, T], arg A) error {
	ctx = withTransition(ctx, transition)
	return t.Action(ctx, arg)
}

type triggerBehaviourResult[T Trigger, A any] struct {
	Handler              triggerBehaviour[T, A]
	UnmetGuardConditions []string
}

type Validatable interface {
	TypeOf(int) reflect.Type
	Len() int
}

// triggerWithParameters associates configured parameters with an underlying trigger value.
type triggerWithParameters[T Trigger] struct {
	Trigger       T
	ArgumentTypes []reflect.Type
}

func (t triggerWithParameters[T]) validateParameters(args Validatable) {
	if args.Len() != len(t.ArgumentTypes) {
		panic(fmt.Sprintf("stateless: An unexpected amount of parameters have been supplied. Expecting '%d' but got '%d'.", len(t.ArgumentTypes), args.Len()))
	}
	for i := range t.ArgumentTypes {
		tp := args.TypeOf(i)
		want := t.ArgumentTypes[i]
		if !tp.ConvertibleTo(want) {
			panic(fmt.Sprintf("stateless: The argument in position '%d' is of type '%v' but must be convertible to '%v'.", i, tp, want))
		}
	}
}
