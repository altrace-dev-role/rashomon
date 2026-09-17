// Package fault injects faults so the panic barrier and the atomic-write
// guarantee can be tested against the failures that actually break them.
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
	PointHookStart            = "hook.start"
	PointHookParsed           = "hook.parsed"
	PointStoreWrite           = "store.write"
	PointHookAfterDeclaration = "hook.after_declaration"
	PointSettingsOpened       = "settings.write.opened"
	PointSettingsBeforeRename = "settings.write.before_rename"
)
