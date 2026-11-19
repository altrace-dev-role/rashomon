package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSafeLink makes link a symlink to target, or skips the test: Windows
// creates one only with the privilege or developer mode.
func writeSafeLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symlink here: %v", err)
	}
}

// writeSafeLinkedSettings writes data as a file in a directory of its own and
// returns the path of a settings.json, in another directory, linked to it:
// the arrangement Write already serves (writeTarget).
func writeSafeLinkedSettings(t *testing.T, data []byte) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "settings.json")
	writeSafeLink(t, target, link)
	return link
}

// writeSafeJSONOf is a settings object of exactly n bytes, n at least 11.
func writeSafeJSONOf(n int) []byte {
	return []byte(`{"pad":"` + strings.Repeat("x", n-11) + "\"}\n")
}

// TestWS1_J4_LoadReadsASettingsFileLinkedOutsideItsFolder: a settings.json
// that is a link to a file in another directory still reads, as Write
// writes it: the read follows links on purpose. Break: refuse a link that
// leaves the settings folder.
func TestWS1_J4_LoadReadsASettingsFileLinkedOutsideItsFolder(t *testing.T) {
	p := writeSafeLinkedSettings(t, []byte(`{"hooks": {}}`+"\n"))
	d, err := Load(p)
	if err != nil {
		t.Fatalf("Load of a settings file linked outside its folder: %v", err)
	}
	if _, ok := d.Get("hooks"); !ok {
		t.Errorf("Load through the link read no hooks member")
	}
}

// TestWS1_J5_LoadRefusesATargetOverTheBound: a link target one byte over
// the bound is refused with one line that names the path and the class and
// no byte of the file; its control, exactly at the bound, reads. Break: read
// a settings file whole, whatever its size.
func TestWS1_J5_LoadRefusesATargetOverTheBound(t *testing.T) {
	p := writeSafeLinkedSettings(t, writeSafeJSONOf(maxFileBytes+1))
	_, err := Load(p)
	want := p + ": over the 65536-byte bound"
	if err == nil || err.Error() != want {
		t.Errorf("Load of a target one byte over the bound = %v; want %q", err, want)
	}
	at := writeSafeLinkedSettings(t, writeSafeJSONOf(maxFileBytes))
	if _, err := Load(at); err != nil {
		t.Errorf("control, a target exactly at the bound: Load = %v; want it read", err)
	}
}

// TestWS1_J6_LoadNamesNoByteOfATargetThatIsNotJSON: a link target that is not
// JSON is refused with the path, the position and the class, and no byte of
// the file: the scanner's own text quotes the byte it stopped at. Break:
// return the scanner's text.
func TestWS1_J6_LoadNamesNoByteOfATargetThatIsNotJSON(t *testing.T) {
	p := writeSafeLinkedSettings(t, []byte("Qz7 not a settings file\n"))
	_, err := Load(p)
	want := p + ":1:1: not JSON"
	if err == nil || err.Error() != want {
		t.Errorf("Load of a target that is not JSON = %v; want %q", err, want)
	}
}
