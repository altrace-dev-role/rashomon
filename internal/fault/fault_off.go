//go:build !attestfault

package fault

// Inject is a no-op in a released binary and compiles away entirely.
func Inject(string) {}

// Enabled reports whether fault injection is compiled in.
func Enabled() bool { return false }
