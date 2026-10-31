package hook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestSR1ae_ANoSpecNoteSkipsARulesOnlyDiscoveryForItsTurnAndDirectory
// (D210): a forced push that exits 0 is a rules-only call. A note that the
// turn's preflight found no spec, for this turn and this directory, makes its
// post skip the discovery; in every other state the post discovers for real
// and raises the rule. The skip is the narrowing the card records: a spec
// that appeared after the preflight looked is raised at the Stop, not at the
// call, and it is untrusted by construction. Break: use the note across
// turns, ignore the directory, read a missing or a malformed note as none,
// or leave the note standing after a call that named .claude/rashomon.
func TestSR1ae_ANoSpecNoteSkipsARulesOnlyDiscoveryForItsTurnAndDirectory(t *testing.T) {
	push := map[string]any{"command": "git push --force origin main"}
	for _, c := range []struct {
		name   string
		note   func(f *stFix) // the note, written before the call
		raises bool
	}{
		{"no note", func(*stFix) {}, true},
		{"a note for this turn and directory", func(f *stFix) {
			note(t, f, "p1", f.cwd)
		}, false},
		{"a note of another turn", func(f *stFix) {
			note(t, f, "p0", f.cwd)
		}, true},
		{"a note from another directory", func(f *stFix) {
			note(t, f, "p1", filepath.Join(f.cwd, "elsewhere"))
		}, true},
		{"a malformed note", func(f *stFix) {
			writeNote(t, f, "none\np1\n")
		}, true},
		{"a note with no word none", func(f *stFix) {
			writeNote(t, f, "p1\n"+specNoneDigest(f.st.Key(), f.cwd, "", "")+"\n")
		}, true},
		{"a note with trailing bytes", func(f *stFix) {
			writeNote(t, f, "none\np1\n"+specNoneDigest(f.st.Key(), f.cwd, "", "")+"\nmore\n")
		}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newSRFix(t, srSpec(srNoForce))
			c.note(f)
			p := f.call("toolu_r", "Bash", push, 0)
			b := p.Steer(stEnv(0))
			if got := b != nil; got != c.raises {
				t.Fatalf("printed %q; the rule raised = %v, want %v", b, got, c.raises)
			}
		})
	}

	t.Run("a call naming .claude/rashomon clears the note", func(t *testing.T) {
		f := newSRFix(t, srSpec(srNoForce))
		note(t, f, "p1", f.cwd)
		naming := map[string]any{"command": "git push --force origin main # .claude/rashomon"}
		f.call("toolu_n", "Bash", naming, 0).Steer(stEnv(0))
		if _, err := os.Stat(filepath.Join(f.st.RunDir(stSession), store.FileSpecNone)); err == nil {
			t.Error("the note stands after a call that named .claude/rashomon")
		}
		if b := f.call("toolu_r", "Bash", push, 0).Steer(stEnv(0)); b == nil {
			t.Error("the next call raised nothing: the note still skipped its discovery")
		}
	})

	t.Run("a found spec is never noted", func(t *testing.T) {
		f := newSRFix(t, srSpec(srNoForce))
		note(t, f, "p1", f.cwd)
		NoteSpecFound(f.st, stSession, "p1", f.cwd, "", "", &spec.Compiled{})
		if f.st.NoSpecNoted(stSession, "p1", specNoneDigest(f.st.Key(), f.cwd, "", "")) {
			t.Error("a found spec left the note standing")
		}
		if b := f.call("toolu_r", "Bash", push, 0).Steer(stEnv(0)); b == nil {
			t.Error("a found spec's rule raised nothing")
		}
		NoteSpecFound(f.st, stSession, "p1", f.cwd, "", "", &spec.Compiled{})
		if _, err := os.Stat(filepath.Join(f.st.RunDir(stSession), store.FileSpecNone)); err == nil {
			t.Error("a found spec wrote a note")
		}
	})
}

func note(t *testing.T, f *stFix, prompt, cwd string) {
	t.Helper()
	if err := f.st.NoteNoSpec(stSession, prompt, specNoneDigest(f.st.Key(), cwd, "", "")); err != nil {
		t.Fatal(err)
	}
}

func writeNote(t *testing.T, f *stFix, content string) {
	t.Helper()
	dir := f.st.RunDir(stSession)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, store.FileSpecNone), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
