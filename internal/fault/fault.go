// Package fault injects faults so the panic barrier and the atomic-write
// guarantee can be tested against the failures that actually break them.
//
// The injection points are compiled in only under the `attestfault` build tag.
// A released binary contains no injection path at all, which matters for more
// than tidiness: an attacker who could set an environment variable and make the
// recorder abandon a run would be attacking exactly the coverage guarantee this
// program exists to provide.
//
// Every kind panics, because only a panic reaches the exit code that blocks a
// tool call, with one exception: "fail=N" returns an error, and it is reached
// through Fail rather than Inject. What it exercises is the retry around a
// transient settings read, and a read that panicked would exercise the panic
// barrier instead -- the retry would never run.
package fault

// Points at which Inject is called. Named constants rather than string literals
// so that a test naming a point that no longer exists fails to compile instead
// of silently injecting nothing.
const (
	PointHookStart            = "hook.start"
	PointHookParsed           = "hook.parsed"
	PointStoreWrite           = "store.write"
	PointHookAfterDeclaration = "hook.after_declaration"
	PointPostStart            = "post.start"
	PointPostParsed           = "post.parsed"
	PointSettingsLoad         = "settings.load"
	PointSettingsOpened       = "settings.write.opened"
	PointSettingsBeforeRename = "settings.write.before_rename"
)
