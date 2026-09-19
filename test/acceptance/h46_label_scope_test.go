package acceptance

import (
	"testing"
)

// pointLabel is the injection point inside the label derivation. Spelled here
// the way every other acceptance file spells its points, so a rename in the
// fault package fails the build rather than silently injecting nothing.
const pointLabel = "hook.label"

// TestH46_OnlyWhereAPathIsRead is H-46.
//
// Version 1 labels the four tools whose input names a file and nothing else.
// Bash is the case that matters: extracting a file target from a command line
// is the recognised-file-command problem, and a label from a guess would be a
// guess wearing a closed vocabulary's clothes.
//
// Break: label Bash from its first path-like token.
func TestH46_OnlyWhereAPathIsRead(t *testing.T) {
	// Gated at the parent, not per subtest. Under schema 2 the field is
	// absent for EVERY tool, so "Bash carries no label" is true of a build
	// that labels Bash enthusiastically and of one that does not: every case
	// here would pass against the break this item is named after. Gating each
	// subtest instead would leave this parent reporting PASS over nothing but
	// skips, which reads as coverage.
	if !labelSchemaLive(t) {
		t.Skip(labelSchemaSkip)
	}
	for _, tc := range []struct {
		name      string
		tool      string
		input     map[string]any
		wantLabel string // empty means the field must be absent or null
	}{
		{name: "Bash", tool: "Bash", input: map[string]any{"command": "cat ~/.ssh/id_rsa"}},
		{name: "Agent", tool: "Agent", input: map[string]any{"prompt": "read ~/.ssh/id_rsa"}},
		{name: "WebFetch", tool: "WebFetch", input: map[string]any{"url": "https://example.com/id_rsa"}},

		{name: "Read", tool: "Read", input: map[string]any{"file_path": "/home/u/.ssh/id_rsa"}, wantLabel: "ssh-key"},
		{name: "Edit", tool: "Edit", input: map[string]any{"file_path": "/srv/.env"}, wantLabel: "env-file"},
		{name: "Write", tool: "Write", input: map[string]any{"file_path": "/etc/ssl/server.pem"}, wantLabel: "certificate"},
		{name: "NotebookEdit", tool: "NotebookEdit", input: map[string]any{"notebook_path": "/k8s/secrets.ipynb"}, wantLabel: "credential-shaped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)

			p := defaultPayload()
			p.ToolName = tc.tool
			p.ToolInput = tc.input
			e.mustHook(p.build(t))

			decls := e.declarations(testSession)
			if len(decls) != 1 {
				t.Fatalf("got %d declarations, want 1", len(decls))
			}
			v, present := decls[0].fields["file_label"]
			if tc.wantLabel == "" {
				if present && v != nil {
					t.Errorf("%s carries file_label %v, want null: this tool names no file in version 1", tc.tool, v)
				}
				return
			}
			if got, _ := v.(string); got != tc.wantLabel {
				t.Errorf("%s carries file_label %q, want %q", tc.tool, got, tc.wantLabel)
			}
		})
	}
}

// TestH46_NoFaultInLabellingCostsTheDeclaration is the fault half of H-46.
//
// The five panicking faults of H-1, injected inside the label derivation. The
// contract is narrower than H-1's and stricter: H-1 says a fault must not
// block the tool call, and at hook.start it is allowed to cost the record.
// Here the record must still land, because labelling happens after the payload
// is understood and a panic there is a failure of an ornament, not of the
// instrument. The recover therefore sits inside the derivation and before the
// append.
//
// Break: recover after the append.
func TestH46_NoFaultInLabellingCostsTheDeclaration(t *testing.T) {
	for _, fault := range panickingFaults {
		t.Run(fault, func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)

			p := defaultPayload()
			p.ToolName = "Read"
			p.ToolInput = map[string]any{"file_path": "/home/u/.ssh/id_rsa"}
			res := e.hook(p.build(t), "RASHOMON_FAULT="+pointLabel+":"+fault)

			if res.exitCode != 0 {
				t.Fatalf("exit code %d, want 0 (2 would block the tool call)", res.exitCode)
			}
			assertNoTraceback(t, res)

			decls := e.declarations(testSession)
			if len(decls) != 1 {
				t.Fatalf("a fault inside labelling produced %d declarations, want 1: the record must outlive the label", len(decls))
			}

			// The assertions above are the item's substance and they run
			// today: the fault fired inside the derivation, the process exited
			// 0, and the declaration still landed. Only the VALUE the field
			// settles on needs schema 3 to be observable, so it is checked
			// when the field is there and not skipped over when it is not --
			// a skip here would hide the three assertions that just passed.
			if v, present := decls[0].fields["file_label"]; present {
				if got, _ := v.(string); got != "unknown" {
					t.Errorf("file_label = %q after a recovered fault, want %q", got, "unknown")
				}
			}
		})
	}
}
