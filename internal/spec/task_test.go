package spec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// Task fixtures, built like the spec's: a base object and the one key a row
// changes (obj, kv and q are parse_test.go's).
var taskBase = []kv{{"id", `"t-1"`}, {"goal", `"Ship the nightly export"`}, {"decision", `"Whether to cut the release"`},
	{"expected_evidence", `["real_data_run", "mechanism_stated"]`}, {"mechanism", `"the export job reads the warehouse"`}}

func taskDoc(over ...kv) string { return obj(taskBase, over...) }

// taskRow is one ParseTask case: reason "" is accepted.
type taskRow struct {
	name   string
	in     string
	reason Reason
	ptr    string
	limit  int
}

func runTaskRows(t *testing.T, rows []taskRow) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			task, digest, err := ParseTask([]byte(r.in))
			if r.reason == "" {
				if err != nil || task == nil || !strings.HasPrefix(digest, DigestPrefix) {
					t.Fatalf("got %+v, %q, %v; want accepted", task, digest, err)
				}
				return
			}
			var e *Error
			if !errors.As(err, &e) || task != nil {
				t.Fatalf("got %+v, %v; want a *Error with reason %s", task, err, r.reason)
			}
			if e.Reason != r.reason || e.Pointer != r.ptr || e.Limit != r.limit {
				t.Errorf("error %+v, want reason %s at %q limit %d", e, r.reason, r.ptr, r.limit)
			}
		})
	}
}

// TestTF1_ParseWithinLimits: every field reads back as written, and the
// mechanism may be present, empty or left out (the last two read as "").
func TestTF1_ParseWithinLimits(t *testing.T) {
	want := Task{ID: "t-1", Goal: "Ship the nightly export", Decision: "Whether to cut the release",
		ExpectedEvidence: []string{"real_data_run", "mechanism_stated"}, Mechanism: "the export job reads the warehouse"}
	for _, c := range []struct {
		name, in, mechanism string
	}{
		{"mechanism present", taskDoc(), want.Mechanism},
		{"mechanism empty", taskDoc(kv{"mechanism", `""`}), ""},
		{"mechanism left out", taskDoc(kv{"mechanism", "-"}), ""},
	} {
		task, digest, err := ParseTask([]byte(c.in))
		if err != nil || task == nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		w := want
		w.Mechanism = c.mechanism
		if !reflect.DeepEqual(*task, w) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, *task, w)
		}
		if !strings.HasPrefix(digest, DigestPrefix) || len(digest) != len(DigestPrefix)+64 {
			t.Errorf("%s: digest %q", c.name, digest)
		}
	}
	runTaskRows(t, []taskRow{
		{name: "id missing", in: taskDoc(kv{"id", "-"}), reason: ReasonRequired, ptr: "/id"},
		{name: "id off the pattern", in: taskDoc(kv{"id", `"T 1"`}), reason: ReasonID, ptr: "/id"},
		{name: "goal missing", in: taskDoc(kv{"goal", "-"}), reason: ReasonRequired, ptr: "/goal"},
		{name: "goal empty", in: taskDoc(kv{"goal", `""`}), reason: ReasonEmpty, ptr: "/goal"},
		{name: "decision missing", in: taskDoc(kv{"decision", "-"}), reason: ReasonRequired, ptr: "/decision"},
		{name: "decision with a control character", in: taskDoc(kv{"decision", `"a\u0007b"`}), reason: ReasonControl, ptr: "/decision"},
		{name: "mechanism not a string", in: taskDoc(kv{"mechanism", `5`}), reason: ReasonType, ptr: "/mechanism"},
		{name: "not an object", in: `["t-1"]`, reason: ReasonType, ptr: ""},
	})
}

// TestTF2_Over4KiBRefused: a document of exactly MaxTaskBytes is read, one
// byte more is refused before it is read, and the error names the cap.
func TestTF2_Over4KiBRefused(t *testing.T) {
	doc := taskDoc()
	at := doc + strings.Repeat(" ", MaxTaskBytes-len(doc))
	if len(at) != 4096 {
		t.Fatalf("premise: %d bytes", len(at))
	}
	if task, _, err := ParseTask([]byte(at)); err != nil || task == nil {
		t.Errorf("4096 bytes: %v, want accepted", err)
	}
	task, digest, err := ParseTask([]byte(at + " "))
	var e *Error
	if !errors.As(err, &e) || e.Reason != ReasonTooLarge || e.Limit != 4096 || task != nil || digest != "" {
		t.Fatalf("4097 bytes: %+v, %q, %v; want file_too_large, limit 4096, no digest", task, digest, err)
	}
	if !strings.HasSuffix(err.Error(), "(limit 4096)") {
		t.Errorf("error %q does not name the cap", err)
	}
}

// TestTF3_UnknownKeyRefused: a key outside the five is refused at its
// pointer, the first in file order; keys are exact.
func TestTF3_UnknownKeyRefused(t *testing.T) {
	runTaskRows(t, []taskRow{
		{name: "all five", in: taskDoc()},
		{name: "owner", in: taskDoc(kv{"owner", `"me"`}), reason: ReasonUnknownKey, ptr: "/owner"},
		{name: "ID in capitals", in: taskDoc(kv{"id", "-"}, kv{"ID", `"t-1"`}), reason: ReasonUnknownKey, ptr: "/ID"},
		{name: "the first of two", in: `{"zz": 1, ` + taskDoc(kv{"aa", `2`})[1:], reason: ReasonUnknownKey, ptr: "/zz"},
	})
}

// TestTF4_EvidenceKindsClosed: each of the five kinds is read alone; an
// unknown kind, a kind twice, an empty list and no list are each refused.
func TestTF4_EvidenceKindsClosed(t *testing.T) {
	var rows []taskRow
	for _, k := range EvidenceKinds {
		rows = append(rows, taskRow{name: k, in: taskDoc(kv{"expected_evidence", `[` + q(k) + `]`})})
	}
	rows = append(rows,
		taskRow{name: "all five", in: taskDoc(kv{"expected_evidence", `["real_data_run", "hostile_input_run", "cross_check", "judgment_disclosed", "mechanism_stated"]`})},
		taskRow{name: "unknown", in: taskDoc(kv{"expected_evidence", `["real_data_run", "prod_run"]`}), reason: ReasonEnum, ptr: "/expected_evidence/1"},
		taskRow{name: "duplicate", in: taskDoc(kv{"expected_evidence", `["cross_check", "real_data_run", "cross_check"]`}), reason: ReasonDuplicateID, ptr: "/expected_evidence/2"},
		taskRow{name: "empty", in: taskDoc(kv{"expected_evidence", `[]`}), reason: ReasonEmpty, ptr: "/expected_evidence"},
		taskRow{name: "missing", in: taskDoc(kv{"expected_evidence", "-"}), reason: ReasonRequired, ptr: "/expected_evidence"},
		taskRow{name: "not a list", in: taskDoc(kv{"expected_evidence", `"cross_check"`}), reason: ReasonType, ptr: "/expected_evidence"},
	)
	runTaskRows(t, rows)
	task, _, err := ParseTask([]byte(taskDoc(kv{"expected_evidence", `["mechanism_stated", "real_data_run"]`})))
	if err != nil || task == nil || !slices.Equal(task.ExpectedEvidence, []string{"mechanism_stated", "real_data_run"}) {
		t.Errorf("the list's order is the file's: %+v, %v", task, err)
	}
}

// TestTF5_LinesCapped: goal, decision and mechanism are read at 300 runes and
// refused at 301, counted in runes: each row's rune is two bytes, so a cap on
// bytes refuses the 300.
func TestTF5_LinesCapped(t *testing.T) {
	var rows []taskRow
	for _, key := range []string{"goal", "decision", "mechanism"} {
		at, past := strings.Repeat("é", MaxWhyRunes), strings.Repeat("é", MaxWhyRunes+1)
		if utf8.RuneCountInString(at) != 300 || len(at) != 600 {
			t.Fatalf("premise: %d runes, %d bytes", utf8.RuneCountInString(at), len(at))
		}
		rows = append(rows,
			taskRow{name: key + " 300 runes", in: taskDoc(kv{key, q(at)})},
			taskRow{name: key + " 301 runes", in: taskDoc(kv{key, q(past)}), reason: ReasonTooLong, ptr: "/" + key, limit: 300},
		)
	}
	runTaskRows(t, rows)
}

// TestTF6_DigestIsJCSStable: the digest is DigestPrefix and the hex SHA-256
// of the RFC 8785 form, so key order and whitespace do not move it and a
// change of content does; a document the schema refuses still has one.
func TestTF6_DigestIsJCSStable(t *testing.T) {
	canon := `{"decision":"d","expected_evidence":["cross_check"],"goal":"g","id":"t"}`
	sum := sha256.Sum256([]byte(canon))
	want := DigestPrefix + hex.EncodeToString(sum[:])
	for name, in := range map[string]string{
		"canonical":        canon,
		"keys reordered":   `{"id":"t","goal":"g","decision":"d","expected_evidence":["cross_check"]}`,
		"whitespace added": "{\n\t\"goal\" : \"g\",\r\n \"id\":\"t\", \"decision\": \"d\",\n \"expected_evidence\": [ \"cross_check\" ]\n}\n",
		"escape spelling":  `{"decision":"\u0064","expected_evidence":["cross_check"],"goal":"g","id":"t"}`,
	} {
		if _, got, err := ParseTask([]byte(in)); err != nil || got != want {
			t.Errorf("%s: %q, %v; want %s", name, got, err, want)
		}
	}
	if _, got, _ := ParseTask([]byte(`{"decision":"d","expected_evidence":["cross_check"],"goal":"G","id":"t"}`)); got == want {
		t.Error("a changed goal kept the digest")
	}
	refused := `{"id":"t","goal":"g","decision":"d","expected_evidence":["cross_check"],"x":1}`
	rsum := sha256.Sum256([]byte(`{"decision":"d","expected_evidence":["cross_check"],"goal":"g","id":"t","x":1}`))
	task, got, err := ParseTask([]byte(refused))
	if err == nil || task != nil || got != DigestPrefix+hex.EncodeToString(rsum[:]) {
		t.Errorf("refused document: %+v, %q, %v; want its digest with the error", task, got, err)
	}
}

// TestTF1b_IsTaskFile: the active task file at the root or under a
// directory, after cleaning (a dot segment, a doubled or a trailing slash);
// never a sibling, a longer name or a name that only ends the same way.
func TestTF1b_IsTaskFile(t *testing.T) {
	for p, want := range map[string]bool{
		TaskFile:                                       true,
		"./.claude/rashomon/task/current.json":         true,
		"sub/.claude/rashomon/task/current.json":       true,
		"/abs/repo/.claude/rashomon/task/current.json": true,
		"a/../.claude/rashomon/task/current.json":      true,
		".claude/rashomon/task/./current.json":         true,
		".claude//rashomon/task/current.json":          true,
		"sub/.claude/rashomon/task/current.json/":      true,
		".claude/rashomon/task/abc.json":               false,
		".claude/rashomon/task/current.json.bak":       false,
		"x.claude/rashomon/task/current.json":          false,
		".claude/rashomon/spec.json":                   false,
		"":                                             false,
	} {
		if got := IsTaskFile(p); got != want {
			t.Errorf("IsTaskFile(%q) = %v, want %v", p, got, want)
		}
	}
}

// TestTF1c_SupplyKindsAreTheRunKinds: EvidenceKinds is the five in the
// canonical order, and less its two text kinds it is the list a gate may
// supply (3.1's supplyKinds), so a gate never supplies a kind no task can
// expect.
func TestTF1c_SupplyKindsAreTheRunKinds(t *testing.T) {
	if want := []string{"real_data_run", "hostile_input_run", "cross_check", "judgment_disclosed", "mechanism_stated"}; !slices.Equal(EvidenceKinds, want) {
		t.Fatalf("EvidenceKinds %q, want %q", EvidenceKinds, want)
	}
	runs := slices.DeleteFunc(slices.Clone(EvidenceKinds), func(k string) bool { return k == "judgment_disclosed" || k == "mechanism_stated" })
	if !slices.Equal(runs, supplyKinds) {
		t.Errorf("EvidenceKinds less the text kinds %q, supplyKinds %q", runs, supplyKinds)
	}
}

// FuzzParseTask: ParseTask reads a file a person writes and an agent may
// reach, so whatever the bytes are it must not panic and must answer within
// its bounds.
//
//   - bounded: an input over MaxTaskBytes is refused as too large with no
//     digest; every error is an *Error with a reason and no Task; a Task's
//     fields are within the caps and its kinds closed and distinct;
//   - deterministic: a second call on a copy gives the same Task, digest and
//     first error;
//   - no retention: overwriting the input leaves the Task as it was.
func FuzzParseTask(f *testing.F) {
	for _, s := range []string{
		taskDoc(), taskDoc(kv{"mechanism", `""`}), taskDoc(kv{"mechanism", "-"}), taskDoc(kv{"owner", `1`}),
		taskDoc(kv{"expected_evidence", `["cross_check", "cross_check"]`}), taskDoc(kv{"goal", q(strings.Repeat("é", 301))}),
		``, `{`, `[]`, `{}`, "\x00", strings.Repeat(" ", MaxTaskBytes) + "{}", strings.Repeat(`{"a":`, 40),
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		kept := append([]byte(nil), data...)
		task, digest, err := ParseTask(data)
		if len(kept) > MaxTaskBytes {
			var e *Error
			if !errors.As(err, &e) || e.Reason != ReasonTooLarge || digest != "" {
				t.Fatalf("%d bytes: %v, digest %q", len(kept), err, digest)
			}
		}
		again, digest2, err2 := ParseTask(append([]byte(nil), kept...))
		if digest2 != digest || (err == nil) != (err2 == nil) || err != nil && err.Error() != err2.Error() || !reflect.DeepEqual(task, again) {
			t.Fatalf("not deterministic: %v / %v, %q / %q", err, err2, digest, digest2)
		}
		if err != nil {
			var e *Error
			if !errors.As(err, &e) || e.Reason == "" || task != nil {
				t.Fatalf("error %v (%T) with task %+v", err, err, task)
			}
			return
		}
		if task == nil || !strings.HasPrefix(digest, DigestPrefix) || !idString(task.ID) {
			t.Fatalf("accepted %+v with digest %q", task, digest)
		}
		for _, s := range []string{task.Goal, task.Decision} {
			if s == "" || utf8.RuneCountInString(s) > MaxWhyRunes {
				t.Fatalf("a line outside its bounds: %q", s)
			}
		}
		if utf8.RuneCountInString(task.Mechanism) > MaxWhyRunes || len(task.ExpectedEvidence) == 0 {
			t.Fatalf("accepted %+v", task)
		}
		for i, k := range task.ExpectedEvidence {
			if !slices.Contains(EvidenceKinds, k) || slices.Contains(task.ExpectedEvidence[:i], k) {
				t.Fatalf("kinds %q", task.ExpectedEvidence)
			}
		}
		before, _ := json.Marshal(task)
		for i := range data {
			data[i] = 0xff
		}
		if after, _ := json.Marshal(task); !bytes.Equal(before, after) {
			t.Fatalf("the Task moved with its input: %s -> %s", before, after)
		}
	})
}
