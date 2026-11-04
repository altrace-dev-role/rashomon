package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSTP1_AFailureCountCarriesThePlatformLine (Stop-precedence, mgr 11-02
// ruling (a)): a turn that edits a/a.go and runs go test ./a red ends with
// recap.Line's sentence, "1 recorded failure", while the platform read owes
// package a (unmet: a red line that named it alone). The Stop's one line
// carries both, the sentence, then the platform line, then the pointer, so
// the person is told which package is unmet (eng-1's gap,
// se1/pw230c-step3g.log:8-9). At StopFailure, where the platform line never
// prints (tech 2.2.11 condition 4), the sentence stands alone; card 2.4 R12
// holds for every other clause, so no done flag or value line joins. Break:
// keep recap.Line's sentence alone, set the platform line after the pointer,
// or print it at StopFailure, and this fails.
func TestSTP1_AFailureCountCarriesThePlatformLine(t *testing.T) {
	files := map[string]string{".github/workflows/ci.yml": h156Files[".github/workflows/ci.yml"]}
	for k, v := range pw1b3Files {
		files[k] = v
	}
	const lead = "※ rashomon: "
	pad := strings.Repeat(" ", len([]rune(lead)))
	pointer := pad + "→ rashomon report --session " + testSession
	for _, tc := range []struct{ event, want string }{
		{"Stop", lead + "1 recorded failure.\n" + pad + "platform: go test ./a/... failed after the edit\n" + pointer},
		{"StopFailure", lead + "1 recorded failure.\n" + pointer},
	} {
		t.Run(tc.event, func(t *testing.T) {
			e := newSteerEnv(t)
			dir := gi1Project(t, files)
			h156Sync(t, dir)
			e.dghCall(dir, "toolu_e", "Edit", pw1b2Edit(dir, "a/a.go"))
			e.dghFailedCall(dir, "toolu_g", "Bash", map[string]any{"command": "go test ./a"}, 1)
			res := e.recap(stopEvent(tc.event, "I changed a/a.go.", dir), stAgent, "CI=")
			var rec recapOutput
			if res.exitCode != 0 || json.Unmarshal([]byte(res.stdout), &rec) != nil {
				t.Fatalf("recap %s: exit %d, stdout %q, stderr %q", tc.event, res.exitCode, res.stdout, res.stderr)
			}
			if rec.SystemMessage != tc.want {
				t.Errorf("the %s line:\n%s\nwant:\n%s", tc.event, rec.SystemMessage, tc.want)
			}
		})
	}
}
