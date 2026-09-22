package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

// H-91 -- a turn with a finding prints once, naming it.
//
// Break: render from session scope and a later clean turn repeats an
// earlier finding. This test exercises exactly that shape: turn one has a
// recorded failure the final message never mentions, and it must print,
// naming the count; turn two in the SAME session is clean, and must print
// nothing. A digest scoped to the session rather than the turn would have
// turn two repeat turn one's failure -- the digest never resets, and the
// line would fire on every turn from that point on.
func TestH91_AFindingPrintsOnceAndALaterCleanTurnStaysSilent(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	// Turn one: a declared call that fails, and a summary naming none of the
	// failure vocabulary (account.go's list).
	p1 := defaultPayload()
	p1.PromptID = "prompt-1"
	p1.ToolUseID = "toolu_1"
	e.mustHook(p1.build(t))
	e.mustPost(failurePayload(t, "toolu_1", "Exit code 1", false, 30))

	line, ok := e.recapLine(stopPayload(testSession, "Ran the command as requested.", false))
	if !ok {
		t.Fatal("turn one has a recorded failure with a silent summary; recap printed nothing")
	}
	if !strings.HasPrefix(line, "※ rashomon: ") {
		t.Errorf("line = %q, does not open with the mark", line)
	}
	if !strings.Contains(line, "1 recorded failure") {
		t.Errorf("line = %q, want it to name the failure count", line)
	}
	if !strings.Contains(line, "--session "+testSession) {
		t.Errorf("line = %q, want the report command naming this session", line)
	}

	// Turn two, same session: a second, healthy call and an honest summary.
	// Nothing about turn one should reappear.
	p2 := defaultPayload()
	p2.PromptID = "prompt-2"
	p2.ToolUseID = "toolu_2"
	e.mustHook(p2.build(t))
	post2 := defaultPost()
	post2.ToolUseID = "toolu_2"
	e.mustPost(post2.build(t))

	line2, ok2 := e.recapLine(stopPayload(testSession, "Ran the second command too, both succeeded.", false))
	if ok2 {
		t.Errorf("turn two is clean but recap printed %q -- turn one's finding leaked across "+
			"the session-scope boundary this item exists to hold", line2)
	}
}

// TestH91_TheReportPointerNamesACommandTheOriginHas pins which report command
// the line points at, which H-91's own check above cannot tell apart: both
// "rashomon report --session X" and "/rashomon:report --session X" contain
// "--session X". The two are not interchangeable. The slash command exists
// only while the plugin is loaded, and the bare CLI command is the only one a
// settings install can promise, so pointing a user at the other origin's
// command sends them to one that is not there.
//
// The middle case is the one that matters. A settings install made FROM the
// plugin's own binary -- what plugin/skills/watch tells a user to run when
// they want the settings form after disabling the plugin -- sits in a plugin
// directory, so asking only where the executable lives calls it the plugin.
// What tells the two apart is the --install marker: only a settings entry's
// command line carries it (install.Spec.Command appends it; the plugin's
// hooks.json passes plain ["recap"]), so the invocation itself says which
// entry ran it.
//
// Break: decide the origin from PluginPresent alone and the middle case
// points at /rashomon:report; hard-code either answer and one of the other
// two cases does.
func TestH91_TheReportPointerNamesACommandTheOriginHas(t *testing.T) {
	const (
		cliPointer    = "→ rashomon report --session " + testSession
		pluginPointer = "→ /rashomon:report --session " + testSession
	)

	// A turn with a recorded failure the summary never mentions: the same
	// finding H-91 prints above, so every case below has a line to read.
	turn := func(t *testing.T, e *env, bin string, marker ...string) string {
		t.Helper()
		run := func(payload string, args ...string) result {
			t.Helper()
			res := e.runBin(bin, payload, append(args, marker...)...)
			if res.exitCode != 0 {
				t.Fatalf("%v: exit %d, stderr %q", args, res.exitCode, res.stderr)
			}
			return res
		}
		run(e.sessionPayload("start", testSession), "probe", "start")
		run(defaultPayload().build(t), "hook")
		run(failurePayload(t, testToolUseID, "Exit code 1", false, 30), "post")
		res := run(stopPayload(testSession, "Ran the command as requested.", false), "recap")
		if res.stdout == "" {
			t.Fatal("a turn with a recorded failure and a silent summary printed nothing")
		}
		var out recapOutput
		if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
			t.Fatalf("recap stdout is not JSON: %v\n%s", err, res.stdout)
		}
		return out.SystemMessage
	}

	t.Run("settings install", func(t *testing.T) {
		e := newEnv(t)
		if res := e.watch(); res.exitCode != 0 {
			t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		line := turn(t, e, rashomonBin, e.installArgs()...)
		if !strings.Contains(line, cliPointer) {
			t.Errorf("line = %q, want it to point at %q, the only command a settings install has", line, cliPointer)
		}
	})

	t.Run("settings install made from the plugin's binary", func(t *testing.T) {
		e := newEnv(t)
		fx := newPluginFixture(t, "rashomon@test")
		if res := e.runBin(fx.bin, "", "watch"); res.exitCode != 0 {
			t.Fatalf("watch via the plugin binary: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		line := turn(t, e, fx.bin, e.installArgs()...)
		if !strings.Contains(line, cliPointer) {
			t.Errorf("line = %q, want it to point at %q: the settings entry ran this, "+
				"and no plugin is loaded to provide /rashomon:report", line, cliPointer)
		}
	})

	t.Run("plugin", func(t *testing.T) {
		e := newEnv(t)
		fx := newPluginFixture(t, "rashomon@test")
		fx.installed(t, e)
		fx.enable(t, e, true)
		line := turn(t, e, fx.bin) // the plugin's entries carry no marker
		if !strings.Contains(line, pluginPointer) {
			t.Errorf("line = %q, want it to point at %q, the plugin's own command", line, pluginPointer)
		}
	})
}
