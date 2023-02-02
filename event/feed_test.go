package event

import (
	"fmt"
	"reflect"
	"testing"
)

func TestFeedPanics(t *testing.T) {
	{
		var f Feed
		f.Send(2)
		want := feedTypeError{op: "Send", got: reflect.TypeOf(uint64(0)), want: reflect.TypeOf(0)}
		if err := checkPanic(want, func() { f.Send(uint64(2)) }); err != nil {
			t.Error(err)
		}
	}
	{
		var f Feed
		ch := make(chan int)
		f.Subscribe(ch)
		want := feedTypeError{op: "Send", got: reflect.TypeOf(uint64(0)), want: reflect.TypeOf(0)}
		if err := checkPanic(want, func() { f.Send(uint64(2)) }); err != nil {
			t.Error(err)
		}
	}
}

func checkPanic(want error, fn func()) (err error) {
	defer func() {
		panic := recover()
		if panic == nil {
			err = fmt.Errorf("didn't panic")
		} else if !reflect.DeepEqual(panic, want) {
			err = fmt.Errorf("panicked with wrong error: got %q, want %q", panic, want)
		}
	}()
	fn()
	return nil
}