package stateless

import (
	"testing"
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
