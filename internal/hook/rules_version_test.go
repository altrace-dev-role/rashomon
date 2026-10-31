package hook

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
)

// locationPins is, per shape.RulesVersion, the sha256 of the cwd digests the
// hook records for the pairs of locationCorpus (locationDigest). cwd_digest is
// a recorded field decided by a rule -- which leading cd steps are read
// (shape's leadingCd), how they fold onto the payload's cwd (startDirectory),
// and the location of a fold that is not read (shape.UnreadCWDDigest) -- so a
// change to the rule needs a bump as a table change does. A bump adds a pin; a
// pin is never edited, and the corpus grows only with a bump. Version 4's pin
// was computed with version 4's handle.go and shape package, startDirectory
// over DeriveWithDirs's directories, when this test was added.
var locationPins = map[int]string{
	4:  "c0f954e27713c24c174615b33ecffbd571aa3fcc7c61a4594d7682b5670e3740",
	5:  "e22b69da181e207739b0bbb3615684f1e2a02d397d9ce71c37584277dc252c50", // `~` read with the hook's home; an unread fold's location in its own domain, keyed on the payload's cwd and the fold (2026-10-24)
	6:  "e22b69da181e207739b0bbb3615684f1e2a02d397d9ce71c37584277dc252c50", // RulesVersion 6 changed checkOnly, make's -j and the flag-table pins; the cwd rule is unchanged
	7:  "e22b69da181e207739b0bbb3615684f1e2a02d397d9ce71c37584277dc252c50", // RulesVersion 7 changed when a runner's line is whole and perl's -0 cluster; the cwd rule is unchanged
	8:  "e3a671f14c9c1922425e28caa9348428c3e1c9fd51480f579fa5a5ae3e58650f", // `pushd DIR &&` folds as `cd DIR &&` does: `pushd /b && go test ./...` and `cd /a && pushd /b && go test ./...` record /b (2026-10-28)
	9:  "b67bdc7531ebeb98a863c1cdada63a92a2b4eac03964a5bc0754100190807057", // a comment ending in a backslash continues nothing: `cd a && # c\` NL `cd b && go test ./...` records a/b, and `# c\` NL `cd a && go test ./...` records a (2026-10-28)
	10: "b67bdc7531ebeb98a863c1cdada63a92a2b4eac03964a5bc0754100190807057", // RulesVersion 10 named a Bash line's write targets; the cwd rule is unchanged
}

// locationCorpus is the (line, payload cwd) pairs locationPins digests, a
// frozen copy: every fold the program search passes and leadingCd does not
// read, the directory options after a runner's words, the named home, the
// fold's bound read and unread, and the pushd lines, from two payload cwds and
// from none.
var locationCorpus = [][2]string{
	{"cd $REPO && go test ./...", "/tmp/project"}, {"cd /tmp; go test ./...", "/tmp/project"},
	{"cd - && go test ./...", "/tmp/project"}, {`cd "/tmp" && go test ./...`, "/tmp/project"},
	{"cd ~/x && go test ./...", "/tmp/project"}, {"cd -P /tmp && go test ./...", "/tmp/project"},
	{"X=1 cd /tmp && go test ./...", "/tmp/project"}, {"cd /a && cd $B && go test ./...", "/tmp/project"},
	{"(cd /tmp && go test ./...)", "/tmp/project"}, {"{ cd /tmp && go test ./...; }", "/tmp/project"},
	{"(cd /tmp && go test ./...) | tail", "/tmp/project"},

	{"make test -C sub", "/tmp/project"}, {"cargo test --manifest-path sub/Cargo.toml", "/tmp/project"},
	{"go test -C sub ./...", "/tmp/project"}, {"npm test --prefix web", "/tmp/project"},

	{"cd /tmp && go test ./...", "/tmp/project"}, {"cd ~/x && go test ./... 2>&1 | tail -20", "/tmp/project"},
	{`cd "$REPO" && go test ./...`, "/tmp/project"}, {"cd ~ && go test ./...", "/tmp/project"},
	{"cd ~user/x && go test ./...", "/tmp/project"}, {"cd ~+/x && go test ./...", "/tmp/project"},
	{"cd ~-/x && go test ./...", "/tmp/project"}, {"cd && go test ./...", "/tmp/project"},
	{"cd sub && cd - && go test ./...", "/tmp/project"}, {"cd /x && cd - && go test ./...", "/tmp/project"},
	{"cd /a && cd ~ && go test ./...", "/tmp/project"}, {"cd ~/x && cd sub && go test ./...", "/tmp/project"},
	{"cd /a && (cd b && go test ./...)", "/tmp/project"}, {"(cd /x); go test ./...", "/tmp/project"},
	{"cd sub && go test ./...", "/tmp/project"}, {"cd .. && go test ./...", "/tmp/project"},
	{"go test ./...", "/tmp/project"}, {"cd /tmp && ls", "/tmp/project"}, {"cd ~/x && ls", "/tmp/project"},
	{"cd /repo\r && ls", "/tmp/project"},
	{strings.Repeat("cd a && ", 64) + "go test ./...", "/tmp/project"},
	{strings.Repeat("cd a && ", 65) + "go test ./...", "/tmp/project"},
	{strings.Repeat("cd $D && ", 65) + "go test ./...", "/tmp/project"},
	{"pushd /b && go test ./...", "/tmp/project"}, {"cd /a && pushd /b && go test ./...", "/tmp/project"},
	{"cd a && # c\\\ncd b && go test ./...", "/tmp/project"}, {"# c\\\ncd a && go test ./...", "/tmp/project"},
	{"cd a && #\\\n cd b && go test ./...", "/tmp/project"},

	{"cd $REPO && go test ./...", "/srv/b"}, {"cd /tmp; go test ./...", "/srv/b"},
	{"cd sub && go test ./...", ""}, {"go test ./...", ""},
}

// locationDigest digests, for each pair of locationCorpus in order, the line,
// the payload cwd and the cwd digest Locate gives them under the key "key" (a
// NUL byte for ""), with the home pinned, since `~` expands with the hook's
// own home.
func locationDigest(t *testing.T) string {
	t.Setenv("HOME", "/home/pin")
	// The pin is the fold's rules, one value on every host. What a corpus
	// path resolves to is the host's (macOS's /tmp is /private/tmp), and the
	// resolving is held by TestCS1_LocateKeysTheDirectoryAsItIsOnDisk.
	old := evalSymlinks
	evalSymlinks = func(p string) (string, error) { return p, nil }
	t.Cleanup(func() { evalSymlinks = old })
	key := []byte("key")
	h := sha256.New()
	for _, c := range locationCorpus {
		in, err := json.Marshal(map[string]string{"command": c[0]})
		if err != nil {
			t.Fatal(err)
		}
		_, d := Locate(key, "Bash", in, c[1])
		if d == "" {
			d = "\x00"
		}
		h.Write([]byte(c[0] + "\x1f" + c[1] + "\x1f" + d + "\x1e"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TestRulesVersion_Locations: a change in which leading cd steps are read,
// how they fold, or where an unread fold is recorded, that moves a cwd digest
// on a corpus pair, cannot ship without a shape.RulesVersion bump, or records
// written before and after it carry the same rules_version and read as keyed
// by the same rule. Break: read `cd DIR;`, or join `~` under the cwd, and
// leave the version as it was.
func TestRulesVersion_Locations(t *testing.T) {
	got := locationDigest(t)
	want, ok := locationPins[shape.RulesVersion]
	if !ok || want != got {
		t.Fatalf("the location corpus digests to %s, and RulesVersion %d pins %q: the location rule changed without a bump. "+
			"Bump shape.RulesVersion and add its pin here (and the shape package's runner-list, write-table and matcher pins, and report's pass-list pin).", got, shape.RulesVersion, want)
	}
}
