package recap

import (
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// maxPlatformLines is how many packages the Stop names; the rest are one
// line that counts them.
const maxPlatformLines = 2

// platformLines is a line for each changed package the Stop's platform read
// owes a go test (PW1b-ON-230 step 2, WED-SLICE row 2): open, no go test
// after the edit, or unmet, a red one that named the package alone. A met
// or unknown row prints nothing: unknown is the ledger's to explain. At most
// maxPlatformLines, then one line counting the rest. The rows pair with
// v.Platform.Families in order (StopView.Platform); the unreadable log's
// row and the cap's row have no family and print nothing. It is the
// systemMessage, the person's channel: no line here reaches the model; the
// next prompt's note gives it the same packages (OwedPackages).
func platformLines(v verify.StopView, fromPlugin bool) string {
	var lines []string
	for _, o := range owedFamilies(v) {
		what := "not run since the edit"
		if o.failed {
			what = "failed after the edit"
		}
		lines = append(lines, platformLine(o.f, what))
	}
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	for _, l := range lines[:min(len(lines), maxPlatformLines)] {
		b.WriteString(l)
	}
	if n := len(lines) - maxPlatformLines; n > 0 {
		where := ": rashomon report --ledger"
		if fromPlugin {
			where = " packages"
		}
		b.WriteString(continuation() + "and " + strconv.Itoa(n) + " more" + where + "\n")
	}
	return b.String()
}

// PlatformLines is platformLines for a caller outside this package:
// recapTurn hands them to LineWith for recap.Line's sentences to carry.
func PlatformLines(v verify.StopView, fromPlugin bool) string { return platformLines(v, fromPlugin) }

// owedFamily is a changed package the Stop's platform read owes a go test:
// open (failed false) or unmet (failed true).
type owedFamily struct {
	f      verify.PlatformFamily
	failed bool
}

// owedFamilies is the families whose platform_impact row is open or unmet,
// the rows paired with v.Platform.Families in order (StopView.Platform).
func owedFamilies(v verify.StopView) []owedFamily {
	var out []owedFamily
	i := 0
	for _, o := range v.Obligations {
		if o.Kind != verify.PlatformImpact {
			continue
		}
		if i >= len(v.Platform.Families) {
			break
		}
		f := v.Platform.Families[i]
		i++
		if o.State == verify.ObligationOpen || o.State == verify.ObligationUnmet {
			out = append(out, owedFamily{f: f, failed: o.State == verify.ObligationUnmet})
		}
	}
	return out
}

// OwedPackages is the project-relative directories of the changed packages
// the Stop's platform read owes a go test, the ones platformLines names to
// the person: what the turn end leaves the next prompt's note to give the
// model (PW1b-ON-230 step 3, ARCH-INLINE1 item 3; Owed.Packages).
func OwedPackages(v verify.StopView) []string {
	var out []string
	for _, o := range owedFamilies(v) {
		out = append(out, o.f.Dir)
	}
	return out
}

// platformLine is one package's line: "platform: go test ./<dir>/... <what>",
// with the module's directory where the package's go.mod is not the
// project's. Past lineColumns the command goes on a line of its own, and
// past that it prints whole (D86).
func platformLine(f verify.PlatformFamily, what string) string {
	rel := f.Dir
	in := ""
	if f.ModuleDir != "." {
		rel = strings.TrimPrefix(strings.TrimPrefix(f.Dir, f.ModuleDir), "/")
		in = " in " + f.ModuleDir
	}
	cmd := "go test ./..."
	if rel != "." && rel != "" {
		cmd = "go test ./" + rel + "/..."
	}
	one := continuation() + "platform: " + cmd + in + " " + what
	if columns(one) <= lineColumns {
		return one + "\n"
	}
	return continuation() + "platform: " + what + ":\n" + continuation() + "  " + cmd + in + "\n"
}
