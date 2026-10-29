package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// shareHere is a store under RASHOMON_HOME holding five delivered lines,
// granted share.outcomes when grant is true, and an empty directory for the
// bundle (card 4.3).
func shareHere(t *testing.T, grant bool) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	t.Setenv("RASHOMON_HOME", root)
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	d := steer.DeliveryDelivered
	var batch []store.SteerLine
	for i := range 5 {
		batch = append(batch, &steer.Line{Type: steer.LineType, V: steer.LineVersion, ID: string(rune('a' + i)),
			AtUnixMS: int64(1000 + i), Pattern: string(pattern.Integrity), State: steer.LineStateDelivered, Delivery: &d})
	}
	if err := st.AppendSteer("run-a", func(io.ReaderAt) ([]store.SteerLine, error) { return batch, nil }); err != nil {
		t.Fatal(err)
	}
	if grant {
		if err := consent.Append(root, consent.Record{V: consent.RecordVersion, Via: consent.ViaConsentYes,
			Unit: "share.outcomes", UnitOn: true, At: 1}); err != nil {
			t.Fatal(err)
		}
	}
	return root, t.TempDir()
}

// failingWriter refuses every write, as a closed stdout does.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("stdout closed") }

// TestSH2_PreviewEqualsWritten: stdout is the bundle, then one line, the
// sha256 of those bytes; the file holds exactly the bundle, mode 0600, and
// nothing else is left in its directory. When the bundle cannot be printed,
// nothing is written.
func TestSH2_PreviewEqualsWritten(t *testing.T) {
	_, dir := shareHere(t, true)
	out := filepath.Join(dir, "outcomes.json")
	var stdout bytes.Buffer
	if err := cmdShare([]string{"build", "outcomes", "--out", out}, &stdout); err != nil {
		t.Fatalf("share build outcomes: %v", err)
	}
	printed := stdout.String()
	i := strings.LastIndex(strings.TrimSuffix(printed, "\n"), "\n")
	if i < 0 {
		t.Fatalf("stdout %q has no hash line after the bundle", printed)
	}
	bundle, hashLine := printed[:i+1], printed[i+1:]
	file, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(file)
	if bundle != string(file) || hashLine != "sha256 "+hex.EncodeToString(sum[:])+"\n" {
		t.Errorf("printed %q then %q; the file is %q with sha256 %x", bundle, hashLine, file, sum)
	}
	if !json.Valid(file) || !strings.Contains(string(file), "\n  \"schema\": \"outcomes/1\",\n") || !strings.Contains(string(file), `"fired": 5,`) ||
		!strings.Contains(string(file), `"fixed": "<5",`) {
		t.Errorf("the bundle is not the indented outcomes document: %s", file)
	}
	if info, err := os.Stat(out); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the bundle's mode is %v (%v), want 0600", info.Mode().Perm(), err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("the bundle's directory holds %d entries, want the bundle alone", len(entries))
	}

	unprinted := filepath.Join(dir, "unprinted.json")
	if err := cmdShare([]string{"build", "outcomes", "--out", unprinted}, failingWriter{}); err == nil {
		t.Error("share with stdout closed: no error")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("a bundle that could not be printed was written: %d entries", len(entries))
	}
}

// TestSH2c_RefusedWithoutConsentWritesNothing: with no share.outcomes grant,
// or with it turned off, share exits 1 naming the consent command, prints no
// bundle and writes no file.
func TestSH2c_RefusedWithoutConsentWritesNothing(t *testing.T) {
	for _, off := range []bool{false, true} {
		root, dir := shareHere(t, off)
		if off {
			if err := consent.Append(root, consent.Record{V: consent.RecordVersion, Via: consent.ViaConsentYes,
				Unit: "share.outcomes", At: 2}); err != nil {
				t.Fatal(err)
			}
		}
		out := filepath.Join(dir, "outcomes.json")
		var stdout, stderr bytes.Buffer
		code := run([]string{"share", "build", "outcomes", "--out", out}, nil, &stdout, &stderr)
		if code != exitFail || stdout.Len() != 0 || !strings.Contains(stderr.String(), "rashomon consent share.outcomes") {
			t.Errorf("off %v: exit %d, stdout %q, stderr %q; want exit 1, nothing printed, the consent command named", off, code, stdout.String(), stderr.String())
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Errorf("off %v: a refused share wrote %d entries", off, len(entries))
		}
	}
}

// TestSH2d_RefusedWithoutOut: share takes one verb and one unit and requires
// --out with a value; anything else, --help included, is refused as an
// argument it does not know, and nothing is printed or written.
func TestSH2d_RefusedWithoutOut(t *testing.T) {
	_, dir := shareHere(t, true)
	t.Chdir(dir)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"share", "build", "outcomes"}, "share expects build outcomes --out FILE"},
		{[]string{"share"}, "share expects build outcomes --out FILE"},
		{[]string{"share", "build", "outcomes", "--out"}, "--out needs a value"},
		{[]string{"share", "build", "lessons", "--out", "x"}, `unknown argument "lessons"`},
		{[]string{"share", "--help"}, `unknown argument "--help"`},
		{[]string{"share", "build", "outcomes", "--out", "x", "--force"}, `unknown argument "--force"`},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(c.args, nil, &stdout, &stderr); code != exitFail || stdout.Len() != 0 || !strings.Contains(stderr.String(), c.want) {
			t.Errorf("rashomon %s: exit %d, stdout %q, stderr %q; want exit 1 and %q", strings.Join(c.args, " "), code, stdout.String(), stderr.String(), c.want)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("a refused share wrote %d entries", len(entries))
	}
}
