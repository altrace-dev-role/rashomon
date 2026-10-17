package acceptance

import (
	"slices"
	"testing"
)

const module = "github.com/altrace-dev-role/rashomon"

// readStep are the packages that open what a report reads outside the store's
// records -- the session transcripts, the sandbox trail's reconciliation and
// the project baseline -- with what each would let a verdict package do.
var readStep = map[string]string{
	module + "/internal/report/gather": "is the report's read step: it opens the session's transcripts",
	module + "/internal/baseline":      "reads and writes the project baseline",
	"database/sql":                     "reaches the proxy's database",
	"os/exec":                          "runs a process",
}

// TestRubricHLD4_TheVerdictPackagesLinkNoReadStep is the H-17-style check
// over the report's split. report is the verdict library: given what a
// session recorded and what the read step read, it decides. gather is the
// read step, and the only package that opens a transcript for it. The turn
// digest promises it never opens the session's transcript, and before the
// split that held by convention: digest linked report, and report opened
// transcripts. Now no package digest links can.
func TestRubricHLD4_TheVerdictPackagesLinkNoReadStep(t *testing.T) {
	// The premise, twice: the read step exists, and it is what joins the
	// verdict library to the files, so its absence below means something.
	gather := deps(t, "./internal/report/gather")
	for _, want := range []string{module + "/internal/report", module + "/internal/baseline", module + "/internal/store"} {
		if !slices.Contains(gather, want) {
			t.Fatalf("premise: the read step does not link %s; it is not the report's read step", want)
		}
	}
	for _, root := range []string{"./internal/report", "./internal/digest", "./internal/shape", "./internal/host"} {
		pkgs := deps(t, root)
		if len(pkgs) < 5 {
			t.Fatalf("premise: %s lists only %d dependencies", root, len(pkgs))
		}
		for _, pkg := range pkgs {
			if why, bad := readStep[pkg]; bad {
				t.Errorf("%s links %s, which %s", root, pkg, why)
			}
		}
	}
}
