package acceptance

import (
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
// The number is provisional and deliberately outside the spec's reservation:
// H-30 is Phase B's and H-31 through H-69 belong to docs/spec-chain-and-scope.md,
// whose own rule greps `H-[3-6][0-9]` against the merge target. H-70 clears
// both and is re-grepped before this merges.

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
// Break: extract ssh hosts from URL schemes only.
func TestH70_SSHDestinationsReachTheRecord(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  string
		want string
	}{
		{name: "ssh user@host", cmd: "ssh deploy@git.example.com", want: "git.example.com"},
		{name: "ssh bare host", cmd: "ssh git.example.com", want: "git.example.com"},
		{name: "scp to remote", cmd: "scp f deploy@host.example.com:/tmp/", want: "host.example.com"},
		{name: "rsync to remote", cmd: "rsync -a ./ deploy@host.example.com:/srv/", want: "host.example.com"},
		{name: "sftp user@host", cmd: "sftp deploy@files.example.com", want: "files.example.com"},
		{name: "flag value is not the host", cmd: "ssh -i /k -p 2222 deploy@h.example.com", want: "h.example.com"},
		{name: "config alias, recorded as named", cmd: "ssh myserver", want: "myserver"},
		{name: "rsync boolean cluster", cmd: "rsync -avP deploy@host.example.com:/srv/ ./", want: "host.example.com"},
		{name: "scp boolean -p", cmd: "scp -p deploy@host.example.com:/a ./", want: "host.example.com"},
		{name: "at sign inside the path", cmd: "scp f deploy@host.example.com:/srv/app@1.2.3/", want: "host.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ssh, wire := sshHostsOf(t, tc.cmd)

			var found bool
			for _, h := range ssh {
				if h == tc.want {
					found = true
				}
			}
			if !found {
				t.Errorf("ssh_hosts = %v, want it to carry %q: the report calls ssh its blind spot, and a destination that reaches no list is not even that", ssh, tc.want)
			}
			// The same host must never reach the wire list. There it would
			// read as something the session was expected to reach and did
			// not, which is a finding rather than a known limitation.
			for _, h := range wire {
				if h == tc.want {
					t.Errorf("hosts = %v, which carries the ssh destination %q: the proxy cannot see it, so it must not be counted as unreached", wire, tc.want)
				}
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

// TestH70_LocalArgumentsAreNotHosts is the negative twin. An extractor that
// satisfied the list above by recording every argument would pass it
// completely, and would fill the store with filenames.
//
// Break: record every non-flag argument as a host.
func TestH70_LocalArgumentsAreNotHosts(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  string
	}{
		{name: "local to local", cmd: "scp a b"},
		{name: "windows drive", cmd: `rsync C:/src /dst`},
		{name: "remote command after the destination", cmd: "ssh h.example.com ls /etc"},
		{name: "not an ssh program", cmd: "cat deploy@h.example.com:/tmp/f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ssh, _ := sshHostsOf(t, tc.cmd)
			for _, h := range ssh {
				switch h {
				case "h.example.com":
					continue // the real destination of the ssh case
				}
				t.Errorf("ssh_hosts = %v for %q, which names no such host", ssh, tc.cmd)
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
