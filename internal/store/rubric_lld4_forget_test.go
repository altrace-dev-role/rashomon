package store

import (
	"testing"
	"time"
)

// forgetFixture is a store holding one declaration that named recorded, in
// hosts or, when ssh is set, in ssh_hosts.
func forgetFixture(t *testing.T, recorded string, ssh bool) *Store {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := Declaration{
		Type: TypeDeclaration, SchemaVersion: SchemaVersion, SessionID: "s-lld4",
		ToolUseID: "toolu_lld4", ToolName: "Bash", RecordedAtMS: 1_000,
	}
	if ssh {
		d.SSHHosts = []string{recorded}
	} else {
		d.Hosts = []string{recorded}
	}
	if err := st.AppendDeclaration(d); err != nil {
		t.Fatal(err)
	}
	return st
}

// TestRubricLLD4_ForgetHostMatchesEverySpelling: a host is recorded in its
// one spelling, so forget --host must ask in that spelling too, whatever the
// user typed. Recording `curl https://167838211/x` stores 10.1.2.3, and a
// forget that compared the raw argument removed nothing while the command
// read as if it had worked: the host the user asked to forget stayed on
// disk. Records written before addresses had one spelling hold them as the
// call wrote them, so each recorded host is canonicalised before the
// comparison as well. The gap's digest is the canonical host's, which is the
// spelling the sandbox trail's events carry, so the report suppresses it.
func TestRubricLLD4_ForgetHostMatchesEverySpelling(t *testing.T) {
	for _, tc := range []struct {
		name, recorded, asked, canonical string
		ssh                              bool
	}{
		{"inet_aton decimal against today's record", "10.1.2.3", "167838211", "10.1.2.3", false},
		{"inet_aton hex", "10.1.2.3", "0xa010203", "10.1.2.3", false},
		{"inet_aton octal and short", "10.1.2.3", "012.1.515", "10.1.2.3", false},
		{"IPv4-mapped IPv6", "10.1.2.3", "[::ffff:10.1.2.3]", "10.1.2.3", false},
		{"expanded upper-case IPv6", "[2001:db8::1]", "[2001:DB8:0:0:0:0:0:1]", "[2001:db8::1]", false},
		{"a name in another case, with the root dot and a port", "pypi.org", "PyPI.org.:443", "pypi.org", false},
		{"a URL", "pypi.org", "https://pypi.org/simple", "pypi.org", false},
		{"an ssh host", "10.1.2.3", "167838211", "10.1.2.3", true},
		{"a record from before one spelling, asked in the new one", "167838211", "10.1.2.3", "10.1.2.3", false},
		{"a record from before one spelling, asked as written", "[2606:4700:0:0:0:0:0:1111]", "[2606:4700:0:0:0:0:0:1111]", "[2606:4700::1111]", false},
	} {
		st := forgetFixture(t, tc.recorded, tc.ssh)
		gaps, err := st.ForgetHost(tc.asked, time.UnixMilli(2_000))
		if err != nil {
			t.Fatalf("%s: forget --host %q: %v", tc.name, tc.asked, err)
		}
		if len(gaps) != 1 || gaps[0].RemovedRecords != 1 {
			t.Errorf("%s: forget --host %q after recording %q removed %v; want one record in one run",
				tc.name, tc.asked, tc.recorded, gaps)
			continue
		}
		forgotten, err := st.ForgottenHost()
		if err != nil {
			t.Fatal(err)
		}
		if !forgotten(tc.canonical) {
			t.Errorf("%s: the gap's digest is not %q's, so the sandbox trail keeps the host", tc.name, tc.canonical)
		}
	}
}

// TestRubricLLD4_ForgetHostRefusesWhatIsNotAHost: an argument host.Canonical
// refuses names no host, so no record can be matched against it. Removing
// nothing and reporting success would read as a forget that worked; it is an
// error instead, and nothing is removed.
func TestRubricLLD4_ForgetHostRefusesWhatIsNotAHost(t *testing.T) {
	for _, bad := range []string{"deploy:secret@pypi.org", "[pypi.org]", "pypi:org:x", "py pi.org", " ", "."} {
		st := forgetFixture(t, "pypi.org", false)
		if gaps, err := st.ForgetHost(bad, time.UnixMilli(2_000)); err == nil {
			t.Errorf("forget --host %q was accepted (%d runs): it names no host", bad, len(gaps))
		}
		gaps, err := st.ForgetHost("pypi.org", time.UnixMilli(3_000))
		if err != nil || len(gaps) != 1 {
			t.Errorf("after refusing %q, forget --host pypi.org removed %v (%v); want the record still there", bad, gaps, err)
		}
	}
}
