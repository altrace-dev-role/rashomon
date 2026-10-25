//go:build !linux

package peakrss

// DisableTHP does nothing off Linux: no other platform this harness runs on
// backs a Go heap with huge pages (darwin/arm64 pages are 16 KiB).
func DisableTHP() error { return nil }
