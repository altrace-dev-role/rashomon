package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// postLineRow is one way a post owes a line of its own: the fixture, the
// post under an arm, the line it owes, and the next edit, of the same kind,
// which says the line unless the post took its marker.
type postLineRow struct {
	name string
	fix  func(t *testing.T) *ovFix
	post func(f *ovFix, arm steer.Arm) []byte
	want string
	next string
}

// postLineRows are the rows: an unclaimed kind's edit that raised nothing
// (the steer step's exit before the route step), the same edit with a
// joined finding (the exit after it), and an edit whose reader's
// configuration step refused go.mod.
func postLineRows() []postLineRow {
	plain := func(t *testing.T) *ovFix { return newOvFix(t, "1\n") }
	return []postLineRow{{
		name: "exit_before_route",
		fix:  plain,
		post: func(f *ovFix, arm steer.Arm) []byte {
			env := stEnv(0)
			env.Arm = arm
			return f.kindFlushed(ovSess, "toolu_a1", "web/a.k0", "").Steer(env)
		},
		want: kindLineOf(".k0"),
		next: "web/b.k0",
	}, {
		name: "exit_after_route",
		fix:  plain,
		post: func(f *ovFix, arm steer.Arm) []byte {
			p := f.kindFlushed(ovSess, "toolu_a1", "web/a.k0", "p1")
			env := stEnv(steer.DisclosureVersion)
			env.Arm = arm
			env.Join = []steer.Finding{joinStandIn(f.t, p, "unitbrief:g:r#unit:go:a@0123456789ab")}
			b := p.Steer(env)
			if ls := steerLinesOf(f.t, f.st, ovSess); len(ls) != 1 || ls[0].Pattern != string(pattern.TaskPreflight) {
				f.t.Fatalf("premise: the post left %d steer lines; want one, the joined finding's, so the step took its exit after the route step", len(ls))
			}
			return b
		},
		want: kindLineOf(".k0"),
		next: "web/b.k0",
	}, {
		name: "refused_configuration",
		fix: func(t *testing.T) *ovFix {
			f := newOvFix(t, "1\n")
			if err := os.Mkdir(filepath.Join(f.repo, gopg.ManifestFile), 0o755); err != nil {
				t.Fatal(err)
			}
			return f
		},
		post: func(f *ovFix, arm steer.Arm) []byte {
			env := stEnv(0)
			env.Arm = arm
			return f.kindFlushed(ovSess, "toolu_a1", "pkg/a.go", "").Steer(env)
		},
		want: unreadLineOf(gopg.ManifestFile, "not a regular file", ".go"),
		next: "pkg/b.go",
	}}
}

// lastSaid is the last line of the system message b carries: a post's own
// line follows any message the route step set, on a line of its own.
func lastSaid(t *testing.T, b []byte) string {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("the post printed %q, not one JSON object: %v", b, err)
	}
	msg, _ := out["systemMessage"].(string)
	return msg[strings.LastIndex(msg, "\n")+1:]
}

// TestMS1_ArmOffSaysNoPostLineAndTakesNoMarker: under arm off a post prints
// none of its own lines, on either exit of the steer step, as the prompt's
// turn notices print none, and takes no marker, so the next edit with no
// arm says the line. Break: read no arm before the line is taken (either
// exit says the line under arm off, and its marker is taken).
func TestMS1_ArmOffSaysNoPostLineAndTakesNoMarker(t *testing.T) {
	for _, r := range postLineRows() {
		t.Run(r.name, func(t *testing.T) {
			f := r.fix(t)
			if b := r.post(f, steer.ArmOff); b != nil {
				t.Errorf("the post under arm off printed %q; want nothing", b)
			}
			b := f.kindPost(ovSess, "toolu_next", r.next)
			if b == nil {
				t.Errorf("the next edit with no arm printed nothing; want the line, its marker not taken under arm off")
			} else if got := kindSaid(t, b); got != r.want {
				t.Errorf("the next edit with no arm said %q; want %q", got, r.want)
			}
		})
	}
}

// TestMS1_ArmUnsetSaysEachPostLineOnce is the witness of the rows: with no
// arm, each row's post says its line last, after any message the route step
// set, and the next edit of the same kind says nothing.
func TestMS1_ArmUnsetSaysEachPostLineOnce(t *testing.T) {
	for _, r := range postLineRows() {
		t.Run(r.name, func(t *testing.T) {
			f := r.fix(t)
			if got := lastSaid(t, r.post(f, "")); got != r.want {
				t.Errorf("the post with no arm said %q last; want %q", got, r.want)
			}
			if b := f.kindPost(ovSess, "toolu_next", r.next); b != nil {
				t.Errorf("the next edit printed %q; want nothing, the line was said", b)
			}
		})
	}
}
