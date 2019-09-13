package stateless

import (
	"context"
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
