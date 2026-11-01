package acceptance

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ra1Pieces are the script's own text for has, attest and the verified-line
// pattern, cut out so the test runs what test/live/ra-1.sh runs, not a copy.
func ra1Pieces(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(moduleRoot, "test", "live", "ra-1.sh"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	inAttest := false
	for _, ln := range strings.Split(string(b), "\n") {
		switch {
		case strings.HasPrefix(ln, "has() {"), strings.HasPrefix(strings.TrimSpace(ln), "vl_re="):
			out = append(out, ln)
		case strings.HasPrefix(ln, "attest() {"):
			inAttest = true
			out = append(out, ln)
		case inAttest:
			out = append(out, ln)
			inAttest = ln != "}"
		}
	}
	if len(out) < 4 {
		t.Fatalf("ra-1.sh no longer holds has, attest and vl_re (read %q)", out)
	}
	return strings.Join(out, "\n")
}

// TestRA1_AttestAcceptsBothVerifiedForms: the A4 attestation of the verified
// line takes the release RC's "go passed" (main plus #147), the merged
// tree's "go test passed" (#149 names the runner) and a trusted spec's gate
// by its id, 'unit' (#162, which A4's spec declares), and no other gate, edit
// or a turn with a rule finding. Break: the script's fixed string for the older form only.
func TestRA1_AttestAcceptsBothVerifiedForms(t *testing.T) {
	pieces := ra1Pieces(t)
	for _, tc := range []struct {
		name, screen string
		want         string
	}{
		{"release form", "x\nrashomon: verified - go passed after the last edit.\n", "yes"},
		{"merged form", "x\nrashomon: verified - go test passed after the last edit.\n", "yes"},
		{"another gate", "x\nrashomon: verified - go build passed after the last edit.\n", "no"},
		{"declared gate", "x\n\u203b rashomon: verified - 'unit' passed after the last edit.\n", "yes"},
		{"declared gate beside a learned one", "x\nrashomon: verified - 'unit' and go passed after the last edit.\n", "yes"},
		{"declared gate with a rule finding", "x\nrashomon: 1 file edited; 'unit' passed after the last edit; 1 record finding(s) in this turn.\n", "no"},
		{"no line", "x\nrashomon: done was claimed; nothing ran.\n", "no"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			screen := filepath.Join(t.TempDir(), "screen.txt")
			if err := os.WriteFile(screen, []byte(tc.screen), 0o600); err != nil {
				t.Fatal(err)
			}
			script := pieces + "\nHEADLESS=1\nattest \"$1\" \"rashomon: verified - go test passed after the last edit.\" \"$vl_re\"\necho \"$SEEN\"\n"
			out, err := exec.Command("sh", "-c", script, "sh", screen).CombinedOutput()
			if err != nil {
				t.Fatalf("%v: %s", err, out)
			}
			if got := strings.TrimSpace(string(out)); !strings.HasPrefix(got, tc.want) {
				t.Errorf("attest on %q = %q, want %q", tc.screen, got, tc.want)
			}
		})
	}
}
