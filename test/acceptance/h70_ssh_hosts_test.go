package acceptance

import (
	"reflect"
	"strings"
	"testing"
)

// H-70 — the destinations of the transport the report calls its blind spot.
//
// The report advertises ssh as something the proxy cannot see, and renders
// those hosts under "not observable" rather than as a finding. That promise is
// only worth anything if the hosts get there. Measured before this item
// existed: `ssh git@github.com` was recorded, but only because the user was
// literally named `git` and the extractor matched the `git@` prefix, while
// `ssh deploy@host`, `ssh host`, `scp f deploy@host:/tmp/` and `rsync -a ./
// deploy@host:/srv/` recorded nothing at all. Nothing false was claimed; the
// report was silently incomplete about the one transport it names.
//
// The number is provisional and leaves a deliberate gap after H-29, which is
// the highest this tree uses. H-30 is claimed by work not yet on the shared
// remote, and H-31 through H-69 are reserved by a specification under review
// on another branch; neither is visible from the merge target, so the gap is
// the only way to avoid a collision that could not be checked here. It is
// re-grepped against the merge target before this merges.

// sshHostsOf drives one Bash call and returns the ssh_hosts its declaration
// carries, and the wire hosts beside them, because the second list is where
// this could go wrong in the way that matters.
func sshHostsOf(t *testing.T, cmd string) (ssh, wire []string) {
	t.Helper()
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	p.ToolInput = map[string]any{"command": cmd, "description": "irrelevant prose"}
	e.mustHook(p.build(t))

	decls := e.declarations(testSession)
	if len(decls) != 1 {
		t.Fatalf("got %d declarations, want 1", len(decls))
	}
	return stringsOf(decls[0].fields["ssh_hosts"]), stringsOf(decls[0].fields["hosts"])
}

func stringsOf(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// TestH70_SSHDestinationsReachTheRecord is H-70.
//
// The expectation is the WHOLE list, not a membership test. Asserting only
// that the wanted host is present cannot fail on a fabricated one, and a
// false host is the failure this item cares most about: measured against an
// extractor that appended a host of its own invention to every call, a
// contains-style assertion stayed green.
//
// Break: extract ssh hosts from URL schemes only.
// Second break: fabricate a host.
func TestH70_SSHDestinationsReachTheRecord(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  string
		want []string
	}{
		{name: "ssh user@host", cmd: "ssh deploy@git.example.com", want: []string{"git.example.com"}},
		{name: "ssh bare host", cmd: "ssh git.example.com", want: []string{"git.example.com"}},
		{name: "scp to remote", cmd: "scp f deploy@host.example.com:/tmp/", want: []string{"host.example.com"}},
		{name: "rsync to remote", cmd: "rsync -a ./ deploy@host.example.com:/srv/", want: []string{"host.example.com"}},
		{name: "sftp user@host", cmd: "sftp deploy@files.example.com", want: []string{"files.example.com"}},
		{name: "flag value is not the host", cmd: "ssh -i /k -p 2222 deploy@h.example.com", want: []string{"h.example.com"}},
		{name: "config alias, recorded as named", cmd: "ssh myserver", want: []string{"myserver"}},
		{name: "rsync boolean cluster", cmd: "rsync -avP deploy@host.example.com:/srv/ ./", want: []string{"host.example.com"}},
		{name: "scp boolean -p", cmd: "scp -p deploy@host.example.com:/a ./", want: []string{"host.example.com"}},
		{name: "at sign inside the path", cmd: "scp f deploy@host.example.com:/srv/app@1.2.3/", want: []string{"host.example.com"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ssh, wire := sshHostsOf(t, tc.cmd)
			if !reflect.DeepEqual(ssh, tc.want) {
				t.Errorf("ssh_hosts = %v for %q, want exactly %v", ssh, tc.cmd, tc.want)
			}
			// None of it may reach the wire list, where it would read as
			// something the session was expected to reach and did not.
			if len(wire) != 0 {
				t.Errorf("hosts = %v for %q, want none: the proxy cannot see ssh", wire, tc.cmd)
			}
		})
	}
}

// TestH70_ForwardingSpecsAreNotHosts is the negative twin, and it is the case
// that actually went wrong: a missing value flag did not merely lose the
// destination, it recorded a PORT as a hostname. A false host in this list is
// worse than a missing one -- it is a destination the report claims the
// session named, and nobody typed it.
//
// Break: one flag table for all four programs.
func TestH70_ForwardingSpecsAreNotHosts(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  string
		want string
		bad  string
	}{
		{name: "-L local forward", cmd: "ssh -L 8080:localhost:80 deploy@bastion.example.com",
			want: "bastion.example.com", bad: "8080"},
		{name: "-D dynamic forward", cmd: "ssh -D 1080 deploy@bastion.example.com",
			want: "bastion.example.com", bad: "1080"},
		{name: "-R remote forward", cmd: "ssh -R 9090:localhost:90 deploy@bastion.example.com",
			want: "bastion.example.com", bad: "9090"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ssh, _ := sshHostsOf(t, tc.cmd)
			var found bool
			for _, h := range ssh {
				if h == tc.bad {
					t.Errorf("ssh_hosts = %v, which carries %q: that is a port out of a forwarding spec, not a host anyone named", ssh, tc.bad)
				}
				if h == tc.want {
					found = true
				}
			}
			if !found {
				t.Errorf("ssh_hosts = %v, want it to carry %q", ssh, tc.want)
			}
		})
	}
}

// TestH70_CommandTextNeverBecomesAHost is the no-content guarantee for the
// positional path, at the record rather than at the function.
//
// It is the failure that actually happened: `ssh $HOST` put "$host" in a
// declaration, in the JSON report and in the text report. A hostname is the
// only thing this path may emit, and an unexpanded variable or a quoted
// fragment is not one.
//
// Break: hand the token to host.Canonical without requiring it to be an
// authority.
func TestH70_CommandTextNeverBecomesAHost(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  string
	}{
		{name: "unexpanded variable", cmd: "ssh $HOST"},
		{name: "quoted punctuation", cmd: "ssh 'host;evil'"},
		{name: "a scheme is not a destination", cmd: "rsync -av rsync://mirror.example/pub/ ./"},
		{name: "local file with an at sign", cmd: "scp a@b c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ssh, wire := sshHostsOf(t, tc.cmd)
			if len(ssh) != 0 {
				t.Errorf("ssh_hosts = %v for %q, want none: only a hostname may leave this path", ssh, tc.cmd)
			}
			if len(wire) != 0 {
				t.Errorf("hosts = %v for %q, want none", wire, tc.cmd)
			}
		})
	}
}

// TestH70_LocalArgumentsAreNotHosts is the negative twin. An extractor that
// satisfied the list above by recording every argument would pass it
// completely, and would fill the store with filenames.
//
// The expected host is per case, not shared. An earlier version whitelisted
// one hostname across all four subtests, which made "not an ssh program"
// unfailable -- that command can produce no other host -- and the whole
// acceptance group stayed green when the sshDestPrograms gate was deleted
// entirely. A whitelist wider than the case it serves is a hole.
//
// Break: record every non-flag argument as a host.
// Second break: treat every program as an ssh program.
func TestH70_LocalArgumentsAreNotHosts(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  string
		want []string // exactly these, in this order
	}{
		{name: "local to local", cmd: "scp a b"},
		{name: "windows drive", cmd: `rsync C:/src /dst`},
		{name: "remote command after the destination", cmd: "ssh h.example.com ls /etc",
			want: []string{"h.example.com"}},
		{name: "not an ssh program", cmd: "cat deploy@h.example.com:/tmp/f"},
		{name: "not an ssh program, scp-shaped", cmd: "tar -cf a.tar deploy@h.example.com:/tmp/f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ssh, _ := sshHostsOf(t, tc.cmd)
			if !reflect.DeepEqual(ssh, tc.want) {
				t.Errorf("ssh_hosts = %v for %q, want %v", ssh, tc.cmd, tc.want)
			}
		})
	}
}

// TestH70_ArgumentsStopAtTheCommandBoundary pins the rule that a program's
// arguments end where the next command begins. Nothing held it in place
// before: deleting the boundary and letting the scan run to the end of the
// token list was detected by no test at all, and it is the code path the
// newline defect ran through.
//
// Break: let a program's arguments run to the end of the line.
func TestH70_ArgumentsStopAtTheCommandBoundary(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  string
		want []string
	}{
		{name: "two ssh calls on one line", cmd: "ssh deploy@h1.example.com; ssh deploy@h2.example.com",
			want: []string{"h1.example.com", "h2.example.com"}},
		{name: "two ssh calls on two lines", cmd: "ssh deploy@h1.example.com\nssh deploy@h2.example.com",
			want: []string{"h1.example.com", "h2.example.com"}},
		{name: "a later command is not an argument", cmd: "scp a b && curl http://example.com/x"},
		{name: "a later command on a new line is not an argument", cmd: "scp a b\ncurl http://example.com/x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ssh, _ := sshHostsOf(t, strings.ReplaceAll(tc.cmd, "\\n", "\n"))
			if !reflect.DeepEqual(ssh, tc.want) {
				t.Errorf("ssh_hosts = %v for %q, want %v", ssh, tc.cmd, tc.want)
			}
		})
	}
}

// TestH70_NoArgumentTextReachesTheStore is the content guarantee for the
// fields this item newly reads. A destination is an argument of a command
// line, and the rest of that line is the user's own text.
//
// Break: record the whole argument instead of its host.
func TestH70_NoArgumentTextReachesTheStore(t *testing.T) {
	const canary = "canary-9b2e77aa-must-not-persist"
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	p.ToolInput = map[string]any{
		"command": "scp /home/u/" + canary + "/f deploy@host.example.com:/srv/" + canary + "/",
	}
	res := e.hook(p.build(t))
	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}
	e.probe("end", testSession)

	for rel, f := range walkStore(t, e.home) {
		if strings.Contains(string(f.body), canary) {
			t.Errorf("%s contains the canary path segment", rel)
		}
	}
	for name, out := range map[string]string{
		"hook stdout":   res.stdout,
		"hook stderr":   res.stderr,
		"report text":   e.run("", nil, "report", "--session", testSession).stdout,
		"report --json": e.run("", nil, "report", "--json", "--session", testSession).stdout,
	} {
		if strings.Contains(out, canary) {
			t.Errorf("%s contains the canary path segment", name)
		}
	}

	// Premise guard: the call really did name a host, so the sweep above is
	// not passing because nothing was extracted.
	ssh, _ := sshHostsOf(t, "scp /home/u/x/f deploy@host.example.com:/srv/")
	if len(ssh) == 0 {
		t.Fatal("the fixture named no host, so this sweep observed nothing")
	}
}
