package acceptance

import (
	"bytes"
	"strings"
	"testing"
)

// labelSchemaGate skips the half of an item that can only be observed once a
// record carries file_label.
//
// The field is derived on every declaration today and dropped before the
// record is written, because docs/store-schema.json pins schema_version to
// [1, 2] under additionalProperties: false and schema 3 is being cut on the
// Phase B branch. This is the same shape as H-17's two halves, which skip when
// unshare or strace is not on the machine: the assertion is real, the
// precondition is not met here, and the skip says which.
//
// It is deliberately written against the RECORD rather than against a
// constant, so the day the reservation lands these halves start running with
// no edit to this file.
func labelSchemaGate(t *testing.T, e *env, sessionID string) {
	t.Helper()
	decls := e.declarations(sessionID)
	if len(decls) == 0 {
		t.Fatal("no declaration to read a schema version from")
	}
	if _, ok := decls[0].fields["file_label"]; ok {
		return
	}
	t.Skip("this build's declarations carry no file_label: schema 3 reserves it and is being cut on the Phase B branch (decision 8)")
}

// TestH44_NoPathEverReachesTheStore is H-44.
//
// The label is derived from a path, and a path is the most content-shaped
// thing any of these tools carries: it names directories the user chose and
// files they wrote. The promise is that deriving a label reads the path and
// keeps nothing of it, so the canary here is a path SEGMENT, placed in
// file_path and in notebook_path, the two fields shape.Label is allowed to
// read.
//
// Break: store the basename.
func TestH44_NoPathEverReachesTheStore(t *testing.T) {
	// Lower case on purpose, unlike H-13's. The label derivation case-folds
	// the basename, so an upper-case canary stored verbatim by a broken
	// derivation would come back folded and a Contains check would miss it --
	// the test would pass against exactly the bug it exists to catch.
	const canary = "canary-4f11c0de-path-must-not-persist"

	for _, tc := range []struct {
		name  string
		tool  string
		field string
		path  string
	}{
		{name: "file_path", tool: "Read", field: "file_path", path: "/home/u/" + canary + "/.ssh/id_rsa"},
		{name: "notebook_path", tool: "NotebookEdit", field: "notebook_path", path: "/home/u/" + canary + "/secrets.ipynb"},
		{name: "the basename itself", tool: "Write", field: "file_path", path: "/home/u/proj/" + canary + ".env"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)

			p := defaultPayload()
			p.ToolName = tc.tool
			p.ToolInput = map[string]any{tc.field: tc.path, "description": "irrelevant prose"}
			res := e.hook(p.build(t))
			if res.exitCode != 0 {
				t.Fatalf("exit code %d, want 0", res.exitCode)
			}
			e.probe("end", testSession)

			repJSON := e.run("", nil, "report", "--json", "--session", testSession)
			repText := e.run("", nil, "report", "--session", testSession)

			for rel, f := range walkStore(t, e.home) {
				if bytes.Contains(f.body, []byte(canary)) {
					t.Errorf("%s contains the canary path segment", rel)
				}
			}
			for name, s := range map[string]string{
				"hook stdout":   res.stdout,
				"hook stderr":   res.stderr,
				"report --json": repJSON.stdout,
				"report text":   repText.stdout,
			} {
				if strings.Contains(s, canary) {
					t.Errorf("%s contains the canary path segment", name)
				}
			}
		})
	}
}

// TestH44_PositiveTwin is the other half of H-44: the same call that leaks no
// path still carries the label the path earns.
//
// A no-content assertion passes trivially against a function that computes
// nothing, which is why this twin exists and why it is in the same item.
func TestH44_PositiveTwin(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	p.ToolName = "Read"
	p.ToolInput = map[string]any{"file_path": "/home/u/.ssh/id_rsa"}
	e.mustHook(p.build(t))

	labelSchemaGate(t, e, testSession)

	decls := e.declarations(testSession)
	if got := decls[0].str("file_label"); got != "ssh-key" {
		t.Errorf("file_label = %q, want %q", got, "ssh-key")
	}
}
