package acceptance

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestH15_Permissions asserts the store is readable only by its owner. The
// records name every tool a session invoked and the path of its transcript,
// which together describe what someone was working on.
func TestH15_Permissions(t *testing.T) {
	home := t.TempDir()
	if res := runHook(t, home, defaultPayload().build(t)); res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}

	want := map[string]fs.FileMode{
		".":                                0o700,
		"runs":                             0o700,
		filepath.Join("runs", testSession): 0o700,
		filepath.Join("runs", testSession, "records.ndjson"):  0o600,
		filepath.Join("runs", testSession, "coverage.ndjson"): 0o600,
		"install.key":  0o600,
		"install.json": 0o600,
	}

	got := walkStore(t, home)
	for rel, mode := range want {
		entry, ok := got[rel]
		if !ok {
			t.Errorf("%s is missing from the store", rel)
			continue
		}
		if entry.mode.Perm() != mode {
			t.Errorf("%s has mode %04o, want %04o", rel, entry.mode.Perm(), mode)
		}
	}
}

// TestH15_NDJSONFraming asserts the framing a downstream reader depends on.
func TestH15_NDJSONFraming(t *testing.T) {
	home := t.TempDir()
	declarationsAfter(t, home, "ls -la", `echo "with a \" quote and a newline\n inside"`, "git log --oneline")

	for _, name := range []string{"records.ndjson", "coverage.ndjson"} {
		path := filepath.Join(home, "runs", testSession, name)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}

		if len(body) == 0 {
			t.Errorf("%s is empty", name)
			continue
		}
		if body[len(body)-1] != '\n' {
			t.Errorf("%s does not end with a newline; a reader cannot tell a truncated last record from a complete one", name)
		}
		for i, line := range bytes.Split(bytes.TrimSuffix(body, []byte("\n")), []byte("\n")) {
			if len(bytes.TrimSpace(line)) == 0 {
				t.Errorf("%s line %d is blank", name, i+1)
			}
		}
	}
}

// TestH15_SchemaVersionOnEveryRecord is what lets a reader skip a record it does
// not understand instead of guessing at it.
func TestH15_SchemaVersionOnEveryRecord(t *testing.T) {
	home := t.TempDir()
	declarationsAfter(t, home, "ls", "pwd")

	var all []record
	all = append(all, readRecords(t, home, testSession, "records.ndjson")...)
	all = append(all, readRecords(t, home, testSession, "coverage.ndjson")...)

	if len(all) == 0 {
		t.Fatal("no records were written")
	}
	for i, r := range all {
		if r.fields["schema_version"] != float64(1) {
			t.Errorf("record %d (type %q) has schema_version %v, want 1", i, r.typ(), r.fields["schema_version"])
		}
		if r.typ() == "" {
			t.Errorf("record %d has no type discriminator", i)
		}
	}
}

// TestH15_SessionIDCannotEscapeTheStore covers a value that arrives from outside
// the process and is then used to build a path. Joined unchecked, a session id
// of ../../.. writes wherever it likes.
func TestH15_SessionIDCannotEscapeTheStore(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "store")

	p := defaultPayload()
	p.SessionID = "../../escaped"

	if res := runHook(t, home, p.build(t)); res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}

	// Nothing may exist outside the store root.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading %s: %v", root, err)
	}
	for _, e := range entries {
		if e.Name() != "store" {
			t.Errorf("%s was created outside the store root", e.Name())
		}
	}

	// And the run still has to land somewhere, under a name derived from the id.
	runs, err := os.ReadDir(filepath.Join(home, "runs"))
	if err != nil {
		t.Fatalf("reading runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d run directories, want 1", len(runs))
	}
	if name := runs[0].Name(); name == "../../escaped" || name == "escaped" {
		t.Errorf("run directory is named %q, which is the raw session id", name)
	}
}
