//go:build !unix

package supervise

import (
	"slices"
	"testing"
)

// TestSTR4j_WithoutSupervisionAnEntryRunsDirect: where supervision is not
// supported, Run is the entry itself, with its own exit status, and nothing
// makes a process the supervised child: the token is dropped and the entry
// runs as if it had not been there.
func TestSTR4j_WithoutSupervisionAnEntryRunsDirect(t *testing.T) {
	if Supported {
		t.Fatal("Supported on a platform built without supervision")
	}
	called := false
	if got := Run(Config{Direct: func() int { called = true; return 0 }}); got != 0 || !called {
		t.Errorf("Run returned %d, Direct called %v; want the entry's own 0", got, called)
	}
	c, rest, ok := Attach([]string{Token, "hook", "--install", "x"})
	if ok || c != nil || !slices.Equal(rest, []string{"hook", "--install", "x"}) {
		t.Errorf("Attach = %v, %q, %v; want no child and the token dropped", c, rest, ok)
	}
	if c.StandDown() {
		t.Error("an unsupervised entry stood down")
	}
}
