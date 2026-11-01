package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// PLUGIN-CONSENT1 (B287): on the plugin route steering never happened. A
// plugin user has no consent screen and no rashomon on PATH, so every note
// was held as consent_missing and 0 B reached the model. Enabling the plugin
// is that route's act of consent (D160): its own SessionStart entry records
// it, plugin_enable, at this build's scope and steer disclosure, and tells
// the person once, on the user channel only.

// pcEnv is a machine with the plugin installed and enabled and nothing from
// watch: no settings entries, no consent.json.
func pcEnv(t *testing.T) (*env, *pluginFixture) {
	t.Helper()
	e := newEnv(t)
	fx := newPluginFixture(t, "rashomon@inline")
	fx.installed(t, e)
	fx.enable(t, e, true)
	return e, fx
}

// pcRun runs the plugin's binary as Claude Code 2.1.304, attended: the
// hook commands of plugin/hooks/hooks.json carry no --install marker.
func (e *env) pcRun(fx *pluginFixture, stdin string, args ...string) result {
	e.t.Helper()
	cmd := exec.Command(fx.bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = e.environ("CI=", stNow304)
	cmd.Dir = e.cwd
	return e.wait(cmd)
}

// pcStart is the plugin's SessionStart for session, and the systemMessage it
// printed; it fails the test on any key but systemMessage, so nothing of the
// start reaches the model.
func (e *env) pcStart(fx *pluginFixture, session string) string {
	e.t.Helper()
	res := e.pcRun(fx, e.sessionPayload("start", session), "probe", "start")
	if res.exitCode != 0 {
		e.t.Fatalf("probe start via the plugin binary: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res.stdout == "" {
		return ""
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		e.t.Fatalf("probe start printed %q, not the hook's JSON: %v", res.stdout, err)
	}
	msg, _ := out["systemMessage"].(string)
	if len(out) != 1 || msg == "" {
		e.t.Fatalf("probe start printed %q; want a systemMessage alone, 0 B to the model", res.stdout)
	}
	return msg
}

// pcCall runs one tool call through the plugin's binary, hook then post (a
// PostToolUseFailure post when exit is not 0), and returns the post's result.
func (e *env) pcCall(fx *pluginFixture, id, tool string, input map[string]any, exit int) result {
	e.t.Helper()
	if res := e.pcRun(fx, stPayload(e.t, "PreToolUse", id, tool, input, nil), "hook"); res.exitCode != 0 {
		e.t.Fatalf("hook %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	event, more := "PostToolUse", map[string]any{"tool_response": map[string]any{"stdout": "", "stderr": "", "interrupted": false}}
	if exit != 0 {
		event, more = "PostToolUseFailure", map[string]any{"error": "Exit code " + strconv.Itoa(exit), "is_interrupt": false, "duration_ms": 30}
	}
	res := e.pcRun(fx, stPayload(e.t, event, id, tool, input, more), "post")
	if res.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	return res
}

// pcRegression is regression through the plugin's binary: a green go test,
// an edit, then the red, the green and the edit backdated.
func (e *env) pcRegression(fx *pluginFixture) result {
	e.t.Helper()
	e.pcCall(fx, "toolu_g", "Bash", stGoTest, 0)
	e.pcCall(fx, "toolu_e", "Edit", stEdit, 0)
	e.backdate("toolu_g", 180_000)
	e.backdate("toolu_e", 120_000)
	return e.pcCall(fx, "toolu_r", "Bash", stGoTest, 1)
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// TestPC1_EnablingThePluginIsConsentAndItsFirstSessionSteers: the first
// session of an enabled plugin writes one plugin_enable record at this
// build's scope, record scope and steer disclosure, hashing the description
// the person read in /plugin and the binary that ran; it sets both markers;
// it tells the person, user channel only, that rashomon is on, how to turn it
// off and where the record is read, naming the plugin's own binary; and a
// regression in that session reaches the model: the note is delivered and
// its text is more than 0 bytes. A second session writes and says nothing
// more. On main the record is never written and the note is a
// consent_missing flag, 0 B to the model. Break: drop the plugin's consent,
// or write it after the probe syncs the markers, and the note is held.
func TestPC1_EnablingThePluginIsConsentAndItsFirstSessionSteers(t *testing.T) {
	e, fx := pcEnv(t)
	if _, err := os.Stat(filepath.Join(e.home, "consent.json")); !os.IsNotExist(err) {
		t.Fatalf("premise: consent.json before the plugin's first session (%v)", err)
	}

	msg := e.pcStart(fx, testSession)
	lines := strings.Split(msg, "\n")
	if lines[0] != disclose.PluginOnLine {
		t.Errorf("the first session's message opens %q, want %q", lines[0], disclose.PluginOnLine)
	}
	if want := filepath.Join(filepath.Dir(fx.bin), "rashomon") + " report"; !strings.Contains(msg, "/rashomon:report last") || !strings.Contains(msg, want) {
		t.Errorf("the first session's message does not name /rashomon:report last and %q:\n%s", want, msg)
	}
	for i, l := range lines[:min(2, len(lines))] {
		if n := utf8.RuneCountInString(l); n > 80 {
			t.Errorf("message line %d is %d columns: %q", i+1, n, l)
		}
	}

	recs := e.consentRecords()
	if len(recs) != 1 {
		t.Fatalf("consent.json after the first session holds %d records, want 1: %v", len(recs), recs)
	}
	r := recs[0]
	bin, err := os.ReadFile(fx.bin)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]any{
		"via": "plugin_enable", "scope_version": float64(disclose.ScopeVersion), "record_scope": "v5",
		"steer_disclosure": float64(steer.DisclosureVersion), "steer": "on", "tty": false,
		"text_sha256": sha256Hex([]byte(disclose.PluginDescription)), "binary_sha256": sha256Hex(bin),
	} {
		if r[k] != want {
			t.Errorf("the plugin_enable record's %s is %v, want %v", k, r[k], want)
		}
	}
	for marker, want := range map[string]string{"steer_disclosure": strconv.Itoa(steer.DisclosureVersion), "record_scope": "v5"} {
		if b, err := os.ReadFile(filepath.Join(e.home, marker)); err != nil || strings.TrimSpace(string(b)) != want {
			t.Errorf("the %s marker is %q (%v), want %s", marker, b, err, want)
		}
	}

	res := e.pcRegression(fx)
	var out struct {
		Hook struct {
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil || len(out.Hook.Context) == 0 {
		t.Errorf("the red's post printed %q (%v); want a note to the model, more than 0 bytes", res.stdout, err)
	}
	steerLines := e.steerLines()
	if len(steerLines) != 1 {
		t.Fatalf("%d steer lines, want 1", len(steerLines))
	}
	l := steerLines[0]
	if l.str("state") != "delivered" || l.str("suppressed_by") != "" || l.str("level") == "flag" || len(l.str("model_text")) == 0 {
		t.Errorf("steer line %s; want a delivered note with model text, not a consent_missing flag", l.raw)
	}

	if again := e.pcStart(fx, "sess-2"); strings.Contains(again, disclose.PluginOnLine) {
		t.Errorf("a second session told the person again: %q", again)
	}
	if n := len(e.consentRecords()); n != 1 {
		t.Errorf("a second session left %d records, want 1", n)
	}
}

// TestPC1_OnlyAPluginEnableIsConsent: the controls. A binary that is not
// the plugin's own (Claude Code runs a plugin's entries only while it is
// enabled, so running as the plugin is the enable's evidence) and a settings
// install write no plugin consent; a consent on file that covers this build is not written again; an
// earlier plugin enable at an older scope is an update, upgrade_notice, and
// says so; alpha.1's watch consent, under a plugin enabled since, is the
// enable's own act, plugin_enable. Every record written reads under alpha.1:
// its Vias hold both values. Break: write on every SessionStart, or from a
// settings entry, and a row here fails.
func TestPC1_OnlyAPluginEnableIsConsent(t *testing.T) {
	t.Run("not the plugin's binary", func(t *testing.T) {
		e, _ := pcEnv(t)
		if res := e.run(e.sessionPayload("start", testSession), nil, "probe", "start"); res.exitCode != 0 || strings.Contains(res.stdout, "rashomon is on") {
			t.Errorf("probe start from a binary outside the plugin: exit %d, stdout %q; want no plugin line", res.exitCode, res.stdout)
		}
		if _, err := os.Stat(filepath.Join(e.home, "consent.json")); !os.IsNotExist(err) {
			t.Errorf("a binary outside the plugin wrote consent.json (%v)", err)
		}
	})

	t.Run("a settings install", func(t *testing.T) {
		e := newEnv(t)
		if res := e.watch(); res.exitCode != 0 {
			t.Fatalf("premise: watch --yes: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		older := strings.Replace(string(e.consentBytes()), `"scope_version":5`, `"scope_version":4`, 1)
		if err := os.WriteFile(filepath.Join(e.home, "consent.json"), []byte(older), 0o600); err != nil {
			t.Fatal(err)
		}
		if res := e.probe("start", testSession); res.exitCode != 0 || strings.Contains(res.stdout, "rashomon is on") {
			t.Errorf("probe start from settings: exit %d, stdout %q; want no plugin line", res.exitCode, res.stdout)
		}
		if string(e.consentBytes()) != older {
			t.Error("a settings entry's SessionStart wrote a consent record: watch's terminal consent is that route's act")
		}
	})

	alpha1 := func(via string) string {
		return `{"v":1,"scope_version":4,"record_scope":"v4","steer_disclosure":2,"steer":"on","text_sha256":"` + strings.Repeat("a", 64) +
			`","binary_version":"1.2.0-alpha.1","binary_sha256":"` + strings.Repeat("b", 64) + `","via":"` + via +
			`","tty":true,"plugin_option_steering":null,"invocation_id":"` + strings.Repeat("c", 32) + `","at":1761900000000}` + "\n"
	}
	for _, tc := range []struct{ name, prior, via string }{
		{"an earlier plugin enable at an older scope", alpha1("plugin_enable"), "upgrade_notice"},
		{"alpha.1's watch consent", alpha1("watch_tty"), "plugin_enable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, fx := pcEnv(t)
			if err := os.MkdirAll(e.home, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(e.home, "consent.json"), []byte(tc.prior), 0o600); err != nil {
				t.Fatal(err)
			}
			if msg := e.pcStart(fx, testSession); !strings.HasPrefix(msg, disclose.PluginOnLine+"\n") {
				t.Errorf("the first session after it did not tell the person: %q", msg)
			}
			recs := e.consentRecords()
			if len(recs) != 2 || recs[1]["via"] != tc.via || recs[1]["steer_disclosure"] != float64(steer.DisclosureVersion) {
				t.Fatalf("consent.json = %v; want the prior record, then %s at steer disclosure %d", recs, tc.via, steer.DisclosureVersion)
			}
			if !strings.HasPrefix(string(e.consentBytes()), tc.prior) {
				t.Error("the plugin's consent rewrote the record before its own")
			}
		})
	}

	t.Run("a consent that covers this build", func(t *testing.T) {
		e, fx := pcEnv(t)
		e.pcStart(fx, testSession)
		covered := e.consentBytes()
		if msg := e.pcStart(fx, "sess-2"); strings.Contains(msg, disclose.PluginOnLine) {
			t.Errorf("a covered machine's session told the person again: %q", msg)
		}
		if string(e.consentBytes()) != string(covered) {
			t.Error("a covered machine's session wrote a consent record")
		}
	})
}

// TestPC1c_TheManifestsCarryTheDisclosureByteForByte: what a person reads in
// /plugin before enabling, plugin.json's description and the marketplace's,
// top-level and the plugin's entry, is disclose.PluginDescription, the text
// a plugin_enable record hashes, never a longer or a different one (the DE's
// condition 3). It names what is kept in the clear at v5 that no plugin user
// saw before (the project's root, the transcript path), and that an Agent SDK
// program loading the plugin is recorded too: the plugin route asks no SDK
// question (the DE's #228 verdict). Break: edit one manifest's description
// alone, or drop either clause, and this fails.
func TestPC1c_TheManifestsCarryTheDisclosureByteForByte(t *testing.T) {
	read := func(rel string) map[string]any {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(moduleRoot, rel))
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		return doc
	}
	plugin := read("plugin/.claude-plugin/plugin.json")
	market := read(".claude-plugin/marketplace.json")
	got := map[string]any{"plugin.json": plugin["description"], "marketplace.json": market["description"]}
	entries, _ := market["plugins"].([]any)
	for _, p := range entries {
		if m, _ := p.(map[string]any); m["name"] == "rashomon" {
			got["marketplace.json's rashomon entry"] = m["description"]
		}
	}
	if len(got) != 3 {
		t.Fatalf("premise: marketplace.json has no rashomon entry: %v", entries)
	}
	for where, d := range got {
		if d != disclose.PluginDescription {
			t.Errorf("%s's description is %q, want disclose.PluginDescription", where, d)
		}
	}
	if n := utf8.RuneCountInString(disclose.PluginOnLine); n > 80 {
		t.Errorf("the plugin's first-session line is %d columns", n)
	}
	for _, w := range []string{"in every project", "never its text", "the project's root and the transcript path",
		"Agent SDK programs that load it are recorded too", "model provider", "Never stores", "/plugin"} {
		if !strings.Contains(disclose.PluginDescription, w) {
			t.Errorf("the plugin description does not say %q", w)
		}
	}
}
