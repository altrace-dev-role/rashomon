package recap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stateOfSize writes a recap.json that parses and is exactly n bytes long.
func stateOfSize(t *testing.T, path string, n int) {
	t.Helper()
	head, tail := `{"spoken":{"s":"`, `"}}`
	b := head + strings.Repeat("a", n-len(head)-len(tail)) + tail
	if err := os.WriteFile(path, []byte(b), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A recap.json one byte over its cap is an error, not a read of it whole.
func TestSRR1_AnOverCapStateFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), StateFileName)
	stateOfSize(t, path, maxStateBytes+1)
	if _, err := readState(path); err == nil {
		t.Error("readState read a state over its cap; want an error")
	}
}
