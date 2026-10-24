package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/share"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

type fakeOutcomes struct{ o share.Outcomes }

func (f fakeOutcomes) Outcomes() (share.Outcomes, error) { return f.o, nil }

var sampleOutcomes = fakeOutcomes{share.Outcomes{
	InstallDigest: strings.Repeat("ab", 32),
	Period:        share.Period{Start: "2026-10-01", End: "2026-10-24"},
	Tallies:       []share.Tally{{Pattern: pattern.RegressionLocation, Fired: 12, Fixed: 7, Ignored: 3, Overridden: 2, Labelled: 6, True: 5}},
}}

// consentedRoot is a store root whose consent.json holds an install consent
// and, when units are named, a grant of each.
func consentedRoot(t *testing.T, units ...consent.Unit) string {
	t.Helper()
	root := t.TempDir()
	rec := consent.Record{
		V: consent.RecordVersion, ScopeVersion: 3, RecordScope: consent.RecordScopeV5, Steer: consent.SteerOn,
		TextSHA256: strings.Repeat("a", 64), BinaryVersion: "test", BinarySHA256: strings.Repeat("b", 64),
		Via: consent.ViaWatchTTY, TTY: true, InvocationID: strings.Repeat("c", 32), At: 1760000000000,
	}
	if err := consent.Append(root, rec); err != nil {
		t.Fatal(err)
	}
	for _, u := range units {
		g := rec
		g.Via, g.Unit, g.ScopeVersion, g.RecordScope = consent.ViaUnitTTY, u, 0, ""
		if err := consent.Append(root, g); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestSH2_PreviewEqualsWritten: the bytes the command printed before the hash
// are the bytes in the file, the hash line is the SHA-256 of those bytes, and
// the file is private. A bundle with different data prints and writes
// different bytes, so the equality is not two copies of one constant.
func TestSH2_PreviewEqualsWritten(t *testing.T) {
	root := consentedRoot(t, consent.UnitShareOutcomes)
	var shown [2][]byte
	for i, rd := range []fakeOutcomes{sampleOutcomes, {share.Outcomes{
		InstallDigest: strings.Repeat("cd", 32), Period: share.Period{Start: "2026-09-01", End: "2026-09-30"},
		Tallies: []share.Tally{{Pattern: pattern.CredSpray, Fired: 40, Labelled: 10, True: 9}},
	}}} {
		file := filepath.Join(t.TempDir(), "outcomes.json")
		var out bytes.Buffer
		if err := shareBuild(root, rd, []string{"build", "outcomes", "--out", file}, &out); err != nil {
			t.Fatalf("shareBuild: %v", err)
		}
		written, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(written)
		want := string(written) + "sha256: " + hex.EncodeToString(sum[:]) + "\n"
		if !strings.HasPrefix(out.String(), want) {
			t.Errorf("stdout does not open with the file's bytes and their hash:\nstdout %q\nfile   %q", out.String(), written)
		}
		if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("the file's mode = %v, %v; want 0600", info.Mode(), err)
		}
		shown[i] = written
	}
	if bytes.Equal(shown[0], shown[1]) {
		t.Error("two different outcomes wrote the same file")
	}

	// --out=<file> is the same flag.
	file := filepath.Join(t.TempDir(), "eq.json")
	if err := shareBuild(root, sampleOutcomes, []string{"build", "outcomes", "--out=" + file}, &bytes.Buffer{}); err != nil {
		t.Fatalf("--out=: %v", err)
	}
	if b, _ := os.ReadFile(file); !bytes.Equal(b, shown[0]) {
		t.Error("--out=<file> wrote different bytes from --out <file>")
	}
}

// TestSH2_RefusalsWriteAndShowNothing: with the unit off, with a file already
// there, with a path that cannot be created, with a reader that has nothing
// and with a command line that is not the one form, the command errors,
// prints no bundle and leaves no file; an existing file keeps its bytes.
func TestSH2_RefusalsWriteAndShowNothing(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "o.json")
	build := []string{"build", "outcomes", "--out", file}
	on := consentedRoot(t, consent.UnitShareOutcomes)

	for name, tc := range map[string]struct {
		root string
		rd   share.Reader
		args []string
		want string
	}{
		"unit never asked":         {consentedRoot(t), sampleOutcomes, build, "rashomon consent share.outcomes"},
		"another unit asked":       {consentedRoot(t, consent.UnitShareLessons), sampleOutcomes, build, "share.outcomes is off"},
		"no consent file":          {t.TempDir(), sampleOutcomes, build, "share.outcomes is off"},
		"no reader yet":            {on, noOutcomes{}, build, "does not record outcomes yet"},
		"a reader that errs":       {on, errReader{}, build, "store unreadable"},
		"a path that cannot exist": {on, sampleOutcomes, []string{"build", "outcomes", "--out", filepath.Join(dir, "no", "such", "o.json")}, "nothing was written"},
		"no subcommand":            {on, sampleOutcomes, nil, "usage: rashomon share"},
		"another bundle":           {on, sampleOutcomes, []string{"build", "lessons", "--out", file}, "usage: rashomon share"},
		"no --out":                 {on, sampleOutcomes, []string{"build", "outcomes"}, "usage: rashomon share"},
		"an empty --out":           {on, sampleOutcomes, []string{"build", "outcomes", "--out="}, "usage: rashomon share"},
		"--out with no file":       {on, sampleOutcomes, []string{"build", "outcomes", "--out"}, "usage: rashomon share"},
		"an extra argument":        {on, sampleOutcomes, append(append([]string{}, build...), "--yes"), "usage: rashomon share"},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			err := shareBuild(tc.root, tc.rd, tc.args, &out)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want one containing %q", err, tc.want)
			}
			if out.Len() != 0 {
				t.Errorf("a refusal printed %q", out.String())
			}
			if _, err := os.Stat(file); err == nil {
				t.Error("a refusal left the file")
			}
		})
	}

	// A file that is already there is refused and keeps its bytes.
	if err := os.WriteFile(file, []byte("mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := shareBuild(on, sampleOutcomes, build, &out); err == nil || out.Len() != 0 {
		t.Errorf("an existing file: err %v, stdout %q; want a refusal that printed nothing", err, out.String())
	}
	if b, _ := os.ReadFile(file); string(b) != "mine\n" {
		t.Errorf("the existing file now holds %q", b)
	}
}

type errReader struct{}

func (errReader) Outcomes() (share.Outcomes, error) {
	return share.Outcomes{}, errors.New("store unreadable")
}

// TestSH2_CmdShareReadsTheStoreRoot: cmdShare, the command's own entry,
// checks consent in RASHOMON_HOME's store, and with the unit on reports that
// this release records no outcomes, writing nothing.
func TestSH2_CmdShareReadsTheStoreRoot(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "o.json")
	args := []string{"build", "outcomes", "--out", file}

	t.Setenv("RASHOMON_HOME", consentedRoot(t))
	if err := cmdShare(args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "share.outcomes is off") {
		t.Errorf("cmdShare with the unit off: %v", err)
	}
	t.Setenv("RASHOMON_HOME", consentedRoot(t, consent.UnitShareOutcomes))
	var out bytes.Buffer
	if err := cmdShare(args, &out); !errors.Is(err, errNoOutcomes) {
		t.Errorf("cmdShare with the unit on: %v, want errNoOutcomes", err)
	}
	if _, err := os.Stat(file); err == nil || out.Len() != 0 {
		t.Errorf("cmdShare wrote %q or left the file", out.String())
	}
}
