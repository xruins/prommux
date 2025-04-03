package handler

import (
	"testing"
)

func TestNotifiableAtomicBool(t *testing.T) {
	b := notifiableAtomicBool{}

	ch := make(chan bool, 1)
	b.subscribe(ch)

	// test Store
	b.Store(true)
	want := true

	// test Store: confirm the value received by channel
	got := <-ch
	if got != want {
		t.Errorf("got an unexpected value for the first receive. got: %t, want: %t", got, want)
	}
	// test Store: confirm the value of Load
	got = b.Load()
	if got != want {
		t.Errorf("got an unexpected value for the first receive. got: %t, want: %t", got, want)
	}

	// test Swap: confirm the value received by channel
	_ = b.Swap(false)
	got = <-ch
	want = false
	if got != want {
		t.Errorf("got an unexpected value for the second receive. got: %t, want: %t", got, want)
	}

	// test Swap: confirm the value of Load
	got = b.Load()
	if got != want {
		t.Errorf("got an unexpected value for the first receive. got: %t, want: %t", got, want)
	}
}
