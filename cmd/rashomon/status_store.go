package main

import (
	"fmt"
	"io"

	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// capForStatus is the cap a hook last recorded, which is what eviction
// actually runs with, or this process's own ResolveCap() when no hook has
// recorded one yet -- a store just created, one no session has touched, or
// none at all (st nil, cmdStatus's "present: no" branch). label says which
// one it is, "" for the recorded value and a note for the fallback, because
// the two can disagree: eviction runs inside Claude Code's hook environment,
// not whatever shell happens to run status (hld-7).
func capForStatus(st *store.Store) (bytes int64, problem, label string) {
	if st != nil {
		if rc, ok, err := st.ResolvedCap(); err == nil && ok {
			return rc.Bytes, rc.Problem, ""
		}
	}
	c := store.ResolveCap()
	return c.Bytes, c.Problem(), " (this shell's value -- no hook has recorded one yet)"
}

// statusSize prints the store's size against the cap in force, or says that
// eviction is off. The size is the one eviction compares against the cap,
// so a store close to its cap is visible before a run is evicted rather
// than after (perf-7).
func statusSize(stdout io.Writer, st *store.Store) {
	bytes, _, label := capForStatus(st)
	size := statusUnknown
	if n, err := st.RunsBytes(); err == nil {
		size = store.FormatBytes(n)
	}
	if bytes == 0 {
		fmt.Fprintf(stdout, "  size: %s (eviction off: %s=0%s)\n", size, store.EnvCapBytes, label)
		return
	}
	fmt.Fprintf(stdout, "  size: %s of a %s cap%s\n", size, store.FormatBytes(bytes), label)
}

// statusLockWait prints the longest any one hook invocation has waited for
// the store's locks, and the run it was in (perf-7). Contention used to be
// invisible until a wait ran out and its record spilled with lock_timeout;
// a wait creeping toward the limit is the warning that comes first.
func statusLockWait(stdout io.Writer, st *store.Store) {
	longest, run, err := st.LongestLockWait()
	switch {
	case err != nil:
		fmt.Fprintf(stdout, "  lock wait: %s\n", statusUnknown)
	case run == "":
		fmt.Fprintln(stdout, "  lock wait: none recorded")
	default:
		fmt.Fprintf(stdout, "  lock wait: longest %d ms, in run %s (a hook may wait %s in all)\n",
			longest.Milliseconds(), run, hook.LockWaitTotal)
	}
}

// capProblem prints, after prefix, why watch's OWN invocation's cap variable
// was not taken, and nothing when it was. This is watch diagnosing itself,
// right now, in its own process -- not a report on what eviction is running
// with, which is statusCapProblem's job.
func capProblem(stdout io.Writer, prefix string) {
	if p := store.ResolveCap().Problem(); p != "" {
		fmt.Fprintf(stdout, "%s%s\n", prefix, p)
	}
}

// statusCapProblem prints, after prefix, why the cap a hook resolved was not
// taken, preferring that recorded value over this process's own shell for
// the same reason statusSize does (hld-7): eviction runs inside Claude
// Code's hook environment, not whatever shell happens to run status.
func statusCapProblem(stdout io.Writer, prefix string, st *store.Store) {
	if _, problem, label := capForStatus(st); problem != "" {
		fmt.Fprintf(stdout, "%s%s%s\n", prefix, problem, label)
	}
}
