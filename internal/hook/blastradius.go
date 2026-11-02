package hook

import "github.com/altrace-dev-role/rashomon/internal/steer"

// BlastSite is where a Go edit lands for the blast-radius read (card
// ARCH-INLINE1, platform.Site).
type BlastSite struct {
	Root, Package, Log string
}

// BlastCaller is one other package with a PROVEN call into the site's
// package (platform.Caller).
type BlastCaller struct {
	Package, Dir string
	Tests        bool
}

// BlastPort is the read's port, cmd's internal/platform.
type BlastPort struct {
	Locate  func(root, file string) (BlastSite, bool)
	Callers func(site BlastSite) ([]BlastCaller, error)
}

// blastDraft is a call's blast-radius finding and the read marker it took.
type blastDraft struct {
	f      steer.Finding
	marker string
}

// blast is the call's blast-radius finding. Red stub: none.
func (p *Post) blast(env SteerEnv, ctx *steer.Context, ok bool) (blastDraft, bool) {
	return blastDraft{}, false
}

// settleBlast settles the read marker. Red stub: nothing.
func (p *Post) settleBlast(d blastDraft, ctx *steer.Context, out steer.Output) {}
