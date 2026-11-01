package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/platform"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// plwSource is pkg/a.go as the sync read it.
const plwSource = "package pkg\n\nfunc F() {}\n"

// plwProject is the lwFixture's work tree as a Go module whose CI runs `go
// test ./...` (the project's own word, so the inferred gate stands for the
// platform rows), with pkg/a.go; synced writes the log and digests `pgraph
// sync` would leave for it, in the formats internal/platform reads.
func plwProject(t *testing.T, synced bool) *lwFixture {
	t.Helper()
	f := newLW(t)
	f.file("go.mod", "module example.com/m\n\ngo 1.25\n")
	f.file(".github/workflows/ci.yml", "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: go test ./...\n")
	f.file("pkg/a.go", plwSource)
	if synced {
		sum := sha256.Sum256([]byte(plwSource))
		f.file(".rashomon/pgraph.log", platform.LogHeader+"\n"+
			"+\tpkg:example.com/m/pkg\tdeclares\tfunc:example.com/m/pkg.F\tPROVEN\tlinux/amd64/cgo=0/tags=\ttree:0\tgotypes@v2\n")
		f.file(".rashomon/pgraph.digests", platform.DigestsHeader+"\nexample.com/m/pkg\tpkg/a.go\t"+hex.EncodeToString(sum[:])+"\n")
	}
	return f
}

// plwRow is the verb's platform_impact row's fields, or nil.
func plwRow(t *testing.T, o lwOut) []string {
	t.Helper()
	for _, line := range strings.Split(o.rows, "\n") {
		if fs := strings.Fields(line); len(fs) == 7 && fs[2] == string(verify.PlatformImpact) {
			return fs
		}
	}
	t.Logf("ledger:\n%s%s", o.all, o.stderr)
	return nil
}

// TestPLW1s_TheStopAndTheLedgerVerbCarryPlatformRows (card PL-WIRE1 c): the
// Stop hook's view, over calls recorded through the hooks, reads the pgraph
// log at the project root and owes the edited package's row, open with the
// edit as evidence, named by the package and its hash; `rashomon report
// --ledger` prints it with a header saying the log is read now; after a
// `go test ./pkg/...` the row is met. Break: leave StopInput.Platform unset
// in the Stop, and the ledger holds no platform row.
func TestPLW1s_TheStopAndTheLedgerVerbCarryPlatformRows(t *testing.T) {
	f := plwProject(t, true)
	f.land("toolu_e", "Edit", lwEdit(filepath.Join(f.cwd, "pkg", "a.go")), nil)
	v := f.stop()
	if len(v.Platform.Families) != 1 || v.Platform.Families[0].Facts != verify.FactsKnown {
		t.Fatalf("the Stop read %+v; want pkg known", v.Platform)
	}
	family := v.Platform.Families[0].Family()
	o := f.out()
	if row := plwRow(t, o); row == nil || row[3] != verify.SourcePlatform || row[4] != string(verify.ObligationOpen) || row[6] != family {
		t.Errorf("after the edit: row %q, want open, source platform, family %s", row, family)
	}
	if o.head["platform"] != "the pgraph log and written Go files are read now, not at the Stop" {
		t.Errorf("header platform %q", o.head["platform"])
	}
	f.land("toolu_g", "Bash", lwBash("go test ./pkg/..."), nil)
	f.stop()
	if row := plwRow(t, f.out()); row == nil || row[4] != string(verify.ObligationMet) || row[6] != family {
		t.Errorf("after go test ./pkg/...: row %q, want met", row)
	}
}

// TestPLW1t_NoLogOrAnEditedFileWithholdsTheHash: with no log the row is
// unknown under the package alone; after the file changes from what the
// sync read, the row is still judged but its family withholds the hash; a
// project the session wrote no Go file in prints no platform header. Break:
// fill LogPath whether or not the file exists, and no log reads unreadable,
// one row with no family.
func TestPLW1t_NoLogOrAnEditedFileWithholdsTheHash(t *testing.T) {
	f := plwProject(t, false)
	f.land("toolu_e", "Edit", lwEdit(filepath.Join(f.cwd, "pkg", "a.go")), nil)
	f.stop()
	if row := plwRow(t, f.out()); row == nil || row[4] != string(verify.ObligationUnknown) || row[6] != "example.com/m/pkg@unknown" {
		t.Errorf("no log: row %q, want unknown for example.com/m/pkg@unknown", row)
	}

	f = plwProject(t, true)
	f.file("pkg/a.go", plwSource+"\nfunc G() {}\n")
	f.land("toolu_e", "Edit", lwEdit(filepath.Join(f.cwd, "pkg", "a.go")), nil)
	f.stop()
	if row := plwRow(t, f.out()); row == nil || row[4] != string(verify.ObligationOpen) || row[6] != "example.com/m/pkg@stale" {
		t.Errorf("an edited file: row %q, want open for example.com/m/pkg@stale", row)
	}

	f = plwProject(t, true)
	f.land("toolu_e", "Edit", lwEdit(f.file("README.txt", "a\n")), nil)
	f.stop()
	if o := f.out(); plwRow(t, o) != nil || strings.Contains(o.all, "platform:") {
		t.Errorf("no Go write: %q, want no platform row and no header", o.all)
	}
}

// TestPLW1u_PlatformReadFillsLogPathOnlyWhereALogIs: the Stop's reader gives
// internal/platform the log under the root where one exists, so its
// families are known, and none where none does, so they are unknown, never
// unreadable; a root of "" reads no log.
func TestPLW1u_PlatformReadFillsLogPathOnlyWhereALogIs(t *testing.T) {
	f := plwProject(t, true)
	if r := platformRead(f.cwd, []string{"pkg/a.go"}); r.Unreadable || len(r.Families) != 1 || r.Families[0].Facts != verify.FactsKnown {
		t.Errorf("a synced root: %+v, want pkg known", r)
	}
	if err := os.Remove(platform.LogPath(f.cwd)); err != nil {
		t.Fatal(err)
	}
	if r := platformRead(f.cwd, []string{"pkg/a.go"}); r.Unreadable || len(r.Families) != 1 || r.Families[0].Facts != verify.FactsUnknown {
		t.Errorf("no log: %+v, want pkg unknown, not unreadable", r)
	}
	if r := platformRead("", []string{"pkg/a.go"}); r.Unreadable || len(r.Families) != 0 {
		t.Errorf("no root: %+v, want nothing", r)
	}
}
