// Package safe is the panic barrier.
//
// Exit code 2 from a PreToolUse hook blocks the tool call, and Claude Code
// documents that a JSON permissionDecision of allow cannot override it. In Go,
// exit code 2 is what an unrecovered panic does. So the difference between an
// observability tool and an enforcer is one missing recover, and the user would
// experience the bug as Claude Code refusing to run their command.
//
// A panic raised in a goroutine cannot be recovered by the goroutine that
// started it: the runtime terminates the process. Guard therefore protects only
// its own call stack, and Go is the only sanctioned way to start a goroutine.
//
// Two classes of failure are outside what recover() can reach, and no amount of
// guarding changes that: runtime fatal errors (concurrent map access, stack
// exhaustion, out of memory, all-goroutine deadlock) and os.Exit, which skips
// deferred functions entirely. They are handled by not writing code that can
// reach them -- no shared maps, bounded reads, and no os.Exit below main.
package safe

import "fmt"

// PanicError reports that a panic was recovered.
//
// It carries the panic value's dynamic type and deliberately nothing else. A
// panic value is frequently a string the program was holding at the time, which
// here means a command line or a prompt, and this error is exactly the value a
// caller is most tempted to write to a log. Naming only the type keeps the
// no-content guarantee true on the failure path, which is where it is hardest
// to hold and easiest to lose.
type PanicError struct {
	// TypeName is the result of %T on the recovered value.
	TypeName string
}

func (e *PanicError) Error() string {
	return "recovered panic of type " + e.TypeName
}

func newPanicError(v any) *PanicError {
	return &PanicError{TypeName: fmt.Sprintf("%T", v)}
}

// Guard runs fn and converts a panic on fn's own stack into an error.
//
// It does not catch a panic raised in a goroutine that fn started, even one
// started while Guard is on the stack. Use Go for those.
func Guard(fn func() error) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = newPanicError(v)
		}
	}()
	return fn()
}

// Go starts fn in a goroutine carrying its own deferred recover, and returns a
// channel closed once fn has returned or its panic has been recovered.
//
// onPanic runs inside the recovered region and is itself guarded, so a panic
// there cannot escape either. Pass nil to discard.
//
// The returned channel is what makes a goroutine panic observable: without
// waiting on it, a process can exit before the goroutine is scheduled, and a
// test asserting "this did not crash" would pass without ever running the code
// it claims to cover.
func Go(fn func(), onPanic func(error)) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		// Deferred first, so it runs last: done closes only after the
		// recover below has finished.
		defer close(done)
		defer func() {
			if v := recover(); v != nil && onPanic != nil {
				defer func() { _ = recover() }()
				onPanic(newPanicError(v))
			}
		}()
		fn()
	}()
	return done
}
