package acceptance

// PW1b-ON-230 step 3 (WED-SLICE row 1(c) and row 4): the platform graph
// reaches the model before it acts. A prompt that names a Go symbol gets,
// in its task-preflight note, the package that declares it and its direct
// callers, read from the pair `pgraph sync` vouched for, and only while
// every package the answer rests on is as the sync read it. The packages a
// turn changed and never tested, which step 2's Stop names to the person,
// reach the model at the next prompt (ARCH-INLINE1 item 3, #218): inside
// the owed gate's note when one is owed, else as a note of their own.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
)

// pw1b3Files is a module whose package a declares CountRows, called from
// b and from c.
var pw1b3Files = map[string]string{
	"go.mod":      "module example.com/m\n\ngo 1.25.0\n",
	"a/a.go":      "package a\n\nfunc CountRows() int { return 1 }\n",
	"a/a_test.go": "package a\n",
	"b/b.go":      "package b\n\nimport \"example.com/m/a\"\n\nfunc SumRows() int { return a.CountRows() + 1 }\n",
	"c/c.go":      "package c\n\nimport \"example.com/m/a\"\n\nfunc Twice() int { return 2 * a.CountRows() }\n",
}

const pw1b3Line = "symbols named: 'CountRows' declared in 'a', direct callers 2 in 2 packages"

// TestPW1b3_APromptNamingASymbolGivesItsPackageAndDirectCallers: after a
// sync, a prompt naming CountRows gives the model its package and its two
// callers, and no symbols-unknown. Controls: with no sync, and after an
// edit to c, a caller's package, the prompt gets no note: the symbol is all
// it names, nothing proves it, and an unknown alone is nothing new to say.
// Break: leave Project.Symbols unset in the prompt path, or skip the
// freshness of the callers' packages.
func TestPW1b3_APromptNamingASymbolGivesItsPackageAndDirectCallers(t *testing.T) {
	for _, tc := range []struct {
		name       string
		sync, edit bool
		line       bool
	}{
		{"synced", true, false, true},
		{"no sync", false, false, false},
		{"a caller edited since the sync", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newSteerEnv(t)
			dir := gi1Project(t, pw1b3Files)
			if tc.sync {
				h156Sync(t, dir)
			}
			if tc.edit {
				if err := os.WriteFile(filepath.Join(dir, "c", "c.go"), []byte(pw1b3Files["c/c.go"]+"\nfunc Thrice() int { return 3 }\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			_, out := e.prompt(dir, "make CountRows return 2", "p1")
			model := out.HookSpecificOutput.AdditionalContext
			t.Logf("note:\n%s", model)
			if got := strings.Contains(model, "\n"+pw1b3Line); got != tc.line {
				t.Errorf("the symbols line is in the note: %v, want %v", got, tc.line)
			}
			if !tc.line && model != "" {
				t.Errorf("a note with no symbol proven:\n%s", model)
			}
			if tc.line && strings.Contains(model, "symbols not matched") {
				t.Errorf("the symbols-unknown beside a proven symbol:\n%s", model)
			}
			if !strings.Contains(out.SystemMessage, pw1b3Line) && tc.line {
				t.Errorf("the person's mirror lacks the symbols line:\n%s", out.SystemMessage)
			}
		})
	}
}

// TestPW1b3_TheNextPromptGivesTheModelTheOwedPackages: in a project whose CI
// runs go test ./..., a turn that edits a/a.go owes package a, and the next
// prompt gives the model that package: inside the inferred gate's note when
// the gate is owed (no go test since the edit; the Stop names a to the
// person), else as its own note (go test ./a ran red: the gate is not owed,
// and a red line that names a alone makes its row unmet; the Stop's
// recorded failure carries the platform line, STOP-PRECEDENCE1, which
// TestSTP1_AFailureCountCarriesThePlatformLine pins). A red go test ./a/... is
// not that case: it names many packages, so a's row reads unknown (verify
// M4). The Stop owes a package only under a declared go test gate (verify's
// goTestGate), so the fixture declares one. Control: go test ./a/...
// passing after the edit owes nothing, and the next prompt gets no note.
// Break: drop Owed.Packages, or give them only with a gate.
func TestPW1b3_TheNextPromptGivesTheModelTheOwedPackages(t *testing.T) {
	const owed = "Go packages the last reply changed with no passing go test since: 'a'."
	files := map[string]string{".github/workflows/ci.yml": h156Files[".github/workflows/ci.yml"]}
	for k, v := range pw1b3Files {
		files[k] = v
	}
	for _, tc := range []struct {
		name string
		run  string // the go test line after the edit, "" for none
		red  bool
		stop string // what the Stop says, "" for no platform line
		want string // the note's text, "" for none; a prefix with a gate
	}{
		{"with the gate's note", "", false, "platform: go test ./a/... not run since the edit", "rashomon record: gate "},
		{"alone", "go test ./a", true, "rashomon: 1 recorded failure.", "rashomon record: " + owed},
		{"tested after the edit", "go test ./a/...", false, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newSteerEnv(t)
			dir := gi1Project(t, files)
			h156Sync(t, dir)
			e.dghCall(dir, "toolu_e", "Edit", pw1b2Edit(dir, "a/a.go"))
			test := map[string]any{"command": tc.run}
			switch {
			case tc.run == "":
			case tc.red:
				e.dghFailedCall(dir, "toolu_g", "Bash", test, 1)
			default:
				e.dghCall(dir, "toolu_g", "Bash", test)
			}
			res := e.recap(stopEvent("Stop", "I changed a/a.go.", dir), stAgent, "CI=")
			var rec recapOutput
			if res.exitCode != 0 || res.stdout != "" && json.Unmarshal([]byte(res.stdout), &rec) != nil {
				t.Fatalf("recap: exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
			}
			if stop := rec.SystemMessage; tc.stop == "" && strings.Contains(stop, "platform: ") || tc.stop != "" && !strings.Contains(stop, tc.stop) {
				ledger := e.run("", nil, "report", "--ledger", "--session", testSession)
				t.Fatalf("premise: want the Stop to say %q (\"\" for no platform line); its stdout:\n%s\nledger:\n%s", tc.stop, res.stdout, ledger.stdout)
			}
			_, out := e.prompt(dir, "carry on", "p2")
			model := out.HookSpecificOutput.AdditionalContext
			t.Logf("next prompt's note:\n%s", model)
			switch {
			case tc.want == "" && model != "":
				t.Errorf("a note with nothing owed:\n%s", model)
			case tc.want != "" && (!strings.HasPrefix(model, tc.want) || !strings.HasSuffix(model, owed)):
				t.Errorf("want a note starting %q and ending %q", tc.want, owed)
			}
		})
	}
}

// TestPW1b3_TheReadIsDisclosed: the full screen's pgraph paragraph says a
// prompt that names a Go symbol makes rashomon read .pgraph and give the
// agent the declaring package and the direct callers, and the prompt
// sentence says the steer line keeps the symbols it names. Break: wire the
// read without the words.
func TestPW1b3_TheReadIsDisclosed(t *testing.T) {
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	var all string
	for _, p := range inv.Paragraphs {
		all += p.Text + "\n"
	}
	for _, w := range []string{
		"When your prompt names a Go function, type or method, rashomon reads .pgraph for it",
		"the package that declares it and how many direct callers it has",
		"only while the files pgraph read are unchanged",
		"the project paths and Go symbols it names at record scope v5",
	} {
		if !strings.Contains(all, w) {
			t.Errorf("the full screen does not say %q", w)
		}
	}
}
