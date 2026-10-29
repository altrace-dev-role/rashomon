package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// wireHas hands the probe consent.Has as main's wireConsentViews does, for
// this test alone.
func wireHas(t *testing.T, has func(root, unit string) (bool, error)) {
	t.Helper()
	prev := ConsentHas
	ConsentHas = has
	t.Cleanup(func() { ConsentHas = prev })
}

// answerBodies appends the user's answer to `rashomon consent bodies`.
func answerBodies(t *testing.T, root string, on bool) {
	t.Helper()
	if err := consent.Append(root, consent.Record{V: consent.RecordVersion, Via: consent.ViaConsentYes,
		Unit: "bodies", UnitOn: on, At: time.Now().UnixMilli()}); err != nil {
		t.Fatal(err)
	}
}

// grantBodies is `rashomon consent bodies --yes` on root, wired as main
// wires it (card 4.2).
func grantBodies(t *testing.T, root string) {
	t.Helper()
	answerBodies(t, root, true)
	wireHas(t, consent.Has)
}

// sessionCopied runs SessionStart and SessionEnd for a project listed with
// bodies on and reports whether SessionEnd copied its transcript.
func sessionCopied(t *testing.T, st *store.Store) bool {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	cwd := t.TempDir()
	if err := store.WriteBodies(st.Root(), store.BodiesSetting{MaxBytes: 1 << 20, Projects: []string{baseline.Key(cwd)}}); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(t.TempDir(), "rc14m.jsonl")
	if err := os.WriteFile(transcript, []byte(`{"type":"user"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]string{"session_id": "rc14m", "cwd": cwd, "transcript_path": transcript})
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{store.PhaseStart, store.PhaseEnd} {
		if err := runProbe(&Signals{}, phase, bytes.NewReader(payload), st, time.Now, LockWaitTotal); err != nil {
			t.Fatal(err)
		}
	}
	n, _, err := st.BodiesTotal()
	if err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// TestRC14m_NoBodiesConsentNoCopy: a project listed with bodies on is copied
// at SessionEnd only under the user's bodies grant. No answer, an off after a
// grant, and a probe built without the consent reader all copy nothing; the
// grant alone copies, so the three are not passing for another reason.
func TestRC14m_NoBodiesConsentNoCopy(t *testing.T) {
	for _, c := range []struct {
		name   string
		answer func(t *testing.T, root string)
		copied bool
	}{
		{"granted", func(t *testing.T, root string) { grantBodies(t, root) }, true},
		{"no answer", func(t *testing.T, root string) { wireHas(t, consent.Has) }, false},
		{"off after a grant", func(t *testing.T, root string) { grantBodies(t, root); answerBodies(t, root, false) }, false},
		{"no consent reader", func(t *testing.T, root string) { answerBodies(t, root, true); wireHas(t, nil) }, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			st := mustOpen(t)
			c.answer(t, st.Root())
			if got := sessionCopied(t, st); got != c.copied {
				t.Errorf("copied %v, want %v", got, c.copied)
			}
		})
	}
}
