package acceptance

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDEMO1_TranscriptMatchesGolden: scripts/demo1.sh, run against the suite's
// binary, prints testdata/demo1.golden byte for byte -- the four printed lines
// (turn 1's flag and recorded failure, turn 2's done flag carrying the red
// forward, turn 3's verified line), every silent beat, and the closing report
// under the masks its header names (card DEMO1 R3, D112). The golden is the
// website's copy: a renderer change moves it in the same commit, and the
// failure names the first line that moved. Regenerate with
// `bash scripts/demo1.sh <a rashomonfault build> > test/acceptance/testdata/demo1.golden`.
// The script seals its own environment (env -i), so the test sets only TMPDIR,
// to keep the scratch directory under the test's own.
func TestDEMO1_TranscriptMatchesGolden(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("demo1.sh is a bash script for macOS and Linux")
	}
	want, err := os.ReadFile(filepath.Join("testdata", "demo1.golden"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(moduleRoot, "scripts", "demo1.sh"), rashomonBin)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "TMPDIR="+t.TempDir())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	got, err := cmd.Output()
	if err != nil {
		t.Fatalf("demo1.sh: %v\nstderr:\n%s", err, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("demo1.sh wrote to stderr:\n%s", stderr.String())
	}
	if bytes.Equal(got, want) {
		return
	}
	g, w := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
	for i := range max(len(g), len(w)) {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl || i >= len(g) || i >= len(w) {
			t.Fatalf("line %d moved:\n got %q\nwant %q\nwhole output:\n%s", i+1, gl, wl, got)
		}
	}
}
