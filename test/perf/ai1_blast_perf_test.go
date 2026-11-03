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
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// blastCase is the post's blast-radius read case (ARCH-INLINE1): an Edit of
// a Go file in a module whose .pgraph holds a vouched 3 MiB log.
const blastCase = "post.edit.pgraph3MiB"

// blastEnv is what a session steering speaks to carries (test/acceptance's
// stAgent): a harness at the floor whose PostToolUse context reaches the
// model, and CI unset, as a runner sets it.
var blastEnv = []string{"AI_AGENT=claude-code_2-1-280_harness", "CI="}

// blastNote is the note an Edit of p000's f.go gives in blastPost's module.
const blastNote = "calling into the package this call edited (at least 1): 'internal/p051'; test files are in 1 of them."

// blastPost returns the payload builder of the blast-radius case and its
// control. The module is pgraphStop's, a log in this repository's shape,
// with p051's files on disk as the digests list them: p051 calls into p000
// ((51*7+3)%120 == 0), so an Edit of p000's f.go has one fresh caller to
// name. A session reads a package's callers once per log, so each run is the
// Edit in a session of its own: mk records the call's PreToolUse through the
// hook entry, unmeasured, and returns its post. The steer disclosure marker
// and a prompt id let the note reach the model. A premise run checks the
// post gives the note, so a case whose read stood down fails rather than
// reads light; control checks every measured post's session gave it, then
// moves .pgraph away: the same posts with nothing to read, the figure the
// row's delta is from.
func (r rig) blastPost(t *testing.T, size int) (mk func(i int) []byte, control func()) {
	t.Helper()
	marker := filepath.Join(r.home, store.FileSteerDisclosure)
	if err := os.WriteFile(marker, []byte(strconv.Itoa(steer.DisclosureVersion)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := map[string]string{
		"go.mod":                  "module " + pgraphModule + "\n\ngo 1.25.0\n",
		"internal/p000/f.go":      "package p000\n\nfunc Func0000() {}\n",
		"internal/p000/f_test.go": "package p000\n",
	}
	for _, f := range []string{"f.go", "f_test.go", "g.go", "h.go"} {
		rel := "internal/p051/" + f
		src[rel] = rel // pgraphFiles lists a file src lacks under the sha256 of its name
	}
	mod, state := pgraphTree(t, size, src, nil)
	transcript := filepath.Join(t.TempDir(), "transcript.jsonl")
	edit := map[string]any{"file_path": filepath.Join(mod, "internal", "p000", "f.go"), "old_string": "package",
		"new_string": "package", "replace_all": false}
	session := func(i int) string { return payload.Session[:len(payload.Session)-2] + fmt.Sprintf("%02x", i) }
	calls := 0
	event := func(sid, name string) []byte {
		p := map[string]any{"hook_event_name": name, "session_id": sid, "prompt_id": "p1", "transcript_path": transcript,
			"cwd": mod, "permission_mode": "default", "tool_name": "Edit", "tool_input": edit,
			"tool_use_id": fmt.Sprintf("toolu_blast_%03d", calls)}
		if name == "PostToolUse" {
			p["tool_response"] = map[string]any{"stdout": "ok\n", "stderr": "", "interrupted": false}
		}
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	run := func(entry string, in []byte) []byte {
		cmd := exec.Command(rashomonBin, r.args([]string{entry})...)
		var stderr bytes.Buffer
		cmd.Stdin, cmd.Env, cmd.Stderr = bytes.NewReader(in), r.env, &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("rashomon %s: %v\n%s", entry, err, stderr.Bytes())
		}
		return out
	}
	mk = func(i int) []byte {
		calls++
		run("hook", event(session(i), "PreToolUse"))
		return event(session(i), "PostToolUse")
	}
	if out := run("post", mk(0xff)); !bytes.Contains(out, []byte(blastNote)) {
		t.Fatalf("premise: the post does not give the blast-radius note, so it read no log:\n%s", out)
	}
	st, err := store.Open(r.home)
	if err != nil {
		t.Fatal(err)
	}
	return mk, func() {
		for i := 0; i < runs; i++ {
			lines, _, err := st.ReadSteer(session(i))
			if err != nil {
				t.Fatal(err)
			}
			given := false
			for _, b := range lines {
				var l struct{ Pattern, State string }
				given = given || json.Unmarshal(b, &l) == nil && l.Pattern == "blast-radius" && l.State == string(steer.LineStateDelivered)
			}
			if !given {
				t.Fatalf("measured post %d gave no blast-radius note, so it read no log", i)
			}
		}
		if err := os.Rename(state, state+".off"); err != nil {
			t.Fatal(err)
		}
	}
}
