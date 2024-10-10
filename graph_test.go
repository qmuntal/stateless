package stateless_test

import (
	"bytes"
	"context"
	"flag"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/qmuntal/stateless"
)

var update = flag.Bool("update", false, "update golden files on failure")

func emptyWithInitial() *stateless.StateMachine[string, string, stateless.Args] {
	return stateless.NewStateMachine[string, string, stateless.Args]("A")
}

func withSubstate() *stateless.StateMachine[string, string, stateless.Args] {
	sm := stateless.NewStateMachine[string, string, stateless.Args]("B")
	sm.Configure("A").Permit("Z", "B")
	sm.Configure("B").SubstateOf("C").Permit("X", "A")
	sm.Configure("C").Permit("Y", "A").Ignore("X")
	return sm
}

func withInitialState() *stateless.StateMachine[string, string, stateless.Args] {
	sm := stateless.NewStateMachine[string, string, stateless.Args]("A")
	sm.Configure("A").
		Permit("X", "B")
	sm.Configure("B").
		InitialTransition("C")
	sm.Configure("C").
		InitialTransition("D").
		SubstateOf("B")
	sm.Configure("D").
		SubstateOf("C")
	return sm
}

func withGuards() *stateless.StateMachine[string, string, stateless.Args] {
	sm := stateless.NewStateMachine[string, string, stateless.Args]("B")
	//sm.SetTriggerParameters("X", reflect.TypeOf(0))
	sm.Configure("A").
		Permit("X", "D", func(_ context.Context, args stateless.Args) bool {
			return args[0].(int) == 3
		})

	sm.Configure("B").
		SubstateOf("A").
		Permit("X", "C", func(_ context.Context, args stateless.Args) bool {
			return args[0].(int) == 2
		})
	return sm
}

func œ(_ context.Context, args stateless.Args) bool {
	return args[0].(int) == 2
}

func withUnicodeNames() *stateless.StateMachine[string, string, stateless.Args] {
	sm := stateless.NewStateMachine[string, string, stateless.Args]("Ĕ")
	sm.Configure("Ĕ").
		Permit("◵", "ų", œ)
	sm.Configure("ų").
		InitialTransition("ㇴ")
	sm.Configure("ㇴ").
		InitialTransition("ꬠ").
		SubstateOf("ų")
	sm.Configure("ꬠ").
		SubstateOf("𒀄")
	sm.Configure("1").
		SubstateOf("𒀄")
	sm.Configure("2").
		SubstateOf("1")
	return sm
}

func phoneCall() *stateless.StateMachine[string, string, stateless.Args] {
	phoneCall := stateless.NewStateMachine[string, string, stateless.Args](stateOffHook)
	//phoneCall.SetTriggerParameters(triggerSetVolume, reflect.TypeOf(0))
	//phoneCall.SetTriggerParameters(triggerCallDialed, reflect.TypeOf(""))

	phoneCall.Configure(stateOffHook).
		Permit(triggerCallDialed, stateRinging)

	phoneCall.Configure(stateRinging).
		OnEntryFrom(triggerCallDialed, func(_ context.Context, _ stateless.Args) error {
			return nil
		}).
		Permit(triggerCallConnected, stateConnected)

	phoneCall.Configure(stateConnected).
		OnEntry(startCallTimer).
		OnExit(func(_ context.Context, _ stateless.Args) error {
			return nil
		}).
		InternalTransition(triggerMuteMicrophone, func(_ context.Context, _ stateless.Args) error {
			return nil
		}).
		InternalTransition(triggerUnmuteMicrophone, func(_ context.Context, _ stateless.Args) error {
			return nil
		}).
		InternalTransition(triggerSetVolume, func(_ context.Context, args stateless.Args) error {
			return nil
		}).
		Permit(triggerLeftMessage, stateOffHook).
		Permit(triggerPlacedOnHold, stateOnHold)

	phoneCall.Configure(stateOnHold).
		SubstateOf(stateConnected).
		OnExitWith(triggerPhoneHurledAgainstWall, func(ctx context.Context, _ stateless.Args) error {
			onWasted()
			return nil
		}).
		Permit(triggerTakenOffHold, stateConnected).
		Permit(triggerPhoneHurledAgainstWall, statePhoneDestroyed)

	return phoneCall
}

func TestStateMachine_ToGraph(t *testing.T) {
	tests := []func() *stateless.StateMachine[string, string, stateless.Args]{
		emptyWithInitial,
		withSubstate,
		withInitialState,
		withGuards,
		withUnicodeNames,
		phoneCall,
	}
	for _, fn := range tests {
		name := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
		sp := strings.Split(name, ".")
		name = sp[len(sp)-1]
		t.Run(name, func(t *testing.T) {
			got := fn().ToGraph()
			name := "testdata/golden/" + name + ".dot"
			want, err := os.ReadFile(name)
			want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
			if *update {
				if !bytes.Equal([]byte(got), want) {
					os.WriteFile(name, []byte(got), 0666)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal([]byte(got), want) {
					t.Fatalf("got:\n%swant:\n%s", got, want)
				}
			}
		})
	}
}

func BenchmarkToGraph(b *testing.B) {
	sm := phoneCall()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sm.ToGraph()
	}
}
