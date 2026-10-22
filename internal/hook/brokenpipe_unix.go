//go:build unix

package hook

import (
	"os"
	"os/signal"
	"syscall"
)

// IgnoreBrokenPipe makes a write to a stream nobody is reading fail with an
// error instead of killing the process.
//
// Claude Code reads a hook's stdout and stderr through pipes and can stop
// reading before the hook writes: the turn is interrupted, the entry's timeout
// fires, Claude Code exits. Go's runtime answers a failed write to fd 1 or
// fd 2 by dying of SIGPIPE unless the program handles or ignores that signal
// (os/signal's package doc), so recap's one line, or a stand-down line on
// stderr, ended the process with status 141 -- a non-zero exit from a hook
// path, and one no recover can catch, because a signal is not a panic (H-111).
// Handled, the write returns EPIPE, which every hook-path write already
// discards.
//
// Notify and not Ignore: SIG_IGN survives exec, and a shell started under it
// cannot even trap SIGPIPE back, so every process a hook starts -- git, the
// learner, claude -p for the memory build -- would run with writes to a
// closed pipe failing instead of ending it. A handler is reset to the default
// at exec. The channel is never read and never stopped: notifying it is what
// marks the signal handled, and brokenPipe holds it for the life of the
// process. `run` still does not call this, so a person piping `rashomon
// report` into head sees it stop the way any other command does.
func IgnoreBrokenPipe() { signal.Notify(brokenPipe, syscall.SIGPIPE) }

// brokenPipe is the channel IgnoreBrokenPipe notifies; see there.
var brokenPipe = make(chan os.Signal, 1)
