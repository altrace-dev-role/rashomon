package hook

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// lu1Cases are lines and the directory they start from, with whether the
// fold of their leading cd steps is one Locate cannot read.
var lu1Cases = []struct {
	name, cmd, cwd string
	unread         bool
}{
	{"cd DIR; is not guarded, so not read", "cd sub; make test 2>&1 | tail", "/repo", true},
	{"a variable target", "cd $REPO && make test | tail", "/repo", true},
	{"a quoted target", `cd "sub" && make test`, "/repo", true},
	{"cd - names the previous directory", "cd - && make test", "/repo", true},
	{"an unread fold with no payload cwd is still unread", "cd $REPO; make test", "", true},
	{"cd DIR && is read", "cd sub && make test", "/repo", false},
	{"a read home step", "cd ~/x && make test", "/repo", false},
	{"no cd at all", "make test | tail", "/repo", false},
	{"no cd, no cwd", "make test", "", false},
	{"read steps with no payload cwd are an unknown location, not an unread one", "cd sub && make test", "", false},
	{"past the read bound is a cut fold, not an unread step", strings.Repeat("cd a && ", maxLeadingCd+1) + "make test", "/repo", false},
}

// TestLU1_AnUnreadFoldIsRecorded (5.14b): the declaration says, in one bit,
// that the line's leading cd steps were ones Locate does not read, which its
// cwd_digest (a keyed digest the report has no key to open) cannot say. Break:
// never set the bit, or set it for any line that has a cd.
func TestLU1_AnUnreadFoldIsRecorded(t *testing.T) {
	t.Setenv("HOME", "/home/loc")
	r := newRecorder(t)
	for _, c := range lu1Cases {
		d := r.capture(c.cmd, c.cwd)
		if d.LocationUnread != c.unread {
			t.Errorf("%s: %q from %q recorded location_unread %v, want %v", c.name, short(c.cmd), c.cwd, d.LocationUnread, c.unread)
		}
		if d.SchemaVersion != store.CallSchemaVersion {
			t.Errorf("%s: schema_version %d, want %d", c.name, d.SchemaVersion, store.CallSchemaVersion)
		}
	}
}
