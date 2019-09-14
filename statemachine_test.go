package stateless

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransition_IsReentry(t *testing.T) {
	tests := []struct {
		name string
		t    *Transition
		want bool
	}{
		{"TransitionIsNotChange", &Transition{"1", "1", "0"}, true},
		{"TransitionIsChange", &Transition{"1", "2", "0"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.t.IsReentry(); got != tt.want {
				t.Errorf("Transition.IsReentry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStateMachine_NewStateMachine(t *testing.T) {
	sm := NewStateMachine(stateA)
	assert.Equal(t, stateA, sm.MustState())
}

func TestStateMachine_NewStateMachineWithExternalStorage(t *testing.T) {
	state := stateB
	sm := NewStateMachineWithExternalStorage(func(_ context.Context) (State, error) {
		return state, nil
	}, func(_ context.Context, s State) error {
		state = s
		return nil
	}, FiringImmediate)
	sm.Configure(stateB).Permit(triggerX, stateC)
	assert.Equal(t, stateB, sm.MustState())
	assert.Equal(t, stateB, state)
	sm.Fire(triggerX)
	assert.Equal(t, stateC, sm.MustState())
	assert.Equal(t, stateC, state)
}

func TestStateMachine_Configure_SubstateIsIncludedInCurrentState(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).SubstateOf(stateC)
	ok, _ := sm.IsInState(stateC)

	assert.Equal(t, stateB, sm.MustState())
	assert.True(t, ok)
}

func TestStateMachine_Configure_InSubstate_TriggerIgnoredInSuperstate_RemainsInSubstate(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).SubstateOf(stateC)
	sm.Configure(stateC).Ignore(triggerX)
	sm.Fire(triggerX)

	assert.Equal(t, stateB, sm.MustState())
}

func TestStateMachine_Configure_PermittedTriggersIncludeSuperstatePermittedTriggers(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateA).Permit(triggerZ, stateB)
	sm.Configure(stateB).SubstateOf(stateC).Permit(triggerX, stateA)
	sm.Configure(stateC).Permit(triggerY, stateA)

	permitted, _ := sm.PermittedTriggers(context.Background())

	assert.Contains(t, permitted, triggerX)
	assert.Contains(t, permitted, triggerY)
	assert.NotContains(t, permitted, triggerZ)
}

func TestStateMachine_PermittedTriggers_PermittedTriggersAreDistinctValues(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).SubstateOf(stateC).Permit(triggerX, stateA)
	sm.Configure(stateC).Permit(triggerX, stateB)

	permitted, _ := sm.PermittedTriggers(context.Background())

	assert.Len(t, permitted, 1)
	assert.Equal(t, permitted[0], triggerX)
}

func TestStateMachine_PermittedTriggers_AcceptedTriggersRespectGuards(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).Permit(triggerX, stateA, func(_ context.Context, _ ...interface{}) bool {
		return false
	})

	permitted, _ := sm.PermittedTriggers(context.Background())

	assert.Len(t, permitted, 0)
}

func TestStateMachine_PermittedTriggers_AcceptedTriggersRespectMultipleGuards(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).Permit(triggerX, stateA, func(_ context.Context, _ ...interface{}) bool {
		return true
	}, func(_ context.Context, _ ...interface{}) bool {
		return false
	})

	permitted, _ := sm.PermittedTriggers(context.Background())

	assert.Len(t, permitted, 0)
}

func TestStateMachine_Fire_DiscriminatedByGuard_ChoosesPermitedTransition(t *testing.T) {
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).
		Permit(triggerX, stateA, func(_ context.Context, _ ...interface{}) bool {
			return false
		}).
		Permit(triggerX, stateC, func(_ context.Context, _ ...interface{}) bool {
			return true
		})

	sm.Fire(triggerX)

	assert.Equal(t, stateC, sm.MustState())
}

func TestStateMachine_Fire_TriggerIsIgnored_ActionsNotExecuted(t *testing.T) {
	fired := false
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...interface{}) error {
			fired = true
			return nil
		}).
		Ignore(triggerX)

	sm.Fire(triggerX)

	assert.False(t, fired)
}

func TestStateMachine_Fire_SelfTransitionPermited_ActionsFire(t *testing.T) {
	fired := false
	sm := NewStateMachine(stateB)
	sm.Configure(stateB).
		OnEntry(func(_ context.Context, _ ...interface{}) error {
			fired = true
			return nil
		}).
		PermitReentry(triggerX)

	sm.Fire(triggerX)

	assert.True(t, fired)
}

func TestStateMachine_Fire_ImplicitReentryIsDisallowed(t *testing.T) {
	sm := NewStateMachine(stateB)
	assert.Panics(t, func() {
		sm.Configure(stateB).
			Permit(triggerX, stateB)
	})
}

func TestStateMachine_Fire_ErrorForInvalidTransition(t *testing.T) {
	sm := NewStateMachine(stateA)
	assert.Error(t, sm.Fire(triggerX))
}

func TestStateMachine_SetTriggerParameters_TriggerParametersAreImmutableOnceSet(t *testing.T) {
	sm := NewStateMachine(stateB)

	sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0))

	assert.Panics(t, func() { sm.SetTriggerParameters(triggerX, reflect.TypeOf(""), reflect.TypeOf(0)) })
}
