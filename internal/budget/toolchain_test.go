package budget

import (
	"strings"
	"testing"
)

// TestRES36g_CheckToolchainNamesBothVersions: the rows are a toolchain's
// figures, so a run under any other toolchain, or a file naming none, is
// refused before a comparison, with both versions in the reason.
func TestRES36g_CheckToolchainNamesBothVersions(t *testing.T) {
	b := Budgets{GoVersion: "go1.27.2"}
	if err := b.CheckToolchain("go1.27.2"); err != nil {
		t.Errorf("same toolchain refused: %v", err)
	}
	err := b.CheckToolchain("go1.25.8")
	if err == nil || !strings.Contains(err.Error(), "go1.27.2") || !strings.Contains(err.Error(), "go1.25.8") {
		t.Errorf("mismatch: %v", err)
	}
	if err := b.CheckToolchain("go1.27.2 X:boringcrypto"); err == nil {
		t.Error("a toolchain with experiments passed as the measured one")
	}
	if err := (Budgets{}).CheckToolchain("go1.27.2"); err == nil || !strings.Contains(err.Error(), "go_version") {
		t.Errorf("a file naming no toolchain: %v", err)
	}
}
