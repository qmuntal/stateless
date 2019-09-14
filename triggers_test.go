package stateless

import (
	"context"
	"reflect"
	"testing"
)

func Test_invocationInfo_String(t *testing.T) {
	tests := []struct {
		name string
		inv  invocationInfo
		want string
	}{
		{"empty", invocationInfo{}, "<nil>"},
		{"named", invocationInfo{Method: "aaa"}, "aaa"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.inv.String(); got != tt.want {
				t.Errorf("invocationInfo.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ignoredTriggerBehaviour_ResultsInTransitionFrom(t *testing.T) {
	tests := []struct {
		name   string
		t      *ignoredTriggerBehaviour
		wantSt State
		wantOk bool
	}{
		{"base", new(ignoredTriggerBehaviour), nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSt, gotOk := tt.t.ResultsInTransitionFrom(context.Background(), stateA)
			if !reflect.DeepEqual(gotSt, tt.wantSt) {
				t.Errorf("ignoredTriggerBehaviour.ResultsInTransitionFrom() gotSt = %v, want %v", gotSt, tt.wantSt)
			}
			if gotOk != tt.wantOk {
				t.Errorf("ignoredTriggerBehaviour.ResultsInTransitionFrom() gotOk = %v, want %v", gotOk, tt.wantOk)
			}
		})
	}
}

func Test_reentryTriggerBehaviour_ResultsInTransitionFrom(t *testing.T) {
	tests := []struct {
		name  string
		t     *reentryTriggerBehaviour
		want  State
		want1 bool
	}{
		{"base", &reentryTriggerBehaviour{Destination: stateA}, stateA, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := tt.t.ResultsInTransitionFrom(context.Background(), stateA)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reentryTriggerBehaviour.ResultsInTransitionFrom() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("reentryTriggerBehaviour.ResultsInTransitionFrom() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_internalTriggerBehaviour_ResultsInTransitionFrom(t *testing.T) {
	type args struct {
		source State
	}
	tests := []struct {
		name  string
		t     *internalTriggerBehaviour
		args  args
		want  State
		want1 bool
	}{
		{"base", new(internalTriggerBehaviour), args{stateA}, stateA, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := tt.t.ResultsInTransitionFrom(context.Background(), tt.args.source)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("internalTriggerBehaviour.ResultsInTransitionFrom() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("internalTriggerBehaviour.ResultsInTransitionFrom() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
