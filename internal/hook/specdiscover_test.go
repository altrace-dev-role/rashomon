package hook

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/arm"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// specTree is a temp tree for discovery: base with symlinks resolved, and
// link, a symlink to base beside it.
type specTree struct {
	t          *testing.T
	base, link string
}

func newSpecTree(t *testing.T) specTree {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(base, link); err != nil {
		t.Fatal(err)
	}
	return specTree{t: t, base: base, link: link}
}

// mkdir makes dir under base and returns its path.
func (s specTree) mkdir(dir ...string) string {
	s.t.Helper()
	path := filepath.Join(append([]string{s.base}, dir...)...)
	if err := os.MkdirAll(path, 0o755); err != nil {
		s.t.Fatal(err)
	}
	return path
}

// put writes doc as dir/.claude/rashomon/spec.json and returns its path.
func (s specTree) put(dir, doc string) string {
	s.t.Helper()
	path := filepath.Join(dir, ".claude", "rashomon", "spec.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		s.t.Fatal(err)
	}
	return path
}

// specOneGate and specOtherGate are two valid specs with one gate each.
const (
	specOneGate   = `{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]}]}`
	specOtherGate = `{"rashomon_spec": 1, "gates": [{"id": "lint", "kind": "lint", "argv": ["go", "vet", "./..."]}]}`
)

// oneSourceDigest is rule 4's digest of a Compiled whose one source is doc,
// computed here from Parse's digest rather than by Discover.
func oneSourceDigest(t *testing.T, doc string) string {
	t.Helper()
	_, d, err := spec.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(d))
	return spec.DigestPrefix + hex.EncodeToString(sum[:])
}

// TestSP15_HookDiscoversTheNearestSpec: the steer step discovers the spec
// for the payload's cwd and judges against it (card 3.1 R3, D119): a
// PostToolUseFailure through Post, whose view's spec is the nearest file's,
// at rule 4's digest. The rows move cwd and the two roots: a symlinked cwd
// and a symlinked CLAUDE_PROJECT_DIR resolve as baseline.Key resolves a cwd
// (macOS's /tmp is /private/tmp), a tree with no .git walks up to
// CLAUDE_PROJECT_DIR, and a managed spec the user owns is refused with its
// line.
func TestSP15_HookDiscoversTheNearestSpec(t *testing.T) {
	for _, tc := range []struct {
		name string
		// tree builds the tree and returns the payload's cwd and the env's
		// ProjectDir and ManagedDir.
		tree   func(s specTree) (cwd, projectDir, managedDir string)
		path   func(s specTree) string
		doc    string
		errors func(s specTree, managedDir string) []string
	}{
		{"the nearer of two under the .git root", func(s specTree) (string, string, string) {
			s.mkdir(".git")
			s.put(s.base, specOtherGate)
			s.put(s.mkdir("a"), specOneGate)
			return s.mkdir("a", "b"), "", ""
		}, func(s specTree) string { return filepath.Join(s.base, "a", ".claude", "rashomon", "spec.json") }, specOneGate, nil},
		{"a symlinked cwd", func(s specTree) (string, string, string) {
			s.mkdir(".git")
			s.put(s.base, specOneGate)
			s.mkdir("a", "b")
			return filepath.Join(s.link, "a", "b"), "", ""
		}, func(s specTree) string { return filepath.Join(s.base, ".claude", "rashomon", "spec.json") }, specOneGate, nil},
		{"no .git, up to CLAUDE_PROJECT_DIR", func(s specTree) (string, string, string) {
			s.put(s.base, specOneGate)
			return s.mkdir("a", "b"), s.base, ""
		}, func(s specTree) string { return filepath.Join(s.base, ".claude", "rashomon", "spec.json") }, specOneGate, nil},
		{"a symlinked CLAUDE_PROJECT_DIR", func(s specTree) (string, string, string) {
			s.put(s.base, specOneGate)
			return s.mkdir("a", "b"), s.link, ""
		}, func(s specTree) string { return filepath.Join(s.base, ".claude", "rashomon", "spec.json") }, specOneGate, nil},
		{"a managed spec the user owns", func(s specTree) (string, string, string) {
			s.mkdir(".git")
			s.put(s.base, specOneGate)
			managed := filepath.Join(s.mkdir("managed"), "rashomon", "spec.json")
			if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
				s.t.Fatal(err)
			}
			if err := os.WriteFile(managed, []byte(specOtherGate), 0o644); err != nil {
				s.t.Fatal(err)
			}
			// Group-writable, so not admin-owned whoever runs the test.
			if err := os.Chmod(managed, 0o664); err != nil {
				s.t.Fatal(err)
			}
			return s.mkdir("a"), "", filepath.Join(s.base, "managed")
		}, func(s specTree) string { return filepath.Join(s.base, ".claude", "rashomon", "spec.json") }, specOneGate,
			func(s specTree, managedDir string) []string {
				return []string{filepath.Join(managedDir, "rashomon", "spec.json") + ": managed spec ignored: not admin-owned"}
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSpecTree(t)
			cwd, projectDir, managedDir := tc.tree(s)
			f := newST(t)
			f.cwd = cwd
			env := stEnv(0)
			env.ProjectDir, env.ManagedDir = projectDir, managedDir
			_, view, ok := f.red("toolu_r").judge(env)
			if !ok || view.Spec == nil {
				t.Fatalf("judge: ok %v, spec %+v; want a spec", ok, view.Spec)
			}
			want := []string{}
			if tc.errors != nil {
				want = tc.errors(s, managedDir)
			}
			if view.Spec.Path != tc.path(s) || view.Spec.Digest != oneSourceDigest(t, tc.doc) || !slices.Equal(view.Spec.Errors, want) {
				t.Errorf("the view's spec %+v; want path %s, digest %s, errors %q", view.Spec, tc.path(s), oneSourceDigest(t, tc.doc), want)
			}
		})
	}
}

// TestSP16_BrokenSpecIsOneFlagNotAStop: a spec Parse refuses, a spec.json
// that is a directory (an I/O error to Discover) and a payload with no cwd
// each give the view a spec with no path and one Errors line, the user's one
// flag, and the step still answers: the note the run raises prints as it
// does with no spec (card 3.1 R3; README 3.1 rule 5).
func TestSP16_BrokenSpecIsOneFlagNotAStop(t *testing.T) {
	for _, tc := range []struct {
		name string
		tree func(s specTree) (cwd, line string)
	}{
		{"a refused spec", func(s specTree) (string, string) {
			s.mkdir(".git")
			return s.base, s.put(s.base, `{"rashomon_spec": 2}`) + ": spec: "
		}},
		{"a directory at spec.json", func(s specTree) (string, string) {
			s.mkdir(".git")
			s.mkdir(".claude", "rashomon", "spec.json")
			return s.base, "spec: " + filepath.Join(s.base, ".claude", "rashomon", "spec.json") + " is not a regular file"
		}},
		{"no cwd", func(specTree) (string, string) { return "", errNoCWD }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cwd, line := tc.tree(newSpecTree(t))
			f := newST(t)
			if cwd != "" {
				f.cwd = cwd
			}
			p := f.red("toolu_r")
			// The no-cwd row's red names none; the calls before it keep
			// the fixture's, so the regression is the one the others see.
			p.cwd = cwd
			_, view, ok := p.judge(stEnv(0))
			if !ok || view.Spec == nil || view.Spec.Path != "" || len(view.Spec.Errors) != 1 || !strings.HasPrefix(view.Spec.Errors[0], line) {
				t.Fatalf("judge: ok %v, spec %+v; want no path and one line starting %q", ok, view.Spec, line)
			}
			if b := p.Steer(stEnv(0)); !printsFlag(t, b) {
				t.Errorf("Steer printed %q; want the regression's flag", b)
			}
		})
	}
}

// TestMS5h_HeldOutReadsTheSpec: the hook's draw reads the discovered spec's
// holdout object (card 3.1 R3, 5.6): with "off" no session is held out, the
// one MS5e's search holds at the default share among them; at rate_bp 2000 a
// session arm holds at 2,000 basis points and treats at 1,000 is held out.
func TestMS5h_HeldOutReadsTheSpec(t *testing.T) {
	st := mustOpen(t)
	held, _ := armSessions(t, st)
	a, err := arm.New(st.Key())
	if err != nil {
		t.Fatal(err)
	}
	at2000 := ""
	for i := 1; i <= 256 && at2000 == ""; i++ {
		s := "s" + strconv.Itoa(i)
		hi, err1 := a.Assign(holdoutFeature, s, 2000)
		lo, err2 := a.Assign(holdoutFeature, s, 1000)
		if err1 != nil || err2 != nil {
			t.Fatal(err1, err2)
		}
		if hi == arm.Holdout && lo == arm.Treatment {
			at2000 = s
		}
	}
	if at2000 == "" {
		t.Fatal("s1 to s256 found no session held at 2000 and treated at 1000")
	}
	drawn := func(doc, session string) bool {
		t.Helper()
		s := newSpecTree(t)
		s.mkdir(".git")
		s.put(s.base, doc)
		p := &Post{st: st, sessionID: session, promptID: "p1", permissionMode: "default", event: FailureEvent, cwd: s.base}
		if p.sp = DiscoverSpec(p.cwd, "", "", ""); p.sp == nil || len(p.sp.Sources) != 1 {
			t.Fatalf("DiscoverSpec: %+v; want the one spec", p.sp)
		}
		return p.contextOf(stEnv(steer.DisclosureVersion), verify.SteerView{ProjectRoot: s.base}).Holdout
	}
	if drawn(`{"rashomon_spec": 1, "holdout": {"off": true}}`, held) {
		t.Errorf("off: %s is held out; want no session held", held)
	}
	if !drawn(`{"rashomon_spec": 1, "holdout": {"rate_bp": 2000}}`, at2000) {
		t.Errorf("rate_bp 2000: %s is treated; want it held out", at2000)
	}
}
