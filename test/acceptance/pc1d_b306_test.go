package acceptance

import "testing"

// TestPC1d_AnEnabledPluginRecordsWhatTheWatchRouteRecords (B306, QA-2's
// plugin-route grade on ea6f100c): with no consent on the plugin route every
// record withheld cwd, file_path and project as no_consent, so the Stop
// counted 0 writes after an edit and printed false lines. Enabling the
// plugin is consent: after its first session start an edit's declaration
// carries cwd, project and file_path in the clear, with no withheld reason,
// as the watch route's does. Break: write the plugin's consent at scope v4,
// or not at all, and the fields read no_consent.
func TestPC1d_AnEnabledPluginRecordsWhatTheWatchRouteRecords(t *testing.T) {
	e, fx := pcEnv(t)
	e.pcStart(fx, testSession)
	e.pcCall(fx, "toolu_e", "Edit", stEdit, 0)
	var edit *record
	for _, d := range e.declarations(testSession) {
		if d.str("tool_use_id") == "toolu_e" {
			d := d
			edit = &d
		}
	}
	if edit == nil {
		t.Fatal("no declaration of the edit")
	}
	for key, want := range map[string]string{"file_path": stEdit["file_path"].(string), "cwd": "/tmp/project"} {
		if got := edit.str(key); got != want {
			t.Errorf("%s = %q, want %q (%s_withheld %v)", key, got, want, key, edit.fields[key+"_withheld"])
		}
	}
	if edit.str("project") == "" {
		t.Errorf("project is null (project_withheld %v)", edit.fields["project_withheld"])
	}
}
