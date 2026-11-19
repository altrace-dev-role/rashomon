package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// Step 1c, recap.json's side: what the prompt after a text-only Stop writes,
// and the file's key set across every writer the card touches.

// recapRaw is recap.json by top-level key; nil when the file is absent.
func (e *env) recapRaw() map[string]json.RawMessage {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.home, "recap.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		e.t.Fatal(err)
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &m); err != nil {
		e.t.Fatalf("recap.json: %v\n%s", err, b)
	}
	return m
}

// evaluated is recap.json's evaluation time, health and sess-1's checked
// prompt.
type evaluated struct {
	AtMS    int64
	Healthy bool
	Checked string
}

func (e *env) evaluated() evaluated {
	e.t.Helper()
	raw := e.recapRaw()
	if raw == nil {
		return evaluated{}
	}
	var v evaluated
	var checked map[string]string
	for _, f := range []struct {
		key string
		dst any
	}{{"last_evaluated_at_unix_ms", &v.AtMS}, {"healthy", &v.Healthy}, {"checked", &checked}} {
		if err := json.Unmarshal(raw[f.key], f.dst); err != nil {
			e.t.Fatalf("recap.json %s: %v", f.key, err)
		}
	}
	v.Checked = checked[testSession]
	return v
}

// TestDONE1t_CatchUpControlATurnWithCallsAndNoStop: the positive control of
// DONE1t's evaluation-time assertion. A turn with calls and no Stop is caught
// up at the next prompt, which reaches ClaimStop: the evaluation time moves
// later, the file stays healthy and checked moves to that turn. ClaimStop is
// the one writer of last_evaluated_at_unix_ms that leaves healthy true
// (RecordFailure sets it false), so in DONE1t's rows the time unchanged across
// prompt 3 shows the catch-up returned before the claim. The control sleeps
// before prompt 3 so a claim there writes a later millisecond than the Stop
// before it.
func TestDONE1t_CatchUpControlATurnWithCallsAndNoStop(t *testing.T) {
	const gap = 5 * time.Millisecond
	e := newSteerEnv(t)
	e.actedTurn(t, "prompt-1")
	e.recapLine(stopIn("prompt-1", "I ran the check.", e.cwd, false))
	noteOf(t, e, "prompt-2")
	e.actedTurn(t, "prompt-2")
	before := e.evaluated()
	if before.Checked != "prompt-1" || before.AtMS == 0 {
		t.Fatalf("premise: before prompt-3 the file reads %+v, want a time and checked prompt-1", before)
	}
	time.Sleep(gap)
	noteOf(t, e, "prompt-3")
	after := e.evaluated()
	t.Logf("CONTROL: before %+v; after %+v", before, after)
	if after.AtMS <= before.AtMS || !after.Healthy || after.Checked != "prompt-2" {
		t.Errorf("control: %+v then %+v, want a later time, healthy and checked prompt-2: the catch-up reached ClaimStop", before, after)
	}
}

// recapTopKeys are recap.json's top-level keys as the stored type writes
// them: no top-level field is omitempty, so every write holds all nine.
var recapTopKeys = []string{"checked", "done_evidence", "healthy", "last_evaluated_at_unix_ms", "noted", "owed", "shown", "spoken", "unverified"}

// doneMarkKeys and owedKeys are the keys one session's done marks and owed
// row can hold, omitempty fields included.
var (
	doneMarkKeys = []string{"delivered", "flagged", "shown"}
	owedKeys     = []string{"from", "gate", "packages", "seq", "stale"}
)

// keysOf is m's keys, sorted.
func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// notIn is the members of a that b does not hold.
func notIn(a, b []string) []string {
	have := map[string]bool{}
	for _, k := range b {
		have[k] = true
	}
	var out []string
	for _, k := range a {
		if !have[k] {
			out = append(out, k)
		}
	}
	return out
}

// TestRecapKeys1c_TheBackedFactAddsNoKey: the claim a later Stop reads and
// backs reaches recap.json with no new key. One session runs through every
// writer the card touches: a flagged Stop, the note taken at the next prompt,
// a second flag, a later Stop of the same prompt that backs it, the prompt
// after it, a text-only Stop and the catch-up. After each step the top-level
// key set equals recapTopKeys and every session value's keys are among
// doneMarkKeys and owedKeys. The gate runs this test on the base tree and on
// the head tree and compares the KEYS lines, which hold keys only; the MARKS
// line beside each, with the evaluation time zeroed, is logged and not
// compared, since the card changes what the marks hold. The keys a run
// reached are logged, since a key no step writes cannot be compared. The
// control: a seeded key is found.
func TestRecapKeys1c_TheBackedFactAddsNoKey(t *testing.T) {
	if extra := notIn(append([]string{"x_seeded"}, recapTopKeys...), recapTopKeys); len(extra) != 1 || extra[0] != "x_seeded" {
		t.Fatalf("control: a seeded key read %v, want [x_seeded]", extra)
	}
	e := newSteerEnv(t)
	reached := map[string]bool{}
	check := func(step string) {
		t.Helper()
		raw := e.recapRaw()
		if raw == nil {
			t.Fatalf("%s: no recap.json", step)
		}
		top := keysOf(raw)
		var session []string
		if extra, missing := notIn(top, recapTopKeys), notIn(recapTopKeys, top); len(extra)+len(missing) > 0 {
			t.Errorf("%s: top-level keys %v: new %v, missing %v", step, top, extra, missing)
		}
		for _, f := range []struct {
			key     string
			allowed []string
		}{{"done_evidence", doneMarkKeys}, {"owed", owedKeys}} {
			var bySession map[string]map[string]json.RawMessage
			if err := json.Unmarshal(raw[f.key], &bySession); err != nil {
				t.Fatalf("%s: recap.json %s: %v", step, f.key, err)
			}
			for _, v := range bySession {
				ks := keysOf(v)
				for _, k := range ks {
					reached[f.key+"."+k] = true
				}
				if extra := notIn(ks, f.allowed); len(extra) > 0 {
					t.Errorf("%s: %s holds new keys %v", step, f.key, extra)
				}
				for _, k := range ks {
					session = append(session, f.key+"."+k)
				}
			}
		}
		sort.Strings(session)
		t.Logf("KEYS %s: top %v; session %v", step, top, session)
		m := e.marks()
		m.AtMS = 0
		t.Logf("MARKS %s: %+v", step, m)
	}

	e.flaggedStopIn(t)
	check("1 a flagged Stop of prompt-1")
	noteOf(t, e, "prompt-2")
	check("2 prompt-2 takes the note")
	e.turnCall(t, "prompt-2", "toolu_edit_late", editOf("late.go"))
	line, _ := e.recapLine(stopIn("prompt-2", "All 42 tests pass.", e.cwd, false))
	check("3 a Stop of prompt-2 over a later edit")
	e.turnCall(t, "prompt-2", "toolu_test_2", goTestRun("ok"))
	line2, _ := e.recapLine(stopIn("prompt-2", "All 42 tests pass.", e.cwd, true))
	check("4 a later Stop of prompt-2 after a passing run")
	noteOf(t, e, "prompt-3")
	check("5 prompt-3")
	e.recapLine(stopIn("prompt-3", "Done.", e.cwd, false))
	check("6 a text-only Stop of prompt-3")
	noteOf(t, e, "prompt-4")
	check("7 prompt-4's catch-up")

	var got []string
	for k := range reached {
		got = append(got, k)
	}
	sort.Strings(got)
	t.Logf("REACHED step 3 printed %q, step 4 printed %q; session keys %v", line, line2, got)
}
