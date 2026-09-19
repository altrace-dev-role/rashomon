package acceptance

import (
	"bytes"
	"strings"
	"testing"
)

// TestH44_NoPathEverReachesTheStore is H-44.
//
// The label is derived from a path, and a path is the most content-shaped
// thing any of these tools carries: it names directories the user chose and
// files they wrote. The promise is that deriving a label reads the path and
// keeps nothing of it, so the canary here is a path SEGMENT, placed in
// file_path and in notebook_path, the two fields shape.Label is allowed to
// read.
//
// This half is NOT gated and always runs, because it covers every route a
// path could take into the store or the output. Under schema 2 the label
// never reaches a record, so it cannot observe the label channel itself --
// H-44's named break, "store the basename", would be swallowed by the writer
// gate. That claim is TestH44_PositiveTwin's, which is gated, and the unit
// tests in internal/shape carry it today.
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

	got := e.declarations(testSession)[0].str("file_label")
	if got != "ssh-key" {
		t.Errorf("file_label = %q, want %q", got, "ssh-key")
	}
	// The label channel is live here, which is what makes the next line an
	// assertion rather than a tautology: a derivation that returned the
	// basename would put it in this field, and the sweep above cannot see
	// that while the writer gate is closed.
	if strings.Contains(got, "id_rsa") {
		t.Errorf("file_label is %q, which carries a piece of the path", got)
	}
}
