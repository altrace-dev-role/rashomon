//go:build !unix

package hook

// IgnoreBrokenPipe does nothing where there is no SIGPIPE to ignore. On
// Windows a write to a pipe nobody reads already fails with an error, and the
// runtime raises nothing for it.
func IgnoreBrokenPipe() {}
