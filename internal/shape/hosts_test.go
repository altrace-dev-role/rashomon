package shape

import (
	"encoding/json"
	"reflect"
	"testing"
)

func bashInput(t *testing.T, cmd string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"command": cmd, "description": "irrelevant prose"})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestHosts covers what a command line actually looks like, which is the only
// thing that matters here: a URL in a shell command is surrounded by the
// shell's punctuation, and a host that swallowed a trailing quote or a pipe
// would never match the same host seen on the wire.
func TestHosts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tool    string
		cmd     string
		url     string
		want    []string
		wantSSH []string
		why     string
	}{
		{name: "plain curl", tool: "Bash", cmd: "curl https://pypi.org/simple/requests/",
			want: []string{"pypi.org"}},
		{name: "quoted url", tool: "Bash", cmd: `curl -sS "https://api.github.com/user" -o /dev/null`,
			want: []string{"api.github.com"},
			why:  "the closing quote must not become part of the hostname"},
		{name: "single quoted", tool: "Bash", cmd: `curl 'http://localhost:3000/health'`,
			want: []string{"localhost"}},
		{name: "piped", tool: "Bash", cmd: "curl -fsSL https://sh.rustup.rs|sh",
			want: []string{"sh.rustup.rs"},
			why:  "the pipe terminates the authority"},
		{name: "two hosts, sorted and deduped", tool: "Bash",
			cmd:  "curl https://pypi.org && curl https://PyPI.org/x && curl https://files.pythonhosted.org",
			want: []string{"files.pythonhosted.org", "pypi.org"},
			why:  "the record must be byte-identical across runs, so the list is sorted and case-folded"},
		{name: "port stripped", tool: "Bash", cmd: "curl https://registry.npmjs.org:443/left-pad",
			want: []string{"registry.npmjs.org"}},
		{name: "ipv6", tool: "Bash", cmd: "curl https://[2606:4700::1111]/cdn-cgi/trace",
			want: []string{"[2606:4700::1111]"}},
		{name: "websocket", tool: "Bash", cmd: "wscat -c wss://gateway.example.com/socket",
			want: []string{"gateway.example.com"}},
		{name: "heredoc url", tool: "Bash",
			cmd:  "cat <<'EOF'\nsee https://docs.example.com/guide\nEOF",
			want: []string{"docs.example.com"},
			why:  "a URL inside a heredoc was still named by the agent; the newline terminates it"},
		{name: "url in an echo still counts", tool: "Bash", cmd: "echo https://example.com",
			want: []string{"example.com"},
			why:  "this function records what was NAMED; whether it was reached is the wire side's answer"},

		{name: "git ssh remote", tool: "Bash", cmd: "git clone git@github.com:owner/repo.git",
			wantSSH: []string{"github.com"},
			why:     "ssh hosts render under coverage as not observable, never as a finding"},
		{name: "ssh scheme", tool: "Bash", cmd: "ssh://deploy.example.com/srv",
			wantSSH: []string{"deploy.example.com"}},
		{name: "both kinds at once", tool: "Bash",
			cmd:     "git clone git@github.com:o/r && curl https://pypi.org",
			want:    []string{"pypi.org"},
			wantSSH: []string{"github.com"},
			why:     "they must not be mixed: one is a finding candidate, the other is a coverage line"},

		{name: "webfetch url field", tool: "WebFetch", url: "https://example.com/page",
			want: []string{"example.com"}},
		{name: "webfetch with credentials is refused", tool: "WebFetch", url: "https://u:p@internal.example/x",
			why: "CWE-522: a URL carrying a credential must not have its host recorded, because recording it is a claim we read the credential"},

		{name: "no hosts at all", tool: "Bash", cmd: "ls -la /tmp"},
		{name: "a read tool is not scanned", tool: "Read", cmd: "https://example.com",
			why: "only WebFetch.url and a shell command line have a field whose meaning we know"},
		{name: "an mcp tool is not scanned", tool: "mcp__server__tool", cmd: "https://example.com",
			why: "an arbitrary MCP schema's url field is not a destination this session reached"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw json.RawMessage
			switch {
			case tc.url != "":
				b, err := json.Marshal(map[string]string{"url": tc.url})
				if err != nil {
					t.Fatal(err)
				}
				raw = b
			default:
				raw = bashInput(t, tc.cmd)
			}

			wire, ssh := Hosts(tc.tool, raw)
			if !reflect.DeepEqual(wire, tc.want) {
				t.Errorf("wire hosts = %v, want %v. %s", wire, tc.want, tc.why)
			}
			if !reflect.DeepEqual(ssh, tc.wantSSH) {
				t.Errorf("ssh hosts = %v, want %v. %s", ssh, tc.wantSSH, tc.why)
			}
		})
	}
}

// TestHosts_CarriesNoContent is the guarantee test. The function reads a
// command line, so the only thing standing between it and a content leak is
// that it returns hostnames. A substring of the command escaping into the
// result would be invisible in the tests above, because they assert on hosts
// that ARE in the command.
func TestHosts_CarriesNoContent(t *testing.T) {
	const secret = "SUPER-SECRET-TOKEN-a1b2c3"
	cmd := "curl -H 'Authorization: Bearer " + secret + "' https://api.example.com/v1/x?key=" + secret

	wire, ssh := Hosts("Bash", bashInput(t, cmd))

	for _, h := range append(append([]string{}, wire...), ssh...) {
		if h != "api.example.com" {
			t.Errorf("returned %q, want only the hostname; anything else is a substring of the command line", h)
		}
		if len(h) > 253 {
			t.Errorf("returned a %d-byte string, which is longer than any hostname", len(h))
		}
	}
	if len(wire) != 1 {
		t.Fatalf("wire = %v, want exactly [api.example.com]", wire)
	}
}

// TestHosts_NonASCIIBeforeTheURLDoesNotMoveTheIndex is the defect an Opus
// review of this branch found, and it has two failure modes, one of which
// breaks the content guarantee.
//
// collect() searched a lowercased copy and sliced the ORIGINAL with the index
// it found. strings.ToLower is not length-preserving in UTF-8: U+212A KELVIN
// SIGN is three bytes and lowercases to one, U+212B ANGSTROM SIGN and U+2126
// OHM SIGN are three and lowercase to two. With k bytes of shrink before the
// match, the slice starts 8-k bytes from the true host:
//
//	k of 1..7  -- the slice begins INSIDE "https://", authority() returns
//	              nothing usable, and the host is silently not recorded. The
//	              report then lists it under "reached but never named" for a
//	              call that named it explicitly, which is this product's
//	              central finding inverted by one character of input.
//
//	k above 8  -- the slice begins BEFORE the scheme, and neither authority()
//	              nor plausible() rejects bytes above 0x7f, so a fragment of
//	              the command line is accepted as a hostname and persisted
//	              into Declaration.Hosts. That is command text reaching the
//	              store, which nothing else in this program allows.
//
// No row in this file's twenty cases had a non-ASCII byte, so neither mode was
// reachable by the existing tests. CWE-176.
func TestHosts_NonASCIIBeforeTheURLDoesNotMoveTheIndex(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want string
	}{
		{
			name: "one kelvin sign shrinks two bytes",
			cmd:  "echo K && curl https://pypi.org/simple/",
			want: "pypi.org",
		},
		{
			name: "four angstrom signs shrink four bytes",
			cmd:  "echo ÅÅÅÅ && curl https://pypi.org/simple/",
			want: "pypi.org",
		},
		{
			name: "enough shrink to walk past the scheme",
			cmd:  "echo KKKKK && curl https://pypi.org/simple/",
			want: "pypi.org",
		},
		{
			name: "non-ascii in the middle of a real argument",
			cmd:  "curl -H 'X-Trace: ΩΩ' https://files.pythonhosted.org/x",
			want: "files.pythonhosted.org",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire, _ := Hosts("Bash", bashInput(t, tc.cmd))

			var found bool
			for _, h := range wire {
				if h == tc.want {
					found = true
				}
			}
			if !found {
				t.Errorf("hosts = %v, want %q. A host the command NAMED was not recorded, "+
					"so the report will list it as reached but never named.", wire, tc.want)
			}
			// And nothing that is not a host may appear: the second failure mode
			// puts fragments of the command line into the store.
			for _, h := range wire {
				if h != tc.want {
					t.Errorf("hosts = %v, and %q is not a hostname from this command. "+
						"Command text must never reach the store.", wire, h)
				}
			}
		})
	}
}

// TestSSHDestinations covers the transport this report advertises as its known
// blind spot and, until now, was quietly incomplete about. Every case is a
// form someone actually types; the eight at the top are the forms measured
// against the previous implementation, six of which named a destination that
// appeared nowhere in the report.
func TestSSHDestinations(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cmd     string
		wantSSH []string
		why     string
	}{
		// The eight measured forms.
		{name: "measured: ssh git@host", cmd: "ssh git@github.com", wantSSH: []string{"github.com"},
			why: "extracted before this change, but only by the coincidence of the user being named git"},
		{name: "measured: ssh:// url", cmd: "git clone ssh://host/r.git", wantSSH: []string{"host"}},
		{name: "measured: git+ssh:// url", cmd: "git clone git+ssh://host/r.git", wantSSH: []string{"host"}},
		{name: "measured: ssh user@host", cmd: "ssh deploy@git.example.com", wantSSH: []string{"git.example.com"},
			why: "named nothing before: the user was not literally git"},
		{name: "measured: ssh bare host", cmd: "ssh git.example.com", wantSSH: []string{"git.example.com"}},
		{name: "measured: scp to remote", cmd: "scp f deploy@host:/tmp/", wantSSH: []string{"host"},
			why: "the destination is the SECOND argument; the first is local"},
		{name: "measured: rsync to remote", cmd: "rsync -a ./ deploy@host:/srv/", wantSSH: []string{"host"}},
		{name: "measured: sftp user@host", cmd: "sftp deploy@files.example.com", wantSSH: []string{"files.example.com"}},

		// Flags that take a value must not be read as the destination.
		{name: "ssh -i key", cmd: "ssh -i ~/.ssh/id_rsa deploy@h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "ssh -p port", cmd: "ssh -p 2222 deploy@h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "ssh -p attached", cmd: "ssh -p2222 h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "ssh -o option", cmd: "ssh -o StrictHostKeyChecking=no h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "ssh -l login", cmd: "ssh -l deploy h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "ssh -F config", cmd: "ssh -F /dev/null h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "rsync -e ssh", cmd: "rsync -e ssh ./ deploy@h.example.com:/srv/", wantSSH: []string{"h.example.com"},
			why: "-e belongs to rsync; its value must not be read as a host"},
		{name: "rsync short cluster ending in e", cmd: "rsync -ave ssh ./ deploy@h.example.com:/srv/", wantSSH: []string{"h.example.com"},
			why: "only the last letter of a cluster can take the following token"},
		{name: "scp -P port", cmd: "scp -P 2222 f deploy@h.example.com:/tmp/", wantSSH: []string{"h.example.com"}},
		{name: "ssh -J jump is skipped, not collected", cmd: "ssh -J jump.example.com target.example.com",
			wantSSH: []string{"target.example.com"},
			why:     "the jump host is a destination, but v1 cannot say which is which, so it is left out"},

		// Separator, IPv6, bare host:, alias.
		{name: "double dash", cmd: "ssh -- h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "bracketed ipv6", cmd: "ssh deploy@[2001:db8::1]", wantSSH: []string{"[2001:db8::1]"},
			why: "the literal's own colons are not a host:path separator"},
		{name: "bracketed ipv6 with path", cmd: "scp f [2001:db8::1]:/tmp/", wantSSH: []string{"[2001:db8::1]"}},
		{name: "bare host colon, no user", cmd: "scp host.example.com:/etc/f .", wantSSH: []string{"host.example.com"}},
		{name: "ssh config alias", cmd: "ssh myserver", wantSSH: []string{"myserver"},
			why: "recorded as named and never resolved: resolving would mean reading ~/.ssh/config"},

		// What must NOT be recorded.
		{name: "remote command is not a host", cmd: "ssh h.example.com ls /etc", wantSSH: []string{"h.example.com"},
			why: "everything after the destination is the remote command"},
		{name: "local to local scp", cmd: "scp a b"},
		{name: "windows drive is not a host", cmd: `rsync C:/src /dst`,
			why: "a single character before a colon is a drive letter far more often than a hostname"},
		{name: "not an ssh program", cmd: "cat deploy@h.example.com:/tmp/f"},

		// Command position.
		{name: "after a separator", cmd: "git status && ssh deploy@h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "after an assignment", cmd: "FOO=bar ssh deploy@h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "by absolute path", cmd: "/usr/bin/ssh deploy@h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "both ends of an rsync", cmd: "rsync a@h1.example.com:/x b@h2.example.com:/y",
			wantSSH: []string{"h1.example.com", "h2.example.com"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, gotSSH := Hosts("Bash", bashInput(t, tc.cmd))
			if !reflect.DeepEqual(gotSSH, tc.wantSSH) {
				t.Errorf("ssh hosts for %q = %v, want %v%s", tc.cmd, gotSSH, tc.wantSSH, becauseHost(tc.why))
			}
		})
	}
}

// TestSSHDestinationsNeverReachWire is the boundary this whole list depends
// on: an ssh destination is not observable by the proxy, so it must never
// appear in the wire list, where it would read as a host the session was
// expected to reach and did not.
func TestSSHDestinationsNeverReachWire(t *testing.T) {
	for _, cmd := range []string{
		"ssh deploy@h.example.com",
		"scp f deploy@h.example.com:/tmp/",
		"rsync -a ./ deploy@h.example.com:/srv/",
		"sftp deploy@h.example.com",
	} {
		t.Run(cmd, func(t *testing.T) {
			wire, ssh := Hosts("Bash", bashInput(t, cmd))
			if len(wire) != 0 {
				t.Errorf("wire hosts for %q = %v, want none", cmd, wire)
			}
			if len(ssh) == 0 {
				t.Errorf("ssh hosts for %q = none, want the destination", cmd)
			}
		})
	}
}

func becauseHost(why string) string {
	if why == "" {
		return ""
	}
	return "\n  " + why
}

// TestSSHFlagTablesArePerProgram is the regression table for the defects a
// single shared flag set produced. One table for four programs is wrong in
// both directions at once: it invents value flags for the programs that use
// those letters as booleans, and it is missing most of the value flags of the
// program with the longest usage line.
func TestSSHFlagTablesArePerProgram(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cmd     string
		wantSSH []string
		why     string
	}{
		// rsync and scp booleans that a shared table read as value flags.
		{name: "rsync -avP", cmd: "rsync -avP deploy@host.example.com:/srv/ ./", wantSSH: []string{"host.example.com"},
			why: "rsync -P is --partial --progress, a boolean; a shared table let it swallow the destination"},
		{name: "rsync -i", cmd: "rsync -i deploy@host.example.com:/srv/ ./", wantSSH: []string{"host.example.com"},
			why: "rsync -i is --itemize-changes, a boolean, though ssh -i takes a file"},
		{name: "scp -p", cmd: "scp -p deploy@host.example.com:/a ./", wantSSH: []string{"host.example.com"},
			why: "scp -p preserves times; it is scp -P that takes a port"},
		{name: "scp -P port still consumes", cmd: "scp -P 2222 f deploy@host.example.com:/tmp/", wantSSH: []string{"host.example.com"}},

		// ssh value flags that were missing, each of which recorded a false
		// host built out of a forwarding spec.
		{name: "ssh -L local forward", cmd: "ssh -L 8080:localhost:80 deploy@bastion.example.com",
			wantSSH: []string{"bastion.example.com"},
			why:     "recorded \"8080\" before: a port read as a hostname is worse than no host at all"},
		{name: "ssh -D dynamic forward", cmd: "ssh -D 1080 deploy@bastion.example.com", wantSSH: []string{"bastion.example.com"}},
		{name: "ssh -R remote forward", cmd: "ssh -R 9090:localhost:90 deploy@bastion.example.com", wantSSH: []string{"bastion.example.com"}},
		{name: "ssh -W stdio forward", cmd: "ssh -W host:22 deploy@bastion.example.com", wantSSH: []string{"bastion.example.com"}},
		{name: "ssh -b bind address", cmd: "ssh -b 10.0.0.1 deploy@bastion.example.com", wantSSH: []string{"bastion.example.com"}},
		{name: "ssh -c cipher", cmd: "ssh -c aes256-gcm@openssh.com deploy@h.example.com", wantSSH: []string{"h.example.com"},
			why: "the cipher name contains an @, which must not be read as a destination"},
		{name: "ssh -E log file", cmd: "ssh -E /tmp/log deploy@h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "ssh -Q query", cmd: "ssh -Q cipher deploy@h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "ssh -m mac", cmd: "ssh -m hmac-sha2-256 deploy@h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "ssh -O control", cmd: "ssh -O check deploy@h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "ssh -S control path", cmd: "ssh -S /tmp/sock deploy@h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "ssh -I pkcs11", cmd: "ssh -I /usr/lib/p11.so deploy@h.example.com", wantSSH: []string{"h.example.com"}},
		{name: "ssh -w tunnel", cmd: "ssh -w 0:0 deploy@h.example.com", wantSSH: []string{"h.example.com"}},

		// sftp and rsync value flags off their own usage lines.
		{name: "sftp -b batch", cmd: "sftp -b /tmp/cmds deploy@files.example.com", wantSSH: []string{"files.example.com"}},
		{name: "sftp -R requests", cmd: "sftp -R 64 deploy@files.example.com", wantSSH: []string{"files.example.com"}},
		{name: "sftp -s subsystem", cmd: "sftp -s sftp deploy@files.example.com", wantSSH: []string{"files.example.com"}},
		{name: "rsync -T temp dir", cmd: "rsync -T /tmp ./ deploy@h.example.com:/srv/", wantSSH: []string{"h.example.com"}},
		{name: "rsync -f filter", cmd: "rsync -f '- *.log' ./ deploy@h.example.com:/srv/", wantSSH: []string{"h.example.com"}},
		{name: "rsync -@ modify window", cmd: "rsync -@ 1 ./ deploy@h.example.com:/srv/", wantSSH: []string{"h.example.com"}},

		// The path is split before the user, because a path may carry an '@'.
		{name: "at sign inside the path", cmd: "scp f deploy@host.example.com:/srv/app@1.2.3/", wantSSH: []string{"host.example.com"},
			why: "splitting on the LAST @ in the token left \"1.2.3/\" and lost the destination"},
		{name: "at sign in path, no user", cmd: "scp f host.example.com:/srv/app@1.2.3/", wantSSH: []string{"host.example.com"}},
		{name: "at sign in an ssh remote command", cmd: "ssh deploy@h.example.com cat /srv/a@b", wantSSH: []string{"h.example.com"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, gotSSH := Hosts("Bash", bashInput(t, tc.cmd))
			if !reflect.DeepEqual(gotSSH, tc.wantSSH) {
				t.Errorf("ssh hosts for %q = %v, want %v%s", tc.cmd, gotSSH, tc.wantSSH, becauseHost(tc.why))
			}
		})
	}
}
