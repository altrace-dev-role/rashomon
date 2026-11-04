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
// platform rows), with pkg/a.go and a test file beside it; synced writes the
// log and digests `pgraph sync` would leave for it, in the formats
// internal/platform reads.
func plwProject(t *testing.T, synced bool) *lwFixture {
	t.Helper()
	f := newLW(t)
	f.file("go.mod", "module example.com/m\n\ngo 1.25\n")
	f.file(".github/workflows/ci.yml", "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: go test ./...\n")
	f.file("pkg/a.go", plwSource)
	f.file("pkg/a_test.go", "package pkg\n")
	if synced {
		sum := sha256.Sum256([]byte(plwSource))
		f.file(".pgraph/pgraph.log", platform.LogHeader+"\n"+
			"+\tpkg:example.com/m/pkg\tdeclares\tfunc:example.com/m/pkg.F\tPROVEN\tlinux/amd64/cgo=0/tags=\ttree:0\tgotypes@v2\n")
		f.file(".pgraph/pgraph.digests", platform.DigestsHeader+"\nexample.com/m/pkg\tpkg/a.go\t"+hex.EncodeToString(sum[:])+"\n")
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

// TestPLW1s_TheLedgerVerbAndTheStopOweTheSamePlatformRow (card PL-WIRE1
// c; PW1b-ON-230 step 2 reverses its S1, the ledger verb alone reading the
// graph): over calls recorded through the hooks, the Stop's view reads the
// pgraph log at the project root and owes the edited package's row, open;
// `rashomon report --ledger` reads it again and owes the same row, named by
// the package and its hash, under a header naming what it read; after `go
// test ./pkg` the row is met. Break: leave the Stop's reader unset, or pass
// none to the verb's replay, and one of the two holds no platform row.
func TestPLW1s_TheLedgerVerbAndTheStopOweTheSamePlatformRow(t *testing.T) {
	f := plwProject(t, true)
	f.land("toolu_e", "Edit", lwEdit(filepath.Join(f.cwd, "pkg", "a.go")), nil)
	v := f.stop()
	var stopRow []verify.Obligation
	for _, o := range v.Obligations {
		if o.Kind == verify.PlatformImpact {
			stopRow = append(stopRow, o)
		}
	}
	if v.Platform.Log != verify.LogRead || len(stopRow) != 1 || stopRow[0].State != verify.ObligationOpen {
		t.Fatalf("the Stop read %+v and owes %+v; want the log read and the package's one row open", v.Platform, stopRow)
	}
	o := f.out()
	row := plwRow(t, o)
	if row == nil || row[1] != verify.ScopeSession || row[3] != verify.SourcePlatform || row[4] != string(verify.ObligationOpen) ||
		!strings.HasPrefix(row[6], "example.com/m/pkg@") || len(strings.TrimPrefix(row[6], "example.com/m/pkg@")) != 64 {
		t.Fatalf("after the edit: row %q, want open in the session scope, source platform, family the package and its hash", row)
	}
	if row[0] != stopRow[0].ID {
		t.Errorf("the ledger owes %s, the Stop %s: want the same row", row[0], stopRow[0].ID)
	}
	if o.head["platform"] != "read now, not at the Stop: pgraph log, digests, go.mod, Go files" {
		t.Errorf("header platform %q", o.head["platform"])
	}
	family := row[6]
	f.land("toolu_g", "Bash", lwBash("go test ./pkg"), nil)
	f.stop()
	if row := plwRow(t, f.out()); row == nil || row[4] != string(verify.ObligationMet) || row[6] != family {
		t.Errorf("after go test ./pkg: row %q, want met", row)
	}
}

// TestPLW1t_TheLedgerHeaderSaysWhatWasRead: with no log the row is unknown
// under the package alone and no header claims a read; after the file
// changes from what the sync read, the row is still judged but its family
// withholds the hash; a log that does not decode is one unknown row with no
// family under a header that says so; a session with no Go write prints no
// platform header. Break: print the read header whatever the log's state,
// and a project nobody synced reads as if its log had been read.
func TestPLW1t_TheLedgerHeaderSaysWhatWasRead(t *testing.T) {
	f := plwProject(t, false)
	f.land("toolu_e", "Edit", lwEdit(filepath.Join(f.cwd, "pkg", "a.go")), nil)
	f.stop()
	o := f.out()
	if row := plwRow(t, o); row == nil || row[4] != string(verify.ObligationUnknown) || row[6] != "example.com/m/pkg@unknown" || o.head["platform"] != "" {
		t.Errorf("no log: row %q, header %q; want unknown for example.com/m/pkg@unknown and no header", row, o.head["platform"])
	}

	f = plwProject(t, true)
	f.file("pkg/a.go", plwSource+"\nfunc G() {}\n")
	f.land("toolu_e", "Edit", lwEdit(filepath.Join(f.cwd, "pkg", "a.go")), nil)
	f.stop()
	if row := plwRow(t, f.out()); row == nil || row[4] != string(verify.ObligationOpen) || row[6] != "example.com/m/pkg@stale" {
		t.Errorf("an edited file: row %q, want open for example.com/m/pkg@stale", row)
	}

	f.file(".pgraph/pgraph.log", "pgraph-log v9\n")
	o = f.out()
	if row := plwRow(t, o); row == nil || row[4] != string(verify.ObligationUnknown) || row[6] != "-" ||
		o.head["platform"] != "the pgraph log could not be read whole, so its row reads unknown" {
		t.Errorf("a corrupt log: row %q, header %q", row, o.head["platform"])
	}

	f = plwProject(t, true)
	f.land("toolu_e", "Edit", lwEdit(f.file("README.txt", "a\n")), nil)
	f.stop()
	if o := f.out(); plwRow(t, o) != nil || strings.Contains(o.all, "platform:") {
		t.Errorf("no Go write: %q, want no platform row and no header", o.all)
	}
}

// TestPLW1u_PlatformReadReadsTheLogWhereOneIs: the verb's reader gives
// internal/platform the log under the root, so where one exists its families
// are judged, and where none does they are unknown, never unreadable; a root
// of "" reads no log. The header names each state it can print in 80
// columns, and none where no log was read. Break: read a missing log as an
// unreadable one, and a project nobody synced reads as a corrupt log.
func TestPLW1u_PlatformReadReadsTheLogWhereOneIs(t *testing.T) {
	f := plwProject(t, true)
	if r := platformRead(f.cwd, []string{"pkg/a.go"}); r.Log != verify.LogRead || len(r.Families) != 1 || r.Families[0].Facts != verify.FactsKnown || !r.Families[0].Tests {
		t.Errorf("a synced root: %+v, want pkg known with a test file", r)
	}
	if err := os.Remove(platform.LogPath(f.cwd)); err != nil {
		t.Fatal(err)
	}
	if r := platformRead(f.cwd, []string{"pkg/a.go"}); r.Log != verify.LogNone || len(r.Families) != 1 || r.Families[0].Facts != verify.FactsUnknown {
		t.Errorf("no log: %+v, want pkg unknown, not unreadable", r)
	}
	if r := platformRead("", []string{"pkg/a.go"}); r.Log != verify.LogNone || len(r.Families) != 0 {
		t.Errorf("no root: %+v, want nothing", r)
	}
	for _, s := range []verify.LogState{verify.LogRead, verify.LogUnreadable, verify.LogTimedOut} {
		if h := platformHeader(s); h == "" || len(strings.TrimSuffix(h, "\n")) > 80 || !strings.HasPrefix(h, "platform: ") {
			t.Errorf("header for %s: %q, want one line of at most 80 columns", s, h)
		}
	}
	if h := platformHeader(verify.LogNone); h != "" {
		t.Errorf("header with no log: %q, want none", h)
	}
}
