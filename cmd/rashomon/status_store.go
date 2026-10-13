package main

import (
	"fmt"
	"io"

	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// statusSize prints the store's size against the cap in force, or says that
// eviction is off. The size is the one eviction compares against the cap,
// so a store close to its cap is visible before a run is evicted rather
// than after (perf-7).
func statusSize(stdout io.Writer, st *store.Store) {
	c := store.ResolveCap()
	size := statusUnknown
	if n, err := st.RunsBytes(); err == nil {
		size = store.FormatBytes(n)
	}
	if c.Off() {
		fmt.Fprintf(stdout, "  size: %s (eviction off: %s=0)\n", size, store.EnvCapBytes)
		return
	}
	fmt.Fprintf(stdout, "  size: %s of a %s cap\n", size, store.FormatBytes(c.Bytes))
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

// capProblem prints, after prefix, why the cap variable's value was not
// taken, and nothing when it was. The hooks that apply the cap cannot print,
// so status and watch say it for them (hld-7).
func capProblem(stdout io.Writer, prefix string) {
	if p := store.ResolveCap().Problem(); p != "" {
		fmt.Fprintf(stdout, "%s%s\n", prefix, p)
	}
}
