package secretshape

import (
	"regexp"
	"sync"
)

// lazyRegexp compiles pattern on first use and never again. A package-scope
// regexp.MustCompile would run in every process that links this package,
// hooks included, whether or not it ever screens a value; this is the
// on-first-use form instead, so a caller that never asks pays for no compile.
type lazyRegexp struct {
	once    sync.Once
	re      *regexp.Regexp
	pattern string
}

func (l *lazyRegexp) get() *regexp.Regexp {
	l.once.Do(func() {
		l.re = regexp.MustCompile(l.pattern)
	})
	return l.re
}
