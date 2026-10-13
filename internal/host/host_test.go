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
		{name: "two trailing dots refused", in: "pypi.org..", ok: false,
			why: "an empty final label is not a hostname, and dropping one dot left a name a second pass changed"},
		{name: "query dropped", in: "https://api.github.com/user?tok=1", want: "api.github.com", ok: true,
			why: "a query string is the likeliest place for a credential to ride along"},
		{name: "loopback kept", in: "127.0.0.1:18080", want: "127.0.0.1", ok: true,
			why: "the report decides to exclude loopback; this function must not make that invisible"},
		{name: "localhost kept", in: "http://localhost:3000", want: "localhost", ok: true},
		{name: "ipv6 brackets kept via authority", in: "[::1]:443", want: "[::1]", ok: true},
		{name: "ipv6 brackets restored via url", in: "https://[2606:4700::1111]/x", want: "[2606:4700::1111]", ok: true,
			why: "url.Hostname strips the brackets and every consumer here expects them"},
		{name: "bare ipv6 without port is bracketed", in: "2606:4700::1111", want: "[2606:4700::1111]", ok: true,
			why: "more than one colon cannot be a port separator, and the bracketed form is the one spelling"},
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

// TestCanonicalName_DoesNotFoldDigits: CanonicalName is Canonical's twin for
// an ssh destination, where a number is as plausible a config alias as any
// other name. Canonical("1234") folds to "0.0.4.210" and Canonical("0")
// folds to "0.0.0.0", which Loopback then reads as this machine -- right for
// a URL a client actually resolves, wrong for an alias nothing resolved.
// CanonicalName leaves every one of them as the name it was typed.
func TestCanonicalName_DoesNotFoldDigits(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"1234", "1234"},
		{"0", "0"},
		{"167838211", "167838211"},
		{"10.1.515", "10.1.515"},
		{"012.1.2.3", "012.1.2.3"},
	} {
		got, ok := CanonicalName(tc.in)
		if !ok || got != tc.want {
			t.Errorf("CanonicalName(%q) = %q,%v, want %q,true", tc.in, got, ok, tc.want)
		}
	}
	// A bracketed IPv6 literal is unambiguous either way: CanonicalName still
	// writes it in its one spelling.
	if got, ok := CanonicalName("[2606:4700:0:0:0:0:0:1111]"); !ok || got != "[2606:4700::1111]" {
		t.Errorf("CanonicalName(ipv6) = %q,%v, want [2606:4700::1111],true", got, ok)
	}
	// Canonical, unchanged, still folds every one of these: CanonicalName is
	// an addition, not a behaviour change to the function URL authorities use.
	for _, tc := range []struct{ in, want string }{
		{"1234", "0.0.4.210"},
		{"0", "0.0.0.0"},
		{"167838211", "10.1.2.3"},
	} {
		got, ok := Canonical(tc.in)
		if !ok || got != tc.want {
			t.Errorf("Canonical(%q) = %q,%v, want %q,true (unchanged)", tc.in, got, ok, tc.want)
		}
	}
}

// TestLoopbackName_DoesNotFoldDigits: LoopbackName must not read a bare
// number as this machine, the same bug as CanonicalName's but observed
// through Loopback's decision rather than through the string it returns.
// Loopback, unchanged, still does: a URL authority of "0" really does reach
// this machine.
func TestLoopbackName_DoesNotFoldDigits(t *testing.T) {
	for _, in := range []string{"0", "1234", "2130706433", "127.1"} {
		if LoopbackName(in) {
			t.Errorf("LoopbackName(%q) = true, want false: not a URL a client resolved", in)
		}
	}
	for _, in := range []string{"0", "2130706433", "127.1"} {
		if !Loopback(in) {
			t.Errorf("Loopback(%q) = false, want true (unchanged)", in)
		}
	}
	// What LoopbackName still catches: localhost and a real loopback literal,
	// neither of which needed the fold to begin with.
	for _, in := range []string{"localhost", "api.localhost", "127.0.0.1", "[::1]"} {
		if !LoopbackName(in) {
			t.Errorf("LoopbackName(%q) = false, want true", in)
		}
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

// TestRubricTest1_CanonicalIsAFixedPoint: a host Canonical accepts is one it
// returns unchanged. With two trailing dots it dropped one and accepted the
// rest, so "a.." recorded "a." and a second pass made it "a": the wire side
// and the declaration side disagreed on the one host both had seen, and
// FuzzHosts found it from `https://x..` under the CI fuzz command.
func TestRubricTest1_CanonicalIsAFixedPoint(t *testing.T) {
	for _, in := range []string{"pypi.org..", "https://pypi.org../x", "pypi.org..:443", "..", "a...", "pypi.org.", "PyPI.org"} {
		got, ok := Canonical(in)
		if !ok {
			continue
		}
		if again, ok := Canonical(got); !ok || again != got {
			t.Errorf("Canonical(%q) = %q, and Canonical(%q) = %q, %v", in, got, got, again, ok)
		}
	}
}
