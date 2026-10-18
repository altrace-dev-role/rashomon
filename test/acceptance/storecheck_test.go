package acceptance

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The store's invariants, checked at the end of every test that made an env,
// whatever the test itself asserted: five files asserted them out of some
// three hundred tests, so a writer that broke them in a path only another
// test reaches went unseen.
//
//   - every line of every NDJSON file in the store parses as one JSON object;
//   - each record is a type its file holds, at a schema version this build
//     reads, with only the keys that type's allowlist names (the lists
//     TestH13_RecordKeySetsAreClosed holds);
//   - in records.ndjson, the ordered stream, every record carries a seq, and
//     the seqs are strictly increasing in file order, which is the order the
//     run's lock gave them. spill.ndjson is exempt: a spilled record is
//     written without the lock and carries no seq, by design.
//
// A test that damages the store on purpose says so with expectDamagedStore,
// naming why, and the check stands down for that test.

// expectDamagedStore turns the end-of-test store check off for this env.
func (e *env) expectDamagedStore(reason string) {
	e.t.Helper()
	if strings.TrimSpace(reason) == "" {
		e.t.Fatal("expectDamagedStore needs a reason")
	}
	e.damaged = reason
}

// checkStore is registered by newEnv and runs after the test.
func (e *env) checkStore() {
	e.checked = true
	if e.damaged != "" {
		return
	}
	for _, p := range storeProblems(e.home) {
		e.t.Errorf("store invariant: %s", p)
	}
}

// storeFile is what one store file may hold.
type storeFile struct {
	types   map[string][]string // record type -> its key allowlist
	ordered bool                // seq present and strictly increasing
}

func storeFiles() map[string]storeFile {
	call := map[string][]string{"declaration": declarationKeys, "execution": executionKeys, "terminal": terminalKeys}
	return map[string]storeFile{
		"records.ndjson":  {types: call, ordered: true},
		"spill.ndjson":    {types: call},
		"coverage.ndjson": {types: map[string][]string{"coverage": coverageKeys}},
		"gaps.ndjson":     {types: map[string][]string{"gap": gapKeys}},
	}
}

// storeProblems walks a store root and lists every invariant it breaks.
func storeProblems(home string) []string {
	var out []string
	known := storeFiles()
	err := filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil && path == home && errors.Is(err, fs.ErrNotExist) {
			return filepath.SkipDir // no store, which detach and some refusals leave
		}
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".ndjson") {
			return nil
		}
		rel, _ := filepath.Rel(home, path)
		spec, ok := known[d.Name()]
		if !ok {
			spec = storeFile{} // any other NDJSON file: lines must parse
		}
		out = append(out, fileProblems(path, rel, spec)...)
		return nil
	})
	if err != nil {
		out = append(out, fmt.Sprintf("walking %s: %v", home, err))
	}
	return out
}

func fileProblems(path, rel string, spec storeFile) []string {
	f, err := os.Open(path)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", rel, err)}
	}
	defer f.Close()
	var out []string
	var last int64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), store.MaxLine)
	for n := 1; sc.Scan(); n++ {
		where := fmt.Sprintf("%s:%d", rel, n)
		var fields map[string]any
		if err := json.Unmarshal(sc.Bytes(), &fields); err != nil || fields == nil {
			out = append(out, fmt.Sprintf("%s is not one JSON object: %q", where, sc.Text()))
			continue
		}
		if spec.types == nil {
			continue
		}
		out = append(out, recordProblems(where, fields, spec)...)
		if !spec.ordered {
			continue
		}
		seq, ok := fields["seq"].(float64)
		switch {
		case !ok || seq != float64(int64(seq)):
			out = append(out, fmt.Sprintf("%s: an ordered record with seq %v", where, fields["seq"]))
		case int64(seq) <= last:
			out = append(out, fmt.Sprintf("%s: seq %d follows seq %d", where, int64(seq), last))
		default:
			last = int64(seq)
		}
	}
	if err := sc.Err(); err != nil {
		out = append(out, fmt.Sprintf("%s: %v", rel, err))
	}
	return out
}

func recordProblems(where string, fields map[string]any, spec storeFile) []string {
	typ, _ := fields["type"].(string)
	allowed, ok := spec.types[typ]
	if !ok {
		return []string{fmt.Sprintf("%s: a %q record in a file that holds none", where, typ)}
	}
	var out []string
	if v, ok := fields["schema_version"].(float64); !ok || float64(int(v)) != v || !readable(typ, int(v)) {
		out = append(out, fmt.Sprintf("%s: schema_version %v, which this build does not read", where, fields["schema_version"]))
	}
	allow := map[string]bool{}
	for _, k := range allowed {
		allow[k] = true
	}
	for k := range pathsOf(record{fields: fields}) {
		if !allow[k] {
			out = append(out, fmt.Sprintf("%s: a %s carries key %q, which is not on its allowlist", where, typ, k))
		}
	}
	return out
}

// readable is whether this build reads a record of this kind at this
// version, asked of the store rather than written out here so the check moves
// with the reader. Both terms hold today: store.Accepts is what the reader
// applies to every record (classifyRunLine), and store.AcceptsKind is the
// per-kind contract, under which coverage and gaps stop at
// SessionSchemaVersion. When P-11 switches the reader to AcceptsKind and
// retires Accepts, the first term goes with it.
func readable(kind string, version int) bool {
	return store.Accepts(version) && store.AcceptsKind(kind, version)
}

// TestRubricTest13_TheStoreCheckCatchesEachBreak is the checker's own test: a
// healthy recorded call passes, and each hand-made break of a copy of it is
// named. Without it the check could pass everything and every test would
// still be green.
func TestRubricTest13_TheStoreCheckCatchesEachBreak(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))
	e.probe("end", testSession)
	if p := storeProblems(e.home); len(p) != 0 {
		t.Fatalf("a healthy store reads as broken: %v", p)
	}
	recordsFile := filepath.Join("runs", testSession, "records.ndjson")
	raw, err := os.ReadFile(filepath.Join(e.home, recordsFile))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("premise: %d ordered records", len(lines))
	}

	for _, tc := range []struct {
		name, want string
		edit       func([]string) []string
	}{
		{"a torn line", "is not one JSON object", func(l []string) []string {
			return append(l, l[0][:len(l[0])/2])
		}},
		{"a key off the allowlist", `key "command"`, func(l []string) []string {
			l[0] = strings.Replace(l[0], `{`, `{"command":"rm -rf /",`, 1)
			return l
		}},
		{"a repeated seq", "follows seq", func(l []string) []string {
			return append(l, l[len(l)-1])
		}},
		{"an ordered record without a seq", "an ordered record with seq <nil>", func(l []string) []string {
			l[0] = strings.Replace(l[0], `"seq":`, `"seq":null,"was_seq":`, 1)
			return l
		}},
		{"a schema from the future", "schema_version 99", func(l []string) []string {
			l[0] = strings.Replace(l[0], `"schema_version":4`, `"schema_version":99`, 1)
			return l
		}},
		{"a coverage record among the calls", `a "coverage" record`, func(l []string) []string {
			return append(l, `{"type":"coverage","schema_version":3}`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			dst := filepath.Join(home, recordsFile)
			if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
				t.Fatal(err)
			}
			body := strings.Join(tc.edit(append([]string(nil), lines...)), "\n") + "\n"
			if err := os.WriteFile(dst, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			p := storeProblems(home)
			if !strings.Contains(strings.Join(p, "\n"), tc.want) {
				t.Errorf("the check did not name %q; it said %v", tc.want, p)
			}
		})
	}

	// Versions are held per kind, in step with the store's reader: coverage
	// and gaps stop at SessionSchemaVersion, and a call record at 5 is one
	// today's reader skips, whatever the 1.2.0 contract will allow.
	for _, tc := range []struct{ file, line, want string }{
		{"coverage.ndjson", `{"type":"coverage","schema_version":4}`, "schema_version 4"},
		{"gaps.ndjson", `{"type":"gap","schema_version":4}`, "schema_version 4"},
		{"spill.ndjson", `{"type":"declaration","schema_version":5}`, "schema_version 5"},
		{"spill.ndjson", `{"type":"declaration","schema_version":2.5}`, "schema_version 2.5"},
	} {
		home := t.TempDir()
		dst := filepath.Join(home, "runs", testSession, tc.file)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, []byte(tc.line+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if p := storeProblems(home); !strings.Contains(strings.Join(p, "\n"), tc.want) {
			t.Errorf("%s holding %s: the check did not name %q; it said %v", tc.file, tc.line, tc.want, p)
		}
	}

	// A spilled record carries no seq, by design, and is not a break.
	spill := filepath.Join(e.home, "runs", testSession, "spill.ndjson")
	unordered := strings.Replace(lines[len(lines)-1], `"seq":`, `"seq":null,"x":`, 1)
	unordered = strings.Replace(unordered, `,"x":`+seqOf(t, lines[len(lines)-1]), ``, 1)
	if err := os.WriteFile(spill, []byte(unordered+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p := storeProblems(e.home); len(p) != 0 {
		t.Errorf("a spilled record without a seq reads as a break: %v", p)
	}
}

func seqOf(t *testing.T, line string) string {
	t.Helper()
	var r struct {
		Seq json.Number `json:"seq"`
	}
	if err := json.Unmarshal([]byte(line), &r); err != nil {
		t.Fatal(err)
	}
	return r.Seq.String()
}

// TestRubricTest13_EveryEnvIsCheckedWhenItsTestEnds holds newEnv to
// registering the check: the env a finished subtest made has been checked.
func TestRubricTest13_EveryEnvIsCheckedWhenItsTestEnds(t *testing.T) {
	var e *env
	t.Run("a test that makes an env", func(t *testing.T) { e = newEnv(t) })
	if e == nil || !e.checked {
		t.Error("the store of an env whose test has ended was never checked")
	}
}
