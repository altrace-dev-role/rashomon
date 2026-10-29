package host

import (
	"strings"
	"testing"
)

// label63 is a 63-byte label, the longest DNS allows.
var label63 = strings.Repeat("a", 63)

// name is a hostname of exactly n bytes made of 63-byte labels.
func name(n int) string {
	var b strings.Builder
	for b.Len() < n {
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(label63[:min(63, n-b.Len())])
	}
	return b.String()
}

// TestRubricLLD2_ShapedAcceptsHostnamesOnly: Shaped is the declaration side's
// question "is this a host at all". A URL's authority is cut where a command
// line's punctuation begins, which is not where a host ends: `=` and `!` are
// neither, so `https://api.example.com=ghp_...!x/` came back as a "host"
// carrying a token, and with no length bound a 300,008-byte one did too, and
// both were recorded as hostnames the call named.
func TestRubricLLD2_ShapedAcceptsHostnamesOnly(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"api.example.com", true},
		{"API.Example.COM", true},
		{"pypi.org.", true},
		{"build_box", true},
		{"localhost", true},
		{"10.1.2.3", true},
		{"[2606:4700::1111]", true},
		{"[::1]", true},
		{"bücher.example", true},
		{"例え.jp", true},
		{name(253), true},

		{"api.example.com=ghp_q4br7xyzsecrettok3n!x", false},
		{"h.example.com!x", false},
		{"host=bad", false},
		{name(254), false},
		{"", false},
		{".", false},
		{"-leading.example", false},
		{"trailing-", false},
		// RFC 1123: no label begins or ends with a hyphen, the last one or
		// any other; a root dot does not hide the label before it (FuzzHosts,
		// "ssh deploy@acm------.", which the destination reader took and
		// CanonicalName then cut to a name ending in one).
		{"acm------.", false},
		{"a-.example.com", false},
		{"a.-b.example", false},
		{"a-b.example.", true},
		{".example.com", false},
		{"a..b", false},
		{"---", false},
		{"[]", false},
		{"[not-an-address]", false},
		{"[10.1.2.3]", false},
		{"[fe80::1%eth0]", false},
		{"2606:4700::1111", false},
		{"a b", false},
		{"bad\xffutf8", false},
		{"emoji😀.example", false},
	} {
		if got := Shaped(tc.in); got != tc.want {
			t.Errorf("Shaped(%.40q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestRubricLLD2_CanonicalBoundsItsLength: no hostname is longer than 253
// bytes, so Canonical returns none: 253 bytes is one, 254 is not.
func TestRubricLLD2_CanonicalBoundsItsLength(t *testing.T) {
	if got, ok := Canonical(name(253)); !ok || got != name(253) {
		t.Errorf("253 bytes refused")
	}
	if _, ok := Canonical(name(254)); ok {
		t.Errorf("254 bytes accepted")
	}
	if _, ok := Canonical("https://" + name(254) + "/x"); ok {
		t.Errorf("254 bytes accepted through a URL")
	}
}

// TestRubricLLD4_SpellingFamiliesCanonicaliseAlike: a client normalises an IP
// literal before it connects -- curl turns [2606:4700:0:0:0:0:0:1111] into
// [2606:4700::1111] and 167838211 into 10.1.2.3 -- so every spelling of one
// address has to reach the one string the other side compares on. Each row
// is a family; every member canonicalises to the first column.
func TestRubricLLD4_SpellingFamiliesCanonicaliseAlike(t *testing.T) {
	for _, tc := range []struct {
		want    string
		members []string
	}{
		{"[2606:4700::1111]", []string{
			"[2606:4700::1111]", "[2606:4700:0:0:0:0:0:1111]", "[2606:4700:0000:0000:0000:0000:0000:1111]",
			"2606:4700::1111", "2606:4700:0:0:0:0:0:1111", "[2606:4700::1111]:443",
			"https://[2606:4700:0:0:0:0:0:1111]:443/x", "[2606:4700::1111].",
		}},
		{"[2606:4700::abcd]", []string{"[2606:4700::ABCD]", "https://[2606:4700:0::AbCd]/"}},
		{"10.1.2.3", []string{
			"10.1.2.3", "10.1.2.3:443", "167838211", "0xa010203", "0XA010203", "012.1.2.3",
			"10.1.515", "10.66051", "[::ffff:10.1.2.3]", "[::ffff:a01:203]", "https://167838211/x", "10.1.2.3.",
		}},
		{"127.0.0.1", []string{"127.1", "2130706433", "0x7f000001", "0177.0.0.1", "[::ffff:127.0.0.1]"}},
		{"[::1]", []string{"[::1]", "[0:0:0:0:0:0:0:1]", "::1", "http://[0::1]:8080/"}},
		{"0.0.0.0", []string{"0", "0.0.0.0", "0x0"}},
	} {
		for _, in := range tc.members {
			if got, ok := Canonical(in); !ok || got != tc.want {
				t.Errorf("Canonical(%q) = %q, %v; want %q", in, got, ok, tc.want)
			}
		}
	}
	// Numeric-looking names that are not addresses stay names, as a client
	// resolves them, rather than becoming an address they are not.
	for _, in := range []string{"1.2.3.4.5", "256.1.1.1", "4294967296", "1.2.3.256", "08.1.2.3", "0x", "1e100.net", "0x1g"} {
		if got, ok := Canonical(in); !ok || got != strings.ToLower(in) {
			t.Errorf("Canonical(%q) = %q, %v; want it unchanged", in, got, ok)
		}
	}
	// A bracket holds an IPv6 address or nothing this function returns.
	for _, in := range []string{"[not-an-address]", "[10.1.2.3]", "[]", "a:b:c"} {
		if got, ok := Canonical(in); ok {
			t.Errorf("Canonical(%q) = %q, want refused", in, got)
		}
	}
}

// TestRubricLLD4_LoopbackIsParsed: whether a host is this machine is decided
// on the parsed address, not by membership in a list of spellings, so
// [0:0:0:0:0:0:0:1], 2130706433 and 127.0.0.2 are loopback as [::1] and
// 127.0.0.1 are, and a name under .localhost is (RFC 6761 6.3).
func TestRubricLLD4_LoopbackIsParsed(t *testing.T) {
	for _, in := range []string{
		"localhost", "LOCALHOST", "api.localhost", "127.0.0.1", "127.0.0.2", "127.255.255.254", "2130706433",
		"127.1", "[::1]", "[0:0:0:0:0:0:0:1]", "::1", "0.0.0.0", "0", "[::]", "[::ffff:127.0.0.1]", "localhost:3000",
	} {
		if !Loopback(in) {
			t.Errorf("Loopback(%q) = false", in)
		}
	}
	for _, in := range []string{
		"example.com", "localhost.example.com", "mylocalhost", "10.0.0.1", "128.0.0.1", "[::2]", "[2606:4700::1111]", "", "user@localhost",
	} {
		if Loopback(in) {
			t.Errorf("Loopback(%q) = true", in)
		}
	}
}
