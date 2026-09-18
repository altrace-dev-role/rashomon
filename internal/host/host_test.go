package host

import "testing"

// TestCanonical is the table for the one function both sides of the comparison
// depend on. Every row that matters is a row where two spellings of one
// destination have to collapse, or where a spelling must be refused.
func TestCanonical(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
		ok   bool
		why  string
	}{
		{name: "bare host", in: "pypi.org", want: "pypi.org", ok: true},
		{name: "case folds", in: "PyPI.ORG", want: "pypi.org", ok: true,
			why: "DNS is case-insensitive and the two sides spell it however their source did"},
		{name: "port stripped from authority", in: "pypi.org:443", want: "pypi.org", ok: true,
			why: "the proxy records host:port and a tool call almost never does"},
		{name: "scheme and path dropped", in: "https://pypi.org/simple/requests/", want: "pypi.org", ok: true},
		{name: "scheme with port", in: "https://pypi.org:443/simple", want: "pypi.org", ok: true},
		{name: "trailing dot dropped", in: "pypi.org.", want: "pypi.org", ok: true,
			why: "the fully-qualified spelling is the same name"},
		{name: "only one trailing dot dropped", in: "pypi.org..", want: "pypi.org.", ok: true},
		{name: "query dropped", in: "https://api.github.com/user?tok=1", want: "api.github.com", ok: true,
			why: "a query string is the likeliest place for a credential to ride along"},
		{name: "loopback kept", in: "127.0.0.1:18080", want: "127.0.0.1", ok: true,
			why: "the report decides to exclude loopback; this function must not make that invisible"},
		{name: "localhost kept", in: "http://localhost:3000", want: "localhost", ok: true},
		{name: "ipv6 brackets kept via authority", in: "[::1]:443", want: "[::1]", ok: true},
		{name: "ipv6 brackets restored via url", in: "https://[2606:4700::1111]/x", want: "[2606:4700::1111]", ok: true,
			why: "url.Hostname strips the brackets and every consumer here expects them"},
		{name: "bare ipv6 without port keeps its colons", in: "2606:4700::1111", want: "2606:4700::1111", ok: true,
			why: "more than one colon cannot be a port separator"},
		{name: "wss scheme", in: "wss://gateway.example.com/socket", want: "gateway.example.com", ok: true},

		// Refusals.
		{name: "userinfo refused, not stripped", in: "https://user:pw@internal.example/x", ok: false,
			why: "CWE-522: stripping would return a clean host for an input carrying a credential, making its presence invisible one function deeper"},
		{name: "bare userinfo refused", in: "deploy@github.com", ok: false,
			why: "the git@ form is an ssh host and belongs in ssh_hosts, not here"},
		{name: "empty refused", in: "", ok: false},
		{name: "whitespace only refused", in: "   ", ok: false},
		{name: "lone dot refused", in: ".", ok: false,
			why: "it would otherwise reduce to the empty string and be recorded as a host"},
		{name: "path fragment refused", in: "/usr/local/bin", ok: false},
		{name: "unclosed bracket refused", in: "[::1", ok: false},
		{name: "embedded newline refused", in: "pypi.org\nevil.example", ok: false,
			why: "an ndjson record is one line; a hostname carrying a newline would forge a second record"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Canonical(tc.in)
			if ok != tc.ok {
				t.Fatalf("Canonical(%q) ok = %v, want %v. %s", tc.in, ok, tc.ok, tc.why)
			}
			if ok && got != tc.want {
				t.Errorf("Canonical(%q) = %q, want %q. %s", tc.in, got, tc.want, tc.why)
			}
			if !ok && got != "" {
				t.Errorf("Canonical(%q) refused but returned %q; a refused input must "+
					"yield nothing a caller could record by mistake", tc.in, got)
			}
		})
	}
}

// TestCanonical_IsIdempotent matters because the wire side canonicalises rows
// that the declaration side may already have canonicalised, and a second pass
// that changed the answer would make the two disagree on exactly the hosts that
// went through both.
func TestCanonical_IsIdempotent(t *testing.T) {
	for _, in := range []string{
		"PyPI.org:443", "https://pypi.org/simple", "pypi.org.", "[::1]:443", "127.0.0.1:18080",
	} {
		once, ok := Canonical(in)
		if !ok {
			t.Fatalf("Canonical(%q) refused a valid input", in)
		}
		twice, ok := Canonical(once)
		if !ok || twice != once {
			t.Errorf("Canonical(Canonical(%q)) = %q,%v, want %q,true", in, twice, ok, once)
		}
	}
}
