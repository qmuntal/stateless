package stateless

import (
	"context"
	"testing"
)

func TestDefaultUnhandledTriggerAction(t *testing.T) {
	type args struct {
		in0         context.Context
		state       State
		trigger     Trigger
		unmetGuards []string
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			DefaultUnhandledTriggerAction(tt.args.in0, tt.args.state, tt.args.trigger, tt.args.unmetGuards)
		})
	}
}
