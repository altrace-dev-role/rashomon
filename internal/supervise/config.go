package supervise

import (
	"io"
	"time"
)

// InternalErrorLine is the one line the supervisor writes to stderr when an
// entry's child failed or was killed. Hook stderr reaches Claude Code's debug
// log, so it is fixed text: nothing from the payload, nothing the child
// said.
const InternalErrorLine = "rashomon: internal error in a hook entry; nothing was emitted"

// RecordLostLine follows InternalErrorLine when the failure could not be
// recorded either, so the absence of the record is not mistaken for the
// absence of the failure.
const RecordLostLine = "rashomon: the failed hook entry could not be recorded"

// Caps on what the parent holds of the child's output.
const (
	// StdoutCap is the most a child may print and still have it forwarded.
	StdoutCap = 64 << 10
	// stderrCap is the most of a child's stderr the parent reads for the
	// fixed lines it relays; the rest is read and dropped.
	stderrCap = 4 << 10
	// pumpBuffer is the parent's one buffer for the payload it streams.
	pumpBuffer = 64 << 10
)

// Config is one supervised invocation.
type Config struct {
	// Args are the entry's arguments, as the parent was invoked with them;
	// the child is started with Token in front of them.
	Args []string
	// Events are the events the entry serves (install.EntryEvents).
	Events []string
	// Start is the parent's process start, and HardStop the time after it
	// at which the child's process group is killed. The child is told both
	// and keeps the same stop.
	Start    time.Time
	HardStop time.Duration
	// StdinCap is the most of the payload the child is given; past it the
	// child's stdin ends and the rest is read and dropped.
	StdinCap int64

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Executable is the binary the child runs: os.Executable.
	Executable func() (string, error)
	// BeforeSpawn runs just before the child is started; an error from it
	// is a spawn that failed.
	BeforeSpawn func() error
	// Relayed reports whether a line the child wrote to stderr is one of the
	// fixed lines passed on when the child exits 0. Everything else the
	// child writes there is dropped: a panic or fatal trace can carry
	// payload text.
	Relayed func(line string) bool
	// Record records a failed entry and reports whether it could.
	Record func(Failure) bool
	// BeforeStdout runs just before forwarded output is written.
	BeforeStdout func()
	// Observe, when set, is given the invocation's figures at the end.
	Observe func(Stats)
	// Direct runs the entry in this process, unsupervised. It is used only
	// where supervision is not supported (Supported).
	Direct func() int
}

// Failure is what the supervisor knows of a child that failed, was killed,
// could not be started, or exited 0 with output the gate refused.
type Failure struct {
	// Reason is the coverage reason due, store.ReasonHookKilled or
	// store.ReasonSupervisorSpawnFailed, and empty when no coverage
	// record is due: output refused from a child that exited 0.
	Reason string
	// Outcome is the supervisor.ndjson outcome (store.Outcome*).
	Outcome string
	// SessionID and Event are read from the payload's first 4 KiB, empty
	// when they were not there.
	SessionID string
	Event     string
	// ChildExit is the child's exit status, -1 when it has none; Signal
	// names the signal that ended it, empty when none did.
	ChildExit int
	Signal    string
}

// Stats are one supervised invocation's figures, for instrumentation.
type Stats struct {
	ChildPID int
	// ParentMaxRSS is the supervisor's own peak resident set, in bytes.
	// ChildMaxRSS is wait4's figure for the child, 0 when it was never
	// reaped; on Linux it is at least the supervisor's size when it forked,
	// because Linux counts the image the child's exec replaced.
	ParentMaxRSS int64
	ChildMaxRSS  int64
	// Outcome is empty for an invocation that forwarded or printed nothing
	// because there was nothing to print.
	Outcome string
}
