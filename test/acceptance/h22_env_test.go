package acceptance

// H-22 — `rashomon env` prints the variables that put the proxy on the path,
// and only when a verified observe-mode proxy is there to receive them.
//
// It prints and does not export, because a command that modified the caller's
// environment would have to be a shell function rather than a binary. The
// operator runs `eval $(rashomon env)`, which is why every line has to be a
// valid shell assignment and nothing else may go to stdout.
//
// The guard is the half this item was rewritten for. env used to print its
// constants unconditionally, so on a machine with no proxy `eval $(rashomon
// env)` pointed the shell's HTTPS at a port where nothing listened, and every
// HTTPS request from that shell failed until someone unset the variables by
// hand. It now asks the question `run` asks -- posture.Read(...).Export, the
// same function, never a weaker check -- and when the answer is no it prints
// NOTHING to stdout, says why on stderr and exits 1.
//
// What it exports is what `run` exports: the address the verified status file
// names. It used to export 127.0.0.1:18080 whatever the proxy wrote, so a
// proxy verified at another address had the shell pointed at a port nothing
// verified was listening on -- the same broken shell, reached past the guard.
//
// And that address is PARSED WHOLE before it is exported, because exporting it
// is printing it into a line that eval runs. The status file is writable by
// the user the agent runs as, and posture once read only the host half, so
// `127.0.0.1:1;cmd` passed and env printed the command after the semicolon
// straight into the operator's shell (CWE-78). posture now accepts HOST:PORT
// and nothing else -- HOST one of 127.0.0.1, localhost, [::1]; PORT 1-65535 in
// plain digits -- and env prints the address rebuilt from those two parts,
// never the file's own string.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	observePort   = "18080"
	wantNoProxy   = "localhost,127.0.0.1,::1,0.0.0.0,*.local"
	wantHTTPSUp   = "export HTTPS_PROXY=http://127.0.0.1:" + observePort
	wantHTTPSDown = "export https_proxy=http://127.0.0.1:" + observePort

	// wantExport is the whole of stdout under a verified proxy at the stock
	// address, byte for byte. The guard is allowed to add a refusal and
	// nothing else: what a verified proxy gets is exactly what every proxy got
	// before the guard existed.
	wantExport = wantHTTPSUp + "\n" + wantHTTPSDown + "\nexport NO_PROXY=" + wantNoProxy + "\n"

	// noStatusFile is the phrase posture uses for a missing status file. The
	// tests below that expect some OTHER refusal assert its absence, so a
	// fixture that failed to place the file cannot pass as the refusal under
	// test.
	noStatusFile = "does not appear to be running"
)

// exportFor is the whole of stdout env must print for a verified listener.
func exportFor(addr string) string {
	return "export HTTPS_PROXY=http://" + addr + "\n" +
		"export https_proxy=http://" + addr + "\n" +
		"export NO_PROXY=" + wantNoProxy + "\n"
}

// observeHome is a HOME whose ~/.altrace/observe/status.json holds fields, or
// holds no status file at all when fields is nil. It returns the environment
// entry that selects it.
//
// HOME is the only seam. env has no --proxy-status flag -- it reads
// posture.DefaultPath() -- and the harness passes the runner's own HOME
// through. Leaving it alone would make this item depend on whether the person
// running the suite has the observe proxy up, and the refusal tests would go
// red on exactly the machine of the developer who has one.
func observeHome(t *testing.T, e *env, fields map[string]any) []string {
	t.Helper()
	if fields == nil {
		return []string{"HOME=" + t.TempDir()}
	}
	body, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return observeHomeRaw(t, body)
}

// observeHomeRaw is observeHome with the status file's exact bytes, for the
// documents json.Marshal cannot produce: ones that are not JSON at all.
func observeHomeRaw(t *testing.T, body []byte) []string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".altrace", "observe")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "status.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	return []string{"HOME=" + home}
}

// forgedLine is a line a status file might try to print under this program's
// name, through a newline in a field a refusal repeats.
const forgedLine = "\nrashomon: observe-mode proxy at 127.0.0.1:18080 is running"

// liveStatusAt is liveStatus with the listener at addr.
func liveStatusAt(addr string) map[string]any {
	m := liveStatus()
	m["listen_addr"] = addr
	return m
}

// TestH22_EnvRefusesWhenNoProxyIsRunning is the live hazard: no status file,
// which is every machine that never installed the proxy.
func TestH22_EnvRefusesWhenNoProxyIsRunning(t *testing.T) {
	e := newEnv(t)
	res := e.run("", observeHome(t, e, nil), "env")

	if res.exitCode != 1 {
		t.Errorf("env with no proxy: exit %d, want 1. A caller who runs env directly, "+
			"or captures it with vars=$(rashomon env) && eval \"$vars\", is told by the "+
			"exit status as well as stderr.", res.exitCode)
	}
	if res.stdout != "" {
		t.Errorf("env with no proxy wrote to stdout, so `eval $(rashomon env)` would "+
			"point this shell's HTTPS at a port where nothing listens:\n%s", res.stdout)
	}
	if !strings.Contains(res.stderr, noStatusFile) {
		t.Errorf("env refused without saying why; stderr %q", res.stderr)
	}
}

// TestH22_EnvRefusesEveryPostureRunRefuses holds the "never a weaker check"
// half. A guard that only asked whether a status file exists would pass the
// test above and export at every row here -- including an enforcing proxy,
// which refuses the client's own API tunnels and breaks the session outright,
// and a routable address, which sends the shell's traffic off the machine.
//
// Each row matches the posture package's FULL reason phrase, never a single
// word. The HOME path carries ".altrace" and the subtest's own name, and the
// path-bearing reasons print that path, so a one-word match such as "altrace"
// or "enforce" passes on whatever refusal happened -- including the missing-
// file one this test exists to rule out.
//
// The subtest names avoid the reason words for the same reason: a subtest's
// name is in its temp path. And every refusal is ONE line: two rows put a
// newline and a forged "observe-mode proxy ... is running" line into a field
// the reason repeats, which must come back quoted, never as a line of its own.
func TestH22_EnvRefusesEveryPostureRunRefuses(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     func(map[string]any)
		raw        string // the file's exact bytes, when it is not JSON at all
		wantReason string
	}{
		{name: "enforcing", status: func(m map[string]any) { m["connect_mode"] = "enforce" },
			wantReason: "mode, not observe"},
		{name: "other product", status: func(m map[string]any) { m["product"] = "something" },
			wantReason: "does not identify an altrace proxy"},
		{name: "process gone", status: func(m map[string]any) { m["pid"] = 0x7FFFFFF0 },
			wantReason: "is no longer running"},
		{name: "routable address", status: func(m map[string]any) { m["listen_addr"] = "203.0.113.7:18080" },
			wantReason: "names a non-loopback listen address"},
		{name: "mode carrying a second line",
			status:     func(m map[string]any) { m["connect_mode"] = "enforce" + forgedLine },
			wantReason: "the proxy is in " + strconv.Quote("enforce"+forgedLine) + " mode, not observe"},
		{name: "routable address carrying a second line",
			status: func(m map[string]any) { m["listen_addr"] = "203.0.113.7:18080" + forgedLine },
			wantReason: "names a non-loopback listen address (" +
				strconv.Quote("203.0.113.7:18080"+forgedLine) + ")"},
		{name: "garbage", raw: `{"product": "altrace", "connect_mode": "obs`,
			wantReason: "is not valid JSON"},
		// A type error in a field env never reads: the decoder still fills
		// every other field, listen_addr included, and the file is refused.
		{name: "wrong type elsewhere", status: func(m map[string]any) { m["health_addr"] = 18081 },
			wantReason: "is not valid JSON"},
		{name: "empty address", status: func(m map[string]any) { m["listen_addr"] = "" },
			wantReason: "names no listen address"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			var home []string
			if tc.raw != "" {
				home = observeHomeRaw(t, []byte(tc.raw))
			} else {
				m := liveStatus()
				tc.status(m)
				home = observeHome(t, e, m)
			}
			res := e.run("", home, "env")

			if res.exitCode != 1 {
				t.Errorf("env against a refused posture: exit %d, want 1", res.exitCode)
			}
			if res.stdout != "" {
				t.Errorf("env exported against a posture run refuses:\n%s", res.stdout)
			}
			if !strings.Contains(res.stderr, tc.wantReason) {
				t.Errorf("env's refusal does not carry the posture's reason (want %q); "+
					"stderr %q", tc.wantReason, res.stderr)
			}
			if strings.Contains(res.stderr, noStatusFile) {
				t.Errorf("premise broken: env found no status file, so this row tested "+
					"the missing-file refusal and not its own; stderr %q", res.stderr)
			}
			if n := strings.Count(res.stderr, "\n"); n != 1 || strings.Contains(res.stderr, "\r") {
				t.Errorf("env's refusal is %d lines, want exactly one: a raw line break from "+
					"the file would print a line of the file's choosing; stderr %q", n, res.stderr)
			}
		})
	}
}

// TestH22_EnvExportsUnderAVerifiedProxy is the positive twin, and it compares
// all of stdout rather than looking for substrings: a verified proxy at the
// stock address must get exactly the bytes it got before the guard existed.
func TestH22_EnvExportsUnderAVerifiedProxy(t *testing.T) {
	e := newEnv(t)
	res := e.run("", observeHome(t, e, liveStatus()), "env")

	if res.exitCode != 0 {
		t.Fatalf("env under a verified proxy: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res.stdout != wantExport {
		t.Errorf("env under a verified proxy printed\n%q\nwant\n%q", res.stdout, wantExport)
	}

	// Plain HTTP is not observed in this release. Printing HTTP_PROXY would
	// route traffic through a proxy that does not record it and then report
	// nothing -- silence read as zero, which is the one thing the coverage
	// rules forbid.
	if strings.Contains(res.stdout, "HTTP_PROXY") || strings.Contains(res.stdout, "http_proxy") {
		t.Errorf("env prints an HTTP_PROXY variable; plain HTTP is not observed:\n%s", res.stdout)
	}

	// The lowercase form is not a duplicate for tidiness: curl reads only
	// lowercase https_proxy, and curl is the first thing anyone tests with.
	if !strings.Contains(res.stdout, "https_proxy") {
		t.Errorf("env omits lowercase https_proxy, the only form curl reads:\n%s", res.stdout)
	}
}

// TestH22_EnvExportsTheVerifiedAddress is the defect the guard alone left open:
// the check passed for a proxy at one address and the export named another.
// env exports what run exports -- the listen_addr the verified file names --
// and never the default port it used to print as a constant.
func TestH22_EnvExportsTheVerifiedAddress(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:19090", "[::1]:18080", "localhost:18080"} {
		t.Run(addr, func(t *testing.T) {
			e := newEnv(t)
			res := e.run("", observeHome(t, e, liveStatusAt(addr)), "env")

			if res.exitCode != 0 {
				t.Fatalf("env under a proxy verified at %s: exit %d, stderr %q",
					addr, res.exitCode, res.stderr)
			}
			if want := exportFor(addr); res.stdout != want {
				t.Errorf("env under a proxy verified at %s printed\n%q\nwant\n%q",
					addr, res.stdout, want)
			}
			if strings.Contains(res.stdout, "127.0.0.1:"+observePort) {
				t.Errorf("env exported the default address although the verified proxy "+
					"listens at %s, so the shell points at a port nothing verified:\n%s",
					addr, res.stdout)
			}
		})
	}
}

// TestH22_EnvRefusesAListenAddressThatIsNotHostPort: every address posture's
// grammar refuses is refused by env, with stdout EMPTY -- the injection rows
// are the reason that assertion exists, since anything printed there reaches
// eval -- and the reason names the address quoted, so the text the file
// carried is shown to the operator and never interpreted.
//
// Each row is loopback to isLoopback, which reads only the host. That is the
// point of the rows: the check that shipped accepted every one of them.
func TestH22_EnvRefusesAListenAddressThatIsNotHostPort(t *testing.T) {
	for _, addr := range []string{
		"localhost",                     // portless
		"::1",                           // portless
		"[::1]",                         // portless, bracketed
		"127.0.0.1:",                    // an empty port
		"127.0.0.1:1;cmd",               // a second command after the port
		"127.0.0.1:1 $(x)",              // a substitution after the port
		"127.0.0.1:18080\ntouch /tmp/x", // a second line, for `eval "$vars"`
		"127.0.0.1:+18080",              // a signed port
		"127.0.0.1:018080",              // a leading zero
		"127.0.0.1:0",                   // out of range
		"127.0.0.1:65536",               // out of range
		"LOCALHOST:18080",               // not one of the three spellings
	} {
		t.Run(addr, func(t *testing.T) {
			e := newEnv(t)
			res := e.run("", observeHome(t, e, liveStatusAt(addr)), "env")

			if res.exitCode != 1 {
				t.Errorf("env against listen_addr %q: exit %d, want 1", addr, res.exitCode)
			}
			if res.stdout != "" {
				t.Errorf("env against listen_addr %q wrote to stdout, which eval runs:\n%s",
					addr, res.stdout)
			}
			if want := strconv.Quote(addr); !strings.Contains(res.stderr, want) ||
				!strings.Contains(res.stderr, "not HOST:PORT") {
				t.Errorf("env's refusal does not name the address as %s with the grammar "+
					"refusal; stderr %q", want, res.stderr)
			}
			if strings.Contains(res.stderr, noStatusFile) {
				t.Errorf("premise broken: no status file was found, so this row tested the "+
					"missing-file refusal; stderr %q", res.stderr)
			}
		})
	}
}

// TestH22_AListenAddressNeverReachesEval is the CWE-78 finding, measured
// through a real shell rather than inferred from stdout. The status file
// carries a command after the port, once after a semicolon for the documented
// `eval $(rashomon env)` and once after a newline for the capture form, whose
// quoting preserves the newline. The capture form is joined with `;` rather
// than the `&&` the design notes recommend, so eval runs whatever env's exit
// status was and the test leans on the empty stdout alone. Each command would
// create a marker file. The marker not existing afterwards is the claim; the
// sentinel HTTPS_PROXY surviving says eval ran nothing else either.
func TestH22_AListenAddressNeverReachesEval(t *testing.T) {
	const sentinel = "http://sentinel.invalid:1"
	for _, tc := range []struct {
		name, sep, script string
	}{
		{name: "eval of the bare substitution", sep: ";",
			script: `eval $("$1" env); printf 'HTTPS_PROXY=[%s]\n' "$HTTPS_PROXY"`},
		{name: "eval of a captured, quoted value", sep: "\n",
			script: `vars=$("$1" env); eval "$vars"; printf 'HTTPS_PROXY=[%s]\n' "$HTTPS_PROXY"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			marker := filepath.Join(t.TempDir(), "injected")
			extra := append(observeHome(t, e, liveStatusAt("127.0.0.1:18080"+tc.sep+"touch "+marker)),
				"HTTPS_PROXY="+sentinel)
			cmd := exec.Command("sh", "-c", tc.script, "sh", rashomonBin)
			cmd.Env = e.environ(extra...)
			cmd.Dir = e.cwd
			res := e.wait(cmd)

			if _, err := os.Stat(marker); err == nil {
				t.Fatalf("the status file's listen_addr ran a command in the operator's shell; "+
					"stderr %q", res.stderr)
			}
			if want := "HTTPS_PROXY=[" + sentinel + "]"; !strings.Contains(res.stdout, want) {
				t.Errorf("after eval the shell holds\n%s\nwant %s", res.stdout, want)
			}
			if !strings.Contains(res.stderr, "not HOST:PORT") {
				t.Errorf("the refusal reached the shell without the grammar reason; stderr %q",
					res.stderr)
			}
		})
	}
}

// TestH22_EnvPortMustNameTheVerifiedListener. --port used to SELECT the
// address; with the verified listener now deciding it, the flag can only
// confirm. Equal, it exports exactly as today. Different, it refuses and names
// the verified address -- exporting the requested port would point the shell
// at a listener nothing verified, and exporting the verified one would
// silently drop a flag the operator typed.
func TestH22_EnvPortMustNameTheVerifiedListener(t *testing.T) {
	t.Run("equal", func(t *testing.T) {
		e := newEnv(t)
		res := e.run("", observeHome(t, e, liveStatus()), "env", "--port", observePort)
		if res.exitCode != 0 {
			t.Fatalf("env --port %s under a proxy verified there: exit %d, stderr %q",
				observePort, res.exitCode, res.stderr)
		}
		if res.stdout != wantExport {
			t.Errorf("env --port %s printed\n%q\nwant\n%q", observePort, res.stdout, wantExport)
		}
	})

	t.Run("different", func(t *testing.T) {
		e := newEnv(t)
		res := e.run("", observeHome(t, e, liveStatus()), "env", "--port", "19090")
		if res.exitCode != 1 {
			t.Errorf("env --port 19090 under a proxy verified at 127.0.0.1:%s: exit %d, "+
				"want 1", observePort, res.exitCode)
		}
		if res.stdout != "" {
			t.Errorf("env --port naming an unverified listener wrote to stdout:\n%s", res.stdout)
		}
		if want := "--port 19090 is not the verified listener 127.0.0.1:" + observePort; !strings.Contains(res.stderr, want) {
			t.Errorf("the refusal does not name the verified address (want %q); stderr %q",
				want, res.stderr)
		}
	})
}

// TestH22_EnvRejectsABadPort refuses rather than falling back, and does so on
// ANY machine: no status file here, so the refusal naming --port proves the
// argument was read before anything else was. A guard that ran first would
// answer every one of these with "no proxy" instead.
func TestH22_EnvRejectsABadPort(t *testing.T) {
	for _, bad := range []string{"0", "-1", "70000", "eighty-eighty", "18080; rm -rf /"} {
		e := newEnv(t)
		res := e.run("", observeHome(t, e, nil), "env", "--port", bad)
		if res.exitCode == 0 {
			t.Errorf("env --port %q exited 0; a bad port must be refused, not defaulted:\n%s",
				bad, res.stdout)
		}
		if res.stdout != "" {
			t.Errorf("env --port %q wrote to stdout while refusing:\n%s", bad, res.stdout)
		}
		if !strings.Contains(res.stderr, "--port") || strings.Contains(res.stderr, noStatusFile) {
			t.Errorf("env --port %q was refused for some other reason than the port; "+
				"stderr %q", bad, res.stderr)
		}
	}
}

// TestH22_EnvArgumentsAnswerWithoutAProxy: --help and an unknown argument are
// questions about the command, not about the machine, and a guard that ran
// before them refused both with "no proxy" -- including the help text the
// usage screen's own comment promises this command keeps.
func TestH22_EnvArgumentsAnswerWithoutAProxy(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{args: []string{"--help"}, want: []string{"prints the proxy variables", "--token",
			"report --proxy-store PATH"}},
		{args: []string{"--not-a-flag"}, want: []string{`unknown argument "--not-a-flag"`}},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			e := newEnv(t)
			res := e.run("", observeHome(t, e, nil), append([]string{"env"}, tc.args...)...)
			if res.stdout != "" {
				t.Errorf("env %v wrote to stdout, which eval would run:\n%s", tc.args, res.stdout)
			}
			for _, w := range tc.want {
				if !strings.Contains(res.stderr, w) {
					t.Errorf("env %v does not say %q; stderr %q", tc.args, w, res.stderr)
				}
			}
			if strings.Contains(res.stderr, noStatusFile) {
				t.Errorf("env %v consulted the proxy before reading its own arguments; "+
					"stderr %q", tc.args, res.stderr)
			}
		})
	}
}

// TestH22_EnvIsEvalSafe runs the documented form, `eval $(rashomon env)`,
// through a real shell and reads the variable back, for both outcomes.
//
// The shell starts with a sentinel HTTPS_PROXY. That is what makes "the
// environment is unchanged" a measurement rather than an inference: an empty
// variable afterwards could mean nothing was exported or that something
// exported an empty value, while the sentinel surviving can only mean eval
// ran nothing.
func TestH22_EnvIsEvalSafe(t *testing.T) {
	const sentinel = "http://sentinel.invalid:1"
	for _, tc := range []struct {
		name   string
		status map[string]any
		want   string
	}{
		{name: "refused", status: nil, want: sentinel},
		{name: "verified", status: liveStatus(), want: "http://127.0.0.1:" + observePort},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			extra := append(observeHome(t, e, tc.status), "HTTPS_PROXY="+sentinel)
			// The binary arrives as $1 rather than spliced into the script, so
			// its path needs no quoting to survive the shell.
			cmd := exec.Command("sh", "-c",
				`eval $("$1" env); printf 'HTTPS_PROXY=[%s]\n' "$HTTPS_PROXY"`, "sh", rashomonBin)
			cmd.Env = e.environ(extra...)
			cmd.Dir = e.cwd
			res := e.wait(cmd)

			if res.exitCode != 0 {
				t.Fatalf("the shell running eval exited %d, stderr %q", res.exitCode, res.stderr)
			}
			if want := "HTTPS_PROXY=[" + tc.want + "]"; !strings.Contains(res.stdout, want) {
				t.Errorf("after `eval $(rashomon env)` the shell holds\n%s\nwant %s",
					res.stdout, want)
			}
			if tc.status == nil && !strings.Contains(res.stderr, noStatusFile) {
				t.Errorf("the refusal reached the shell without saying why; stderr %q", res.stderr)
			}
		})
	}

	// And line by line, for the export: one stray word of prose on stdout and
	// eval tries to run it as a command.
	e := newEnv(t)
	res := e.run("", observeHome(t, e, liveStatus()), "env")
	for i, line := range strings.Split(strings.TrimSpace(res.stdout), "\n") {
		if !strings.HasPrefix(line, "export ") {
			t.Errorf("stdout line %d is not an export assignment: %q. `eval $(rashomon "+
				"env)` would try to run it as a command.", i+1, line)
		}
		if strings.ContainsAny(line, "`$(;&|") {
			t.Errorf("stdout line %d carries shell metacharacters: %q", i+1, line)
		}
	}
}

// TestH22_EnvNeedsNoStore keeps the command usable before anything has been
// recorded. It reads the proxy's status file and nothing of ours; requiring a
// store would make it depend on having already run something else.
func TestH22_EnvNeedsNoStore(t *testing.T) {
	e := newEnv(t)
	res := e.run("", observeHome(t, e, liveStatus()), "env")
	if res.exitCode != 0 {
		t.Fatalf("env on a machine with no store: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, wantHTTPSUp) {
		t.Errorf("env printed nothing useful without a store:\n%s", res.stdout)
	}
	if _, err := os.Stat(filepath.Join(e.home, "install.json")); err == nil {
		t.Error("env created a store; it must read nothing of ours and create nothing")
	}
}

// TestH22_EnvTokenIsCapabilityGated is run's compatibility rule, held for env.
//
// A proxy that does not understand the credential is entitled to answer 407
// to every CONNECT carrying one, and `eval $(rashomon env --token)` is harder
// to undo than a single run. So --token prints a tagged URL only when the
// VERIFIED status file says the proxy accepts one, and ABSENT MEANS NO. A
// refusal prints nothing at all rather than falling back to an untagged
// export, because the user asked for a tag and silently not getting one is
// the dropped-flag defect --port refuses for.
//
// The tagged row uses a non-default listener so the test also proves the tag
// rides on the verified address, not on a constant.
func TestH22_EnvTokenIsCapabilityGated(t *testing.T) {
	const addr = "127.0.0.1:19090"
	runEnv := func(t *testing.T, mutate func(map[string]any)) result {
		t.Helper()
		e := newEnv(t)
		if res := e.watch(); res.exitCode != 0 {
			t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		fields := liveStatusAt(addr)
		if mutate != nil {
			mutate(fields)
		}
		return e.run("", observeHome(t, e, fields), "env", "--token")
	}

	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "absent means no"},
		{name: "explicit false means no", mutate: func(m map[string]any) { m["session_token"] = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := runEnv(t, tc.mutate)
			if res.exitCode != 1 {
				t.Errorf("env --token against a proxy that never advertised the capability: "+
					"exit %d, want 1", res.exitCode)
			}
			if res.stdout != "" {
				t.Errorf("env --token printed although the proxy may answer 407 to a tagged "+
					"CONNECT:\n%s", res.stdout)
			}
			if !strings.Contains(res.stderr, "does not advertise session_token") {
				t.Errorf("env --token refused without saying why; stderr %q", res.stderr)
			}
		})
	}

	t.Run("true tags the verified address", func(t *testing.T) {
		res := runEnv(t, func(m map[string]any) { m["session_token"] = true })
		if res.exitCode != 0 {
			t.Fatalf("env --token: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		for _, want := range []string{
			"export HTTPS_PROXY=http://rashomon:rt_",
			"export https_proxy=http://rashomon:rt_",
			"@" + addr + "\n",
		} {
			if !strings.Contains(res.stdout, want) {
				t.Errorf("env --token does not print %q:\n%s", want, res.stdout)
			}
		}
	})
}
