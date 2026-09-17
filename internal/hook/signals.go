package hook

import (
	"os"
	"os/signal"
	"syscall"
)

// Signals watches for the termination signals a hook can receive. SIGTERM is
// what a hook timeout cancellation delivers, and it is catchable; SIGKILL is
// not, and nothing here or anywhere else runs when it arrives.
type Signals struct {
	ch   chan os.Signal
	seen bool
}

// WatchSignals starts watching. It starts no goroutine of ours: the runtime
// delivers straight into the channel.
func WatchSignals() *Signals {
	s := &Signals{ch: make(chan os.Signal, 1)}
	signal.Notify(s.ch, os.Interrupt, syscall.SIGTERM)
	return s
}

// Stop restores default handling.
func (s *Signals) Stop() { signal.Stop(s.ch) }

// Delivered reports whether a termination signal has arrived, and stays true
// once it has.
//
// This reads the channel the runtime delivers into, so the answer is current at
// the instant of the call. A context cancelled by signal.NotifyContext is not:
// its cancel runs on a goroutine of its own, and a caller that checks ctx.Err()
// right after the signal lands can find it still nil because that goroutine has
// not been scheduled yet. The SIGTERM acceptance test found exactly that race.
func (s *Signals) Delivered() bool {
	if s.seen {
		return true
	}
	select {
	case <-s.ch:
		s.seen = true
	default:
	}
	return s.seen
}
