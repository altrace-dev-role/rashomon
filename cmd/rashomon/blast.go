package main

import (
	"time"

	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/platform"
)

// blastGuard is the blast-radius read's guard at a PostToolUse (card
// ARCH-INLINE1), well inside the post entry's 5 s timeout and half the
// Stop's (stopPlatformGuard): a read that stalls is an error, which the hook
// reads as nothing to say. This repository's 3 MB log decodes in 11 to 17
// ms (internal/platform's deadline note).
const blastGuard = time.Second

// blastPort is the post's blast-radius port: internal/platform's Locate and
// Callers, which the hook package does not link, as the Stop's platform read
// is cmd's (platformReadWithin).
func blastPort() hook.BlastPort {
	return hook.BlastPort{
		Locate: func(root, file string) (hook.BlastSite, bool) {
			s, ok := platform.Locate(root, file)
			return hook.BlastSite(s), ok
		},
		Callers: func(site hook.BlastSite) ([]hook.BlastCaller, error) {
			cs, err := platform.Callers(platform.Site(site), blastGuard)
			out := make([]hook.BlastCaller, len(cs))
			for i, c := range cs {
				out[i] = hook.BlastCaller(c)
			}
			return out, err
		},
	}
}
