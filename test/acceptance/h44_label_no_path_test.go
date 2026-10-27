package acceptance

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
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
// One channel is sanctioned beside it: a declaration carries the path a
// file-changing tool names, in its own file_path (shape.DeclaredPath), so that
// a reader can tell which file a call touched. A path is less than content, the
// store is local and nothing in this program sends it anywhere. For those tools
// the record's file_path is checked against the path as written and then
// blanked, by parsing each line and writing it again, before the sweep;
// anything else that carries the canary -- the label, a second copy, any other
// file -- still fails. A string replacement of the canary would erase a second
// copy too and pass blind, which is why the blank is structural. Read is not
// among those tools, so its row sweeps the store whole.
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
			// cwd and project are sanctioned channels too, and this test
			// does not blank them: the canary must never be moved into the
			// directory, where it would reach the store by a route this
			// test cannot tell from a leak.
			if strings.Contains(p.CWD, canary) {
				t.Fatalf("premise: the payload's cwd %q carries the canary", p.CWD)
			}
			p.ToolName = tc.tool
			p.ToolInput = map[string]any{tc.field: tc.path, "description": "irrelevant prose"}
			res := e.hook(p.build(t))
			if res.exitCode != 0 {
				t.Fatalf("exit code %d, want 0", res.exitCode)
			}
			e.probe("end", testSession)

			sanctioned := declaresPath(tc.tool)
			if sanctioned {
				decls := e.declarations(testSession)
				if len(decls) != 1 {
					t.Fatalf("read %d declarations, want 1", len(decls))
				}
				if got := decls[0].str("file_path"); got != tc.path {
					t.Errorf("file_path = %q, want the path as written, %q", got, tc.path)
				}
				for _, key := range []string{"file_label", "cwd", "project"} {
					if strings.Contains(decls[0].str(key), canary) {
						t.Errorf("%s = %q carries the canary path segment", key, decls[0].str(key))
					}
				}
			}

			repJSON := e.run("", nil, "report", "--json", "--session", testSession)
			repText := e.run("", nil, "report", "--session", testSession)

			for rel, f := range walkStore(t, e.home) {
				body := f.body
				if sanctioned {
					body = blankDeclaredPaths(t, body)
				}
				if bytes.Contains(body, []byte(canary)) {
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

// declaresPath reports whether a tool's declaration carries the path its
// input names, asked of shape.DeclaredPath itself with an input that names a
// path in both of its table's fields.
func declaresPath(tool string) bool {
	return shape.DeclaredPath(tool, json.RawMessage(`{"file_path":"/x","notebook_path":"/x"}`)) != nil
}

// blankDeclaredPaths is body with the top-level file_path of every
// declaration whose tool declares a path set to null, each such line parsed
// and written again; every other line is kept byte for byte.
func blankDeclaredPaths(t *testing.T, body []byte) []byte {
	t.Helper()
	lines := bytes.Split(body, []byte("\n"))
	for i, ln := range lines {
		var rec map[string]any
		if json.Unmarshal(ln, &rec) != nil || rec["type"] != "declaration" {
			continue
		}
		if tool, _ := rec["tool_name"].(string); !declaresPath(tool) {
			continue
		}
		rec["file_path"] = nil
		out, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		lines[i] = out
	}
	return bytes.Join(lines, []byte("\n"))
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
