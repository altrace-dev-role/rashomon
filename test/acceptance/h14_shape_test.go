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

func TestH14_KeyFileIsOwnerOnly(t *testing.T) {
	e := newEnv(t)
	e.declarationsAfter("ls")
	if entry, ok := walkStore(t, e.home)["install.key"]; !ok {
		t.Fatal("install.key is missing")
	} else if entry.mode.Perm() != 0o600 {
		t.Errorf("install.key has mode %04o, want 0600", entry.mode.Perm())
	}
}
