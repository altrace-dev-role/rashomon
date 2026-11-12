//go:build perf

package perf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// unitBriefCase is the post's unit-brief read case (W-M2.3 hook half): the
// post of an Edit of a Go file in a unit the session's prompt named, whose
// brief the session holds for the post to give.
const unitBriefCase = "post.edit.unitbrief"

// unitBriefLogBytes is the size of the log the case's index is built from:
// pgraphPrompt's 3 MiB, this repository's own shape.
const unitBriefLogBytes = 3 << 20

// unitBriefCallers are the case's callers of CountRows, one package each
// from p001 on: more consumer files than the prompt brief's MaxLines lines
// hold, so the unit brief, which repeats no line the prompt brief named
// (engine.Deltas), still has consumers to give. With pgraphPrompt's two,
// the prompt brief names both and p000's unit brief is empty, so no file
// is stored and the post has nothing to read.
var unitBriefCallers = []string{"SumRows", "Twice", "Halve", "Square", "Negate", "Clamp", "Round", "Floor"}

// vouchSymbols adds to countRowsTree's .pgraph at state the symbols file a
// sync writes beside its log and digests, and rewrites the walk file to
// vouch for all three (internal/platform snapshot.go, walk.go): the Go
// producer reads a sync only with one, while the prompt's read of the log
// does not need it. Its rows place CountRows and each of callers, the
// symbols whose files are on disk, in id order (p000, then p001 on); the
// facts' other symbols index unplaced, declared by their unit.
func vouchSymbols(t *testing.T, state string, callers []string) {
	t.Helper()
	logb, err := os.ReadFile(filepath.Join(state, "pgraph.log"))
	if err != nil {
		t.Fatal(err)
	}
	digests, err := os.ReadFile(filepath.Join(state, "pgraph.digests"))
	if err != nil {
		t.Fatal(err)
	}
	var sym strings.Builder
	sym.WriteString("pgraph-symbols v0\ndigests\t" + sha256Hex(digests) + "\n")
	rows := [][3]string{{"p000", "CountRows", "3"}}
	for i, c := range callers {
		rows = append(rows, [3]string{fmt.Sprintf("p%03d", i+1), c, "5"})
	}
	for _, row := range rows {
		pkg := pgraphModule + "/internal/" + row[0]
		sym.WriteString("func:" + pkg + "." + row[1] + "\t" + pkg + "\tinternal/" + row[0] + "/f.go\t" + row[2] + "\t\n")
	}
	symb := []byte(sym.String())
	walk := "pgraph-walk v0\nwalk\t" + sha256Hex(append(append([]byte(nil), logb...), digests...)) +
		"\nlog\t" + sha256Hex(logb) + "\ndigests\t" + sha256Hex(digests) + "\nsymbols\t" + sha256Hex(symb) + "\n"
	if err := os.WriteFile(filepath.Join(state, "pgraph.symbols"), symb, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "pgraph.walk"), []byte(walk), 0o600); err != nil {
		t.Fatal(err)
	}
}

// unitBriefPost returns the payload builder of the unit-brief case and its
// control. The module is pgraphPrompt's (countRowsTree) with
// unitBriefCallers and the symbols file a sync writes (vouchSymbols),
// indexed once by the real graph index
// over a configuration declaring the root a Go unit;
// .pgraph is then moved away, so no measured post reads the log (blastPost's
// read) and the post's one read of the graph side is the unit brief's. A
// session gives a unit's brief once, so each run is a session of its own:
// mk sends the session's prompt, "make CountRows return 2", through recap
// pending, which writes the brief of each unit the prompt's brief names,
// then records the Edit's PreToolUse through the hook entry, both
// unmeasured, and returns the Edit's post, padded in its tool_response to
// exactly size bytes so the case reads the same payload on every machine,
// whatever the length of its temporary directory. The steer disclosure
// marker, the session-content marker (the brief's gate on the prompt side
// and the post side) and a prompt id let the note reach the model. A
// premise run checks the post gives the brief, so a case whose read stood
// down fails rather than reads light; control checks every measured post's
// session gave it, then has mk send, in sessions no run has used, a prompt
// that names nothing the index holds, so no brief is stored (each control
// run checks): the same posts, heard alike, with no brief held, each an
// absent miss (a session already given would read its steer log, which is
// not nothing to read), the figure the row's delta is from.
func (r rig) unitBriefPost(t *testing.T, size int) (mk func(i int) []byte, control func()) {
	t.Helper()
	marker := filepath.Join(r.home, store.FileSteerDisclosure)
	if err := os.WriteFile(marker, []byte(strconv.Itoa(steer.DisclosureVersion)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.home, store.FileSessionContent), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mod, state := countRowsTree(t, unitBriefLogBytes, unitBriefCallers)
	vouchSymbols(t, state, unitBriefCallers)
	if err := os.MkdirAll(filepath.Join(mod, ".rashomon"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mod, ".rashomon", "graph.json"), []byte(`{"units":[{"path":".","lang":"go"}]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	index := exec.Command(rashomonBin, "graph", "index", mod)
	index.Env = r.env
	if out, err := index.CombinedOutput(); err != nil {
		t.Fatalf("rashomon graph index: %v\n%s", err, out)
	}
	if err := os.Rename(state, state+".off"); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(t.TempDir(), "transcript.jsonl")
	file := filepath.Join(mod, "internal", "p000", "f.go")
	edit := map[string]any{"file_path": file, "old_string": "package", "new_string": "package", "replace_all": false}
	session := func(i int) string { return payload.Session[:len(payload.Session)-2] + fmt.Sprintf("%02x", i) }
	run := func(command []string, in []byte) []byte {
		cmd := exec.Command(rashomonBin, r.args(command)...)
		var stderr bytes.Buffer
		cmd.Stdin, cmd.Env, cmd.Stderr = bytes.NewReader(in), r.env, &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("rashomon %s: %v\n%s", strings.Join(command, " "), err, stderr.Bytes())
		}
		return out
	}
	prompt := func(sid, text string) []byte {
		var fields map[string]any
		if err := json.Unmarshal(payload.Prompt(text), &fields); err != nil {
			t.Fatal(err)
		}
		fields["session_id"], fields["cwd"] = sid, mod
		b, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	event := func(sid, name, id, pad string) []byte {
		p := map[string]any{"hook_event_name": name, "session_id": sid, "prompt_id": "p1", "transcript_path": transcript,
			"cwd": mod, "permission_mode": "default", "tool_name": "Edit", "tool_input": edit, "tool_use_id": id}
		if name == "PostToolUse" {
			p["tool_response"] = map[string]any{"stdout": "ok\n" + pad, "stderr": "", "interrupted": false}
		}
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	held := true
	mk = func(i int) []byte {
		if !held {
			i += 0x40
		}
		sid, id := session(i), fmt.Sprintf("toolu_ub_%03d", i)
		if held {
			run([]string{"recap", "pending"}, prompt(sid, "make CountRows return 2"))
		} else {
			run([]string{"recap", "pending"}, prompt(sid, "tidy the wording of the readme"))
			if _, err := os.Stat(graph.UnitBriefDir(r.home, sid)); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("control: the prompt that names nothing stored a unit brief (%v)", err)
			}
		}
		run([]string{"hook"}, event(sid, "PreToolUse", id, ""))
		fixed := len(event(sid, "PostToolUse", id, ""))
		if fixed > size {
			t.Fatalf("the post's fixed part, %d bytes, is over %d", fixed, size)
		}
		p := event(sid, "PostToolUse", id, strings.Repeat("x", size-fixed))
		if len(p) != size {
			t.Fatalf("payload is %d bytes, want %d", len(p), size)
		}
		return p
	}
	st, err := store.Open(r.home)
	if err != nil {
		t.Fatal(err)
	}
	gave := func(i int) bool {
		lines, _, err := st.ReadSteer(session(i))
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range lines {
			var l struct{ Pattern, State, Key string }
			if json.Unmarshal(b, &l) == nil && l.Pattern == "task-preflight" && l.State == string(steer.LineStateDelivered) && strings.HasPrefix(l.Key, "unitbrief:") {
				return true
			}
		}
		return false
	}
	// why is what the session left: its misses, the briefs its prompt
	// stored and its steer lines, so a premise that fails names the side
	// that stood down.
	why := func(i int) string {
		var b strings.Builder
		if rn, err := st.ReadRun(session(i)); err == nil {
			for _, m := range rn.UnitBriefMisses {
				fmt.Fprintf(&b, "miss %s %s\n", m.Unit, m.Reason)
			}
		}
		ents, err := os.ReadDir(graph.UnitBriefDir(r.home, session(i)))
		fmt.Fprintf(&b, "unit-brief directory: %d files (%v)\n", len(ents), err)
		lines, _, _ := st.ReadSteer(session(i))
		for _, l := range lines {
			var s struct{ Pattern, State, Key string }
			if json.Unmarshal(l, &s) == nil {
				fmt.Fprintf(&b, "steer %s %s %s\n", s.Pattern, s.State, s.Key)
			}
		}
		return b.String()
	}
	if out := run([]string{"post"}, mk(0x3f)); !gave(0x3f) {
		t.Fatalf("premise: the post gave no unit brief, so it read none:\n%s\n%s", out, why(0x3f))
	}
	if ents, err := os.ReadDir(graph.UnitBriefDir(r.home, session(0x3f))); err == nil {
		for _, e := range ents {
			if info, err := e.Info(); err == nil {
				t.Logf("%s: the session holds a unit brief of %d bytes", unitBriefCase, info.Size())
			}
		}
	}
	return mk, func() {
		for i := 0; i < runs; i++ {
			if !gave(i) {
				t.Fatalf("measured post %d gave no unit brief, so it read none", i)
			}
		}
		held = false
	}
}
