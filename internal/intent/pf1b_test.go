package intent_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

func keyedWith(key []byte) func(string) string {
	return func(p string) string { return intent.PathDigest(key, p) }
}

// pathItems is the last steer line's slots.paths.items, and the line.
func pathItems(t *testing.T, st *store.Store) (items []string, line string) {
	t.Helper()
	lines, _, err := st.ReadSteer("s1")
	if err != nil || len(lines) == 0 {
		t.Fatalf("ReadSteer: %v, %d lines", err, len(lines))
	}
	line = string(lines[len(lines)-1])
	var l struct {
		Slots struct {
			Paths struct {
				Items []string `json:"items"`
			} `json:"paths"`
		} `json:"slots"`
	}
	if err := json.Unmarshal(lines[len(lines)-1], &l); err != nil {
		t.Fatal(err)
	}
	return l.Slots.Paths.Items, line
}

// TestPF1w_DigestIsKeyed: the slot digest of a path is a keyed HMAC, 64 hex,
// that differs between two stores (two keys) and is not sha256(path), which
// the repository's own file list would reverse (QA-2 note 2).
func TestPF1w_DigestIsKeyed(t *testing.T) {
	const p = "internal/verify/stop.go"
	k1, k2 := []byte("key-one-key-one-key-one-key-one!"), []byte("key-two-key-two-key-two-key-two!")
	a, b := intent.PathDigest(k1, p), intent.PathDigest(k2, p)
	plain := sha256.Sum256([]byte(p))
	if !hex64.MatchString(a) || !hex64.MatchString(b) {
		t.Fatalf("digests %q, %q are not 64 hex", a, b)
	}
	if a == b {
		t.Error("two keys digest one path alike: the digest is not keyed")
	}
	if a == hex.EncodeToString(plain[:]) || b == hex.EncodeToString(plain[:]) {
		t.Error("the digest is sha256(path)")
	}
	if a != intent.PathDigest(k1, p) {
		t.Error("one key does not digest one path to one value")
	}
}

// TestPF1w_RecordFormHoldsDigestsAndCounts: with a digest the finding's Record
// is the steer line's form, each path as a keyed digest and the text a count,
// while the message that prints still names the path. The line is written
// from Record.
func TestPF1w_RecordFormHoldsDigestsAndCounts(t *testing.T) {
	root := projectTree(t)
	p := projectOf(root, specOf(root, spec.Trusted), indexOf())
	key := []byte("key-one-key-one-key-one-key-one!")
	f, ok := intent.FindingKeyed(intent.Resolve([]byte("fix internal/verify/stop.go and internal/verify/index.go"), false, p), intent.Delivered{}, nil, 1, keyedWith(key))
	if !ok {
		t.Fatal("no finding")
	}
	if err := f.Check(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.Message.Text(), "'internal/verify/stop.go', 'internal/verify/index.go'") {
		t.Errorf("the printed note does not name the paths:\n%s", f.Message.Text())
	}
	if !strings.Contains(f.Record.Text(), "paths named: 2\n") || strings.Contains(f.Record.Text(), "stop.go") {
		t.Errorf("the record text carries a path or not the count:\n%s", f.Record.Text())
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := routeAt(t, st, "p1", false, f)
	if !strings.Contains(out.AdditionalContext, "'internal/verify/stop.go'") {
		t.Fatalf("the delivered note lost its paths: %+v", out)
	}
	items, line := pathItems(t, st)
	want := []string{keyedWith(key)("internal/verify/stop.go"), keyedWith(key)("internal/verify/index.go")}
	if !slices.Equal(items, want) {
		t.Errorf("slot items = %q, want the keyed digests %q", items, want)
	}
	for _, leak := range []string{"stop.go", "index.go", "internal/verify"} {
		if strings.Contains(line, leak) {
			t.Errorf("the steer line holds %q: %s", leak, line)
		}
	}
}

// TestPF1w_V4DeliveredSetDedupes: lines holding digests are the delivered
// set: a second request naming a delivered path and a new one counts the first
// as given earlier, so the candidate is digested before the check.
func TestPF1w_V4DeliveredSetDedupes(t *testing.T) {
	root := projectTree(t)
	p := projectOf(root, specOf(root, spec.Trusted), indexOf())
	digest := keyedWith([]byte("key-one-key-one-key-one-key-one!"))
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, _ := intent.FindingKeyed(intent.Resolve([]byte("fix internal/verify/stop.go"), false, p), intent.Delivered{}, nil, 1, digest)
	routeAt(t, st, "p1", false, first)
	d := delivered(t, st)
	if !d.Paths[digest("internal/verify/stop.go")] {
		t.Fatalf("premise: the delivered set %v lacks the digest", d.Paths)
	}
	second, ok := intent.FindingKeyed(intent.Resolve([]byte("fix internal/verify/stop.go and internal/verify/index.go"), false, p), d, nil, 1, digest)
	if !ok {
		t.Fatal("no second finding")
	}
	if !strings.Contains(second.Message.Text(), "paths named: 'internal/verify/index.go'; 1 given earlier\n") {
		t.Errorf("the second note does not count the first path as given earlier:\n%s", second.Message.Text())
	}
	if _, ok := intent.FindingKeyed(intent.Resolve([]byte("fix internal/verify/stop.go"), false, p), d, nil, 1, digest); ok {
		t.Error("a request naming only a delivered path got a note")
	}
}

// subdirProject is projectTree with a .git root marker and the cwd of a
// request made from <root>/internal.
func subdirProject(t *testing.T) (root string, p intent.Project) {
	t.Helper()
	root = projectTree(t)
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	p = projectOf(root, specOf(root, spec.Trusted), indexOf())
	p.Cwd = filepath.Join(root, "internal")
	return root, p
}

// TestPF1x_SubdirectoryCwdResolvesFromTheRoot: from <project>/internal a path
// named from the project root resolves (it is not a false "no named path
// exists"), a path named from the cwd resolves to the same file, and the
// anchored rule covers it. Control: the same request from the root.
func TestPF1x_SubdirectoryCwdResolvesFromTheRoot(t *testing.T) {
	root, p := subdirProject(t)
	for _, c := range []struct{ name, cwd, prompt string }{
		{"from the root, cwd the root", root, "fix internal/verify/stop.go"},
		{"from the root, cwd below it", p.Cwd, "fix internal/verify/stop.go"},
		{"from the cwd", p.Cwd, "fix verify/stop.go"},
	} {
		q := p
		q.Cwd = c.cwd
		in := intent.Resolve([]byte(c.prompt), false, q)
		if !slices.Equal(in.Paths, []string{"internal/verify/stop.go"}) {
			t.Errorf("%s: paths = %q, want the file under the root", c.name, in.Paths)
		}
		if !slices.ContainsFunc(in.Rules, func(r intent.Rule) bool { return r.ID == "conv-errors" }) {
			t.Errorf("%s: the anchored rule internal/** does not cover it: %+v", c.name, in.Rules)
		}
		if slices.Contains(in.Unknown, "no named path exists in the project") {
			t.Errorf("%s: says no named path exists", c.name)
		}
	}
}

// TestPF1x_FileExistsUnknownFixture: the unknown line's "no named path exists
// in the project" is said for a file that is not there and for it alone, from
// the root and from a subdirectory, so the fixture that names an existing file
// proves resolution and the one that names a missing file proves the line.
func TestPF1x_FileExistsUnknownFixture(t *testing.T) {
	_, p := subdirProject(t)
	for _, cwd := range []string{p.Root, p.Cwd} {
		q := p
		q.Cwd = cwd
		have := intent.Resolve([]byte("fix internal/verify/stop.go"), false, q)
		gone := intent.Resolve([]byte("fix internal/verify/gone.go"), false, q)
		if slices.Contains(have.Unknown, "no named path exists in the project") || len(have.Paths) != 1 {
			t.Errorf("cwd %q, existing file: unknown %q, paths %q", cwd, have.Unknown, have.Paths)
		}
		if !slices.Contains(gone.Unknown, "no named path exists in the project") || len(gone.Paths) != 0 {
			t.Errorf("cwd %q, missing file: unknown %q, paths %q, want the line", cwd, gone.Unknown, gone.Paths)
		}
	}
}

// TestPF1x_PathOutsideTheProjectStaysRefused: from a subdirectory,
// ../../etc/passwd and an absolute path outside the project name nothing (R7).
func TestPF1x_PathOutsideTheProjectStaysRefused(t *testing.T) {
	root, p := subdirProject(t)
	outside := filepath.Join(filepath.Dir(root), "outside.go")
	if err := os.WriteFile(outside, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outside)
	for _, prompt := range []string{"read ../../etc/passwd", "read ../../outside.go", "read ../outside.go", "read " + outside, "read /etc/passwd"} {
		if in := intent.Resolve([]byte(prompt), false, p); len(in.Paths) != 0 {
			t.Errorf("%q kept %q", prompt, in.Paths)
		}
	}
}
