package main

import (
	"io"
	"runtime"
	"testing"
	"time"
)

// gatedReader blocks every Read until release closes, then reports EOF: a
// Stop payload pipe that stays open past recap's budget and closes later.
type gatedReader struct{ release chan struct{} }

func (g gatedReader) Read([]byte) (int, error) {
	<-g.release
	return 0, io.EOF
}

// TestRubricCONC7_AbandonedStdinReadEndsWhenTheReadReturns pins the bound on
// the goroutine readRecapPayload leaves behind when stdin outlives its
// budget. The goroutine stays blocked in Read for as long as the pipe is open
// -- the process lifetime, at worst, which is what readRecapPayload's doc
// assumes -- and it ends the moment the Read returns, without anyone
// receiving its result. With an unbuffered result channel it would block
// forever on the send instead, one goroutine per call for the life of any
// longer-lived caller.
func TestRubricCONC7_AbandonedStdinReadEndsWhenTheReadReturns(t *testing.T) {
	before := runtime.NumGoroutine()
	in := gatedReader{release: make(chan struct{})}

	start := time.Now()
	got := readRecapPayload(in, 20*time.Millisecond)
	if got != (recapPayload{}) {
		t.Fatalf("a payload that never arrived read as %+v", got)
	}
	if waited := time.Since(start); waited > 2*time.Second {
		t.Fatalf("readRecapPayload waited %v on a pipe that never closed; its budget was 20ms", waited)
	}
	if n := runtime.NumGoroutine(); n <= before {
		t.Fatalf("goroutines: %d before, %d after the budget ran out; the reader should still be blocked in Read", before, n)
	}

	close(in.release)
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("goroutines: %d before, %d five seconds after its Read returned; the abandoned reader never ended",
				before, runtime.NumGoroutine())
		}
		time.Sleep(5 * time.Millisecond)
	}
}
