package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// T1C -- the T1c slice (test/live/autosteer/t1c) is frozen at its
// pre-registration: every file in it by SHA-256, the shared judge/common.py
// its judges import, and no file added or removed.
//
// T1c is slice T1 re-run under the six deltas its pins.txt names (the G arm on
// the 0e630250 engine freeze, the primary A1a all-four, the A3' to A5'
// secondaries, the cli fixture overlay, the pinned model, the money reads),
// registered before any T1c run. A run judged under a moved judge, pin or
// fixture no longer reads what was registered, so an edit after the first run
// opens a new slice, and an amendment before it changes this map in the same
// commit.
//
// Break: row.py's primary rule, a pin, a money read or the cli overlay moving
// after the pre-registration, or a file joining the slice unregistered.
func TestT1C_TheT1cSliceIsFrozen(t *testing.T) {
	frozen := map[string]string{
		"t1c/batch.sh":                                  "2d77165901305c1acdbb67a8505630afc58a1da31ffd5909f43fbf9f9886f438",
		"t1c/channel.py":                                "ab706c960a0b9a2d9c8ed9deeaf366526c08b8603262d7eb459c5ea34cc93c2e",
		"t1c/consumers.txt":                             "9f89eb6e727de983cff7a3dd8920ce0a181557c1e973370f346d87f8ee533c76",
		"t1c/graph.json":                                "f5681815397cf20e9cf297b584977aaed75b47d40d0f284df66743d2743eccba",
		"t1c/judge/a1.py":                               "6824b3bf806bd8e026e9d1c950773e740c85d3c4b4734419b0e757974bbc94d8",
		"t1c/judge/a2.sh":                               "ecbb74661135af3695f63c630b5d6e91d12355a66131e52fd623fff749470503",
		"t1c/judge/a3.sh":                               "9f02113d93c9b01fb75ad01e6b35488d62e39c8972256d2c5666a1936a57fb95",
		"t1c/judge/a3v2.py":                             "a7f283a3d0cd6a2dc1f2ce64c8e5804cd066fa3d9e2a6dc73d3807cf8a0c968a",
		"t1c/judge/a6.sh":                               "4058253b95adf7c17951fe7d6c2ce55f0760dd10ee0067162031415a7c9e263a",
		"t1c/judge/a7.sh":                               "6c8a8bfdb498726d9c0a944751c2718974cca2899bab1f1f042d98a85bf921e3",
		"t1c/judge/a8.py":                               "e53ca5a3680ada783e744e167f374d864d69a78052ecb4009265ef99e102e160",
		"t1c/judge/judge.sh":                            "87ec6abf5292c9f94efa1508440ee73cea34f9d1571fde10f7542b74f618338c",
		"t1c/judge/oracle.py":                           "f263d7526a6f0ff32d082280a36617c58abb994ec14307b6c0cb011fdda75e40",
		"t1c/judge/row.py":                              "6fc7954e16552300aa3572bfc7501766c16f04c8d92d93e836c8c59c65b2a1a0",
		"t1c/oracle-symbols.txt":                        "9bb974460b8ecabb10d69ec54accab9dd985ec3286cb0c607bf358d61c961082",
		"t1c/pins.txt":                                  "c6737ae66149f7882cb4810873dfd40902bb453993b8bdd5d20f7be323d4ba8d",
		"t1c/prompt.txt":                                "304d88a2755a93e02a85ca5595568be4d4bf70796e756edd1c0adfe2c1e79865",
		"t1c/smoke-prompt.txt":                          "80bab31400b11e5b7597cdbb2f0fa0417db14f0363c7bf83d928660552899960",
		"t1c/smoke.sh":                                  "5c98a0f3dc729ab8d74c39dc64942ac93df0dff7ea45034ebebdccdb2b61ce3c",
		"t1c/testdata/dispatch-cli/cli/cli.go":          "f2cbb5f5041d781ac6055ca0bab6665b82a91ef05dad62100d5199bac0d2043f",
		"t1c/testdata/dispatch-reflect/app/app.go":      "813e1a84a43fdad5c08fe695c87afc6402c2fdea29f24ef6584efff2afeca134",
		"t1c/testdata/dispatch-reflect/extra/byname.go": "889dc9466db17bd6ddd9f4a4f369f7300feaa0af1045a26af69028ca3f4a103f",
		"t1c/testdata/dispatch-reflect/extra/extra.go":  "2aff6ed0ba0d1a637584badef3c83814d1627df5e0058fdfd4bd8e79a339a51e",
		"t1c/testdata/dispatch-reflect/go.mod":          "b449d2fb7cbd5900837391a32e947237d920fd1f98e176773919c15a5df2969c",
		"t1c/testdata/dispatch-reflect/labels.tsv":      "caafe10c9084c2911af5973ecd1a082c6a4019c559bed5b3806bc438b8dd0cfc",
		"t1c/testdata/dispatch-reflect/shape/shape.go":  "e4a5a545682ade309f49b4854f3053cd4d01ba2f728e0466f58999da7a27ddae",
		"judge/common.py":                               "5e05aa65cd6501a127302e9eda661a5baaac3f0577a00404469f47b0b92f1554",
	}
	root := filepath.Join(autosteerDir(), "t1c")
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(autosteerDir(), p)
		if err != nil {
			return err
		}
		if _, ok := frozen[filepath.ToSlash(rel)]; !ok {
			t.Errorf("%s is in the T1c slice and not in its frozen set (a change opens a new slice)", filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for rel, want := range frozen {
		b, err := os.ReadFile(filepath.Join(autosteerDir(), filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			continue
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s is at %s; the T1c slice is frozen at %s (a change opens a new slice)", rel, got, want)
		}
	}
}
