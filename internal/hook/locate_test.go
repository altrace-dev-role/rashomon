package hook

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// recorder captures shell calls through the hook, one session in one store,
// and returns each call's declaration as the store holds it.
type recorder struct {
	t  *testing.T
	st *store.Store
	n  int
}

func newRecorder(t *testing.T) *recorder {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &recorder{t: t, st: st}
}

func (r *recorder) capture(cmd, cwd string) store.Declaration {
	r.t.Helper()
	payload, err := json.Marshal(map[string]any{
		"session_id": "s-loc", "tool_use_id": fmt.Sprintf("toolu_%d", r.n), "tool_name": "Bash",
		"cwd": cwd, "hook_event_name": "PreToolUse", "tool_input": map[string]any{"command": cmd},
	})
	if err != nil {
		r.t.Fatal(err)
	}
	r.n++
	h := New(r.st, time.Now)
	h.Close(&Signals{}, h.Capture(strings.NewReader(string(payload))))
	run, err := r.st.ReadRun("s-loc")
	if err != nil || len(run.Declarations) != r.n {
		r.t.Fatalf("%q: %d declarations, %v", cmd, len(run.Declarations), err)
	}
	return run.Declarations[r.n-1]
}

// short is a line cut to fit a test message.
func short(cmd string) string {
	if len(cmd) > 60 {
		return cmd[:40] + "..." + cmd[len(cmd)-15:]
	}
	return cmd
}

// TestLocation: the cwd digest Capture records is the directory the line's
// runner starts in where the leading cd steps say it: the payload's cwd with
// the steps leadingCd reads folded on, `~` and `~/...` expanded with the
// hook's own home; past MaxLeadingCd steps it is unknown, ""; and for a fold
// the program search passes but leadingCd does not read, a digest in a
// domain of its own, never a read directory's and never "" while the
// payload's cwd is known. Break: join `~` under the cwd, take `cd DIR;` as
// read, keep the payload's cwd for an unread fold, or key one with no payload
// cwd on the fold alone, and a row here names the wrong directory.
func TestLocation(t *testing.T) {
	t.Setenv("HOME", "/home/loc")
	r := newRecorder(t)
	key := r.st.Key()
	const cwd = "/tmp/project"
	const unread = "unread"
	// Every directory a row below could have been keyed on: an unread
	// location equals none of them.
	read := []string{cwd, cwd + "/sub", cwd + "/x", cwd + "/~/x", cwd + "/~", "/tmp", "/a", "/b", "/x",
		"/home/loc", "/home/loc/x", "/home/loc/x/sub", "/home/user/x"}
	for _, tc := range []struct{ cmd, want string }{
		{"cd ~/x && go test ./...", "/home/loc/x"},
		{"cd ~ && go test ./...", "/home/loc"},
		{"cd /a && cd ~ && go test ./...", "/home/loc"},
		{"cd ~/x && cd sub && go test ./...", "/home/loc/x/sub"},
		{"cd ~/x && ls", "/home/loc/x"},
		{"cd /tmp && go test ./...", "/tmp"},
		{"cd sub && go test ./...", cwd + "/sub"},

		{"cd ~user/x && go test ./...", unread},
		{"cd ~+/x && go test ./...", unread},
		{"cd ~-/x && go test ./...", unread},
		// `cd DIR;` stays unread: the shell does not guard the fold, so DIR is
		// where the line meant the runner to run, not where it ran.
		{"cd /tmp; go test ./...", unread},
		{"cd $REPO && go test ./...", unread},
		{"cd - && go test ./...", unread},
		{"cd /x && cd - && go test ./...", unread},
		{`cd "/tmp" && go test ./...`, unread},
		{"cd -P /tmp && go test ./...", unread},
		{"X=1 cd /tmp && go test ./...", unread},
		{"cd /a && cd $B && go test ./...", unread},
		{"(cd /tmp && go test ./...)", unread},
		{"{ cd /tmp && go test ./...; }", unread},
		{"cd && go test ./...", unread},
		{"cd /a && pushd /b && go test ./...", unread},
		{strings.Repeat("cd $D && ", maxLeadingCd+1) + "go test ./...", unread},

		{"go test ./...", cwd},
		// The start directory, which is what a gate's directory names; the
		// option is in the runner's identity.
		{"make test -C sub", cwd},
		{"go test -C sub ./...", cwd},
		{"cd /repo\r && ls", cwd},
		// ACCEPTED GAP: pushd is not a fold (the program search names pushd
		// as the program), so the line keeps the payload's cwd, though the
		// runner starts in /b. Its runner digest is null, so no runner
		// family takes it in.
		{"pushd /b && go test ./...", cwd},
		// ACCEPTED GAP: a comment ending in a backslash continues nothing to
		// the shell, but the program search's reading joins the next line to
		// it, so the steps it reads stop short of where the runner starts
		// (/a/b, /a, /a/b). The two readings fold apart there, so the runner
		// digest is null (shape's TestFoldKeepsRunnerIdentity) and no runner
		// family takes the line in.
		{"cd a && # c\\\ncd b && go test ./...", cwd + "/a"},
		{"# c\\\ncd a && go test ./...", cwd},
		{"cd a && #\\\n cd b && go test ./...", cwd + "/a"},

		{strings.Repeat("cd a && ", maxLeadingCd+1) + "go test ./...", ""},
	} {
		got := r.capture(tc.cmd, cwd).CWDDigest
		switch tc.want {
		case unread:
			if got == "" {
				t.Errorf("%q: cwd digest empty; want an unread location", short(tc.cmd))
			}
			for _, d := range read {
				if got == shape.CWDDigest(key, d) {
					t.Errorf("%q: cwd digest is %s's; want an unread location", short(tc.cmd), d)
				}
			}
		case "":
			if got != "" {
				t.Errorf("%q: cwd digest set; want unknown, empty", short(tc.cmd))
			}
		default:
			if want := shape.CWDDigest(key, tc.want); got != want {
				t.Errorf("%q: cwd digest is not %s's", short(tc.cmd), tc.want)
			}
		}
	}
	// With no payload cwd an unread fold is unknown too: keyed on the fold
	// alone, the same line run in two projects would be one location.
	if got := r.capture("cd $REPO && go test ./...", "").CWDDigest; got != "" {
		t.Errorf("an unread fold with no payload cwd: cwd digest set; want unknown, empty")
	}
}

// TestUnreadLocationDomain: an unread fold's location is keyed on the payload's
// cwd and the fold as the tokenizer reads it, and on nothing else: the same
// fold from the same cwd is one location, whatever follows it on the line and
// however it is spaced; another cwd, other words, other quoting or another
// separator are another. Break: leave the cwd out, and two unread folds from
// two projects are one location (two runs of `cd $REPO && go test` in two
// repositories pair); leave the words out, and `cd $A` and `cd $B` are; drop
// the quoting byte, and `cd "$REPO"` is `cd $REPO`, which splits the
// variable's value where the quoted form does not.
func TestUnreadLocationDomain(t *testing.T) {
	r := newRecorder(t)
	a := r.capture("cd $REPO && go test ./...", "/tmp/project").CWDDigest
	again := r.capture("cd $REPO && go test ./...", "/tmp/project").CWDDigest
	piped := r.capture("cd $REPO && go test ./... 2>&1 | tail -5", "/tmp/project").CWDDigest
	spaced := r.capture("cd  $REPO && go test ./...", "/tmp/project").CWDDigest
	other := r.capture("cd $REPO && go test ./...", "/srv/b").CWDDigest
	words := r.capture("cd $OTHER && go test ./...", "/tmp/project").CWDDigest
	quoted := r.capture(`cd "$REPO" && go test ./...`, "/tmp/project").CWDDigest
	braced := r.capture("cd ${REPO} && go test ./...", "/tmp/project").CWDDigest
	semi := r.capture("cd $REPO; go test ./...", "/tmp/project").CWDDigest
	plain := r.capture("go test ./...", "/tmp/project").CWDDigest
	if a == "" || a != again || a != piped || a != spaced {
		t.Errorf("one fold from one cwd: set %v, equal again %v, with a different rest %v, spaced apart %v; want all true",
			a != "", a == again, a == piped, a == spaced)
	}
	if a == other || a == words || a == quoted || a == braced || a == semi || a == plain {
		t.Errorf("equal from another cwd %v, with other words %v, quoted %v, braced %v, with `;` %v, to the plain run's %v; want all false",
			a == other, a == words, a == quoted, a == braced, a == semi, a == plain)
	}
}

// TestLocationFamilies: runs of one runner group on (runner digest, cwd
// digest), the key a runner family is built on, and on six calls from two
// directories that key merges no two runs that started in different
// directories. A run's real directory is known here by construction. The
// one split is `cd sub; go test` from /p, which ran in /p/sub but whose `;`
// fold is unread, so it is a family of its own. Break: keep the payload's cwd
// for an unread fold, and `cd $REPO && go test` from /p joins `go test` run in
// /p; drop the runner digest of an unread fold, and it is in no family.
func TestLocationFamilies(t *testing.T) {
	r := newRecorder(t)
	calls := []struct{ cmd, cwd, ran string }{
		{"go test ./...", "/p/sub", "/p/sub"},
		{"cd sub && go test ./...", "/p", "/p/sub"},
		{"cd .. && go test ./...", "/p/sub", "/p"},
		{"go test ./...", "/p", "/p"},
		{"cd sub; go test ./...", "/p", "/p/sub"},
		{"cd $REPO && go test ./...", "/p", "$REPO"},
	}
	groups := map[[2]string][]int{}
	var keys [][2]string
	for i, c := range calls {
		d := r.capture(c.cmd, c.cwd)
		if d.Shape.RunnerDigest == nil {
			t.Errorf("%q: no runner digest; every call here runs go test", c.cmd)
			continue
		}
		k := [2]string{*d.Shape.RunnerDigest, d.CWDDigest}
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], i)
	}
	var got [][]int
	merges := 0
	for _, k := range keys {
		g := groups[k]
		got = append(got, g)
		for _, i := range g[1:] {
			if calls[i].ran != calls[g[0]].ran {
				merges++
			}
		}
	}
	want := [][]int{{0, 1}, {2, 3}, {4}, {5}}
	if merges != 0 || !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("groups %v with %d false merges; want %v, 0", got, merges, want)
	}
}

// TestLocateIsWhatCaptureRecords: Capture records Locate's shape and
// location, under the store's key, so the report's tests that build records
// through Locate read what the hook writes. Break: compute the location in
// Capture apart from Locate, and the two drift.
func TestLocateIsWhatCaptureRecords(t *testing.T) {
	t.Setenv("HOME", "/home/loc")
	r := newRecorder(t)
	for _, c := range [][2]string{
		{"cd ~/x && go test ./... 2>&1 | tail -20", "/tmp/project"},
		{"cd $REPO && make test | tail", "/tmp/project"},
		{"cd /tmp; go test ./...", "/srv/b"},
		{"cd sub && go test ./...", ""},
		{strings.Repeat("cd a && ", maxLeadingCd+1) + "go test ./...", "/tmp/project"},
		{"go test ./...", "/tmp/project"},
	} {
		d := r.capture(c[0], c[1])
		in, err := json.Marshal(map[string]string{"command": c[0]})
		if err != nil {
			t.Fatal(err)
		}
		s, loc := Locate(r.st.Key(), "Bash", in, c[1])
		if loc != d.CWDDigest || s.Digest != d.Shape.Digest || !sameDigest(s.RunnerDigest, d.Shape.RunnerDigest) {
			t.Errorf("%q from %q: Locate's location equal %v, shape digest equal %v, runner digest equal %v; want all true",
				short(c[0]), c[1], loc == d.CWDDigest, s.Digest == d.Shape.Digest, sameDigest(s.RunnerDigest, d.Shape.RunnerDigest))
		}
	}
}

func sameDigest(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
