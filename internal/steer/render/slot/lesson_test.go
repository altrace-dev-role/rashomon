package slot

import (
	"errors"
	"strings"
	"testing"
)

// TestSTR11b_LessonIDCharset: a lesson id is learn's id form, verify.SlotID's
// charset `^[a-z0-9][a-z0-9._:@-]{0,63}$`, and renders as it is, unquoted.
// The charset is not the whole check: an id comes with an import row, so it
// runs the secret screens too, and an id in the charset shaped like
// `user:token@` or a 64-hex digest is refused as secret-shaped. learn.Message
// runs the same check at approval, so no such id is ever approved.
func TestSTR11b_LessonIDCharset(t *testing.T) {
	checkSlot(t,
		[]LessonID{"go-test-cwd", "x", "a.b_c@d:e-f", "0", LessonID(strings.Repeat("x", 64))},
		[]LessonID{"", "Go-test", "-lead", ".lead", "has space", "it's", "#unchecked", LessonID(strings.Repeat("a", 65))})
	for _, id := range []LessonID{"a.b_c:d@e-f", LessonID(strings.Repeat("a", 64))} {
		if err := id.Validate(); !errors.Is(err, ErrSecretShaped) {
			t.Errorf("LessonID(%s).Validate() = %v, want the secret screen's refusal", id, err)
		}
	}
	if got := LessonID("go-test-cwd").Text(); got != "go-test-cwd" {
		t.Errorf("LessonID.Text() = %q, want it unquoted", got)
	}
}

// TestLRN17i_HostScreen: RefuseHosts refuses a word that is a bare hostname,
// holds a dot and ends in a label on the closed list (L13 as ruled), with
// trailing punctuation trimmed. File names and version strings, whose last
// label is off the list, pass; so does a host under an unlisted label, the
// stated limit. A refusal names the word.
func TestLRN17i_HostScreen(t *testing.T) {
	for _, w := range []string{"go.mod", "main.go", "package.json", "setup.py", "v1.2.3"} {
		for _, s := range []string{w, "edit " + w + ".", "after " + w + ", run it", "see " + w + ";", "(" + w + ")"} {
			if err := RefuseHosts(s); err != nil {
				t.Errorf("RefuseHosts(%q) = %v, want nil", s, err)
			}
		}
	}
	for _, s := range []string{"run go mod tidy after editing go.mod", "internal/verify", "ratio 3:1", "", "run go test from the module root"} {
		if err := RefuseHosts(s); err != nil {
			t.Errorf("RefuseHosts(%q) = %v, want nil", s, err)
		}
	}
	for _, w := range []string{"build.example.com", "db.corp", "api.internal", "Build.Example.COM"} {
		for _, s := range []string{w, "push to " + w + ".", "on " + w + ", then", "(" + w + ")"} {
			err := RefuseHosts(s)
			if err == nil || !strings.Contains(err.Error(), w) {
				t.Errorf("RefuseHosts(%q) = %v, want a refusal naming %q", s, err, w)
			}
		}
	}
	if err := RefuseHosts("cache at files.example.museum"); err != nil {
		t.Errorf("a host under an unlisted label was refused (%v); the closed list is the stated limit", err)
	}
}
