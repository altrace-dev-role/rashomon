package baseline

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/safe"
)

// both runs a and b at once and waits for both.
func both(t *testing.T, a, b func()) {
	t.Helper()
	start := make(chan struct{})
	fail := func(err error) { t.Error(err) }
	da := safe.Go(func() { <-start; a() }, fail)
	db := safe.Go(func() { <-start; b() }, fail)
	close(start)
	<-da
	<-db
}

// onDisk is the host set of the project file under root, as saved.
func onDisk(t *testing.T, root, key string) map[string]entry {
	t.Helper()
	f, err := parse(filepath.Join(root, "baseline", fileName(key)))
	if err != nil {
		t.Fatal(err)
	}
	return f.Hosts
}

// TestRubricHLD2_ConcurrentRendersLoseNoHost: two renders of two sessions in
// one project fold their hosts into one file. Each loaded the file, folded
// and renamed its copy into place with no lock, so the second rename threw
// away the first render's host -- in 200 of 200 rounds of the probe -- and a
// later session was told that host was new.
func TestRubricHLD2_ConcurrentRendersLoseNoHost(t *testing.T) {
	for round := 0; round < 40; round++ {
		root, key := t.TempDir(), "/proj"
		seed := Update(root, key, "s0", t0, []string{"seed.example"})
		if seed.Err != nil {
			t.Fatal(seed.Err)
		}
		var ra, rb Result
		both(t,
			func() { ra = Update(root, key, "s1", t0.Add(time.Hour), []string{"one.example"}) },
			func() { rb = Update(root, key, "s2", t0.Add(2*time.Hour), []string{"two.example"}) })
		if ra.Err != nil || rb.Err != nil {
			t.Fatalf("round %d: %v, %v", round, ra.Err, rb.Err)
		}
		hosts := onDisk(t, root, key)
		for _, h := range []string{"seed.example", "one.example", "two.example"} {
			if _, ok := hosts[h]; !ok {
				t.Fatalf("round %d: %s lost; the file holds %v", round, h, keys(hosts))
			}
		}
	}
}

// TestRubricHLD2_ForgetAndRenderLoseNoChange: `forget --host` rewrites the
// same file a render does, and each could undo the other: the render saving
// a copy loaded before the forget put the forgotten host back, and the
// forget saving a copy loaded before the render dropped the render's host.
func TestRubricHLD2_ForgetAndRenderLoseNoChange(t *testing.T) {
	for round := 0; round < 40; round++ {
		root, key := t.TempDir(), "/proj"
		if r := Update(root, key, "s0", t0, []string{"gone.example", "kept.example"}); r.Err != nil {
			t.Fatal(r.Err)
		}
		var r Result
		var n int
		var ferr error
		both(t,
			func() { r = Update(root, key, "s1", t0.Add(time.Hour), []string{"new.example"}) },
			func() { n, ferr = ForgetFile(filepath.Join(root, "baseline", fileName(key)), "gone.example") })
		if r.Err != nil || ferr != nil || n != 1 {
			t.Fatalf("round %d: render %v, forget %d, %v", round, r.Err, n, ferr)
		}
		hosts := onDisk(t, root, key)
		if _, ok := hosts["gone.example"]; ok {
			t.Fatalf("round %d: the forgotten host came back: %v", round, keys(hosts))
		}
		for _, h := range []string{"kept.example", "new.example"} {
			if _, ok := hosts[h]; !ok {
				t.Fatalf("round %d: %s lost: %v", round, h, keys(hosts))
			}
		}
	}
}

// TestRubricHLD2_FoldIsPure: Fold is the whole decision and touches nothing
// it was handed, so the locked load-fold-save around it is the only place a
// file changes.
func TestRubricHLD2_FoldIsPure(t *testing.T) {
	in := File{Version: Version, Project: "/p", Basis: Basis, EstablishedBySession: "s0", EstablishedAtUnixMS: 1,
		Hosts: map[string]entry{"a.example": {FirstSeenSession: "s0", FirstSeenAtUnixMS: 1, LastSeenAtUnixMS: 1}}}
	before := fmt.Sprint(in.Hosts)
	out, res, changed := Fold(in, true, "s1", time.UnixMilli(5), []string{"a.example", "b.example"})
	if fmt.Sprint(in.Hosts) != before {
		t.Fatalf("Fold changed the file it was handed: %v, was %v", in.Hosts, before)
	}
	if !changed || len(out.Hosts) != 2 || !reflect.DeepEqual(res.Novel, []string{"b.example"}) {
		t.Fatalf("changed %v, hosts %v, novel %v", changed, keys(out.Hosts), res.Novel)
	}
	if _, _, changed := Fold(out, true, "s1", time.UnixMilli(5), []string{"a.example", "b.example"}); changed {
		t.Fatalf("folding the same session again changed the file")
	}
}

// fileWith is an established baseline of n hosts h0..h(n-1), host i last
// seen at i+1.
func fileWith(n int) File {
	f := File{Version: Version, Project: "/p", Basis: Basis, EstablishedBySession: "s0", EstablishedAtUnixMS: 1,
		Hosts: make(map[string]entry, n)}
	for i := 0; i < n; i++ {
		f.Hosts[fmt.Sprintf("h%d.example", i)] = entry{FirstSeenSession: "s0", FirstSeenAtUnixMS: 1, LastSeenAtUnixMS: int64(i + 1)}
	}
	return f
}

// TestRES11_EveryCapAtNAndNPlusOne (the baseline's row): a project baseline
// holds at most maxBaselineHosts hosts. At N nothing goes; at N+1 the least
// recently seen host goes, never one the folding session named, and the
// eviction is counted. A session that alone names more than the cap cannot
// be folded without dropping its own hosts, so its novelty is unknown and
// nothing is written.
func TestRES11_EveryCapAtNAndNPlusOne(t *testing.T) {
	if maxBaselineHosts != 4096 {
		t.Fatalf("maxBaselineHosts = %d, want 4096", maxBaselineHosts)
	}
	at, res, _ := Fold(fileWith(maxBaselineHosts-1), true, "s1", time.UnixMilli(1<<40), []string{"new.example"})
	if res.Err != nil || len(at.Hosts) != maxBaselineHosts || res.Evicted != 0 {
		t.Fatalf("at N: %d hosts, %d evicted, %v", len(at.Hosts), res.Evicted, res.Err)
	}
	over, res, _ := Fold(fileWith(maxBaselineHosts), true, "s1", time.UnixMilli(1<<40), []string{"new.example", "h0.example"})
	if res.Err != nil || len(over.Hosts) != maxBaselineHosts || res.Evicted != 1 {
		t.Fatalf("at N+1: %d hosts, %d evicted, %v", len(over.Hosts), res.Evicted, res.Err)
	}
	if _, ok := over.Hosts["h1.example"]; ok {
		t.Errorf("at N+1 the least recently seen host not named by this session, h1, survived")
	}
	for _, h := range []string{"h0.example", "new.example", "h2.example"} {
		if _, ok := over.Hosts[h]; !ok {
			t.Errorf("at N+1 %s went", h)
		}
	}
	if !reflect.DeepEqual(res.Novel, []string{"new.example"}) {
		t.Errorf("novel %v, want [new.example]", res.Novel)
	}
	// An old session rendered late names h0, the least recently seen host
	// of all: it stays, because this fold reports on it, and h1 goes.
	old, res, _ := Fold(fileWith(maxBaselineHosts), true, "s-old", time.UnixMilli(1), []string{"new.example", "h0.example"})
	if _, ok := old.Hosts["h0.example"]; !ok || res.Evicted != 1 {
		t.Errorf("an old session's own host was evicted (%d evicted)", res.Evicted)
	}
	if _, ok := old.Hosts["h1.example"]; ok {
		t.Errorf("an old session's fold kept h1, the least recently seen host it did not name")
	}

	many := make([]string, maxBaselineHosts+1)
	for i := range many {
		many[i] = fmt.Sprintf("m%d.example", i)
	}
	root := t.TempDir()
	if r := Update(root, "/p", "s9", t0, many); !errors.Is(r.Err, ErrTooManyHosts) {
		t.Fatalf("a session naming %d hosts: %v, want ErrTooManyHosts", len(many), r.Err)
	}
	if _, err := os.Stat(filepath.Join(root, "baseline", fileName("/p"))); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a baseline was written for a session past the cap: %v", err)
	}
	if r := Update(root, "/p", "s9", t0, many[:maxBaselineHosts]); r.Err != nil || r.Known != maxBaselineHosts {
		t.Errorf("a session naming exactly %d hosts: known %d, %v", maxBaselineHosts, r.Known, r.Err)
	}
}

// TestRubricLLD10_AnUnchangedRenderWritesNothing: a render that learns
// nothing new -- the same session rendered again -- leaves the file as it
// is, rather than rewriting every host the project has seen.
func TestRubricLLD10_AnUnchangedRenderWritesNothing(t *testing.T) {
	root, key := t.TempDir(), "/proj"
	if r := Update(root, key, "s0", t0, []string{"a.example"}); r.Err != nil {
		t.Fatal(r.Err)
	}
	if r := Update(root, key, "s1", t0.Add(time.Hour), []string{"a.example", "b.example"}); r.Err != nil {
		t.Fatal(r.Err)
	}
	path := filepath.Join(root, "baseline", fileName(key))
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	r := Update(root, key, "s1", t0.Add(time.Hour), []string{"a.example", "b.example"})
	if r.Err != nil || !reflect.DeepEqual(r.Novel, []string{"b.example"}) {
		t.Fatalf("re-render: novel %v, %v", r.Novel, r.Err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Errorf("a render that changed nothing replaced the file")
	}
}

// TestRubricLLD10_AnOversizedFileIsUnknown: a baseline file is read whole,
// so it is read only up to maxBaselineBytes: a file of exactly that size is
// read, and one byte more is not read at all -- novelty is unknown, as for a
// corrupt file, rather than an unbounded read.
func TestRubricLLD10_AnOversizedFileIsUnknown(t *testing.T) {
	sized := func(n int) []byte {
		head := []byte(`{"version":1,"project":"/proj","basis":"declared","established_by_session":"s0","established_at_unix_ms":1,"hosts":{},"pad":"`)
		body := append(head, bytes.Repeat([]byte("x"), n-len(head)-2)...)
		return append(body, `"}`...)
	}
	for _, tc := range []struct {
		size int
		read bool
	}{{maxBaselineBytes, true}, {maxBaselineBytes + 1, false}} {
		root, key := t.TempDir(), "/proj"
		dir := filepath.Join(root, "baseline")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		body := sized(tc.size)
		if len(body) != tc.size {
			t.Fatalf("built %d bytes, want %d", len(body), tc.size)
		}
		if err := os.WriteFile(filepath.Join(dir, fileName(key)), body, 0o600); err != nil {
			t.Fatal(err)
		}
		r := Update(root, key, "s1", t0, []string{"a.example"})
		if read := r.Err == nil; read != tc.read {
			t.Errorf("a %d-byte baseline: read %v (%v), want %v", tc.size, read, r.Err, tc.read)
		}
	}
}

func keys(m map[string]entry) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
