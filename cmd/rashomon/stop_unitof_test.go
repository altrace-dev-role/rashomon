package main

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// indexAt gives the fixture's work tree a graph index at dir, a slash path
// under it ("" for its top): the index file FindIndex looks for and a
// configuration declaring units, a JSON array.
func (f *lwFixture) indexAt(dir, units string) {
	f.t.Helper()
	f.file(path.Join(dir, graph.GraphFile), "")
	f.file(path.Join(dir, index.ConfigFile), `{"units": `+units+`}`)
}

// stopUnits is the units v's turn rows name, in the ledger's order.
func stopUnits(v verify.StopView) []string {
	var out []string
	for _, o := range v.Obligations {
		if o.Scope == verify.ScopeTurn && o.Unit != "" {
			out = append(out, o.Unit)
		}
	}
	return out
}

// TestStopUnitOf_TheStopAndTheReplayNameOneUnit (W-M3.3): with a graph
// index at the project root, the live Stop's unit row names the written
// file's unit in a conflict row's form, the index root's repository id and
// the Go package directory, and the replay the ledger verb prints names the
// same unit in its ninth field: UnitOf is filled at the Stop and at the
// replay alike. The repository has a clean origin url, so the printed unit
// is pinned as its text: the url with its scheme and .git removed, # and the
// unit's id.
func TestStopUnitOf_TheStopAndTheReplayNameOneUnit(t *testing.T) {
	f := newLW(t)
	f.indexAt("", `[{"path": ".", "lang": "go"}]`)
	f.file(".git/config", "[remote \"origin\"]\n\turl = https://host.example/org/repo.git\n")
	f.bind(lwSpec("unit"))
	f.land("toolu_e", "Edit", lwEdit(f.file("sub/a.go", "package sub\n")), nil)
	f.land("toolu_g", "Bash", lwBash("go test ./..."), nil)
	v := f.stop()
	want := []string{"g:host.example/org/repo#unit:go:sub"}
	if got := stopUnits(v); !slices.Equal(got, want) {
		t.Fatalf("the Stop's unit rows name %q, want %q; rows %+v", got, want, v.Obligations)
	}
	o := f.out()
	var printed []string
	for _, line := range strings.Split(strings.TrimSuffix(o.rows, "\n"), "\n") {
		if fields := strings.Fields(line); len(fields) == 9 && fields[8] != "-" {
			printed = append(printed, fields[8])
		}
	}
	if !slices.Equal(printed, want) {
		t.Errorf("the verb's replay prints units %q, want the Stop's %q; rows\n%s", printed, want, o.rows)
	}
	// Neither stores the origin: after the Stop and the replay no file under
	// the store holds its host. Control: the same scan finds it in the tree.
	scan := func(root string) (n int, hit []string) {
		_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
			if err == nil && fi.Mode().IsRegular() {
				n++
				if b, _ := os.ReadFile(p); strings.Contains(string(b), "host.example") {
					hit = append(hit, p)
				}
			}
			return nil
		})
		return n, hit
	}
	if _, hit := scan(f.cwd); len(hit) == 0 {
		t.Fatal("control: the scan finds no host.example under the work tree")
	}
	if n, hit := scan(f.st.Root()); n == 0 || len(hit) > 0 {
		t.Errorf("the store holds the origin's host in %q (%d files read)", hit, n)
	}
}

// TestStopUnitOf_AnIndexBelowTheRootNamesNoUnit (W-M3.3): an index in a
// directory below the project root is not found from the root, so the Stop
// names no unit for a write under it and adds no unit row, not an unknown
// one: a unit row's id digests its unit, so with no unit named there is no
// id for one apart from the gate's row of the whole turn, which still owes
// the write. Witness: that whole-turn row is there.
func TestStopUnitOf_AnIndexBelowTheRootNamesNoUnit(t *testing.T) {
	f := newLW(t)
	f.indexAt("sub", `[{"path": ".", "lang": "go"}]`)
	f.bind(lwSpec("unit"))
	f.land("toolu_e", "Edit", lwEdit(f.file("sub/a.go", "package sub\n")), nil)
	f.land("toolu_g", "Bash", lwBash("go test ./..."), nil)
	v := f.stop()
	whole := 0
	for _, o := range v.Obligations {
		if o.Scope == verify.ScopeTurn && o.Unit == "" {
			whole++
		}
	}
	if got := stopUnits(v); len(got) != 0 || whole == 0 {
		t.Errorf("turn rows: units %q and %d of the whole turn; want none and at least one", got, whole)
	}
	if got := stopUnitOf(f.cwd)("sub/a.go"); got != "" {
		t.Errorf("stopUnitOf(root)(sub/a.go) = %q, want none with the index below the root", got)
	}
}

// TestStopUnitOf_AConfigOverItsCapNamesNoUnit (W-M3.3): a configuration over
// index.MaxConfigBytes does not read, so the Stop names no unit and adds no
// unit row, and the hook does not fail: the whole turn's row still owes the
// write. The ledger verb's replay, which reads the configuration when it
// runs, prints byte for byte what it prints with no configuration at all.
// Witness: the same configuration under the cap names the unit there.
func TestStopUnitOf_AConfigOverItsCapNamesNoUnit(t *testing.T) {
	f := newLW(t)
	f.indexAt("", `[{"path": ".", "lang": "go"}]`)
	at := filepath.Join(f.cwd, filepath.FromSlash(index.ConfigFile))
	under, err := os.ReadFile(at)
	if err != nil {
		t.Fatal(err)
	}
	f.file(index.ConfigFile, string(under)+strings.Repeat(" ", index.MaxConfigBytes+1-len(under)))
	f.bind(lwSpec("unit"))
	f.land("toolu_e", "Edit", lwEdit(f.file("sub/a.go", "package sub\n")), nil)
	f.land("toolu_g", "Bash", lwBash("go test ./..."), nil)
	v := f.stop()
	whole := 0
	for _, o := range v.Obligations {
		if o.Scope == verify.ScopeTurn && o.Unit == "" {
			whole++
		}
	}
	if got := stopUnits(v); len(got) != 0 || whole == 0 {
		t.Errorf("over the cap, turn rows: units %q and %d of the whole turn; want none and at least one", got, whole)
	}
	over := f.out()
	if err := os.Remove(at); err != nil {
		t.Fatal(err)
	}
	none := f.out()
	if over.all != none.all || over.stderr != none.stderr {
		t.Errorf("the replay over the cap differs from the replay with no configuration:\n%s%s\n---\n%s%s", over.all, over.stderr, none.all, none.stderr)
	}
	f.file(index.ConfigFile, string(under))
	if fine := f.out(); fine.all == none.all {
		t.Errorf("witness: under the cap the replay prints what it prints with no configuration:\n%s", fine.all)
	}
}

// TestStopUnitOf_AConfigEditedAfterTheIndexNamesItsOwnUnit (W-M3.3): the
// Stop reads the configuration when it runs and never the index's records
// (the index file only says where the root is; here it is empty), so after
// the configuration is edited the Stop names the unit the edited
// configuration gives, with no rebuild of the index.
func TestStopUnitOf_AConfigEditedAfterTheIndexNamesItsOwnUnit(t *testing.T) {
	f := newLW(t)
	f.indexAt("", `[{"path": "lib", "lang": "go"}]`)
	f.file("lib/l.go", "package lib\n")
	first := stopUnitOf(f.cwd)
	if got, lib := first("sub/a.go"), first("lib/l.go"); got != "" || lib == "" {
		t.Fatalf("precondition: under the first configuration sub/a.go is in %q and lib/l.go in %q, want none and a unit", got, lib)
	}
	f.file(index.ConfigFile, `{"units": [{"path": ".", "lang": "go"}]}`)
	f.bind(lwSpec("unit"))
	f.land("toolu_e", "Edit", lwEdit(f.file("sub/a.go", "package sub\n")), nil)
	f.land("toolu_g", "Bash", lwBash("go test ./..."), nil)
	rid, _ := baseline.RepoID(f.cwd)
	want := []string{string(graph.GlobalID(rid, "unit:go:sub"))}
	if got := stopUnits(f.stop()); !slices.Equal(got, want) {
		t.Errorf("after the configuration's edit the Stop's unit rows name %q, want %q", got, want)
	}
}

// TestStopUnitOf_ReadsNothingUntilItMapsAPath (B2): the Stop's UnitOf opens
// .rashomon/graph.json and the repository's git files when it is first asked
// about a written path, never when it is made, so a Stop that maps no path
// opens neither. Each file is moved away after a UnitOf is made and before
// it is asked: with the configuration gone it names no unit, and with the
// git config gone the unit's repository id is the path's hash, not the
// origin's. Witness: made and asked with both in place, it names the unit
// under the origin's id.
func TestStopUnitOf_ReadsNothingUntilItMapsAPath(t *testing.T) {
	f := newLW(t)
	f.indexAt("", `[{"path": ".", "lang": "go"}]`)
	f.file(".git/config", "[remote \"origin\"]\n\turl = https://host.example/org/repo.git\n")
	f.file("sub/a.go", "package sub\n")
	ridOrigin, _ := baseline.RepoID(f.cwd)
	if got, want := stopUnitOf(f.cwd)("sub/a.go"), string(graph.GlobalID(ridOrigin, "unit:go:sub")); got != want {
		t.Fatalf("witness: with both files in place stopUnitOf names %q, want %q", got, want)
	}
	away := func(rel string) (back func()) {
		at := filepath.Join(f.cwd, filepath.FromSlash(rel))
		if err := os.Rename(at, at+".away"); err != nil {
			t.Fatal(err)
		}
		return func() {
			if err := os.Rename(at+".away", at); err != nil {
				t.Fatal(err)
			}
		}
	}
	u := stopUnitOf(f.cwd)
	back := away(index.ConfigFile)
	if got := u("sub/a.go"); got != "" {
		t.Errorf("made before the configuration was moved away, stopUnitOf names %q, want none: it read the configuration when made", got)
	}
	back()
	u = stopUnitOf(f.cwd)
	back = away(".git/config")
	defer back()
	ridPath, _ := baseline.RepoID(f.cwd)
	if ridPath == ridOrigin {
		t.Fatalf("premise: with no git config the repository id is the origin's, %q", ridPath)
	}
	if got, want := u("sub/a.go"), string(graph.GlobalID(ridPath, "unit:go:sub")); got != want {
		t.Errorf("made before the git config was moved away, stopUnitOf names %q, want %q: it read the origin when made", got, want)
	}
}
