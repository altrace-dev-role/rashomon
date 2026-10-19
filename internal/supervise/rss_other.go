//go:build unix && !linux

package supervise

// selfMaxRSS is this process's own peak resident set, in bytes. Outside
// Linux, getrusage counts this image alone.
func selfMaxRSS() int64 { return rusageMaxRSS() }
