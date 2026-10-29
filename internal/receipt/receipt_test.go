package receipt

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// seedRun writes one run's records file holding lines, last written at ms.
func seedRun(t *testing.T, st *store.Store, session string, ms int64, lines ...string) {
	t.Helper()
	dir := st.RunDir(session)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var body []byte
	for _, l := range lines {
		body = append(body, l+"\n"...)
	}
	path := filepath.Join(dir, store.FileRecords)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	at := time.UnixMilli(ms)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// checkpoint is a checkpoint line for project recorded at ms, its keys spaced
// as json.Marshal never spaces them, so a re-marshal cannot reproduce it.
func checkpoint(project string, ms int64) string {
	return fmt.Sprintf(`{"type": "verify", "schema_version": %d, "recorded_at_unix_ms": %d, "project_root": %q}`,
		store.EventSchemaVersion, ms, project)
}

// TestRG1e_LatestIsTheProjectsNewestCheckpointAsStored (5.5, D121): Latest
// is the newest checkpoint by recorded_at among the project's runs, not the
// newest written run, and its bytes are the stored line's. Another project's
// newer record is not it; a run last written before the best record found is
// not read (its file cannot hold a newer record; this one, clock-skewed,
// says it does, so reading it would change the answer). Break: take the
// first match, drop the stop, re-marshal the line, or skip the project
// check, and the first row names it.
func TestRG1e_LatestIsTheProjectsNewestCheckpointAsStored(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const p, q = "/work/p", "/work/q"
	want := checkpoint(p, 1_900_000)
	seedRun(t, st, "sess-other", 4_000_000, checkpoint(q, 3_900_000))
	seedRun(t, st, "sess-later-write", 3_000_000, checkpoint(p, 1_500_000), `{"type":"prompt","schema_version":5}`)
	seedRun(t, st, "sess-best", 2_000_000, checkpoint(p, 1_000_000), want)
	seedRun(t, st, "sess-skewed", 1_000_000, checkpoint(p, 2_500_000))

	got, ok, err := Latest(st, p)
	if err != nil || !ok || string(got) != want {
		t.Errorf("Latest(%s) = %q, ok %t, err %v; want %q", p, got, ok, err, want)
	}
	if got, ok, err := Latest(st, "/work/none"); err != nil || ok || got != nil {
		t.Errorf("a project with no checkpoint: %q, ok %t, err %v; want none", got, ok, err)
	}
	empty, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got, ok, err := Latest(empty, p); err != nil || ok || got != nil {
		t.Errorf("a store with no runs: %q, ok %t, err %v; want none", got, ok, err)
	}
}

// TestRG1e_TheTrailerIsTheRecordsDigest: the digest is SHA-256 over the
// bytes as given, lowercase hex behind "sha256:", and the trailer is the key,
// a colon and a space, and the digest. Break: digest anything else, and the
// known vector fails.
func TestRG1e_TheTrailerIsTheRecordsDigest(t *testing.T) {
	const abc = "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := Digest([]byte("abc")); got != abc {
		t.Errorf("Digest(abc) = %s, want %s", got, abc)
	}
	if got, want := Trailer([]byte("abc")), "Rashomon-Receipt: "+abc; got != want {
		t.Errorf("Trailer(abc) = %q, want %q", got, want)
	}
}
