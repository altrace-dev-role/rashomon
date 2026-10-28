package hook

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestRC14g_SessionEndCopiesWhenOn: with the switch on for the session's
// project, SessionEnd leaves a byte copy of the transcript under
// bodies/<project>/<session>.jsonl, the project named as verify names its
// index files, less the extension; SessionStart on the same payload copies
// nothing.
func TestRC14g_SessionEndCopiesWhenOn(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st := mustOpen(t)
	cwd := t.TempDir()
	key := baseline.Key(cwd)
	if err := store.WriteBodies(st.Root(), store.BodiesSetting{MaxBytes: 1 << 20, Projects: []string{key}}); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"type":"user","message":{"content":"hi"}}` + "\n" + `{"type":"assistant"`)
	transcript := filepath.Join(t.TempDir(), "rc14g.jsonl")
	if err := os.WriteFile(transcript, body, 0o600); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]string{"session_id": "rc14g", "cwd": cwd, "transcript_path": transcript})
	if err != nil {
		t.Fatal(err)
	}
	bodies := filepath.Join(st.Root(), store.DirBodies)

	if err := runProbe(&Signals{}, store.PhaseStart, bytes.NewReader(payload), st, time.Now, LockWaitTotal); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bodies); !os.IsNotExist(err) {
		t.Fatalf("SessionStart touched %s (%v)", store.DirBodies, err)
	}

	if err := runProbe(&Signals{}, store.PhaseEnd, bytes.NewReader(payload), st, time.Now, LockWaitTotal); err != nil {
		t.Fatal(err)
	}
	dir := strings.TrimSuffix(baseline.FileName(key), baseline.FileExt)
	got, err := os.ReadFile(filepath.Join(bodies, dir, st.DirName("rc14g")+".jsonl"))
	if err != nil {
		t.Fatalf("no copy after SessionEnd: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("the copy is %q, want the transcript's bytes %q", got, body)
	}
}

// TestRC14e_OnlySessionEndTouchesBodies: the PRD's claim that copying costs
// no tool call any wall time, made checkable without a benchmark: CopyBody's
// one caller in the module's code is internal/hook/probe.go, which only
// SessionEnd reaches with a transcript path. A call from the Stop path, the
// recorders or a CLI command fails here, and names its file.
func TestRC14e_OnlySessionEndTouchesBodies(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	var callers []string
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(f string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() || !strings.HasSuffix(f, ".go") || strings.HasSuffix(f, "_test.go") {
				return err
			}
			src, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			scanned++
			for _, line := range strings.Split(string(src), "\n") {
				if strings.Contains(line, "CopyBody(") && !strings.HasPrefix(strings.TrimSpace(line), "func ") &&
					!strings.HasPrefix(strings.TrimSpace(line), "//") {
					rel, _ := filepath.Rel(root, f)
					callers = append(callers, filepath.ToSlash(rel))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if scanned < 100 {
		t.Fatalf("scanned %d files: the walk did not reach the module's code", scanned)
	}
	if len(callers) != 1 || callers[0] != "internal/hook/probe.go" {
		t.Errorf("CopyBody is called from %v, want internal/hook/probe.go alone (SessionEnd)", callers)
	}
}
