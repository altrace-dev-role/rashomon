package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestForgetHost_AHostNoRecordNamedIsStillForgotten is #45 at the store: a
// forget that removed nothing must still make the predicate answer yes, because
// the report reads sources this store never held -- the sandbox's trail and the
// proxy's store. Before the store-level record, ForgetHost wrote nothing when
// it removed nothing, and the host was listed again in the next report.
func TestForgetHost_AHostNoRecordNamedIsStillForgotten(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gaps, err := st.ForgetHost("github.com", time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatalf("ForgetHost: %v", err)
	}
	if len(gaps) != 0 {
		t.Fatalf("premise: nothing named the host, so no gap is due; got %d", len(gaps))
	}
	forgotten, err := st.ForgottenHost()
	if err != nil {
		t.Fatal(err)
	}
	if !forgotten("github.com") {
		t.Error("a host only another source held is not forgotten: the next report lists it again (#45)")
	}
	if forgotten("pypi.org") {
		t.Error("the forget covers a host nobody asked to forget")
	}
}

// TestForgetHost_TheStoreLevelRecordHoldsNoName: the record carries the keyed
// digest and never the hostname, which is the whole point of forgetting it.
func TestForgetHost_TheStoreLevelRecordHoldsNoName(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ForgetHost("acme-secret.internal", time.Unix(1_700_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(st.Root(), FileForgottenHosts))
	if err != nil {
		t.Fatalf("premise: no store-level record was written: %v", err)
	}
	if bytes.Contains(raw, []byte("acme-secret")) {
		t.Errorf("the store-level forget record holds the hostname:\n%s", raw)
	}
	if !bytes.Contains(raw, []byte(st.HostDigest("acme-secret.internal"))) {
		t.Errorf("the store-level forget record does not hold the host's digest:\n%s", raw)
	}
}

// TestForgetHost_AnySpellingOfTheHostIsForgotten: every host the report holds
// has been through host.Canonical, and so has every host a declaration names.
// A forget that hashed the argument as typed matched none of them for any
// other spelling -- `forget --host GitHub.com` removed nothing, suppressed
// nothing, and the CLI said later reports would leave the host out.
func TestForgetHost_AnySpellingOfTheHostIsForgotten(t *testing.T) {
	for _, spelled := range []string{"PyPI.org", "pypi.org.", "https://pypi.org/", "pypi.org:443", " pypi.org "} {
		t.Run(spelled, func(t *testing.T) {
			st, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := st.AppendDeclaration(Declaration{
				Type: TypeDeclaration, SchemaVersion: SchemaVersion, SessionID: "s1",
				ToolUseID: "toolu_pypi", ToolName: "Bash", RecordedAtMS: 1_700_000_000_000,
				Hosts: []string{"pypi.org"},
			}); err != nil {
				t.Fatal(err)
			}
			gaps, err := st.ForgetHost(spelled, time.Unix(1_700_000_100, 0))
			if err != nil {
				t.Fatalf("ForgetHost(%q): %v", spelled, err)
			}
			if len(gaps) != 1 || gaps[0].RemovedRecords != 1 {
				t.Errorf("ForgetHost(%q) left the declaration naming pypi.org: gaps %+v", spelled, gaps)
			}
			forgotten, err := st.ForgottenHost()
			if err != nil {
				t.Fatal(err)
			}
			if !forgotten("pypi.org") {
				t.Errorf("ForgetHost(%q) does not suppress pypi.org, the spelling every report holds", spelled)
			}
		})
	}
}

// TestForgetHost_WhatIsNotAHostnameIsRefused: a value host.Canonical rejects
// -- a path, a credential, whitespace inside -- names no destination, so a
// forget of it can only be a no-op that reads as a success. Refused, and
// nothing is written.
func TestForgetHost_WhatIsNotAHostnameIsRefused(t *testing.T) {
	for _, bad := range []string{"pypi.org/simple", "user:pw@pypi.org", "py pi.org", "   "} {
		t.Run(bad, func(t *testing.T) {
			st, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := st.ForgetHost(bad, time.Unix(1_700_000_000, 0)); err == nil {
				t.Errorf("ForgetHost(%q) succeeded on a value that is not a hostname", bad)
			}
			if _, err := os.Stat(filepath.Join(st.Root(), FileForgottenHosts)); err == nil {
				t.Errorf("ForgetHost(%q) wrote a store-level record for a value that names no host", bad)
			}
		})
	}
}

// TestForgetHost_ASecondForgetOfTheSameHostAddsNoLine: the file is outside
// every size cap, so a forget that says nothing new must not grow it. A
// different host still gets its own record.
func TestForgetHost_ASecondForgetOfTheSameHostAddsNoLine(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	lines := func() int {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(st.Root(), FileForgottenHosts))
		if err != nil {
			t.Fatal(err)
		}
		return bytes.Count(raw, []byte("\n"))
	}
	for i := 0; i < 3; i++ {
		if _, err := st.ForgetHost("acme-secret.internal", now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if got := lines(); got != 1 {
		t.Errorf("three forgets of one host left %d records, want 1", got)
	}
	if _, err := st.ForgetHost("github.com", now); err != nil {
		t.Fatal(err)
	}
	if got := lines(); got != 2 {
		t.Errorf("a forget of a second host left %d records, want 2", got)
	}
	forgotten, err := st.ForgottenHost()
	if err != nil {
		t.Fatal(err)
	}
	if !forgotten("acme-secret.internal") || !forgotten("github.com") {
		t.Error("a host forgotten more than once, or after another, is no longer forgotten")
	}
}
