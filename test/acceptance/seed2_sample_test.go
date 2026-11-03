package acceptance

// The canonical sample card (SEED1, README-LAUNCH1 and the clip): a set of
// synthetic transcripts under testdata/seed-sample, three made-up projects
// and ten sessions, none of them anyone's real data, whose `rashomon audit`
// output is card.golden byte for byte, every line under 80 columns
// (PRINT-WIDTH1). A change to the card's words or counting shows here as a
// diff the README and the clip must follow.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSEED2f_TheSampleCardIsTheGolden(t *testing.T) {
	e := newEnv(t)
	src := filepath.Join("testdata", "seed-sample", "projects")
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(e.configDir, "projects", rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		return os.WriteFile(dst, body, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "seed-sample", "card.golden"))
	if err != nil {
		t.Fatal(err)
	}
	res := e.run("", nil, "audit")
	if res.exitCode != 0 || res.stdout != string(want) {
		t.Errorf("audit over the sample: exit %d, stderr %q\n got:\n%s\nwant:\n%s", res.exitCode, res.stderr, res.stdout, want)
	}
	for i, l := range strings.Split(res.stdout, "\n") {
		if n := len([]rune(l)); n >= 80 {
			t.Errorf("sample card line %d is %d columns, not under 80: %q", i+1, n, l)
		}
	}
}
