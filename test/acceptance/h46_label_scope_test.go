package acceptance

import (
	"encoding/json"
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
			// The unknown label is not the only trace: the call's coverage
			// record names the recovered panic, so it cannot be mistaken for
			// a path field that was simply empty.
			assertCallCoverage(t, e, testSession, "unverified", "recovered_panic")
		})
	}
}

// TestRubricERRORS8_LabelPanicIsNamedInCoverage: a panic recovered inside
// the label derivation leaves the declaration and an ok terminal, and the
// call's coverage record carries recovered_panic. Before, the label read
// unknown and the coverage record read verified, so the panic left no trace
// a reader could tell from an empty path field.
func TestRubricERRORS8_LabelPanicIsNamedInCoverage(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	p.ToolName = "Read"
	p.ToolInput = map[string]any{"file_path": "/home/u/notes.txt"}
	if res := e.hook(p.build(t), "RASHOMON_FAULT="+pointLabel+":plain_panic"); res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}
	if terms := e.terminals(testSession); len(terms) != 1 || terms[0].str("outcome") != "ok" {
		t.Errorf("terminals %v, want one with outcome ok: the call itself was captured", terms)
	}
	assertCallCoverage(t, e, testSession, "unverified", "recovered_panic")
}

// TestRubricERRORS8_HookEntryAbsentOutranksARecoveredLabelPanic: a call whose
// own hook entry is absent, and whose label derivation also panics, reads
// hook_entry_absent -- the costlier problem -- not recovered_panic. Before,
// Close named recovered_panic whenever the label panicked and the call
// itself raised no error, which is true whether or not the hook entry that
// made the call's coverage report possible at all was there, so an absent
// entry went unnamed behind the cheaper, cosmetic fault.
func TestRubricERRORS8_HookEntryAbsentOutranksARecoveredLabelPanic(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	var doc map[string]any
	if err := json.Unmarshal(e.settingsBytes(), &doc); err != nil {
		t.Fatal(err)
	}
	hooks, ok := doc["hooks"].(map[string]any)
	if !ok || hooks["PreToolUse"] == nil {
		t.Fatalf("premise broken: watch installed no PreToolUse entry:\n%s", e.settingsBytes())
	}
	delete(hooks, "PreToolUse")
	out, _ := json.MarshalIndent(doc, "", "  ")
	e.writeSettings(string(out) + "\n")

	p := defaultPayload()
	p.ToolName = "Read"
	p.ToolInput = map[string]any{"file_path": "/home/u/notes.txt"}
	if res := e.hook(p.build(t), "RASHOMON_FAULT="+pointLabel+":plain_panic"); res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}
	assertCallCoverage(t, e, testSession, "unverified", "hook_entry_absent")
}
