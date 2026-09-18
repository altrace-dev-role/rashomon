//go:build !rashomonfault

package fault

// Inject is a no-op in a released binary and compiles away entirely.
func Inject(string) {}

// Fail is a no-op in a released binary: no point ever fails.
func Fail(string) error { return nil }

// Enabled reports whether fault injection is compiled in.
func Enabled() bool { return false }
