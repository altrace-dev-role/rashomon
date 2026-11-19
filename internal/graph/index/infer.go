package index

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// Inferred is the configuration Infer made for a root with none: the
// configuration, the bytes of its file, each unit with its language and
// the manifest and rule it came from, and each path left out with why.
type Inferred struct {
	Config Config
	Bytes  []byte
	Units  []InferredUnit
	Left   []lang.Left
}

// InferredUnit is one inferred unit and its language.
type InferredUnit struct {
	Lang string
	lang.Unit
}

// Infer is the configuration of root inferred by each language Wire
// registers that is a lang.Inferrer, in Wire's order, over one walk of root
// that skips what Build's walk skips and every directory below root that
// holds a configuration of its own (another index's root, left out and
// named). Its file is encoded as the configuration file is read, one unit
// to a line. No unit is ErrNoConfig, the error naming the manifests looked
// for; a file over MaxConfigBytes is an error naming the cap.
func Infer(root string) (Inferred, error) {
	files, left, err := inferWalk(root)
	inf := Inferred{Left: left}
	if err != nil {
		return inf, err
	}
	var manifests []string
	for _, i := range Wire().Inferrers() {
		l := i.(lang.Resolver).Lang()
		manifests = append(manifests, i.Manifests()...)
		got := i.Infer(files)
		for _, u := range got.Units {
			if !nameable(u) {
				inf.Left = append(inf.Left, lang.Left{Path: u.Path, Why: "a name that is not UTF-8, which the file cannot hold"})
				continue
			}
			inf.Units = append(inf.Units, InferredUnit{Lang: l, Unit: u})
			inf.Config.Units = append(inf.Config.Units, Unit{Path: u.Path, ModRoot: u.ModRoot, Modules: u.Modules, Lang: l})
		}
		inf.Left = append(inf.Left, got.Left...)
	}
	slices.SortFunc(inf.Left, func(a, b lang.Left) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), strings.Compare(a.Why, b.Why))
	})
	if len(inf.Units) == 0 {
		return inf, fmt.Errorf("%w: no unit inferred from %s", ErrNoConfig, strings.Join(manifests, ", "))
	}
	b := encodeConfig(inf.Config.Units)
	if len(b) > MaxConfigBytes {
		return inf, fmt.Errorf("%s: inferred %w", ConfigFile, &regfile.OverCapError{Size: int64(len(b)), Max: MaxConfigBytes})
	}
	inf.Config.ServiceMap = map[string]string{}
	inf.Config.Knowledge = []Knowledge{}
	inf.Bytes = b
	return inf, nil
}

// inferWalk is every regular file under root, as sorted slash paths
// relative to it: a link is listed as no file, so no rule takes a manifest
// that is a link. It skips what Build's walk skips, and each directory below
// root holding a configuration of its own, which it names as left out.
func inferWalk(root string) ([]string, []lang.Left, error) {
	var files []string
	var left []lang.Left
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		switch {
		case !d.IsDir():
			if d.Type().IsRegular() {
				files = append(files, rel)
			}
		case rel == ".":
		case skipDir(d.Name()):
			return filepath.SkipDir
		default:
			if _, err := os.Lstat(filepath.Join(p, filepath.FromSlash(ConfigFile))); err == nil {
				left = append(left, lang.Left{Path: rel, Why: "holds its own " + ConfigFile + ", another index's root"})
				return filepath.SkipDir
			}
		}
		return nil
	})
	slices.Sort(files)
	return files, left, err
}

// nameable reports whether each name of u is UTF-8, so the file, JSON,
// names it as it is on disk.
func nameable(u lang.Unit) bool {
	return utf8.ValidString(u.Path) && utf8.ValidString(u.ModRoot) && !slices.ContainsFunc(u.Modules, func(m string) bool { return !utf8.ValidString(m) })
}

// encodeConfig is the file of a configuration of units and nothing else, in
// the shape a person writes it: one unit to a line, then an empty service
// map and knowledge list.
func encodeConfig(us []Unit) []byte {
	var b bytes.Buffer
	b.WriteString("{\n  \"units\": [\n")
	for i, u := range us {
		mods := make([]string, len(u.Modules))
		for j, m := range u.Modules {
			mods[j] = jsonString(m)
		}
		fmt.Fprintf(&b, "    {\"path\": %s, \"mod_root\": %s, \"modules\": [%s], \"lang\": %s}",
			jsonString(u.Path), jsonString(u.ModRoot), strings.Join(mods, ", "), jsonString(u.Lang))
		if i < len(us)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ],\n  \"service_map\": {},\n  \"knowledge\": []\n}\n")
	return b.Bytes()
}

// jsonString is s as a JSON string; a valid UTF-8 string always encodes.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
