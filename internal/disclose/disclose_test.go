package disclose

import (
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

func jsonTags(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		out = append(out, strings.Split(t.Field(i).Tag.Get("json"), ",")[0])
	}
	return out
}

// The inventory's keys, a literal copy of 7.2.2.
func TestInventoryCarriesTheSpecsKeys(t *testing.T) {
	cases := map[reflect.Type][]string{
		reflect.TypeOf(Inventory{}):     {"version", "scope_version", "entries", "files", "settings_writes", "outputs", "never_emit", "never_write", "switches", "env_vars", "screen_paragraphs"},
		reflect.TypeOf(HookEntry{}):     {"event", "matcher", "subcommand", "timeout_s", "why", "prints", "owner"},
		reflect.TypeOf(OutputRule{}):    {"event", "field", "audience", "mirror_verified", "mirror_source", "template_ids"},
		reflect.TypeOf(FileRule{}):      {"base", "pattern", "mode", "purpose", "holds", "never_holds", "removed_by", "owner"},
		reflect.TypeOf(SettingsWrite{}): {"file", "json_ptr", "writer"},
		reflect.TypeOf(Forbidden{}):     {"event", "field", "why", "except"},
		reflect.TypeOf(Switch{}):        {"name", "default", "command", "stored", "ratchet"},
		reflect.TypeOf(EnvVar{}):        {"name", "effect", "test_only"},
		reflect.TypeOf(Paragraph{}):     {"owner", "text", "cite"},
	}
	for typ, want := range cases {
		if got := jsonTags(typ); !reflect.DeepEqual(got, want) {
			t.Errorf("%s keys = %v, want %v", typ.Name(), got, want)
		}
	}
}

func TestTRU8b_ScopeVersionIsPinned(t *testing.T) {
	if ScopeVersion != 3 {
		t.Errorf("ScopeVersion = %d; a bump re-asks consent and must be deliberate", ScopeVersion)
	}
	inv, err := Default()
	if err != nil {
		t.Errorf("Default() = %v; the file list is filled, so this build is no longer a stub", err)
	}
	if inv.ScopeVersion != ScopeVersion {
		t.Errorf("Default().ScopeVersion = %d, want %d", inv.ScopeVersion, ScopeVersion)
	}
}

// A caller that ignores the error must not see a scope version attached to
// an inventory missing either half: that reads as consent recorded for a
// build whose files, or whose screen, nobody was shown. The stub kept to
// this by returning the zero Inventory; the filled one carries its scope
// version beside its files and its paragraphs. Each half is checked on its
// own: "nothing else in it" stopped meaning "no files" once the paragraphs
// joined the inventory, and a check on the whole value passed with the files
// gone.
func TestDefaultNeverPairsAScopeVersionWithAnEmptyInventory(t *testing.T) {
	inv, _ := Default()
	if inv.ScopeVersion == 0 {
		return
	}
	if len(inv.Files) == 0 {
		t.Errorf("Default() carries scope version %d and no files", inv.ScopeVersion)
	}
	if len(inv.Paragraphs) == 0 {
		t.Errorf("Default() carries scope version %d and no screen paragraphs", inv.ScopeVersion)
	}
}

// TestTRU23_EveryWrittenPathIsInTheInventory drives the real store package,
// and the two sibling packages that write under the same store root --
// recap and baseline -- through every write each has: Open's two root
// files, a declaration, a spilled execution, a coverage record, a probe
// mark, a gap, a host-scoped forget, a machine pause, a confirmed work tree
// (and its replacement file), a recap claim and
// failure, and a baseline update. It checks every resulting path on disk
// against Default().Files. A path any of the three writes that is not
// covered here is the regression this test exists to catch: a FileRule list
// that someone forgot to extend when a new file was added.
//
// recap and baseline are driven here, not inside internal/store's own
// tests, because what this test checks is cross-package: the FileRule list
// this package renders has to cover every path ANY of the packages that
// write under a store root create, and recap.StateFileName and
// baseline.DirName are owned by packages store does not import.
func TestTRU23_EveryWrittenPathIsInTheInventory(t *testing.T) {
	root := t.TempDir()
	s, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	const sessionID = "sess-tru23"

	if err := s.AppendDeclaration(store.Declaration{
		Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
		SessionID: sessionID, ToolUseID: "tu-1", ToolName: "Bash",
	}); err != nil {
		t.Fatalf("AppendDeclaration: %v", err)
	}
	if err := s.SpillExecution(store.Execution{
		Type: store.TypeExecution, SchemaVersion: store.SchemaVersion,
		SessionID: sessionID, ToolUseID: "tu-1",
	}); err != nil {
		t.Fatalf("SpillExecution: %v", err)
	}
	if err := s.AppendCoverage(store.Coverage{
		Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion,
		SessionID: sessionID, InstallID: s.InstallID(), Phase: "pretool",
		State: "ok", HookEntry: "PreToolUse", Probe: "fresh",
	}); err != nil {
		t.Fatalf("AppendCoverage: %v", err)
	}
	if err := s.MarkProbe(sessionID, time.Now()); err != nil {
		t.Fatalf("MarkProbe: %v", err)
	}
	if err := s.AppendGap(store.Gap{
		Type: store.TypeGap, SchemaVersion: store.SessionSchemaVersion,
		SessionID: sessionID, Reason: store.GapForget,
	}); err != nil {
		t.Fatalf("AppendGap: %v", err)
	}
	if _, err := s.ForgetHost("example.com", time.Now()); err != nil {
		t.Fatalf("ForgetHost: %v", err)
	}
	if err := s.AppendSteer(sessionID, func(io.ReaderAt) ([]store.SteerLine, error) {
		return []store.SteerLine{tru23Steer{Type: "steer"}}, nil
	}); err != nil {
		t.Fatalf("AppendSteer: %v", err)
	}
	if _, err := store.RecordHarnessMismatch(root, "claude@cursor", sessionID, []string{"cursor_version"}, time.Now()); err != nil {
		t.Fatalf("RecordHarnessMismatch: %v", err)
	}
	if err := s.RecordResolvedCap(store.CapSetting{Bytes: store.DefaultCapBytes, Raw: "lots", Invalid: true}); err != nil {
		t.Fatalf("RecordResolvedCap: %v", err)
	}
	if _, _, err := store.Pause(root, time.Now()); err != nil {
		t.Fatalf("store.Pause: %v", err)
	}
	if err := store.WriteRecordScopeMarker(root, store.RecordScopeV5); err != nil {
		t.Fatalf("store.WriteRecordScopeMarker: %v", err)
	}
	if err := store.WriteSteerDisclosureMarker(root, 2); err != nil {
		t.Fatalf("store.WriteSteerDisclosureMarker: %v", err)
	}
	if err := store.SetRootInWorkTree(root, root); err != nil {
		t.Fatalf("store.SetRootInWorkTree: %v", err)
	}
	// Their replacements exist only between the write and the rename; make
	// one where each would be, to check its rule.
	for _, pattern := range []string{"." + store.FileInstallMeta + ".tmp-*", "." + store.FileHarnessCounts + ".rashomon-*",
		"." + store.FileRecordScope + ".tmp-*", "." + store.FileSteerDisclosure + ".tmp-*", recap.StateTempPrefix + "*"} {
		tmp, err := os.CreateTemp(root, pattern)
		if err != nil {
			t.Fatal(err)
		}
		if err := tmp.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := recap.Claim(root, sessionID, "prompt-1", time.Now(), true); err != nil {
		t.Fatalf("recap.Claim: %v", err)
	}
	recap.RecordFailure(root, time.Now())
	// The one shared temp name builds through v1.1.0 left; nothing writes it
	// now, and a store one of them wrote may still hold it.
	if err := os.WriteFile(filepath.Join(root, recap.StateTempName), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if res := baseline.Update(root, "tru23-project", sessionID, time.Now(), []string{"example.org"}); res.Err != nil {
		t.Fatalf("baseline.Update: %v", res.Err)
	}
	index := verify.IndexPath(root, "/tru23/project")
	if _, err := verify.WriteIndex(index, &verify.ProjectIndex{ProjectKey: "/tru23/project"}); err != nil {
		t.Fatalf("verify.WriteIndex: %v", err)
	}
	if tmp, err := os.CreateTemp(filepath.Dir(index), "."+filepath.Base(index)+".tmp-*"); err != nil {
		t.Fatal(err)
	} else if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}

	inv, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}

	var created []string
	if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		created = append(created, rel)
		return nil
	}); err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	if len(created) == 0 {
		t.Fatal("the test drove the store but no file was written; it is checking nothing")
	}
	for _, rel := range created {
		if !matchesAFileRule(rel, inv.Files) {
			t.Errorf("store wrote %q, which Default().Files does not cover", rel)
		}
	}
}

// matchesAFileRule reports whether rel, a path relative to the store root,
// is described by one of rules. <session> in a rule's Base stands for one
// path segment, matched the same way path.Match matches a "*".
func matchesAFileRule(rel string, rules []FileRule) bool {
	relSlash := filepath.ToSlash(rel)
	for _, r := range rules {
		pattern := r.Pattern
		if r.Base != "" {
			pattern = r.Base + "/" + r.Pattern
		}
		pattern = strings.ReplaceAll(pattern, "<session>", "*")
		if ok, _ := path.Match(pattern, relSlash); ok {
			return true
		}
	}
	return false
}

// writeCallSite matches a line that can create or overwrite a file: the four
// stdlib calls that put new bytes on disk, and store's own writeSmall and
// ReplaceFile, which wrap them (ReplaceFile also where another package calls
// it, as store.ReplaceFile). It deliberately does not match the line defining
// writeSmall itself (TestTRU23_WriteCallSitesArePinned excludes lines that
// start the way a top-level function declaration does), so the primitive's
// one real OpenFile call is counted once, at writeSmall's definition, and
// every caller that reaches a NEW path through it is counted again at the
// call site -- which is what lets a new call site change the count at all.
var writeCallSite = regexp.MustCompile(`os\.OpenFile\(|os\.WriteFile\(|os\.Create\(|os\.CreateTemp\(|writeSmall\(|ReplaceFile\(`)

// countWriteCallSites counts writeCallSite matches in dir's non-test .go
// files, one count per line so a multi-call line is not undercounted.
func countWriteCallSites(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", dir, err)
	}
	total := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		for _, line := range strings.Split(string(body), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "func ") {
				// A declaration line, not a call: writeSmall's own
				// `func writeSmall(...)` must not count as a second call to
				// itself, and a line like `func flushFile(path string) error`
				// never matches writeCallSite anyway, but a future
				// `func writeSmall(...)`-shaped helper could.
				continue
			}
			total += len(writeCallSite.FindAllString(line, -1))
		}
	}
	return total
}

// TestTRU23_WriteCallSitesArePinned is TRU-23's trip-wire for the gap
// TestTRU23_EveryWrittenPathIsInTheInventory cannot close by itself: that
// test only sees a write that the lines it drives actually reach, so a new
// write added to a code path it does not call (a new verb, a new error
// branch) would write a file with nobody checking it against Default().Files.
//
// It is deliberately a blunt instrument, not a semantic one: it counts
// textual call sites of the four file-creating stdlib functions plus
// writeSmall and ReplaceFile across store's, recap's, baseline's, consent's and verify's own non-test source, and
// fails the moment that count moves, in either direction. It cannot tell a
// new call site that writes an already-covered path from one that writes an
// uncovered one -- that distinction needs a human, which is the point. A
// failure here means: find the new or removed call site (`git diff` against
// these three packages), decide whether it reaches a path Default().Files
// does not already describe, update defaultFiles and drive the new path
// through TestTRU23_EveryWrittenPathIsInTheInventory if so, and only then
// move this count.
func TestTRU23_WriteCallSitesArePinned(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	disclosureDir := filepath.Dir(thisFile)
	internalDir := filepath.Dir(disclosureDir)

	cases := []struct {
		pkg   string
		count int
	}{
		// Pinned 2026-10-22: internal/store/store.go (writeSmall's own
		// OpenFile, MarkProbe, two append opens, writeSmall x2,
		// createExclusive's CreateTemp and its no-link fallback OpenFile) +
		// pause.go (one writeSmall) + gaps.go (three append/rewrite opens) +
		// evict.go (the gap log's open) + install_record.go (replaceFile's
		// CreateTemp, which rewrites install.json in place) + capbytes.go
		// (RecordResolvedCap's writeSmall, resolved-cap) + lockwait.go
		// (noteLockWait's open, runs/<session>/lockwait, written only after a
		// contended lock, so the inventory test does not reach it) +
		// lockpath.go (LockPath's open, baseline/.lock) + steerlog.go
		// (AppendSteer's append open, runs/<session>/steer.ndjson) +
		// harnesscounts.go (the lock sidecar's open, harness_counts.lock,
		// and the replacement's CreateTemp, harness_counts.json).
		// Moved 2026-10-27 (1.2q) to 22: replaceFile joined the pattern, so
		// install_record.go's one call to it counts, and scope.go's
		// WriteRecordScopeMarker (replaceFile, record_scope) is the new site.
		// Moved 2026-10-27 (2.2c) to 23: WriteSteerDisclosureMarker's
		// ReplaceFile, steer_disclosure, the second view of consent.json.
		{"store", 23},
		// Pinned 2026-08: internal/recap/state.go's writeState, one
		// os.WriteFile.
		{"recap", 1},
		// Pinned 2026-08: internal/baseline/baseline.go's save, one
		// os.CreateTemp.
		{"baseline", 1},
		// Pinned 2026-10: internal/consent/record.go's Append, one
		// os.OpenFile, and replace, one os.CreateTemp.
		{"consent", 2},
		// Pinned 2026-10-27 (1.6): internal/verify/index.go's WriteIndex, one
		// store.ReplaceFile, verify/projects/<name>.json, the project index
		// the Stop hook's recap reads and writes since 2.4 (indexFiles in the
		// inventory, verify's tree in purge's plan). store's 22 was unchanged
		// by the export, since the pattern follows replaceFile to ReplaceFile.
		{"verify", 1},
	}
	for _, c := range cases {
		got := countWriteCallSites(t, filepath.Join(internalDir, c.pkg))
		if got != c.count {
			t.Errorf("internal/%s: %d write call site(s), want %d (pinned) -- see this test's doc comment", c.pkg, got, c.count)
		}
	}
}

// tru23Steer is the least steer line AppendSteer writes: the store names only
// CapText of steer.Line, so the inventory test needs nothing more to reach
// the steer log's path.
type tru23Steer struct {
	Type string `json:"type"`
}

func (tru23Steer) CapText() bool { return false }
