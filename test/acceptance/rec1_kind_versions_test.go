package acceptance

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestREC1_KindVersions: every record a hook path writes carries the version
// KindVersion names for its kind -- declarations, executions and terminals at
// 5, coverage and gaps at 3 -- and every call record names the binary that
// wrote it -- the build-time version, not the default -- so records from
// before and after an upgrade can be told apart by more than their shape. The v5 keys are all present, null where nothing
// looked. Break: stamp a call record at 4, a coverage record at 5, or drop
// the writer.
func TestREC1_KindVersions(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.probe("start", testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))
	if res := e.pause(); res.exitCode != 0 {
		t.Fatalf("pause: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.resume(); res.exitCode != 0 {
		t.Fatalf("resume: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	e.probe("end", testSession)

	writer := strings.TrimSpace(e.run("", nil, "version").stdout)
	if writer != binaryVersion {
		t.Fatalf("rashomon version printed %q, want the build-time %q", writer, binaryVersion)
	}
	all := append(e.records(testSession), e.read(testSession, "coverage.ndjson")...)
	all = append(all, e.gaps()...)
	seen := map[string]bool{}
	for i, r := range all {
		kind, _ := r.fields["type"].(string)
		seen[kind] = true
		want, ok := store.KindVersion(kind)
		if !ok {
			t.Errorf("record %d is of kind %q, which KindVersion does not name", i, kind)
			continue
		}
		if got, _ := r.fields["schema_version"].(float64); int(got) != want {
			t.Errorf("%s record %d is at schema_version %v, want %d", kind, i, r.fields["schema_version"], want)
		}
		switch kind {
		case store.TypeDeclaration, store.TypeExecution, store.TypeTerminal:
			if got, _ := r.fields["writer"].(string); got != writer {
				t.Errorf("%s record %d: writer %q, want %q", kind, i, got, writer)
			}
		}
	}
	for _, kind := range []string{store.TypeDeclaration, store.TypeExecution, store.TypeTerminal, store.TypeCoverage, store.TypeGap} {
		if !seen[kind] {
			t.Errorf("no %s record was written", kind)
		}
	}
	d := e.declarations(testSession)[0]
	for _, k := range []string{"command", "command_withheld", "cwd", "project", "file_path", "written_paths", "target"} {
		if v, ok := d.fields[k]; !ok || v != nil {
			t.Errorf("declaration %s = %v (present %v); want present and null, since nothing looked", k, v, ok)
		}
	}
}
