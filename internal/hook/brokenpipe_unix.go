//go:build unix

package hook

import (
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
// Ignored, the write returns EPIPE, which every hook-path write already
// discards.
//
// Ignore and not Notify: nothing has to keep a channel alive until the last
// write, and no Stop elsewhere can hand the default back early. What Ignore
// costs is that SIG_IGN survives exec, which is why this is called by the hook
// entry points alone, none of which starts a process. `run` does, and its
// child must get the disposition the user's own shell would give it; a person
// piping `rashomon report` into head should see it stop the way any other
// command does, not as an error on stderr.
func IgnoreBrokenPipe() { signal.Ignore(syscall.SIGPIPE) }
