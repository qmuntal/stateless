package stateless

import (
	"reflect"
	"runtime"
)

// InfocationInfo describes the context of a guard method.
type InvocationInfo struct {
	Method      string
	Description string
	IsAsync     bool
}

func newInvocationInfo(method interface{}, description string, isAsync bool) InvocationInfo {
	return InvocationInfo{
		Method:      runtime.FuncForPC(reflect.ValueOf(method).Pointer()).Name(),
		Description: description,
		IsAsync:     isAsync,
	}
}

func (inv InvocationInfo) String() string {
	if inv.Description != "" {
		return inv.Description
	}
	if inv.Method != "" {
		return inv.Method
	}
	return "<nil>"
}

// TriggerInfo describes a trigger.
type TriggerInfo Trigger

func (t TriggerInfo) String() string {
	return string(t)
}

// DynamicStateInfo describes a dynamic state.
type DynamicStateInfo struct {
	DestinationState string
	Criterion        string
}

// TransitionInfo describes a transition.
type TransitionInfo struct {
	Trigger           TriggerInfo
	GuardDescriptions []InvocationInfo
}

// DynamicTransitionInfo describes a transition that can be initiated from a trigger,
// but whose result is non-deterministic.
type DynamicTransitionInfo struct {
	TransitionInfo
	DestinationStateSelectorDescription InvocationInfo
	PossibleDestinationStates           []DynamicStateInfo
}
