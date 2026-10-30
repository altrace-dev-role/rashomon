package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// managedAt is the managed spec's path in the map FS tests; its directory is
// filepath.Dir of it.
const managedAt = "/etc/claude-code/rashomon/spec.json"

func repoAt(dir string) string { return filepath.Join(dir, ".claude", "rashomon", "spec.json") }

type ownerOf struct {
	uid  int
	mode fs.FileMode
}

// mapFS is an FS over maps: an error set for a path wins, then a file's
// bytes; a path's owner is its owners entry, else a user's regular file when
// files has it, else missing. Nothing touches a disk or a home directory.
type mapFS struct {
	files  map[string]string
	owners map[string]ownerOf
	errs   map[string]error
}

func newFS() mapFS {
	return mapFS{files: map[string]string{}, owners: map[string]ownerOf{}, errs: map[string]error{}}
}

func (m mapFS) ReadFile(path string) ([]byte, error) {
	if err := m.errs[path]; err != nil {
		return nil, err
	}
	if s, ok := m.files[path]; ok {
		return []byte(s), nil
	}
	return nil, fs.ErrNotExist
}

func (m mapFS) Owner(path string) (int, fs.FileMode, error) {
	if err := m.errs[path]; err != nil {
		return -1, 0, err
	}
	if o, ok := m.owners[path]; ok {
		return o.uid, o.mode, nil
	}
	if _, ok := m.files[path]; ok {
		return 501, 0o644, nil
	}
	return -1, 0, fs.ErrNotExist
}

// managed puts doc at managedAt, admin-owned: root's 0644 file in root's 0755
// directory.
func (m mapFS) managed(doc string) mapFS {
	m.files[managedAt] = doc
	m.owners[managedAt] = ownerOf{0, 0o644}
	m.owners[filepath.Dir(managedAt)] = ownerOf{0, fs.ModeDir | 0o755}
	return m
}

func (m mapFS) repo(dir, doc string) mapFS {
	m.files[repoAt(dir)] = doc
	return m
}

// discover is Discover over Locations(cwd, root) and managedAt, failing the
// test on an I/O error.
func discover(t *testing.T, m mapFS, cwd, root string) *Compiled {
	t.Helper()
	c, err := Discover(m, Locations(cwd, root), managedAt)
	if err != nil {
		t.Fatalf("Discover from %s: %v", cwd, err)
	}
	return c
}

// found is discover where the call must give a Compiled.
func found(t *testing.T, m mapFS, cwd, root string) *Compiled {
	t.Helper()
	c := discover(t, m, cwd, root)
	if c == nil {
		t.Fatalf("Discover from %s: nil, want a Compiled", cwd)
	}
	return c
}

func paths(c *Compiled) []string {
	var out []string
	for _, s := range c.Sources {
		out = append(out, s.Path)
	}
	return out
}

func gateIDs(c *Compiled) []string {
	var out []string
	for _, g := range c.Gates {
		out = append(out, g.ID)
	}
	return out
}

// digestsOver is "sha256:" and the hex SHA-256 of the digests concatenated.
func digestsOver(digests ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(digests, "")))
	return DigestPrefix + hex.EncodeToString(sum[:])
}

var (
	docA = withGate(kv{"argv", `["go", "test", "./a/..."]`})
	docB = withGate(kv{"argv", `["go", "test", "./b/..."]`})
	// broken is a document Parse refuses.
	broken = `{"rashomon_spec": 2}`
)

// TestSP5_NearestRepoSpecWins: with specs at /r and /r/a, a call in /r/a/b
// reads /r/a's alone; without it, /r's.
func TestSP5_NearestRepoSpecWins(t *testing.T) {
	m := newFS().repo("/r", docA).repo("/r/a", docB)
	c := found(t, m, "/r/a/b", "/r")
	if want := []string{repoAt("/r/a")}; !slices.Equal(paths(c), want) {
		t.Fatalf("sources %v, want %v", paths(c), want)
	}
	if c.Sources[0].Digest != digestOf(t, docB) || c.Sources[0].Trust != Untrusted {
		t.Errorf("source %+v, want docB's digest, untrusted", c.Sources[0])
	}
	delete(m.files, repoAt("/r/a"))
	if c := found(t, m, "/r/a/b", "/r"); !slices.Equal(paths(c), []string{repoAt("/r")}) {
		t.Errorf("without /r/a's spec: sources %v, want /r's", paths(c))
	}
}

// TestVER38_DiscoveryNeverAboveRepoRoot: Locations runs from cwd to root and
// stops there; a cwd not under root, a relative path or a ".." that climbs
// out gives nothing, so a spec in root's parent is never read.
func TestVER38_DiscoveryNeverAboveRepoRoot(t *testing.T) {
	for _, tc := range []struct {
		cwd, root string
		want      []string
	}{
		{"/r/a/b", "/r", []string{repoAt("/r/a/b"), repoAt("/r/a"), repoAt("/r")}},
		{"/r", "/r", []string{repoAt("/r")}},
		{"/r/a/", "/r/", []string{repoAt("/r/a"), repoAt("/r")}},
		{"/r/a/../b", "/r", []string{repoAt("/r/b"), repoAt("/r")}},
		{"/a", "/", []string{repoAt("/a"), repoAt("/")}},
		{"/x/a", "/r", nil},
		{"/rx/a", "/r", nil},
		{"/r/../x", "/r", nil},
		{"/", "/r", nil},
		{"r/a", "/r", nil},
		{"/r/a", "r", nil},
		{"r/a", "r", nil},
		{"", "", nil},
		{"", "/r", nil},
		{"/r/a", "", nil},
	} {
		if got := Locations(tc.cwd, tc.root); !slices.Equal(got, tc.want) {
			t.Errorf("Locations(%q, %q) = %q, want %q", tc.cwd, tc.root, got, tc.want)
		}
	}
	m := newFS().repo("/", docA)
	if c := discover(t, m, "/r/a", "/r"); c != nil {
		t.Errorf("a spec in root's parent applied: %v", paths(c))
	}
	if c := discover(t, m, "/r/a", "/"); c == nil {
		t.Error("the same spec with / as the root did not apply")
	}
}

// TestVER38b_ClaudeProjectDirStopsTheWalk: the walk stops at the nearer of
// the .git root and CLAUDE_PROJECT_DIR; one that is not at or above cwd is
// not a root.
func TestVER38b_ClaudeProjectDirStopsTheWalk(t *testing.T) {
	for _, tc := range []struct {
		name, git, env, want string
	}{
		{"the env nearer than the .git", "/r", "/r/a", "/r/a"},
		{"the .git nearer than the env", "/r/a", "/r", "/r/a"},
		{"the env alone", "", "/r/a", "/r/a"},
		{"the .git alone", "/r", "", "/r"},
		{"the env beside cwd", "/r", "/s", "/r"},
		{"the env below cwd", "/r", "/r/a/b/c", "/r"},
		{"a relative env", "/r", "r/a", "/r"},
		{"neither", "", "", ""},
		{"the env with a trailing slash", "/r", "/r/a/", "/r/a"},
	} {
		if got := WalkRoot("/r/a/b", tc.git, tc.env); got != tc.want {
			t.Errorf("%s: WalkRoot = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := WalkRoot("r/a/b", "r", "r/a"); got != "" {
		t.Errorf("a relative cwd: WalkRoot = %q, want none", got)
	}
	m := newFS().repo("/r", docA)
	if c := discover(t, m, "/r/a/b", WalkRoot("/r/a/b", "/r", "/r/a")); c != nil {
		t.Errorf("CLAUDE_PROJECT_DIR /r/a did not stop the walk: %v", paths(c))
	}
	if c := discover(t, m, "/r/a/b", WalkRoot("/r/a/b", "/r", "")); c == nil {
		t.Error("without CLAUDE_PROJECT_DIR the .git root's spec did not apply")
	}
}

// TestSP6_WalkDepthCapped: 63 levels below root names 64 directories, root
// the last; 64 and 70 levels below name the nearest 64, root not among them.
func TestSP6_WalkDepthCapped(t *testing.T) {
	deep := func(n int) string { return "/r" + strings.Repeat("/d", n) }
	for _, tc := range []struct{ n, last int }{{63, 0}, {64, 1}, {70, 7}} {
		got := Locations(deep(tc.n), "/r")
		if len(got) != MaxWalkDepth {
			t.Errorf("%d below: %d paths, want %d", tc.n, len(got), MaxWalkDepth)
			continue
		}
		if got[0] != repoAt(deep(tc.n)) || got[MaxWalkDepth-1] != repoAt(deep(tc.last)) {
			t.Errorf("%d below: %q to %q, want cwd's to %q", tc.n, got[0], got[MaxWalkDepth-1], repoAt(deep(tc.last)))
		}
	}
}

// TestVER41_ManagedSpecMustBeRootOwned: a table over the pure check, each row
// also through Discover, where a refused managed spec is the Errors line and
// applies nothing.
func TestVER41_ManagedSpecMustBeRootOwned(t *testing.T) {
	const dir = fs.ModeDir
	for _, tc := range []struct {
		name    string
		file    ownerOf
		dir     ownerOf
		applies bool
	}{
		{"root's 0644 in root's 0755", ownerOf{0, 0o644}, ownerOf{0, dir | 0o755}, true},
		{"root's 0600 in root's 0700", ownerOf{0, 0o600}, ownerOf{0, dir | 0o700}, true},
		{"root's 0444 in root's 0555", ownerOf{0, 0o444}, ownerOf{0, dir | 0o555}, true},
		{"the file uid 501", ownerOf{501, 0o644}, ownerOf{0, dir | 0o755}, false},
		{"the file uid unknown", ownerOf{-1, 0o644}, ownerOf{0, dir | 0o755}, false},
		{"the file 0664", ownerOf{0, 0o664}, ownerOf{0, dir | 0o755}, false},
		{"the file 0666", ownerOf{0, 0o666}, ownerOf{0, dir | 0o755}, false},
		{"the file 0646", ownerOf{0, 0o646}, ownerOf{0, dir | 0o755}, false},
		{"the file a symlink", ownerOf{0, fs.ModeSymlink | 0o755}, ownerOf{0, dir | 0o755}, false},
		{"the file a directory", ownerOf{0, dir | 0o755}, ownerOf{0, dir | 0o755}, false},
		{"the directory uid 501", ownerOf{0, 0o644}, ownerOf{501, dir | 0o755}, false},
		{"the directory 0775", ownerOf{0, 0o644}, ownerOf{0, dir | 0o775}, false},
		{"the directory 0757", ownerOf{0, 0o644}, ownerOf{0, dir | 0o757}, false},
		{"the directory a symlink", ownerOf{0, 0o644}, ownerOf{0, fs.ModeSymlink | 0o755}, false},
	} {
		if got := adminOwned(tc.file.uid, tc.file.mode, tc.dir.uid, tc.dir.mode); got != tc.applies {
			t.Errorf("%s: adminOwned %t, want %t", tc.name, got, tc.applies)
		}
		m := newFS().managed(docA)
		m.owners[managedAt], m.owners[filepath.Dir(managedAt)] = tc.file, tc.dir
		c := found(t, m, "/r", "/r")
		if tc.applies {
			if !slices.Equal(paths(c), []string{managedAt}) || c.Sources[0].Trust != Managed || c.Errors != nil {
				t.Errorf("%s: sources %v, errors %q; want the managed spec, managed, no error", tc.name, paths(c), c.Errors)
			}
			continue
		}
		if want := []string{managedAt + ": managed spec ignored: not admin-owned"}; c.Sources != nil || !slices.Equal(c.Errors, want) {
			t.Errorf("%s: got %+v, want no source and the line %q", tc.name, c, want)
		}
	}
}

// TestSP7_AdditiveMergeManagedWinsId: the managed spec's gates, rules and
// facts come first; a repository entry with a managed id is Dropped with the
// repository digest, and its others stay. Alone, the repository's stay whole.
func TestSP7_AdditiveMergeManagedWinsId(t *testing.T) {
	managed := top(`"gates": [` + gate() + `], "rules": [` + rule() + `], "facts": [` + fact() + `]`)
	repo := top(`"gates": [` + gate(kv{"argv", `["go", "test", "./..."]`}) + `, ` +
		gate(kv{"id", `"lint"`}, kv{"kind", `"lint"`}, kv{"argv", `["golangci-lint", "run"]`}) + `], ` +
		`"rules": [` + rule(kv{"why", `"repo"`}) + `, ` + rule(kv{"id", `"r2"`}) + `], ` +
		`"facts": [` + fact(kv{"value", `"prod"`}) + `, ` + fact(kv{"id", `"f2"`}) + `]`)
	c := found(t, newFS().managed(managed).repo("/r", repo), "/r", "/r")
	if want := []string{"spec:unit", "spec:lint"}; !slices.Equal(gateIDs(c), want) {
		t.Fatalf("gates %v, want %v", gateIDs(c), want)
	}
	if g := c.Gates[0]; g.Source != 0 || !slices.Equal(g.Gate.Argv, []string{"go", "test"}) {
		t.Errorf("spec:unit is %+v, want the managed gate", g)
	}
	if c.Gates[1].Source != 1 {
		t.Errorf("spec:lint source %d, want 1", c.Gates[1].Source)
	}
	if len(c.Rules) != 2 || c.Rules[0].ID != "spec:r1" || c.Rules[0].Rule.Why != "w" || c.Rules[1].ID != "spec:r2" || c.Rules[1].Source != 1 {
		t.Errorf("rules %+v, want the managed r1 and the repository r2", c.Rules)
	}
	if len(c.Facts) != 2 || c.Facts[0].ID != "spec:f1" || c.Facts[0].Fact.Value != "dev" || c.Facts[1].ID != "spec:f2" {
		t.Errorf("facts %+v, want the managed f1 and the repository f2", c.Facts)
	}
	d := digestOf(t, repo)
	want := []Dropped{{"spec:unit", d}, {"spec:r1", d}, {"spec:f1", d}}
	if !slices.Equal(c.Dropped, want) {
		t.Errorf("dropped %v, want %v", c.Dropped, want)
	}
	alone := found(t, newFS().repo("/r", repo), "/r", "/r")
	if !slices.Equal(gateIDs(alone), []string{"spec:unit", "spec:lint"}) || len(alone.Rules) != 2 || len(alone.Facts) != 2 || alone.Dropped != nil {
		t.Errorf("the repository spec alone: %+v", alone)
	}
	if alone.Gates[0].Source != 0 || !slices.Equal(alone.Gates[0].Gate.Argv, []string{"go", "test", "./..."}) {
		t.Errorf("the repository spec alone: spec:unit is %+v", alone.Gates[0])
	}
}

// TestMS5j_ManagedHoldoutWins: the managed spec's holdout object wins; with
// none, a trusted repository spec's applies, and an untrusted one's is not
// read, so a repository cannot opt its user into the holdout (card
// VERSION-GATE1); with neither, the zero Holdout.
func TestMS5j_ManagedHoldoutWins(t *testing.T) {
	on := withHoldout(`{"rate_bp": 500}`)
	off := withHoldout(`{"off": true}`)
	for _, tc := range []struct {
		name          string
		managed, repo string
		trusted       bool
		want          Holdout
	}{
		{"both", on, off, false, Holdout{RateBP: 500}},
		{"both, the repository trusted", on, off, true, Holdout{RateBP: 500}},
		{"the managed one only", on, top(""), false, Holdout{RateBP: 500}},
		{"the repository's only, trusted", top(""), off, true, Holdout{Off: true, RateBP: DefaultHoldoutBP}},
		{"the repository's only, untrusted", top(""), off, false, Holdout{}},
		{"neither", top(""), top(""), true, Holdout{}},
	} {
		c, err := DiscoverBound(newFS().managed(tc.managed).repo("/r", tc.repo), Locations("/r", "/r"), managedAt,
			func(string, string) bool { return tc.trusted })
		if err != nil || c == nil {
			t.Fatalf("%s: DiscoverBound = %v, %v", tc.name, c, err)
		}
		if c.Holdout != tc.want {
			t.Errorf("%s: holdout %+v, want %+v", tc.name, c.Holdout, tc.want)
		}
	}
}

// TestSP8_CiIdSpaces: ci_commands entries are numbered per source, ci:<n>
// for the repository and ci:managed:<n> for the managed spec, each a
// required custom gate matched by prefix.
func TestSP8_CiIdSpaces(t *testing.T) {
	managed := top(`"ci_commands": [["make", "check"]]`)
	repo := top(`"ci_commands": [["go", "vet", "./..."], ["go", "build", "./..."]]`)
	c := found(t, newFS().managed(managed).repo("/r", repo), "/r", "/r")
	if want := []string{"ci:managed:0", "ci:0", "ci:1"}; !slices.Equal(gateIDs(c), want) {
		t.Fatalf("gates %v, want %v", gateIDs(c), want)
	}
	for i, argv := range [][]string{{"make", "check"}, {"go", "vet", "./..."}, {"go", "build", "./..."}} {
		g := c.Gates[i].Gate
		if !slices.Equal(g.Argv, argv) || g.Kind != "custom" || g.Match != "prefix" || g.Required == nil || !*g.Required || g.ID != "" {
			t.Errorf("%s: %+v, want argv %q, custom, prefix, required", c.Gates[i].ID, g, argv)
		}
	}
	if c.Gates[0].Source != 0 || c.Gates[2].Source != 1 || c.Dropped != nil {
		t.Errorf("sources %d and %d, dropped %v", c.Gates[0].Source, c.Gates[2].Source, c.Dropped)
	}
	alone := found(t, newFS().managed(managed), "/r", "/r")
	if !slices.Equal(gateIDs(alone), []string{"ci:managed:0"}) {
		t.Errorf("the managed spec alone: gates %v", gateIDs(alone))
	}
}

// TestSP9_DigestOverSourcesInOrder: Digest is over the source digests in
// Sources order, so the same two files in the other roles give another
// Digest; one source's Digest is over its digest, not the digest itself.
func TestSP9_DigestOverSourcesInOrder(t *testing.T) {
	a, b := digestOf(t, docA), digestOf(t, docB)
	ab := found(t, newFS().managed(docA).repo("/r", docB), "/r", "/r")
	ba := found(t, newFS().managed(docB).repo("/r", docA), "/r", "/r")
	if ab.Digest != digestsOver(a, b) || ba.Digest != digestsOver(b, a) {
		t.Errorf("digests %s and %s, want %s and %s", ab.Digest, ba.Digest, digestsOver(a, b), digestsOver(b, a))
	}
	if ab.Digest == ba.Digest {
		t.Errorf("swapping the files kept the digest %s", ab.Digest)
	}
	one := found(t, newFS().repo("/r", docA), "/r", "/r")
	if one.Digest != digestsOver(a) || one.Digest == a {
		t.Errorf("one source: digest %s, want %s", one.Digest, digestsOver(a))
	}
}

// TestSP10_TrustIsLowestFolded: managed and repository is untrusted, managed
// alone is managed, the repository alone untrusted; TrustOf reads a source's
// trust, and SourceImplied is builtin even with no sources.
func TestSP10_TrustIsLowestFolded(t *testing.T) {
	both := found(t, newFS().managed(docA).repo("/r", docB), "/r", "/r")
	for _, tc := range []struct {
		name string
		c    *Compiled
		want Trust
	}{
		{"managed and repository", both, Untrusted},
		{"managed alone", found(t, newFS().managed(docA), "/r", "/r"), Managed},
		{"repository alone", found(t, newFS().repo("/r", docB), "/r", "/r"), Untrusted},
		{"a refused file alone", found(t, newFS().repo("/r", broken), "/r", "/r"), Untrusted},
	} {
		if tc.c.Trust != tc.want {
			t.Errorf("%s: trust %q, want %q", tc.name, tc.c.Trust, tc.want)
		}
	}
	if both.TrustOf(0) != Managed || both.TrustOf(1) != Untrusted || both.TrustOf(SourceImplied) != Builtin {
		t.Errorf("TrustOf: %q, %q, %q", both.TrustOf(0), both.TrustOf(1), both.TrustOf(SourceImplied))
	}
	if got := (&Compiled{}).TrustOf(SourceImplied); got != Builtin {
		t.Errorf("TrustOf(SourceImplied) with no sources = %q, want %q", got, Builtin)
	}
	if slices.Contains(Trusts, Builtin) || Lowest(Builtin, Managed) != Untrusted {
		t.Error("builtin is a source trust")
	}
}

// TestSP11_BrokenRepoSpecRecordedNotFatal: a refused nearest spec is one
// Errors line with Parse's pointer; the managed spec still applies and the
// farther repository spec does not. Alone, it is a Compiled with the line and
// no source, never nil.
func TestSP11_BrokenRepoSpecRecordedNotFatal(t *testing.T) {
	_, _, perr := Parse([]byte(broken))
	line := repoAt("/r/a") + ": " + perr.Error()
	if !strings.Contains(line, "/rashomon_spec") {
		t.Fatalf("the line %q has no pointer", line)
	}
	m := newFS().managed(docA).repo("/r", docB).repo("/r/a", broken)
	c := found(t, m, "/r/a/b", "/r")
	if !slices.Equal(paths(c), []string{managedAt}) || !slices.Equal(c.Errors, []string{line}) {
		t.Errorf("sources %v, errors %q; want the managed spec and %q", paths(c), c.Errors, line)
	}
	alone := found(t, newFS().repo("/r", docB).repo("/r/a", broken), "/r/a/b", "/r")
	if alone.Sources != nil || alone.Gates != nil || !slices.Equal(alone.Errors, []string{line}) || alone.Trust != Untrusted || alone.Digest != digestsOver() {
		t.Errorf("a refused spec alone: %+v", alone)
	}
}

// TestSP12_NoSpecIsNil: no file is nil and no error; an I/O error other than
// a missing file is no Compiled and the error, wherever it is met.
func TestSP12_NoSpecIsNil(t *testing.T) {
	m := newFS()
	m.owners[filepath.Dir(managedAt)] = ownerOf{0, fs.ModeDir | 0o755}
	for _, managed := range []string{managedAt, ""} {
		if c, err := Discover(m, Locations("/r/a", "/r"), managed); c != nil || err != nil {
			t.Errorf("no file, managed %q: %+v, %v", managed, c, err)
		}
	}
	if c, err := Discover(m, nil, ""); c != nil || err != nil {
		t.Errorf("no candidates: %+v, %v", c, err)
	}
	for _, at := range []string{repoAt("/r/a"), managedAt, filepath.Dir(managedAt)} {
		m := newFS().managed(docA).repo("/r", docB)
		m.errs[at] = fs.ErrPermission
		c, err := Discover(m, Locations("/r/a", "/r"), managedAt)
		if c != nil || !errors.Is(err, fs.ErrPermission) {
			t.Errorf("permission denied at %s: %+v, %v", at, c, err)
		}
	}
}

// TestSP12b_OSFSReadsOnlyARegularFileToTheCap: OSFS refuses a directory
// (and so a FIFO) at the path before opening it, reads a file one byte past
// MaxBytes at most, and reports a missing file as fs.ErrNotExist.
func TestSP12b_OSFSReadsOnlyARegularFileToTheCap(t *testing.T) {
	dir := t.TempDir()
	if _, err := (OSFS{}).ReadFile(dir); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("a directory: %v, want not a regular file", err)
	}
	if _, err := (OSFS{}).ReadFile(filepath.Join(dir, "none")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing file: %v, want fs.ErrNotExist", err)
	}
	big := filepath.Join(dir, "big.json")
	if err := os.WriteFile(big, make([]byte, MaxBytes+10), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, err := (OSFS{}).ReadFile(big); err != nil || len(b) != MaxBytes+1 {
		t.Errorf("a file over the cap: %d bytes, %v; want %d", len(b), err, MaxBytes+1)
	}
	small := filepath.Join(dir, "small.json")
	if err := os.WriteFile(small, []byte(docA), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, err := (OSFS{}).ReadFile(small); err != nil || string(b) != docA {
		t.Errorf("a small file: %q, %v", b, err)
	}
}

// under is the fuzz's own oracle for "p is root or below it", by
// filepath.Rel rather than discover.go's within.
func under(p, root string) bool {
	if !filepath.IsAbs(p) || !filepath.IsAbs(root) {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(p))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// FuzzLocations: whatever the paths, WalkRoot is the nearer of the roots at
// or above cwd, or none; Locations names spec paths exactly when cwd is at or
// below its root, from cwd upward, never above root, none twice, at most
// MaxWalkDepth, reaching root unless capped, and the same list on a second
// call.
func FuzzLocations(f *testing.F) {
	for _, s := range [][3]string{
		{"/r/a/b", "/r", ""}, {"/r/a/b", "/r", "/r/a"}, {"/", "/", ""}, {"/r/../x", "/r", ""},
		{"r/a", "r", ""}, {"/r" + strings.Repeat("/d", 70), "/r", ""}, {"//r//a/", "/r/", "/r/a/"},
		{"", "", ""}, {"/rx/a", "/r", "/rx"}, {"/r/a\x00/b", "/r", ""}, {"/r/..x", "/r", "/r/..x"},
	} {
		f.Add(s[0], s[1], s[2])
	}
	f.Fuzz(func(t *testing.T, cwd, projectRoot, projectDir string) {
		want := ""
		for _, r := range []string{projectRoot, projectDir} {
			if under(cwd, r) && len(filepath.Clean(r)) > len(want) {
				want = filepath.Clean(r)
			}
		}
		root := WalkRoot(cwd, projectRoot, projectDir)
		if root != want {
			t.Fatalf("WalkRoot(%q, %q, %q) = %q, want %q", cwd, projectRoot, projectDir, root, want)
		}
		for _, r := range []string{root, projectRoot} {
			got := Locations(cwd, r)
			if !slices.Equal(got, Locations(cwd, r)) {
				t.Fatalf("Locations(%q, %q) differs on a second call", cwd, r)
			}
			if len(got) > MaxWalkDepth || (len(got) > 0) != under(cwd, r) {
				t.Fatalf("Locations(%q, %q): %d paths, cwd under root %t", cwd, r, len(got), under(cwd, r))
			}
			var prev string
			for i, p := range got {
				dir := filepath.Dir(filepath.Dir(filepath.Dir(p)))
				if p != repoAt(dir) || slices.Contains(got[:i], p) {
					t.Fatalf("Locations(%q, %q)[%d] = %q: not a spec path, or twice", cwd, r, i, p)
				}
				if !under(dir, r) {
					t.Fatalf("Locations(%q, %q) names %q, above its root", cwd, r, dir)
				}
				if (i == 0 && dir != filepath.Clean(cwd)) || (i > 0 && dir != filepath.Dir(prev)) {
					t.Fatalf("Locations(%q, %q)[%d] = %q does not follow %q", cwd, r, i, dir, prev)
				}
				prev = dir
			}
			if len(got) > 0 && len(got) < MaxWalkDepth && prev != filepath.Clean(r) {
				t.Fatalf("Locations(%q, %q) stops at %q, short of root", cwd, r, prev)
			}
		}
	})
}

// BenchmarkDiscover: one call's discovery over a map FS, cwd five levels
// below the root and the repository spec (specExample) at the root, the
// managed spec absent: six candidate paths, five misses, one read and one
// Parse.
func BenchmarkDiscover(b *testing.B) {
	m := newFS().repo("/r", specExample)
	cwd := "/r/a/b/c/d/e"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if c, err := Discover(m, Locations(cwd, "/r"), managedAt); err != nil || len(c.Sources) != 1 {
			b.Fatalf("Discover: %+v, %v", c, err)
		}
	}
}
