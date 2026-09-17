// Package fault injects faults that panic rather than return an error, so the
// panic barrier can be tested against the failures that actually produce exit
// code 2.
//
// The injection points are compiled in only under the `attestfault` build tag.
// A released binary contains no injection path at all, which matters for more
// than tidiness: an attacker who could set an environment variable and make the
// recorder abandon a run would be attacking exactly the coverage guarantee this
// program exists to provide.
package fault

// Points at which Inject is called. Named constants rather than string literals
// so that a test naming a point that no longer exists fails to compile instead
// of silently injecting nothing.
const (
	PointHookStart  = "hook.start"
	PointHookParsed = "hook.parsed"
	PointStoreWrite = "store.write"
)
