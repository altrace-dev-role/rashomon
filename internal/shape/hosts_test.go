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
