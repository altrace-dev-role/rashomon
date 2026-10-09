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
