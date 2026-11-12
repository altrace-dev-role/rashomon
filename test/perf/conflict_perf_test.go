//go:build perf

package perf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// conflictCase is the post's conflict check case (W-M2.4 seam): the post of
// a Write of a Go file a protect_path rule covers, the conflict base the
// session's prompt wrote held.
const conflictCase = "post.edit.conflict"

// conflictSpecDoc is the case's spec: one protect_path rule over p000's Go
// files, so the base holds the rule and those files' digests.
const conflictSpecDoc = `{"rashomon_spec": 1, "rules": [{"id": "keep", "kind": "protect_path", "paths": ["internal/p000/*.go"], "why": "perf row: the post's conflict check", "hard_control": "CI"}]}`

// conflictCaseNote is the note the case's post gives.
const conflictCaseNote = "raises protected_path under spec rule keep."

// conflictPost returns the payload builder of the conflict case and its
// control. The module is unitBriefPost's (countRowsTree with
// unitBriefCallers, vouchSymbols), indexed once by the real graph index over
// a configuration declaring the root a Go unit; .pgraph is then moved away,
// so no measured post reads the log. conflictSpecDoc is trusted in the
// rig's store as `rashomon spec trust` binds it, and p000's f.go is then
// rewritten on disk, so the file differs from the digest its index and the
// base hold. A session gives a conflict's note once, so each run is a
// session of its own: mk sends the session's prompt, one that names nothing
// the index holds, through recap pending, which stores no unit brief and
// writes the conflict base, records the Write's PreToolUse through the hook
// entry, both unmeasured, and returns the Write's post, padded in its
// tool_response to exactly size bytes so the case reads the same payload on
// every machine. Each measured post reads the base, diffs the file against
// it, records one conflict_check of one protected_path row and gives the
// note. A premise run checks the note; control checks each measured post's
// session recorded its row, then has mk remove each base after its prompt,
// in sessions no run has used: the same posts with the base absent, the
// unit brief an absent miss in both, the figure the row's delta is from.
func (r *rig) conflictPost(t *testing.T, size int) (mk func(i int) []byte, control func()) {
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
	path := filepath.Join(mod, ".claude", "rashomon", "spec.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(conflictSpecDoc), 0o600); err != nil {
		t.Fatal(err)
	}
	_, digest, err := spec.Parse([]byte(conflictSpecDoc))
	if err != nil {
		t.Fatal(err)
	}
	bound := &spec.Bindings{}
	bound.Bind(spec.Binding{ProjectRoot: hook.SpecProject(path), SpecPath: path, SpecDigest: digest, BoundAtMS: 1})
	if err := hook.WriteSpecBindings(r.home, bound); err != nil {
		t.Fatal(err)
	}
	r.env = append(append([]string(nil), r.env...), "CLAUDE_PROJECT_DIR="+mod)
	file := filepath.Join(mod, "internal", "p000", "f.go")
	const body = "package p000\n\nfunc CountRows() int { return 2 }\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(t.TempDir(), "transcript.jsonl")
	write := map[string]any{"file_path": file, "content": body}
	session := func(i int) string { return payload.Session[:len(payload.Session)-2] + fmt.Sprintf("%02x", i) }
	base := func(sid string) string {
		return filepath.Join(graph.UnitBriefDir(r.home, sid), graph.ConflictBaseFileName(mod))
	}
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
	prompt := func(sid string) []byte {
		var fields map[string]any
		if err := json.Unmarshal(payload.Prompt("tidy the wording of the readme"), &fields); err != nil {
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
			"cwd": mod, "permission_mode": "default", "tool_name": "Write", "tool_input": write, "tool_use_id": id}
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
		sid, id := session(i), fmt.Sprintf("toolu_cf_%03d", i)
		run([]string{"recap", "pending"}, prompt(sid))
		if _, err := os.Stat(base(sid)); err != nil {
			t.Fatalf("the prompt left no conflict base: %v", err)
		}
		if !held {
			if err := os.Remove(base(sid)); err != nil {
				t.Fatal(err)
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
	// rows is the row count of each conflict_check session i recorded.
	rows := func(i int) []int {
		rn, err := st.ReadRun(session(i))
		if err != nil {
			t.Fatal(err)
		}
		var n []int
		for _, c := range rn.ConflictChecks {
			n = append(n, len(c.Rows))
		}
		return n
	}
	if out := run([]string{"post"}, mk(0x3f)); !bytes.Contains(out, []byte(conflictCaseNote)) {
		t.Fatalf("premise: the post printed %q, want the conflict's note; its checks' rows %v", out, rows(0x3f))
	}
	if info, err := os.Stat(base(session(0x3f))); err == nil {
		t.Logf("%s: the session holds a conflict base of %d bytes", conflictCase, info.Size())
	}
	return mk, func() {
		for i := 0; i < runs; i++ {
			if got := rows(i); len(got) != 1 || got[0] != 1 {
				t.Fatalf("measured post %d left checks of %v rows, want one check of one row", i, got)
			}
		}
		held = false
	}
}
