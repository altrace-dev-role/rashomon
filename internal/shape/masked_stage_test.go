package shape

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"unsafe"
)

func mo1Derive(t *testing.T, cmd string) Shape {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"command": cmd})
	if err != nil {
		t.Fatal(err)
	}
	return Derive("Bash", raw, []byte("key"))
}

func mo1Word(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// TestMO1_RulesVersionIsEleven: the version the runner's name and the masking
// stage are decided at. A literal, not a comparison with the pins' own key.
func TestMO1_RulesVersionIsEleven(t *testing.T) {
	if RulesVersion != 11 {
		t.Errorf("RulesVersion = %d, want 11 (MASKED-OUTCOME1: runner_name and masked_by)", RulesVersion)
	}
}

// TestMO1_TheMaskingStageIsNamed: a masked runner's line names the stage of
// the pipe that hid it, a keeping one (tail, tee, a bare cat) where every
// stage after the runner keeps the summary, else the first that drops it.
func TestMO1_TheMaskingStageIsNamed(t *testing.T) {
	for _, tc := range []struct {
		cmd, runner, by string
	}{
		{"python3 -m pytest -q 2>&1 | tail -15", "python3 -m pytest", "tail"},
		{"python3 -m pytest -q | tail", "python3 -m pytest", "tail"},
		{"python3 -u -m pytest -q | tail -n 20", "python3 -m pytest", "tail"},
		{"go test ./... 2>&1 | tail -n 5", "go test", "tail"},
		{"go test ./... 2>&1 | tail -5 | tee out.log", "go test", "tail"},
		{"cargo test 2>&1 | tail -20", "cargo test", "tail"},
		{"go test ./... | tee test.log", "go test", "tee"},
		{"go test ./... 2>&1 | cat", "go test", "cat"},
		{"make test |& cat", "make test", "cat"},
		{"go test ./... | tail -n +3", "go test", "tail"},
		{"make release 2>&1 | tail", "make", "tail"},
		{"python3 -m pytest | head -20", "python3 -m pytest", "head"},
		{"python3 -m pytest | grep -E 'passed|failed'", "python3 -m pytest", "grep"},
		{"go test ./... | wc -l", "go test", "wc"},
		{"go test ./... | sort", "go test", "sort"},
		{"go test ./... | cut -c1-80", "go test", "cut"},
		{"go test ./... | sed s/a/b/", "go test", "other"},
		{"go test ./... | tail | head -3", "go test", "head"},
		{"go test ./... | head -3 | tail", "go test", "head"},
		{"go test ./... | tail -c 100", "go test", "other"},
		{"go test ./... | tail --bytes=100", "go test", "other"},
		{"go test ./... | tail -qc 9", "go test", "other"},
		{"go test ./... | tail somefile", "go test", "other"},
		{"go test ./... | tail -n $N", "go test", "tail"}, // the count is passed over unread
		{"go test ./... | tail -n", "go test", "other"},
		{"go test ./... | cat -n", "go test", "other"},
		{"go test ./... | cat out.txt", "go test", "other"},
		{"go test ./... | cat -", "go test", "cat"},
		{"(go test ./...) | tail -3", "go test", "tail"},
		{"cd /repo && python3 -m pytest | tail", "python3 -m pytest", "tail"},
	} {
		s := mo1Derive(t, tc.cmd)
		if got := mo1Word(s.RunnerName); got != tc.runner {
			t.Errorf("%q: runner_name = %s, want %s", tc.cmd, got, tc.runner)
		}
		if got := mo1Word(s.MaskedBy); got != tc.by {
			t.Errorf("%q: masked_by = %s, want %s", tc.cmd, got, tc.by)
		}
	}
}

// TestMO1_OnlyAPipeIsAMaskingStage: a hidden status that no pipe hid has no
// stage, and a line whose runner's status reaches it has none either; the
// runner is still named.
func TestMO1_OnlyAPipeIsAMaskingStage(t *testing.T) {
	for _, tc := range []struct {
		cmd, runner string
	}{
		{"go test ./... || true", "go test"},
		{"go test ./...; echo $?", "go test"},
		{"go test ./... &", "go test"},
		{"go test ./... && echo ok", "go test"},
		{"set -o pipefail; go test ./... | tail", "go test"},
		{"echo hi | go test ./...", "go test"},
	} {
		s := mo1Derive(t, tc.cmd)
		if s.MaskedBy != nil {
			t.Errorf("%q: masked_by = %s, want null: no pipe hid it", tc.cmd, *s.MaskedBy)
		}
		if got := mo1Word(s.RunnerName); got != tc.runner {
			t.Errorf("%q: runner_name = %s, want %s", tc.cmd, got, tc.runner)
		}
	}
}

// TestMO1_TheWordsAreTheBinarys is D229's condition for leaving ScopeVersion
// at 4: runner_name and masked_by are words the binary ships, never a word of
// the line. A runner no row names yields null; a canary runner, argument and
// stage that no list names appear in neither field; and the two values on
// two lines are the same string in memory, the binary's.
func TestMO1_TheWordsAreTheBinarys(t *testing.T) {
	// Unknown runner: null, whatever its stage.
	s := mo1Derive(t, "zzrunner --canaryarg 2>&1 | tail -5")
	if s.RunnerName != nil || s.MaskedBy != nil {
		t.Errorf("an unknown runner: runner_name %s masked_by %s, want both null", mo1Word(s.RunnerName), mo1Word(s.MaskedBy))
	}

	// A known runner with a canary argument and a canary stage.
	const argC, stageC, progC = "canaryargument9f3", "canarystage7c1", "canaryprog4d2"
	for _, cmd := range []string{
		"go test ./" + argC + "/... 2>&1 | " + stageC + " -x",
		"go test -run " + argC + " ./... | tail " + argC,
		progC + " go test ./... | " + stageC,
		"make " + argC + " | " + stageC,
		"cd " + argC + " && go test ./... | " + stageC + " " + argC,
	} {
		s := mo1Derive(t, cmd)
		for _, p := range []*string{s.RunnerName, s.MaskedBy} {
			if p != nil && (strings.Contains(*p, "canary") || strings.Contains(cmd, *p) && !isListWord(*p)) {
				t.Errorf("%q: a field holds %q, which is a word of the line", cmd, *p)
			}
		}
	}

	// Every value is the binary's own string: the same bytes at the same
	// address on two different lines.
	a := mo1Derive(t, "python3 -m pytest tests/a --canary1 2>&1 | tail -3")
	b := mo1Derive(t, "python3 -u -m pytest tests/b -k other | tail -n 9")
	if a.RunnerName == nil || b.RunnerName == nil || a.MaskedBy == nil || b.MaskedBy == nil {
		t.Fatalf("runner_name/masked_by null: %v %v", a, b)
	}
	if unsafe.StringData(*a.RunnerName) != unsafe.StringData(*b.RunnerName) {
		t.Error("runner_name is not the binary's own string: two lines hold it at two addresses")
	}
	if unsafe.StringData(*a.MaskedBy) != unsafe.StringData(*b.MaskedBy) {
		t.Error("masked_by is not the binary's own string: two lines hold it at two addresses")
	}
	if got := *a.RunnerName; got != "python3 -m pytest" {
		t.Errorf("runner_name = %q, want python3 -m pytest", got)
	}
}

// isListWord reports a word that is on a runner list or in the stage words,
// which a line may also hold: `go test` is in the line and in the row.
func isListWord(w string) bool {
	return slices.Contains(testCommandNames, w) || slices.Contains(buildCommandNames, w) || slices.Contains(MaskedByWords(), w)
}

// TestMO1_EveryNameIsAListedWord: whatever line is derived, the two fields
// are one of the rows' words or one of the stage words, never anything else.
func TestMO1_EveryNameIsAListedWord(t *testing.T) {
	for _, cmd := range []string{
		"go test ./... | tail", "make x | y", "nox -s z 2>&1 | cut -d: -f1", "npm run build | sed q",
		"zzz | tail", "`go test` | tail", "bash -c 'go test ./...' | tail", "go test ./... | $PAGER", "go test ./... | \"tail\"",
	} {
		s := mo1Derive(t, cmd)
		if s.RunnerName != nil && !slices.Contains(testCommandNames, *s.RunnerName) && !slices.Contains(buildCommandNames, *s.RunnerName) {
			t.Errorf("%q: runner_name %q is on no list", cmd, *s.RunnerName)
		}
		if s.MaskedBy != nil && !slices.Contains(MaskedByWords(), *s.MaskedBy) {
			t.Errorf("%q: masked_by %q is no stage word", cmd, *s.MaskedBy)
		}
	}
}

// TestMO1_RowNamesAreTheRows holds the names to the lists: one per row, in
// order, each the row's words joined by a space.
func TestMO1_RowNamesAreTheRows(t *testing.T) {
	for _, c := range []struct {
		names []string
		rows  [][]string
	}{{testCommandNames, testCommands}, {buildCommandNames, buildCommands}} {
		if len(c.names) != len(c.rows) {
			t.Fatalf("%d names for %d rows", len(c.names), len(c.rows))
		}
		for i, r := range c.rows {
			if want := strings.Join(r, " "); c.names[i] != want {
				t.Errorf("name %d = %q, want %q", i, c.names[i], want)
			}
		}
	}
}

// TestMO1_TheStageWordsAreClosed: the vocabulary the schema enum is held to,
// and which of its words keep the summary.
func TestMO1_TheStageWordsAreClosed(t *testing.T) {
	want := []string{"other", "tail", "tee", "cat", "head", "grep", "wc", "sort", "cut"}
	if got := MaskedByWords(); !slices.Equal(got, want) {
		t.Errorf("MaskedByWords = %v, want %v", got, want)
	}
	for _, w := range want {
		keeps := w == "tail" || w == "tee" || w == "cat"
		if got := MaskedByKeeps(w); got != keeps {
			t.Errorf("MaskedByKeeps(%q) = %v, want %v", w, got, keeps)
		}
	}
	if MaskedByKeeps("") || MaskedByKeeps("python3") {
		t.Error("a word that is no stage keeps the summary")
	}
}

// TestMO1_TheFieldsAreNotOnTheDeclaration: runner_name and masked_by are
// json:"-", so a declaration's shape does not change with them.
func TestMO1_TheFieldsAreNotOnTheDeclaration(t *testing.T) {
	s := mo1Derive(t, "python3 -m pytest | tail")
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"runner_name", "masked_by", "RunnerName", "MaskedBy", "pytest"} {
		if strings.Contains(string(b), k) {
			t.Errorf("the shape marshals %q: %s", k, b)
		}
	}
}

// TestMO1_TheIdsAreThePlacesInTheLists: the one-byte ids verify holds in place
// of the two words, round trip over every word, 0 for a word that is none, and
// the lists fit a byte.
func TestMO1_TheIdsAreThePlacesInTheLists(t *testing.T) {
	names := RunnerNames()
	if len(names) > 254 {
		t.Fatalf("%d runner names do not fit an id of one byte", len(names))
	}
	for i, n := range names {
		id := RunnerNameID(n)
		if id == 0 || int(id) > i+1 {
			t.Errorf("RunnerNameID(%q) = %d, want the place of its first row, at most %d", n, id, i+1)
		}
		if got := RunnerNameOf(id); got != n {
			t.Errorf("RunnerNameOf(%d) = %q, want %q", id, got, n)
		}
	}
	for _, w := range MaskedByWords() {
		id := MaskedByID(w)
		if id == 0 || MaskedByOf(id) != w {
			t.Errorf("MaskedByID(%q) = %d reads back %q", w, id, MaskedByOf(id))
		}
	}
	for _, none := range []string{"", "python3", "canary", "Tail"} {
		if RunnerNameID(none) != 0 || MaskedByID(none) != 0 {
			t.Errorf("%q has an id", none)
		}
	}
	if RunnerNameOf(0) != "" || RunnerNameOf(255) != "" || MaskedByOf(0) != "" || MaskedByOf(255) != "" {
		t.Error("an id past the lists names a word")
	}
}

// TestMO1_TheListsFitAOneByteId: RunnerNameID and MaskedByID hold a word's
// place in a uint8, and RunnerNameID stops counting at 254 words, so a 255th
// row would map to 0 -- "no word" -- without a sound. Each list, and the two
// runner lists together (an id counts across them), must fit, so that a 255th
// row fails CI instead.
func TestMO1_TheListsFitAOneByteId(t *testing.T) {
	for _, l := range []struct {
		name  string
		words []string
	}{
		{"testCommandNames", testCommandNames},
		{"buildCommandNames", buildCommandNames},
		{"RunnerNames", RunnerNames()},
		{"MaskedByWords", MaskedByWords()},
	} {
		if len(l.words) > 254 {
			t.Errorf("%s has %d words, want at most 254 for a one-byte id", l.name, len(l.words))
		}
	}
	if got, want := len(RunnerNames()), len(testCommandNames)+len(buildCommandNames); got != want {
		t.Errorf("RunnerNames has %d words, want the two lists' %d", got, want)
	}
}
