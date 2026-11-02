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
// systemMessage, the person's channel: no line here reaches the model.
func platformLines(v verify.StopView, fromPlugin bool) string {
	var lines []string
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
		switch o.State {
		case verify.ObligationOpen:
			lines = append(lines, platformLine(f, "not run since the edit"))
		case verify.ObligationUnmet:
			lines = append(lines, platformLine(f, "failed after the edit"))
		}
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
