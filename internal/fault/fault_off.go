//go:build !rashomonfault

package fault

// Inject is a no-op in a released binary and compiles away entirely.
func Inject(string) {}

// Fail is a no-op in a released binary: no point ever fails.
func Fail(string) error { return nil }

// Enabled reports whether fault injection is compiled in.
func Enabled() bool { return false }

// RecordSync, LockTaken and LockReleased are no-ops in a released binary:
// the sync recorder exists only under the rashomonfault tag.
func RecordSync(kind, path string) {}

func LockTaken() {}

func LockReleased() {}
