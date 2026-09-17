package acceptance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// otherInstall is a second install's id. It is well formed because ownership is
// read out of the command line: an id that did not parse as one would be
// claimed by nobody and would not describe the arrangement under test.
const otherInstall = "ffffffffffffffffffffffffffffffff"

// seedOtherInstall appends a second install's entries to the settings file, as
// that install's own watch would have written them. This is H-6's arrangement
// seen from the recording side: two installs, one settings file, and -- because
// Claude Code runs every entry with one environment -- one store.
func seedOtherInstall(e *env) {
	e.t.Helper()
	otherEntry := func(sub string) map[string]any {
		m := map[string]any{"hooks": []map[string]any{{
			"type": "command", "command": attestBin + " " + sub + " --install " + otherInstall, "timeout": 5,
		}}}
		if sub == "hook" {
			m["matcher"] = "*"
		}
		return m
	}
	var doc map[string]any
	if err := json.Unmarshal(e.settingsBytes(), &doc); err != nil {
		e.t.Fatal(err)
	}
	hooks := doc["hooks"].(map[string]any)
	for event, sub := range map[string]string{"PreToolUse": "hook", "SessionStart": "probe start", "SessionEnd": "probe end"} {
		hooks[event] = append(hooks[event].([]any), otherEntry(sub))
	}
	out, _ := json.MarshalIndent(doc, "", "  ")
	e.writeSettings(string(out) + "\n")
}

// foreignHook and foreignProbe run what the other install's entries run: the
// same binary in the same environment, naming their own install.
func (e *env) foreignHook(payload string) result {
	e.t.Helper()
	return e.run(payload, nil, "hook", "--install", otherInstall)
}

func (e *env) foreignProbe(phase, sessionID string) result {
	e.t.Helper()
	return e.run(e.sessionPayload(phase, sessionID), nil, "probe", phase, "--install", otherInstall)
}

// TestH6_ForeignInstallEntryStandsDown is the one-owner concept from the
// recording side. detach leaves another install's entries alone, so they go on
// firing here; what they must not do is record, because both entries fire for
// the same tool call and the store would otherwise hold two of everything.
func TestH6_ForeignInstallEntryStandsDown(t *testing.T) {
	t.Run("both installs' entries fire and only ours records", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		seedOtherInstall(e)
		if res := e.watch(); res.exitCode != 0 {
			t.Fatalf("watch with another install present: exit %d, stderr %q", res.exitCode, res.stderr)
		}

		const n = 3
		var want []string
		for i := 0; i < n; i++ {
			p := defaultPayload()
			p.ToolUseID = fmt.Sprintf("toolu_ours_%d", i)
			want = append(want, p.ToolUseID)
			e.mustHook(p.build(t))

			p.ToolUseID = fmt.Sprintf("toolu_theirs_%d", i)
			if res := e.foreignHook(p.build(t)); res.exitCode != 0 {
				t.Fatalf("the other install's entry: exit %d, stderr %q", res.exitCode, res.stderr)
			}
		}

		decls := e.declarations(testSession)
		if len(decls) != n {
			t.Errorf("got %d declarations, want %d", len(decls), n)
		}
		if terms := e.terminals(testSession); len(terms) != n {
			t.Errorf("got %d terminal records, want %d", len(terms), n)
		}
		var got []string
		for _, d := range decls {
			got = append(got, d.str("tool_use_id"))
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("recorded ids are %v, want %v: the store holds what the other install's entry declared", got, want)
		}
	})

	t.Run("standing down leaves no run directory", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		before := walkStore(t, e.home)

		const fresh = "sess-never-seen"
		p := defaultPayload()
		p.SessionID = fresh
		p.ToolUseID = "toolu_never_seen"
		for _, res := range []result{
			e.foreignHook(p.build(t)),
			e.foreignProbe("start", fresh),
			e.foreignProbe("end", fresh),
		} {
			if res.exitCode != 0 {
				t.Fatalf("standing down: exit %d, stderr %q", res.exitCode, res.stderr)
			}
		}

		// A session this store has never seen is where any trace shows: an
		// empty run directory, a probe marker, a coverage record saying the
		// run could not measure itself. None of them is a record this
		// environment has any business holding.
		dir := filepath.Join(e.home, "runs", fresh)
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("a stand-down created the run directory %s (stat: %v)", dir, err)
		}
		after := walkStore(t, e.home)
		for rel, a := range after {
			switch b, ok := before[rel]; {
			case !ok:
				t.Errorf("a stand-down created %s", rel)
			case !bytes.Equal(a.body, b.body):
				t.Errorf("a stand-down wrote to %s", rel)
			}
		}
		for rel := range before {
			if _, ok := after[rel]; !ok {
				t.Errorf("a stand-down removed %s", rel)
			}
		}
	})

	t.Run("standing down says so on stderr and nothing on stdout", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		res := e.foreignHook(defaultPayload().build(t))

		if res.exitCode != 0 {
			t.Errorf("exit %d, want 0", res.exitCode)
		}
		if res.stdout != "" {
			t.Errorf("stdout is %q, which Claude Code parses as control output", res.stdout)
		}
		line := strings.TrimSuffix(res.stderr, "\n")
		if line == "" {
			t.Fatalf("stderr is empty; the debug log is the only place this can be said")
		}
		if strings.Contains(line, "\n") {
			t.Errorf("stderr is more than one line: %q", res.stderr)
		}
		// Fixed, not composed: hook stderr reaches Claude Code's debug log, and
		// a line quoting its own invocation writes one install's identity into
		// the other's log.
		for _, id := range []string{otherInstall, e.installID()} {
			if strings.Contains(line, id) {
				t.Errorf("the stand-down line names an install id: %q", line)
			}
		}
	})

	t.Run("watch names the other install", func(t *testing.T) {
		e := newEnv(t)
		first := e.watch()
		if first.exitCode != 0 {
			t.Fatalf("watch: exit %d, stderr %q", first.exitCode, first.stderr)
		}
		if got := strings.Count(first.stdout, "\n"); got != 1 {
			t.Errorf("watch wrote %d lines with no other install present: %q", got, first.stdout)
		}
		seedOtherInstall(e)

		res := e.watch()
		if res.exitCode != 0 {
			t.Fatalf("watch with another install present: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		named := strings.Index(res.stdout, otherInstall)
		if named < 0 {
			t.Fatalf("watch does not name the install whose entries will fire and record nothing: %q", res.stdout)
		}
		if named < strings.Index(res.stdout, "watching") {
			t.Errorf("the warning precedes the watching line: %q", res.stdout)
		}
		// Reported, not adjudicated: the install still proceeds and the other
		// install's entries are still its own.
		for _, event := range []string{"PreToolUse", "SessionStart", "SessionEnd"} {
			if got := len(ours(e.settings().Hooks[event])); got != 2 {
				t.Errorf("under %s, %d install entries, want 2 (ours and theirs)", event, got)
			}
		}
	})
}
