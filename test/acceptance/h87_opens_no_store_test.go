package acceptance

import (
	"os"
	"path/filepath"
	"testing"
)

// H-87 -- the digest opens no store.
//
// Run against a machine with none; assert none is created. Break: use
// store.Open and asking for a digest installs an identity.
func TestH87_TheDigestOpensNoStore(t *testing.T) {
	e := newEnv(t)
	// No watch, no hook, no probe: this machine has never recorded anything.

	entriesBefore, err := os.ReadDir(e.home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entriesBefore) != 0 {
		t.Fatalf("premise: the store root should be empty before this test, got %v", entriesBefore)
	}

	d := e.digest("--session", testSession, "--prompt", "prompt-1")
	if !d.Unknown {
		t.Errorf("unknown = false on a machine with no store at all")
	}

	if _, err := os.Stat(filepath.Join(e.home, "install.json")); err == nil {
		t.Fatal("install.json exists after `rashomon digest` on a machine with no store -- " +
			"asking for a digest minted an install identity")
	}
	entriesAfter, err := os.ReadDir(e.home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entriesAfter) != 0 {
		t.Errorf("the store root is no longer empty after `rashomon digest`: %v", entriesAfter)
	}
}
