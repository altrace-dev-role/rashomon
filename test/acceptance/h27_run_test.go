package acceptance

// H-27 — `rashomon run` makes the safe thing the easy thing.
//
// Exporting the proxy variables by hand works, and gets you a broken session
// the day the proxy is enforcing or has crashed: an enforcing proxy refuses the
// client's own API tunnels, so the failure is not "no destinations recorded"
// but "the agent cannot reach the API at all". So run checks first.
//
// When the check fails it launches ANYWAY, without the variables, and says why
// in one line. The user asked to run their command; a wrapper that declined
// because a status file was missing would be worse than one that runs without
// recording. The single exception is that nothing is recording at all, where
// the command would finish and produce nothing and the user would reasonably
// conclude the tool is broken.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// statusFile writes a proxy status document and returns its path.
func statusFile(t *testing.T, e *env, fields map[string]any) string {
	t.Helper()
	path := filepath.Join(e.home, "status.json")
	body, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func liveStatus() map[string]any {
	return map[string]any{
		"product":      "altrace",
		"connect_mode": "observe",
		"listen_addr":  "127.0.0.1:18080",
		"health_addr":  "127.0.0.1:18081",
		"pid":          os.Getpid(),
		"started_at":   "2026-09-18T20:30:00Z",
		"causal_db":    "/tmp/observe/causal.db",
	}
}

// TestH27_ExportsWhenTheProxyIsObservingAndAlive is the only path that exports.
// The child prints its own environment so the test reads what it actually got,
// rather than trusting what the parent believes it set.
func TestH27_ExportsWhenTheProxyIsObservingAndAlive(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d", res.exitCode)
	}
	status := statusFile(t, e, liveStatus())

	res := e.run("", nil, "run", "--proxy-status", status, "--", "sh", "-c",
		"echo HTTPS_PROXY=$HTTPS_PROXY; echo https_proxy=$https_proxy; echo NO_PROXY=$NO_PROXY")

	if res.exitCode != 0 {
		t.Fatalf("run: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	for _, want := range []string{
		"HTTPS_PROXY=http://127.0.0.1:18080",
		"https_proxy=http://127.0.0.1:18080",
		"NO_PROXY=localhost,127.0.0.1,::1,0.0.0.0,*.local",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("the child did not receive %q:\n%s", want, res.stdout)
		}
	}
	if strings.Contains(res.stdout, "HTTP_PROXY=http") {
		t.Errorf("the child received HTTP_PROXY; plain HTTP is not observed:\n%s", res.stdout)
	}
}

// TestH27_LaunchesWithoutVariablesAndSaysWhy covers every refusal. The command
// still runs: that is the whole point.
func TestH27_LaunchesWithoutVariablesAndSaysWhy(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   func(map[string]any)
		absent   bool
		wantWord string
	}{
		{name: "enforcing", status: func(m map[string]any) { m["connect_mode"] = "enforce" }, wantWord: "enforce"},
		{name: "other product", status: func(m map[string]any) { m["product"] = "something" }, wantWord: "altrace"},
		{name: "process gone", status: func(m map[string]any) { m["pid"] = 0x7FFFFFF0 }, wantWord: "no longer running"},
		{name: "no status file", absent: true, wantWord: "does not appear to be running"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			if res := e.watch(); res.exitCode != 0 {
				t.Fatalf("watch: exit %d", res.exitCode)
			}
			path := filepath.Join(e.home, "absent-status.json")
			if !tc.absent {
				m := liveStatus()
				tc.status(m)
				path = statusFile(t, e, m)
			}

			res := e.run("", nil, "run", "--proxy-status", path, "--", "sh", "-c",
				"echo ran; echo proxy=[$HTTPS_PROXY]")

			if res.exitCode != 0 {
				t.Fatalf("run: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			if !strings.Contains(res.stdout, "ran") {
				t.Errorf("the command did not run; a refusal must not stop the user's "+
					"command:\n%s", res.stdout)
			}
			if !strings.Contains(res.stdout, "proxy=[]") {
				t.Errorf("the child received proxy variables anyway:\n%s", res.stdout)
			}
			if !strings.Contains(res.stderr, tc.wantWord) {
				t.Errorf("stderr does not say why (want %q):\n%s", tc.wantWord, res.stderr)
			}
			if !strings.Contains(res.stderr, "WITHOUT proxy variables") {
				t.Errorf("stderr does not state that it ran without recording:\n%s", res.stderr)
			}
		})
	}
}

// TestH27_RefusesWhenNothingIsRecording is the one case worth stopping for:
// without the hooks installed the command would run, finish, and produce
// nothing at all.
func TestH27_RefusesWhenNothingIsRecording(t *testing.T) {
	e := newEnv(t)
	status := statusFile(t, e, liveStatus())

	res := e.run("", nil, "run", "--proxy-status", status, "--", "sh", "-c", "echo should-not-run")

	if res.exitCode == 0 {
		t.Error("run succeeded with nothing installed")
	}
	if strings.Contains(res.stdout, "should-not-run") {
		t.Errorf("the command ran although nothing was recording:\n%s", res.stdout)
	}
	if !strings.Contains(res.stderr, "rashomon watch") {
		t.Errorf("stderr does not tell the user what to do:\n%s", res.stderr)
	}
}

// TestH27_ReturnsTheChildsExitCode. A wrapper that returned its own status
// would break every script checking the exit code of the command it thought it
// was running.
func TestH27_ReturnsTheChildsExitCode(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d", res.exitCode)
	}
	status := statusFile(t, e, liveStatus())

	res := e.run("", nil, "run", "--proxy-status", status, "--", "sh", "-c", "exit 7")
	if res.exitCode != 7 {
		t.Errorf("exit code = %d, want the child's 7", res.exitCode)
	}
}

// TestH27_ReportsAfterTheCommand is why the launcher spawns and waits instead
// of replacing this process: there has to be something left to render the
// report.
func TestH27_ReportsAfterTheCommand(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.probe("end", testSession)
	status := statusFile(t, e, liveStatus())

	res := e.run("", nil, "run", "--proxy-status", status, "--", "sh", "-c", "echo done")

	if res.exitCode != 0 {
		t.Fatalf("run: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	// The report goes to stderr, so the child's stdout stays pipeable.
	if !strings.Contains(res.stderr, "rashomon report") {
		t.Errorf("no report was rendered after the command:\n%s", res.stderr)
	}
	if strings.Contains(res.stdout, "rashomon report") {
		t.Errorf("the report was written to the child's stdout, which would corrupt any "+
			"pipe the command was part of:\n%s", res.stdout)
	}
}

// TestH27_NeedsACommandAfterTheSeparator: an empty argv is a usage error, not
// a silent success.
func TestH27_NeedsACommandAfterTheSeparator(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d", res.exitCode)
	}
	for _, args := range [][]string{{"run"}, {"run", "--"}} {
		res := e.run("", nil, args...)
		if res.exitCode == 0 {
			t.Errorf("%v exited 0 with no command to run", args)
		}
	}
}
