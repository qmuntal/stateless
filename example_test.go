package stateless_test

import (
	"context"
	"fmt"
	"reflect"

	"github.com/qmuntal/stateless"
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
	ctx := context.Background()
	phoneCall := stateless.NewStateMachine(stateOffHook)
	phoneCall.SetTriggerParameters(triggerSetVolume, reflect.TypeOf(0))
	phoneCall.SetTriggerParameters(triggerCallDialed, reflect.TypeOf(""))

	phoneCall.Configure(stateOffHook).
		Permit(triggerCallDialed, stateRinging)

	phoneCall.Configure(stateRinging).
		OnEntryFrom(triggerCallDialed, func(_ context.Context, args ...interface{}) error {
			onDialed(args[0].(string))
			return nil
		}).
		Permit(triggerCallConnected, stateConnected)

	phoneCall.Configure(stateConnected).
		OnEntry(func(_ context.Context, _ ...interface{}) error {
			startCallTimer()
			return nil
		}).
		OnExit(func(_ context.Context, _ ...interface{}) error {
			stopCallTimer()
			return nil
		}).
		InternalTransition(triggerMuteMicrophone, func(_ context.Context, _ ...interface{}) error {
			onMute()
			return nil
		}).
		InternalTransition(triggerUnmuteMicrophone, func(_ context.Context, _ ...interface{}) error {
			onUnmute()
			return nil
		}).
		InternalTransition(triggerSetVolume, func(_ context.Context, args ...interface{}) error {
			onSetVolume(args[0].(int))
			return nil
		}).
		Permit(triggerLeftMessage, stateOffHook).
		Permit(triggerPlacedOnHold, stateOnHold)

	phoneCall.Configure(stateOnHold).
		SubstateOf(stateConnected).
		Permit(triggerTakenOffHold, stateConnected).
		Permit(triggerPhoneHurledAgainstWall, statePhoneDestroyed)

	phoneCall.Fire(ctx, triggerCallDialed, "qmuntal")
	phoneCall.Fire(ctx, triggerCallConnected)
	phoneCall.Fire(ctx, triggerSetVolume, 2)
	phoneCall.Fire(ctx, triggerPlacedOnHold)
	phoneCall.Fire(ctx, triggerMuteMicrophone)
	phoneCall.Fire(ctx, triggerUnmuteMicrophone)
	phoneCall.Fire(ctx, triggerTakenOffHold)
	phoneCall.Fire(ctx, triggerSetVolume, 11)
	phoneCall.Fire(ctx, triggerPlacedOnHold)
	phoneCall.Fire(ctx, triggerPhoneHurledAgainstWall)
	fmt.Printf("State is %s\n", phoneCall.MustState(ctx))

	// Output:
	// [Phone Call] placed for : [qmuntal]
	// [Timer:] Call started at 11:00am
	// Volume set to 2!
	// Microphone muted!
	// Microphone unmuted!
	// Volume set to 11!
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

func startCallTimer() {
	fmt.Println("[Timer:] Call started at 11:00am")
}

func stopCallTimer() {
	fmt.Println("[Timer:] Call ended at 11:30am")
}
