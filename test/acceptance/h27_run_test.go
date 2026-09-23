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
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		raw      string // the file's exact bytes, when it is not JSON at all
		wantWord string
	}{
		{name: "enforcing", status: func(m map[string]any) { m["connect_mode"] = "enforce" }, wantWord: "enforce"},
		{name: "other product", status: func(m map[string]any) { m["product"] = "something" }, wantWord: "altrace"},
		{name: "process gone", status: func(m map[string]any) { m["pid"] = 0x7FFFFFF0 }, wantWord: "no longer running"},
		{name: "no status file", absent: true, wantWord: "does not appear to be running"},
		// Loopback to isLoopback, which reads only the host; refused by the
		// grammar, so the child never receives a proxy URL built from it.
		{name: "junk after the port", status: func(m map[string]any) { m["listen_addr"] = "127.0.0.1:18080;echo x" },
			wantWord: "not HOST:PORT"},
		{name: "garbage", raw: `{"product": "altrace", "connect_mode": "obs`, wantWord: "is not valid JSON"},
		{name: "wrong type elsewhere", status: func(m map[string]any) { m["health_addr"] = 18081 },
			wantWord: "is not valid JSON"},
		{name: "empty address", status: func(m map[string]any) { m["listen_addr"] = "" },
			wantWord: "names no listen address"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			if res := e.watch(); res.exitCode != 0 {
				t.Fatalf("watch: exit %d", res.exitCode)
			}
			path := filepath.Join(e.home, "absent-status.json")
			switch {
			case tc.raw != "":
				path = filepath.Join(e.home, "status.json")
				if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
					t.Fatal(err)
				}
			case !tc.absent:
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
	status := statusFile(t, e, liveStatus())

	// The CHILD records the session, which is the only arrangement that makes
	// this item mean what its name says. Recording one first and then running
	// a command that records nothing asserts that a report appears for a
	// session the command did not produce -- which is the behaviour
	// TestH27_ReportsNothingWhenTheCommandRecordedNothing now forbids.
	child := fmt.Sprintf("%s hook %s <<'EOF'\n%s\nEOF",
		shQuote(rashomonBin), strings.Join(e.installArgs(), " "), defaultPayload().build(t))
	res := e.run("", nil, "run", "--proxy-status", status, "--", "sh", "-c", child)

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

// TestH27_ReportsNothingWhenTheCommandRecordedNothing: a command that produces
// no session gets no report, even when the store holds an older one.
//
// Measured before this item existed: `rashomon run -- date` printed a complete
// report for a session recorded minutes earlier, under a heading that reads as
// though the command had just produced it -- `failed calls: 1` and all. That
// is this program's own discipline broken, a claim with no evidence behind it,
// and the message for the empty case already existed and said the right thing.
//
// Break: report the newest run without comparing it to the launch.
func TestH27_ReportsNothingWhenTheCommandRecordedNothing(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.probe("end", testSession)
	status := statusFile(t, e, liveStatus())

	// A second of separation, because the guard compares the run's
	// modification time against the launch and a same-second write is a real
	// ambiguity rather than a bug to paper over.
	time.Sleep(1100 * time.Millisecond)

	res := e.run("", nil, "run", "--proxy-status", status, "--", "sh", "-c", "echo done")

	if res.exitCode != 0 {
		t.Fatalf("run: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stderr, "no session was recorded for that command") {
		t.Errorf("a command that recorded nothing did not say so:\n%s", res.stderr)
	}
	if strings.Contains(res.stderr, "session "+testSession) {
		t.Errorf("the report named %s, a session this command did not produce:\n%s",
			testSession, res.stderr)
	}
}

// shQuote wraps a path for the POSIX shell the child command is handed to.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// TestH27_SessionTokenIsCapabilityGated is the compatibility rule, end to end.
//
// A proxy that does not understand the credential is entitled to answer 407 to
// a CONNECT carrying one, which would break every request of every session
// against any build older than the feature. So the launcher sends a tag only
// when the proxy's own status file says it accepts one, and ABSENT MEANS NO --
// the safe direction, and the one every existing status file on disk takes.
//
// This is the test that would have caught shipping it unconditionally: the
// default fixture has no such field, and it asserts the plain URL is unchanged.
func TestH27_SessionTokenIsCapabilityGated(t *testing.T) {
	run := func(t *testing.T, mutate func(map[string]any)) string {
		t.Helper()
		e := newEnv(t)
		if res := e.watch(); res.exitCode != 0 {
			t.Fatalf("watch: exit %d", res.exitCode)
		}
		fields := liveStatus()
		if mutate != nil {
			mutate(fields)
		}
		status := statusFile(t, e, fields)
		res := e.run("", nil, "run", "--proxy-status", status, "--", "sh", "-c",
			"echo HTTPS_PROXY=$HTTPS_PROXY")
		if res.exitCode != 0 {
			t.Fatalf("run: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		return res.stdout
	}

	t.Run("absent means no", func(t *testing.T) {
		out := run(t, nil)
		if strings.Contains(out, "@") || strings.Contains(out, "rashomon:") {
			t.Errorf("a proxy that never advertised the capability received a credential; "+
				"an older build may answer 407 to every CONNECT:\n%s", out)
		}
	})

	t.Run("explicit false means no", func(t *testing.T) {
		out := run(t, func(m map[string]any) { m["session_token"] = false })
		if strings.Contains(out, "@") {
			t.Errorf("session_token:false still produced a credential:\n%s", out)
		}
	})

	t.Run("true mints one", func(t *testing.T) {
		out := run(t, func(m map[string]any) { m["session_token"] = true })
		if !strings.Contains(out, "http://rashomon:rt_") {
			t.Errorf("session_token:true did not produce a tagged proxy URL:\n%s", out)
		}
		if !strings.Contains(out, "@127.0.0.1:18080") {
			t.Errorf("the tagged URL lost its host:\n%s", out)
		}
	})
}

// TestH27_ARefusedPostureReadsNoProxyStore: the automatic report reads a
// proxy store only when the posture was ACCEPTED.
//
// It used to read one either way -- the status file's causal_db, else the
// default path -- and posture.Read fills v.File from any parseable JSON before
// it decides Export. So a status file refused as enforcing, dead, routable or
// malformed still chose the database the report read as the wire.
//
// What the gate delivers, and no more: a stale, crashed, enforcing, non-
// loopback or unparseable status file cannot steer the report. It does NOT
// stop a session choosing the database. The status file is writable by the
// user the agent runs as, and a forged one naming a live pid -- pid 1 answers
// EPERM, which posture counts as alive -- an observe mode and a loopback
// address passes, causal_db and all. Refused means no store at all, from
// either source, and the proxy block is the single not-observed line.
//
// The seeded row carries a foreign tag and a host nothing declared, so if ANY
// of it is read it shows up -- as a destination, or as the join diagnostic a
// spuriously minted token used to produce. Minting is inside the accepted
// branch, so the refused path cannot reach it at all.
//
// The accepted rows are the premise, and each has to prove a READ, not merely
// a named store: the seeded host or the join line is on the page, and no
// not-observed reason is. One names the store in the status file; the other
// names none and leaves the database at the default path, which is run's
// fallback and would otherwise be the one line in this feature nothing reads.
func TestH27_ARefusedPostureReadsNoProxyStore(t *testing.T) {
	for _, tc := range []struct {
		name string
		// atDefault puts the database at ~/.altrace/observe/causal.db and names
		// none in the status file, instead of naming it there.
		atDefault bool
		accepted  bool
		// refuse makes the posture refuse; nil means an enforcing proxy.
		refuse func(map[string]any)
	}{
		{name: "refused, status names the store"},
		{name: "refused, store at the default path", atDefault: true},
		// The decoder fills every field it can before it reports the type
		// error, so this file still names causal_db -- and SessionToken true
		// -- while posture refuses it as not valid JSON.
		{name: "refused by a wrong type elsewhere, status names the store",
			refuse: func(m map[string]any) { m["health_addr"] = 18081 }},
		{name: "accepted, status names the store", accepted: true},
		{name: "accepted, store at the default path", atDefault: true, accepted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)

			home := t.TempDir()
			fields := liveStatus()
			fields["session_token"] = true // the capability says yes either way
			if !tc.accepted {
				refuse := tc.refuse
				if refuse == nil {
					refuse = func(m map[string]any) { m["connect_mode"] = "enforce" }
				}
				refuse(fields) // refused: Export will be false
			}
			db := filepath.Join(e.home, "causal.db")
			if tc.atDefault {
				dir := filepath.Join(home, ".altrace", "observe")
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				db = filepath.Join(dir, "causal.db")
				delete(fields, "causal_db")
			} else {
				fields["causal_db"] = db
			}
			seedForeignRow(t, db)
			status := statusFile(t, e, fields)

			// THE CHILD RECORDS THE SESSION. `run` reports only a session the
			// command itself produced (the since guard), so one recorded
			// beforehand renders nothing and every assertion below would be
			// unobservable.
			child := fmt.Sprintf("%s hook %s <<'EOF'\n%s\nEOF",
				shQuote(rashomonBin), strings.Join(e.installArgs(), " "), defaultPayload().build(t))
			res := e.run("", []string{"HOME=" + home},
				"run", "--proxy-status", status, "--", "sh", "-c", child)
			if res.exitCode != 0 {
				t.Fatalf("run: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			if strings.Contains(res.stderr, "no session was recorded") {
				t.Fatalf("premise: the child recorded nothing, so nothing below can be "+
					"observed:\n%s", res.stderr)
			}

			if tc.accepted {
				// A named store renders the full block whether or not it could be
				// read, so the block alone proves nothing. The seeded row does:
				// its host, or the join line its foreign tag produces, reaches the
				// page only through a read, and a store that was named but not
				// read renders "destinations: not observed (<reason>)".
				if !strings.Contains(res.stderr, "other.example") && !strings.Contains(res.stderr, "join:") {
					t.Errorf("premise broken: an accepted posture's report shows nothing of the "+
						"seeded store, so the refused rows' absences prove nothing:\n%s", res.stderr)
				}
				for _, unread := range []string{"destinations: not observed (", alphaDestinations} {
					if strings.Contains(res.stderr, unread) {
						t.Errorf("an accepted posture's report says %q, so the store was not "+
							"read:\n%s", unread, res.stderr)
					}
				}
				return
			}
			if !strings.Contains(res.stderr, alphaDestinations+"\n") {
				t.Errorf("a refused posture's report does not collapse to %q:\n%s",
					alphaDestinations, res.stderr)
			}
			for _, leaked := range []string{"other.example", "join:", "NO row carried it", "proxy on path"} {
				if strings.Contains(res.stderr, leaked) {
					t.Errorf("a refused posture's report carries %q, so it read the store an "+
						"unverified status file pointed at:\n%s", leaked, res.stderr)
				}
			}
		})
	}
}

// seedForeignRow writes a proxy store with a single row carrying a foreign tag.
func seedForeignRow(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(`CREATE TABLE causal_records (
		sequence_num INTEGER PRIMARY KEY,
		record_id TEXT NOT NULL DEFAULT '',
		request_id TEXT NOT NULL DEFAULT '',
		run_id TEXT NOT NULL DEFAULT '',
		timestamp DATETIME NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL DEFAULT '',
		target_host TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UTC().Format("2006-01-02 15:04:05.999999999 -0700 MST")
	if _, err := db.Exec(
		`INSERT INTO causal_records (sequence_num, request_id, run_id, timestamp, action, target_host)
		 VALUES (1, 'r1', 'rt_another_run', ?, 'ALLOW', 'other.example')`, stamp); err != nil {
		t.Fatal(err)
	}
}
