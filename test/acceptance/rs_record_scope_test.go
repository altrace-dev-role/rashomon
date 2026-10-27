package acceptance

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The record-scope gate: the hook writes a v5 content field -- where the call
// ran, its project, the file an edit names, and the command text once it has
// a producer -- only when the consent on file says record_scope v5. Without
// it each is null and its *_withheld key says no_consent. The hook learns the
// scope from one small marker under the store root that every consent write
// keeps equal to consent.json's answer; no marker is v4.

// scopeCalls drives the same three hook calls on e: an Edit inside a
// repository, a Bash call that names a host, and that call's failure. It
// returns the Edit's declaration, the Bash call's and the execution.
func scopeCalls(t *testing.T, e *env) (edit, bash, exec record) {
	t.Helper()
	_, inner := gitRepo(t, resolvedTempDir(t))
	p := defaultPayload()
	p.CWD = inner
	p.ToolName = "Edit"
	p.ToolUseID = "tu-edit"
	p.ToolInput = map[string]any{"file_path": "/home/u/proj/internal/x.go", "old_string": "a", "new_string": "b"}
	e.mustHook(p.build(t))

	p = defaultPayload()
	p.CWD = inner
	p.ToolUseID = testToolUseID
	p.ToolInput = map[string]any{"command": "curl -s https://example.com/health"}
	e.mustHook(p.build(t))
	e.mustPost(failurePayload(t, testToolUseID, "Exit code 7", false, 30))

	for _, d := range e.declarations(testSession) {
		switch d.str("tool_use_id") {
		case "tu-edit":
			edit = d
		case testToolUseID:
			bash = d
		}
	}
	if edit.fields == nil || bash.fields == nil {
		t.Fatalf("premise: the two declarations were not both written: %d read", len(e.declarations(testSession)))
	}
	execs := e.executions(testSession)
	if len(execs) != 1 {
		t.Fatalf("premise: read %d executions, want 1", len(execs))
	}
	return edit, bash, execs[0]
}

// wantRecordedAnyway checks what the v4 record carries at every scope: the
// tool, the shape's digest, the cwd's keyed digest, the host the command
// named and the exit code.
func wantRecordedAnyway(t *testing.T, edit, bash, exec record) {
	t.Helper()
	for _, d := range []record{edit, bash} {
		if d.str("tool_name") == "" || d.str("cwd_digest") == "" {
			t.Errorf("declaration %s lost its v4 fields: %s", d.str("tool_use_id"), d.raw)
		}
		if sh, _ := d.fields["shape"].(map[string]any); sh == nil || sh["digest"] == "" || sh["digest"] == nil {
			t.Errorf("declaration %s has no shape digest: %s", d.str("tool_use_id"), d.raw)
		}
	}
	if !slices.Contains(stringsOf(bash.fields["hosts"]), "example.com") {
		t.Errorf("hosts = %v, want example.com recorded at every scope", bash.fields["hosts"])
	}
	if got := exec.fields["exit_code"]; got != float64(7) {
		t.Errorf("exit_code = %v, want 7 recorded at every scope", got)
	}
}

// wantNullUnstamped checks fields the hook has no producer for: null, with a
// null withheld key, at any scope, since the gate stamps only a value that
// is there.
func wantNullUnstamped(t *testing.T, r record, withheld string, keys ...string) {
	t.Helper()
	for _, k := range append(keys, withheld) {
		if v, ok := r.fields[k]; !ok || v != nil {
			t.Errorf("%s = %v (present %v), want present and null: nothing produced it", k, v, ok)
		}
	}
}

// TestRS1_NoConsentWithholdsEveryContentField: a store with no consent on
// file -- no marker -- records the v4 shape and nothing the user did not
// agree to: cwd, project and file_path null with no_consent; the command
// fields, which have no producer, null with nothing stamped.
func TestRS1_NoConsentWithholdsEveryContentField(t *testing.T) {
	e := newEnv(t)
	edit, bash, exec := scopeCalls(t, e)
	if _, err := os.Stat(filepath.Join(e.home, "record_scope")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("premise: a store nobody consented on has a record_scope marker (%v)", err)
	}
	for _, d := range []record{edit, bash} {
		wantNoPath(t, d, "cwd", "no_consent")
		wantNoPath(t, d, "project", "no_consent")
		wantNullUnstamped(t, d, "command_withheld", "command", "command_cut", "masked")
	}
	wantNoPath(t, edit, "file_path", "no_consent")
	wantNoPath(t, bash, "file_path", "")
	wantNullUnstamped(t, exec, "executed_command_withheld", "executed_command", "executed_command_cut", "executed_masked")
	wantNullUnstamped(t, exec, "head_after_withheld", "head_after")
	wantRecordedAnyway(t, edit, bash, exec)
}

// TestRS1b_NoConsentComesBeforeOversize: the gate runs before the caps, so a
// path over its cap under no consent reads no_consent, not oversize, and the
// call's coverage record does not claim record_oversize for a value the scope
// left out anyway. After watch the same call is the cap's: oversize, and
// record_oversize on the coverage record, which is also the proof this test
// can see the reason when it is there.
//
// The coverage row uses an over-cap file_path under an ordinary cwd: the
// coverage record carries the session's cwd itself, at every scope, and caps
// it on its own, so an over-cap cwd claims record_oversize whatever the
// declaration does. The cwd row checks the declaration alone.
func TestRS1b_NoConsentComesBeforeOversize(t *testing.T) {
	const maxPathBytes = 4096
	over := "/" + strings.Repeat("o", maxPathBytes)
	call := func(t *testing.T, watched bool, cwd, path string) (record, bool) {
		t.Helper()
		e := newEnv(t)
		if watched {
			e.watched(testSession)
		}
		p := defaultPayload()
		p.CWD = cwd
		p.ToolName = "Edit"
		p.ToolInput = map[string]any{"file_path": path, "old_string": "a", "new_string": "b"}
		e.mustHook(p.build(t))
		decls := e.declarations(testSession)
		if len(decls) != 1 {
			t.Fatalf("watched %v: read %d declarations, want 1", watched, len(decls))
		}
		oversize := false
		for _, c := range e.coverage(testSession, "call") {
			oversize = oversize || strings.Contains(c.raw, "record_oversize")
		}
		return decls[0], oversize
	}
	for _, watched := range []bool{false, true} {
		want := "no_consent"
		if watched {
			want = "oversize"
		}
		d, oversize := call(t, watched, defaultPayload().CWD, over)
		wantNoPath(t, d, "file_path", want)
		if oversize != watched {
			t.Errorf("watched %v: the call's coverage claims record_oversize %v, want %v", watched, oversize, watched)
		}
		d, _ = call(t, watched, over, "/home/u/proj/x.go")
		wantNoPath(t, d, "cwd", want)
		wantNoPath(t, d, "project", want)
	}
}

// TestRS2_V5RecordsThem: the same calls after watch, whose consent names
// record_scope v5, record cwd, project and file_path, and the marker says v5.
func TestRS2_V5RecordsThem(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	b, err := os.ReadFile(filepath.Join(e.home, "record_scope"))
	if err != nil || string(b) != "v5\n" {
		t.Fatalf("after watch the marker holds %q (%v), want \"v5\\n\"", b, err)
	}
	edit, bash, exec := scopeCalls(t, e)
	for _, d := range []record{edit, bash} {
		if _, ok := pathValue(t, d, "cwd"); !ok {
			t.Errorf("declaration %s: cwd is null at v5", d.str("tool_use_id"))
		}
		if _, ok := pathValue(t, d, "project"); !ok {
			t.Errorf("declaration %s: project is null at v5", d.str("tool_use_id"))
		}
		for _, k := range []string{"cwd_withheld", "project_withheld"} {
			if v := d.fields[k]; v != nil {
				t.Errorf("declaration %s: %s = %v at v5, want null", d.str("tool_use_id"), k, v)
			}
		}
	}
	wantPath(t, edit, "file_path", "/home/u/proj/internal/x.go")
	wantRecordedAnyway(t, edit, bash, exec)
}

// TestRS2b_MarkerIsReadPerCall: the scope is read on every call, not cached
// across them: removing the marker between two calls of one session
// withholds the second call's paths.
func TestRS2b_MarkerIsReadPerCall(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	e.mustHook(p.build(t))
	if err := os.Remove(filepath.Join(e.home, "record_scope")); err != nil {
		t.Fatal(err)
	}
	p.ToolUseID = "tu-second"
	e.mustHook(p.build(t))
	decls := e.declarations(testSession)
	if len(decls) != 2 {
		t.Fatalf("read %d declarations, want 2", len(decls))
	}
	if _, ok := pathValue(t, decls[0], "cwd"); !ok {
		t.Error("the call before the marker went has no cwd")
	}
	wantNoPath(t, decls[1], "cwd", "no_consent")
}

// TestRS9_StatusNamesTheScopeAndItsSource: status prints the record scope the
// hook will apply and where it came from, and names consent.json's own answer
// when the two differ, so a v4 that withholds paths is never a mystery.
func TestRS9_StatusNamesTheScopeAndItsSource(t *testing.T) {
	scopeLine := func(t *testing.T, e *env) string {
		t.Helper()
		res := e.status()
		if res.exitCode != 0 {
			t.Fatalf("status: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		for _, line := range strings.Split(res.stdout, "\n") {
			if s, ok := strings.CutPrefix(line, "  record scope: "); ok {
				return s
			}
		}
		t.Fatalf("status printed no record scope line:\n%s", res.stdout)
		return ""
	}
	marker := func(e *env) string { return filepath.Join(e.home, "record_scope") }

	t.Run("no consent", func(t *testing.T) {
		e := newEnv(t)
		if got, want := scopeLine(t, e), "v4 (no record_scope marker)"; got != want {
			t.Errorf("record scope line = %q, want %q", got, want)
		}
	})
	t.Run("watched", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		if got, want := scopeLine(t, e), "v5 (record_scope marker)"; got != want {
			t.Errorf("record scope line = %q, want %q", got, want)
		}
	})
	t.Run("marker gone, consent on file", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		if err := os.Remove(marker(e)); err != nil {
			t.Fatal(err)
		}
		if got, want := scopeLine(t, e), "v4 (no record_scope marker; consent.json says v5)"; got != want {
			t.Errorf("record scope line = %q, want %q", got, want)
		}
	})
	t.Run("unknown word", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		if err := os.WriteFile(marker(e), []byte("v6\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, want := scopeLine(t, e), "v4 (record_scope marker holds no scope word; consent.json says v5)"; got != want {
			t.Errorf("record scope line = %q, want %q", got, want)
		}
	})
}
