package stateless

import (
	"context"
	"reflect"
	"runtime"
)

type invocationInfo struct {
	Method string
}

func newinvocationInfo(method interface{}) invocationInfo {
	return invocationInfo{
		Method: runtime.FuncForPC(reflect.ValueOf(method).Pointer()).Name(),
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

type triggerBehaviour interface {
	GuardConditionMet(context.Context, ...interface{}) bool
	UnmetGuardConditions(context.Context, ...interface{}) []string
	ResultsInTransitionFrom(context.Context, State, ...interface{}) (State, bool)
	GetTrigger() Trigger
}

type baseTriggerBehaviour struct {
	Guard   transitionGuard
	Trigger Trigger
}

func (t *baseTriggerBehaviour) GetTrigger() Trigger {
	return t.Trigger
}

func (t *baseTriggerBehaviour) GuardConditionMet(ctx context.Context, args ...interface{}) bool {
	return t.Guard.GuardConditionMet(ctx, args...)
}

func (t *baseTriggerBehaviour) UnmetGuardConditions(ctx context.Context, args ...interface{}) []string {
	return t.Guard.UnmetGuardConditions(ctx, args...)
}

type ignoredTriggerBehaviour struct {
	baseTriggerBehaviour
}

func (t *ignoredTriggerBehaviour) ResultsInTransitionFrom(_ context.Context, _ State, _ ...interface{}) (State, bool) {
	return 0, false
}

type reentryTriggerBehaviour struct {
	baseTriggerBehaviour
	Destination State
}

func (t *reentryTriggerBehaviour) ResultsInTransitionFrom(_ context.Context, _ State, _ ...interface{}) (State, bool) {
	return t.Destination, true
}

type transitioningTriggerBehaviour struct {
	baseTriggerBehaviour
	Destination State
}

func (t *transitioningTriggerBehaviour) ResultsInTransitionFrom(_ context.Context, _ State, _ ...interface{}) (State, bool) {
	return t.Destination, true
}

type dynamicTriggerBehaviour struct {
	baseTriggerBehaviour
	Destination func(context.Context, ...interface{}) (State, error)
}

func (t *dynamicTriggerBehaviour) ResultsInTransitionFrom(ctx context.Context, _ State, args ...interface{}) (st State, ok bool) {
	var err error
	st, err = t.Destination(ctx, args...)
	if err == nil {
		ok = true
	}
	return
}

type internalTriggerBehaviour struct {
	baseTriggerBehaviour
	Action ActionFunc
}

func (t *internalTriggerBehaviour) ResultsInTransitionFrom(_ context.Context, source State, _ ...interface{}) (State, bool) {
	return source, false
}

func (t *internalTriggerBehaviour) Execute(ctx context.Context, transition Transition, args ...interface{}) error {
	return t.Action(ctx, transition, args...)
}

type triggerBehaviourResult struct {
	Handler              triggerBehaviour
	UnmetGuardConditions []string
}

// TriggerWithParameters associates configured parameters with an underlying trigger value.
type TriggerWithParameters struct {
	Trigger       Trigger
	ArgumentTypes []reflect.Type
}

func (t TriggerWithParameters) validateParameters(args ...interface{}) bool {
	if len(args) != len(t.ArgumentTypes) {
		return false
	}
	for i := range t.ArgumentTypes {
		if t.ArgumentTypes[i] != reflect.TypeOf(args[i]) {
			return false
		}
	}
	return true
}

type onTransitionEvents []func(context.Context, Transition)

func (e onTransitionEvents) Invoke(ctx context.Context, transition Transition) {
	for _, event := range e {
		event(ctx, transition)
	}
}

type queuedTrigger struct {
	Trigger Trigger
	Args    []interface{}
}
