package disclose

import (
	"io/fs"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
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
// an otherwise-empty inventory: that reads as consent recorded for a build
// that has nothing in it. The stub kept to this by returning the zero
// Inventory; the filled one carries its scope version beside its files.
func TestDefaultNeverPairsAScopeVersionWithAnEmptyInventory(t *testing.T) {
	inv, _ := Default()
	if inv.ScopeVersion != 0 && reflect.DeepEqual(inv, Inventory{ScopeVersion: inv.ScopeVersion}) {
		t.Errorf("Default() = %+v: a scope version on an inventory with nothing else in it", inv)
	}
}

// TestTRU23_EveryWrittenPathIsInTheInventory drives the real store package
// through every write it has -- Open's two root files, a declaration, a
// spilled execution, a coverage record, a probe mark, a gap and a host-scoped
// forget -- and checks every resulting path on disk against Default().Files.
// A path the store writes that is not covered here is the regression this
// test exists to catch: a FileRule list that someone forgot to extend when a
// new file was added.
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
