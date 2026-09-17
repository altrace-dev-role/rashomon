package acceptance

import (
	"regexp"
	"testing"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestH14_ShapeFieldsArePresent(t *testing.T) {
	e := newEnv(t)
	d := e.declarationsAfter("git status --short")[0]

	if got, _ := nested(d, "shape.program"); got != "git" {
		t.Errorf("program is %v, want \"git\"", got)
	}
	if got, _ := nested(d, "shape.verb_class"); got != "vcs" {
		t.Errorf("verb_class is %v, want \"vcs\"", got)
	}
	if got, _ := nested(d, "shape.argc"); got != float64(3) {
		t.Errorf("argc is %v, want 3", got)
	}
	digest, _ := nested(d, "shape.digest")
	if s, _ := digest.(string); !hex64.MatchString(s) {
		t.Errorf("digest is %v, want 64 hex characters", digest)
	}
	if got := d.fields["schema_version"]; got != float64(1) {
		t.Errorf("schema_version is %v, want 1", got)
	}
}

// TestH14_ProgramIsTheBasename keeps the absolute path of whatever was run out
// of the record. A path like /Users/someone/clients/acme-migration/bin/deploy
// is a description of the user's work, and the classification only ever needed
// the last element.
func TestH14_ProgramIsTheBasename(t *testing.T) {
	e := newEnv(t)
	d := e.declarationsAfter("/usr/local/opt/private-project/bin/curl https://example.invalid")[0]

	if got, _ := nested(d, "shape.program"); got != "curl" {
		t.Errorf("program is %v, want \"curl\"", got)
	}
	if got, _ := nested(d, "shape.verb_class"); got != "network" {
		t.Errorf("verb_class is %v, want \"network\"", got)
	}
}

// TestH14_MCPToolsKeepTheirClass covers a tool input that happens to carry a
// "command" key without being a shell invocation.
func TestH14_MCPToolsKeepTheirClass(t *testing.T) {
	e := newEnv(t)
	p := defaultPayload()
	p.ToolName = "mcp__deploy__run"
	p.ToolInput = map[string]any{"command": "restart web"}
	e.mustHook(p.build(t))

	d := e.declarations(testSession)[0]
	if got, _ := nested(d, "shape.verb_class"); got != "mcp" {
		t.Errorf("verb_class is %v, want \"mcp\": an MCP tool's input is not a shell line", got)
	}
	if got, _ := nested(d, "shape.program"); got != nil {
		t.Errorf("program is %v, want null", got)
	}
}

// TestH14_UntokenizableCommandRecordsNullArgc is the "never render zero when we
// mean unknown" rule at its smallest. A count of 0 is a claim that the command
// had no arguments; what actually happened is that we could not tell.
func TestH14_UntokenizableCommandRecordsNullArgc(t *testing.T) {
	e := newEnv(t)
	d := e.declarationsAfter(`echo "unterminated`)[0]

	argc, present := nested(d, "shape.argc")
	if !present {
		t.Fatal("argc is missing from the record; unknown has to be recorded, not omitted")
	}
	if argc != nil {
		t.Errorf("argc is %#v, want null", argc)
	}
	// The program is still knowable even when the count is not, and dropping it
	// would discard something we actually observed.
	if got, _ := nested(d, "shape.program"); got != "echo" {
		t.Errorf("program is %v, want \"echo\"", got)
	}
}

// TestH14_DigestsDifferPerInstall stops the store being usable as a lookup
// table. Under a shared key, anyone holding a dictionary of common commands
// could read a colleague's store by matching digests.
func TestH14_DigestsDifferPerInstall(t *testing.T) {
	const command = "kubectl get secrets --namespace production"
	first := newEnv(t).declarationsAfter(command)
	second := newEnv(t).declarationsAfter(command)

	a, _ := nested(first[0], "shape.digest")
	b, _ := nested(second[0], "shape.digest")
	if a == b {
		t.Errorf("two installs produced the same digest %v for the same command; the key is not per-install", a)
	}
}

// TestH14_SameInstallDigestsAreStable is the other half: the digest is only
// useful for grouping if identical input digests identically within one store.
func TestH14_SameInstallDigestsAreStable(t *testing.T) {
	e := newEnv(t)
	decls := e.declarationsAfter("npm run build", "npm run build")

	a, _ := nested(decls[0], "shape.digest")
	b, _ := nested(decls[1], "shape.digest")
	if a != b {
		t.Errorf("the same command digested differently within one install: %v vs %v", a, b)
	}
}

// TestH14_ShellDigestCoversTheCommandAlone is what makes stability hold for the
// tool the store sees most. Claude Code attaches a free-text description to a
// Bash call, and that wording differs from call to call, so a digest over the
// whole tool_input would give `git status` a fresh value every time and group
// nothing. The fixture the stability test uses carries no description, which is
// why that test passes either way and this one is needed beside it.
func TestH14_ShellDigestCoversTheCommandAlone(t *testing.T) {
	e := newEnv(t)
	for _, in := range []map[string]any{
		{"command": "git status", "description": "Check the working tree"},
		{"command": "git status", "description": "See what changed before committing"},
		{"command": "git diff", "description": "Check the working tree"},
	} {
		p := defaultPayload()
		p.ToolInput = in
		e.mustHook(p.build(t))
	}

	decls := e.declarations(testSession)
	if len(decls) != 3 {
		t.Fatalf("got %d declarations, want 3", len(decls))
	}
	same, reworded, other := digestOf(t, decls[0]), digestOf(t, decls[1]), digestOf(t, decls[2])
	if same != reworded {
		t.Errorf("the same command digested differently under a different description: %s vs %s", same, reworded)
	}
	if same == other {
		t.Errorf("two different commands share the digest %s", same)
	}
}

// TestH14_NonShellDigestCoversTheWholeInput is the other side of that split. A
// Write call has no command line, so the whole canonical input is all there is
// to tell two of them apart.
func TestH14_NonShellDigestCoversTheWholeInput(t *testing.T) {
	e := newEnv(t)
	for _, content := range []string{"first draft", "second draft"} {
		p := defaultPayload()
		p.ToolName = "Write"
		p.ToolInput = map[string]any{"file_path": "/tmp/project/notes.md", "content": content}
		e.mustHook(p.build(t))
	}

	decls := e.declarations(testSession)
	if len(decls) != 2 {
		t.Fatalf("got %d declarations, want 2", len(decls))
	}
	if a, b := digestOf(t, decls[0]), digestOf(t, decls[1]); a == b {
		t.Errorf("two Write calls with different content share the digest %s", a)
	}
}

func digestOf(t *testing.T, r record) string {
	t.Helper()
	v, _ := nested(r, "shape.digest")
	s, ok := v.(string)
	if !ok || !hex64.MatchString(s) {
		t.Fatalf("digest is %v, want 64 hex characters", v)
	}
	return s
}

func TestH14_KeyFileIsOwnerOnly(t *testing.T) {
	e := newEnv(t)
	e.declarationsAfter("ls")
	if entry, ok := walkStore(t, e.home)["install.key"]; !ok {
		t.Fatal("install.key is missing")
	} else if entry.mode.Perm() != 0o600 {
		t.Errorf("install.key has mode %04o, want 0600", entry.mode.Perm())
	}
}
