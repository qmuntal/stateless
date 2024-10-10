package stateless_test

import (
	"context"
	"fmt"
	"github.com/qmuntal/stateless"
	"reflect"
)

const (
	triggerCallDialed             = "CallDialed"
	triggerCallConnected          = "CallConnected"
	triggerLeftMessage            = "LeftMessage"
	triggerPlacedOnHold           = "PlacedOnHold"
	triggerTakenOffHold           = "TakenOffHold"
	triggerPhoneHurledAgainstWall = "PhoneHurledAgainstWall"
	triggerMuteMicrophone         = "MuteMicrophone"
	triggerUnmuteMicrophone       = "UnmuteMicrophone"
	triggerSetVolume              = "SetVolume"
)

const (
	stateOffHook        = "OffHook"
	stateRinging        = "Ringing"
	stateConnected      = "Connected"
	stateOnHold         = "OnHold"
	statePhoneDestroyed = "PhoneDestroyed"
)

func Example() {
	phoneCall := stateless.NewStateMachine[string, string, stateless.Args](stateOffHook)
	phoneCall.SetTriggerParameters(triggerSetVolume, reflect.TypeOf(0))
	phoneCall.SetTriggerParameters(triggerCallDialed, reflect.TypeOf(""))

	phoneCall.Configure(stateOffHook).
		Permit(triggerCallDialed, stateRinging)

	phoneCall.Configure(stateRinging).
		OnEntryFrom(triggerCallDialed, func(_ context.Context, args stateless.Args) error {
			onDialed(args[0].(string))
			return nil
		}).
		Permit(triggerCallConnected, stateConnected)

	phoneCall.Configure(stateConnected).
		OnEntry(startCallTimer).
		OnExit(func(_ context.Context, args stateless.Args) error {
			stopCallTimer()
			return nil
		}).
		InternalTransition(triggerMuteMicrophone, func(_ context.Context, _ stateless.Args) error {
			onMute()
			return nil
		}).
		InternalTransition(triggerUnmuteMicrophone, func(_ context.Context, _ stateless.Args) error {
			onUnmute()
			return nil
		}).
		InternalTransition(triggerSetVolume, func(_ context.Context, args stateless.Args) error {
			onSetVolume(args[0].(int))
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

	phoneCall.ToGraph()

	phoneCall.Fire(triggerCallDialed, stateless.Args{"qmuntal"})
	phoneCall.Fire(triggerCallConnected, nil)
	phoneCall.Fire(triggerSetVolume, stateless.Args{2})
	phoneCall.Fire(triggerPlacedOnHold, nil)
	phoneCall.Fire(triggerMuteMicrophone, nil)
	phoneCall.Fire(triggerUnmuteMicrophone, nil)
	phoneCall.Fire(triggerTakenOffHold, nil)
	phoneCall.Fire(triggerSetVolume, stateless.Args{11})
	phoneCall.Fire(triggerPlacedOnHold, nil)
	phoneCall.Fire(triggerPhoneHurledAgainstWall, nil)
	fmt.Printf("State is %v\n", phoneCall.MustState())

	// Output:
	// [Phone Call] placed for : [qmuntal]
	// [Timer:] Call started at 11:00am
	// Volume set to 2!
	// Microphone muted!
	// Microphone unmuted!
	// Volume set to 11!
	// Wasted!
	// [Timer:] Call ended at 11:30am
	// State is PhoneDestroyed

}

func onSetVolume(volume int) {
	fmt.Printf("Volume set to %d!\n", volume)
}

func onUnmute() {
	fmt.Println("Microphone unmuted!")
}

func onMute() {
	fmt.Println("Microphone muted!")
}

func onDialed(callee string) {
	fmt.Printf("[Phone Call] placed for : [%s]\n", callee)
}

func onWasted() {
	fmt.Println("Wasted!")
}

func startCallTimer(_ context.Context, _ stateless.Args) error {
	fmt.Println("[Timer:] Call started at 11:00am")
	return nil
}

func stopCallTimer() {
	fmt.Println("[Timer:] Call ended at 11:30am")
}
