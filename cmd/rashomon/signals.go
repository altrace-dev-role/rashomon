package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// signalGate is how watch and consent answer SIGINT and SIGHUP (Technical
// spec 7.2.3 steps 5 and 6). Until hold, a signal ends the command at once
// with exit 1: nothing has been written, so there is nothing to finish. From
// hold on, a signal is kept rather than acted on, so the writes either all
// land or are all undone; the command reports it after they have.
//
// The process exits while the gate's lock is held, so a hold racing the
// signal either happens before it, and the signal is kept, or never returns.
type signalGate struct {
	mu      sync.Mutex
	holding bool
	held    os.Signal

	ch     chan os.Signal
	done   chan struct{}
	stderr io.Writer
}

func startSignalGate(stderr io.Writer) *signalGate {
	g := &signalGate{ch: make(chan os.Signal, 2), done: make(chan struct{}), stderr: stderr}
	signal.Notify(g.ch, os.Interrupt, syscall.SIGHUP)
	go g.loop()
	return g
}

func (g *signalGate) loop() {
	for {
		select {
		case s := <-g.ch:
			g.mu.Lock()
			if !g.holding {
				fmt.Fprintf(g.stderr, "\nrashomon: %s; nothing was written\n", s)
				os.Exit(exitFail)
			}
			if g.held == nil {
				g.held = s
			}
			g.mu.Unlock()
		case <-g.done:
			return
		}
	}
}

// hold starts keeping signals instead of acting on them. It is called just
// before the first write.
func (g *signalGate) hold() {
	g.mu.Lock()
	g.holding = true
	g.mu.Unlock()
}

// heldSignal reports the first signal kept since hold, or nil.
func (g *signalGate) heldSignal() os.Signal {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.held
}

// stop stops listening; a signal after it takes its default action.
func (g *signalGate) stop() {
	signal.Stop(g.ch)
	close(g.done)
}
