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

// TestRES36l_CheckPlatformRefusesAForeignGOOSOrGOARCH: the benchmark rows
// are one platform's figures (long $TMPDIR paths on darwin skew B/op), so a
// run on any other GOOS/GOARCH, or a file naming none, is refused before a
// comparison, the same way CheckToolchain refuses a foreign toolchain.
func TestRES36l_CheckPlatformRefusesAForeignGOOSOrGOARCH(t *testing.T) {
	b := Budgets{BenchmarkPlatform: "darwin/arm64"}
	if err := b.CheckPlatform("darwin/arm64"); err != nil {
		t.Errorf("same platform refused: %v", err)
	}
	err := b.CheckPlatform("linux/amd64")
	if err == nil || !strings.Contains(err.Error(), "darwin/arm64") || !strings.Contains(err.Error(), "linux/amd64") {
		t.Errorf("mismatch: %v", err)
	}
	if err := (Budgets{}).CheckPlatform("darwin/arm64"); err == nil || !strings.Contains(err.Error(), "benchmark_platform") {
		t.Errorf("a file naming no platform: %v", err)
	}
}

// TestRES36m_DecodeRefusesAnUnknownField: a misspelled key (case_budget for
// case_budgets, say) is a defect in the file, not a silent zero. Decode must
// refuse it rather than leave the override it was meant to set unfilled.
func TestRES36m_DecodeRefusesAnUnknownField(t *testing.T) {
	if _, err := Decode([]byte(`{"budgets_version": 1, "go_version": "go1.27.2"}`)); err != nil {
		t.Errorf("a file with only known fields: %v", err)
	}
	if _, err := Decode([]byte(`{"budgets_version": 1, "go_vershun": "go1.27.2"}`)); err == nil {
		t.Error("a misspelled top-level key decoded without error")
	}
	if _, err := Decode([]byte(`{"platforms": {"darwin/arm64": {"case_budget": {}}}}`)); err == nil {
		t.Error("a misspelled nested key decoded without error")
	}
}
