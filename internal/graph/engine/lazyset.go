package engine

import "sync"

// lazySet is a word set built on first use. RES-2 holds the binary's init
// to a budget, and this package is linked into every hook entry while only
// the prompt path builds a brief, so a package-scope map literal would be
// paid for by every PostToolUse and Stop process; this is the on-first-use
// form slot's lazyRegexp takes for the same rule. A package-scope
// &lazySet{f: ...} is laid out statically and allocates nothing at init.
type lazySet[V any] struct {
	once sync.Once
	m    map[string]V
	f    func() map[string]V
}

// get builds the set on the first call and returns it on every call.
func (l *lazySet[V]) get() map[string]V {
	l.once.Do(func() { l.m = l.f() })
	return l.m
}
