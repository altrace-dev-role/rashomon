package index

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
)

// wireRefusal is the refusal of Wire's Claim of rel under root, failing t
// unless the Claim is nil and a *lang.Refusal.
func wireRefusal(t *testing.T, root, rel string) *lang.Refusal {
	t.Helper()
	r, ok, err := Wire().Claim(root, rel)
	var rf *lang.Refusal
	if !ok || r != nil || !errors.As(err, &rf) {
		t.Fatalf("Claim of %s = %v, %v, %v; want nil and a *lang.Refusal", rel, r, ok, err)
	}
	return rf
}

// TestWire_ARefusalNamesItsFileAndReason: each refused configuration is a
// *lang.Refusal naming the file its step reads, a slash path under the
// root, and the reason from the closed set: a configuration that is a
// directory is not regular; a file past MaxConfigBytes is over the size
// limit; a file that does not parse (a stray word, a cut-off array) is not
// JSON; a file that parses with a value of another kind (units a string, a
// path a number) is of the wrong shape; a go.mod that is a directory is
// not regular, naming go.mod. Break: one reason for every refusal, or name
// the Python file for the Go step's refusal.
func TestWire_ARefusalNamesItsFileAndReason(t *testing.T) {
	gomodDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(gomodDir, gopg.ManifestFile), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := func(text string) string { return wireRoot(t, map[string]string{ConfigFile: text}) }
	for _, c := range []struct {
		name, root, rel, file string
		want                  lang.Reason
	}{
		{"a directory", wireRefused(t), "inv/core/calc.py", ConfigFile, lang.NotRegular},
		{"past the limit", cfg(`{"units":[]}` + strings.Repeat(" ", MaxConfigBytes)), "inv/core/calc.py", ConfigFile, lang.OverCap},
		{"a stray word", cfg("units: [inv]\n"), "inv/core/calc.py", ConfigFile, lang.NotJSON},
		{"cut off", cfg(`{"units":[`), "inv/core/calc.py", ConfigFile, lang.NotJSON},
		{"units a string", cfg(`{"units":"inv"}`), "inv/core/calc.py", ConfigFile, lang.WrongShape},
		{"a path a number", cfg(`{"units":[{"path":7,"lang":"py"}]}`), "inv/core/calc.py", ConfigFile, lang.WrongShape},
		{"go.mod a directory", gomodDir, "a/a.go", gopg.ManifestFile, lang.NotRegular},
	} {
		t.Run(c.name, func(t *testing.T) {
			if rf := wireRefusal(t, c.root, c.rel); rf.File != c.file || rf.Reason != c.want {
				t.Errorf("the refusal names %q, reason %d; want %q, reason %d", rf.File, rf.Reason, c.file, c.want)
			}
		})
	}
}

// TestWire_AConfigPastTheLimitNamesTheFileAndTheLimit: ReadConfig's error
// for a configuration past MaxConfigBytes still reads as the file's path
// and the limit in bytes, the words rashomon graph index prints, and it
// matches the sentinel the reason is read from. Break: drop the path or
// the bytes from the words, or build the error without the sentinel.
func TestWire_AConfigPastTheLimitNamesTheFileAndTheLimit(t *testing.T) {
	root := wireRoot(t, map[string]string{ConfigFile: `{"units":[]}` + strings.Repeat(" ", MaxConfigBytes)})
	_, err := ReadConfig(root)
	want := filepath.Join(root, filepath.FromSlash(ConfigFile)) + ": over 65536 bytes"
	if err == nil || err.Error() != want || !errors.Is(err, errConfigOverCap) {
		t.Errorf("ReadConfig past the limit = %v; want %q, the sentinel kept", err, want)
	}
}
