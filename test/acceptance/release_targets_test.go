package acceptance

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

// The release builds, read from .goreleaser.yml.
//
// What the binary links depends on the build: a file named _windows.go, or
// one built only without cgo, is compiled into some archives and not others.
// A check run on the test machine's own build sees one of them, and CI's is
// linux with cgo on, which no archive is. The release builds every GOOS/GOARCH
// pair the file lists, with CGO_ENABLED=0. So the whole-binary checks --
// TRU-40, RES-3 and H-17's -- list each of those builds and judge the union,
// as the notices check does: one notices file ships in every archive.
//
// The pairs are read from the file rather than typed here, so the matrix and
// the checks cannot drift apart. A reader that read nothing would pass every
// check on an empty set, so this one fails on any shape it was not written
// for, and every line of every listing has to say it was listed for its own
// target with cgo off.

// modulePath is this module's own path.
const modulePath = "github.com/altrace-dev-role/rashomon"

// releaseTarget is one GOOS/GOARCH pair a release archive is built for.
type releaseTarget struct{ goos, goarch string }

func (r releaseTarget) String() string { return r.goos + "/" + r.goarch }

// releaseMatrix is what .goreleaser.yml builds: the pairs, and the
// environment every one of them is built with.
type releaseMatrix struct {
	targets []releaseTarget
	env     []string
}

// rootKeys are the top-level keys this reader allows. Only builds is read;
// the others name the project, the archives, the checksums and the changelog,
// none of which changes what a build links. Any other key is refused, not
// read past: a root env reaches every build, and a before hook can write
// code ahead of one, and neither would reach the listing.
var rootKeys = map[string]bool{
	"version": true, "project_name": true, "builds": true,
	"archives": true, "checksum": true, "changelog": true,
}

// buildKeys are the keys of a build this reader models. Any other key --
// tags, overrides, dir, builder, ignore, targets -- can change which targets
// ship or what each one links, in a way the listing would not see, so it is
// refused rather than read past. The values marked true are lists.
var buildKeys = map[string]bool{
	"id": false, "main": false, "binary": false,
	"env": true, "goos": true, "goarch": true, "flags": true, "ldflags": true,
}

// buildFlags are the build flags this reader models. None changes which
// packages a build links; -tags would, and so would -race or -mod, so any
// flag outside the set is refused.
var buildFlags = map[string]bool{"-trimpath": true}

// parseReleaseMatrix reads the one build in the builds block: its main must be
// ./cmd/rashomon, its env must carry CGO_ENABLED=0, and its pairs are every
// goos against every goarch, each a pair `go tool dist list` names (known).
// It reads a key only at the build's own indent and a list item only two
// spaces under its key, and refuses everything else: a key it does not model,
// at the root or in the build, a quoted value or a YAML alias it would read
// differently from goreleaser, a second build, an empty or repeated list
// entry. A reader that read past a shape would list a build the release does
// not make.
func parseReleaseMatrix(yml string, known map[releaseTarget]bool) (releaseMatrix, error) {
	var m releaseMatrix
	lists := map[string][]string{}
	inBuilds, builds, key, main := false, 0, "", ""
	for n, line := range strings.Split(yml, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 0 {
			name, _, _ := strings.Cut(trimmed, ":")
			if !rootKeys[name] {
				return m, fmt.Errorf("line %d: the file has %q at its root, which this reader does not model", n+1, name)
			}
			inBuilds = trimmed == "builds:"
			continue
		}
		if !inBuilds {
			continue
		}
		if strings.ContainsAny(trimmed, "\"'\t") {
			return m, fmt.Errorf("line %d: %q is quoted, and this reader does not unquote", n+1, trimmed)
		}
		switch {
		case indent == 6 && key != "" && strings.HasPrefix(trimmed, "- "):
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if yamlSyntax(item) {
				return m, fmt.Errorf("line %d: %q is YAML this reader would not read as goreleaser does", n+1, item)
			}
			lists[key] = append(lists[key], item)
			continue
		case indent == 2 && strings.HasPrefix(trimmed, "- "):
			builds++
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		case indent != 4:
			return m, fmt.Errorf("line %d: %q is at an indent this reader does not model", n+1, trimmed)
		}
		name, value, ok := strings.Cut(trimmed, ":")
		value = strings.TrimSpace(value)
		isList, modelled := buildKeys[name]
		switch {
		case yamlSyntax(value):
			return m, fmt.Errorf("line %d: %q is YAML this reader would not read as goreleaser does", n+1, value)
		case !ok || !modelled:
			return m, fmt.Errorf("line %d: the build has %q, which this reader does not model", n+1, trimmed)
		case isList && value != "":
			return m, fmt.Errorf("line %d: %s is written inline, and this reader reads a list only one item to a line", n+1, name)
		case isList:
			key = name
		default:
			key = ""
			if name == "main" {
				main = value
			}
		}
	}
	if err := checkMatrix(builds, main, lists); err != nil {
		return m, err
	}
	m.env = lists["env"]
	for _, goos := range lists["goos"] {
		for _, goarch := range lists["goarch"] {
			t := releaseTarget{goos, goarch}
			if !known[t] {
				return m, fmt.Errorf("the matrix names %s, which `go tool dist list` does not", t)
			}
			m.targets = append(m.targets, t)
		}
	}
	return m, nil
}

// yamlSyntax reports a value that opens YAML this reader does not model: an
// anchor, an alias, a tag, a block scalar, a flow collection or a directive.
func yamlSyntax(v string) bool {
	return v != "" && strings.ContainsAny(v[:1], "&*!|>[{%@`")
}

// checkMatrix is what the build's lists must say for the listing to be the
// release's: one build, of ./cmd/rashomon, with CGO_ENABLED=0, only flags
// this reader models, and every list entry present once.
func checkMatrix(builds int, main string, lists map[string][]string) error {
	switch {
	case builds != 1:
		return fmt.Errorf("%d builds, and this reader reads exactly one", builds)
	case main != "./cmd/rashomon":
		return fmt.Errorf("the build's main is %q, not ./cmd/rashomon", main)
	case len(lists["goos"]) == 0 || len(lists["goarch"]) == 0:
		return errors.New("the build lists no goos or no goarch")
	}
	for key, values := range lists {
		seen := map[string]bool{}
		for _, v := range values {
			if v == "" || seen[v] {
				return fmt.Errorf("the build's %s lists %q twice or empty", key, v)
			}
			seen[v] = true
		}
	}
	for _, f := range lists["flags"] {
		if !buildFlags[f] {
			return fmt.Errorf("the build passes %s, which this reader does not model", f)
		}
	}
	for _, kv := range lists["env"] {
		if kv == "CGO_ENABLED=0" {
			return nil
		}
	}
	return errors.New("the build's env does not say CGO_ENABLED=0")
}

// distTargets is every GOOS/GOARCH pair this toolchain builds. go list takes
// any GOOS without complaint, so a matrix entry it would list as a build that
// cannot exist has to be refused against this.
func distTargets() (map[releaseTarget]bool, error) {
	out, err := exec.Command("go", "tool", "dist", "list").Output()
	if err != nil {
		return nil, fmt.Errorf("go tool dist list: %w", err)
	}
	known := map[releaseTarget]bool{}
	for _, line := range strings.Fields(string(out)) {
		goos, goarch, ok := strings.Cut(line, "/")
		if !ok {
			return nil, fmt.Errorf("go tool dist list printed %q", line)
		}
		known[releaseTarget{goos, goarch}] = true
	}
	if len(known) < 10 {
		return nil, fmt.Errorf("go tool dist list named only %d pairs", len(known))
	}
	return known, nil
}

// listFormat prints, for each package in a build, the build it was listed
// for, whether cgo was on, the package, and its module and version if it has
// one.
const listFormat = "{{context.GOOS}}/{{context.GOARCH}} {{context.CgoEnabled}} {{.ImportPath}}" +
	"{{with .Module}} {{.Path}} {{.Version}}{{end}}"

// listedPackage is one line of a listing.
type listedPackage struct{ path, module, version string }

// parseListing reads one target's listing. A line listed for another target,
// or with cgo on, means the environment the release builds with did not reach
// the listing, and is an error.
func parseListing(out string, want releaseTarget) ([]listedPackage, error) {
	var pkgs []listedPackage
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			return nil, fmt.Errorf("an unreadable listing line: %q", line)
		}
		if f[0] != want.String() || f[1] != "false" {
			return nil, fmt.Errorf("a line listed for %s with cgo %s, where the release builds %s with cgo off: %q",
				f[0], f[1], want, line)
		}
		p := listedPackage{path: f[2]}
		if len(f) > 3 {
			p.module = f[3]
		}
		if len(f) > 4 {
			p.version = f[4]
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// listingPremises are what show a listing is the build it names: this
// module's own packages are in it, with this module's path in the module
// column, and it links the one syscall package its GOOS does. A windows build
// links internal/syscall/windows, a unix one internal/syscall/unix, and no
// build links both; a GOOS that is no build at all links neither.
func listingPremises(target releaseTarget, pkgs []listedPackage) error {
	ours, windows, unix := 0, false, false
	for _, p := range pkgs {
		if strings.HasPrefix(p.path, modulePath+"/") && p.module == modulePath {
			ours++
		}
		switch p.path {
		case "internal/syscall/windows":
			windows = true
		case "internal/syscall/unix":
			unix = true
		}
	}
	if ours < 5 {
		return fmt.Errorf("the listing for %s names only %d of this module's packages; it is not looking at the build",
			target, ours)
	}
	isWindows := target.goos == "windows"
	if windows != isWindows || unix == isWindows {
		return fmt.Errorf("the listing for %s has internal/syscall/windows = %v and internal/syscall/unix = %v, "+
			"so it is not the %s build", target, windows, unix, target.goos)
	}
	return nil
}

// releaseListing lists ./cmd/rashomon for every release target, once per test
// process: the matrix does not change while the tests run, and every check
// listing it again would be the same answer at six times the load.
func releaseListing(t *testing.T) map[releaseTarget][]listedPackage {
	t.Helper()
	listed.once.Do(func() { listed.byTarget, listed.err = listReleases() })
	if listed.err != nil {
		t.Fatal(listed.err)
	}
	return listed.byTarget
}

var listed struct {
	once     sync.Once
	byTarget map[releaseTarget][]listedPackage
	err      error
}

func listReleases() (map[releaseTarget][]listedPackage, error) {
	body, err := os.ReadFile(filepath.Join(moduleRoot, ".goreleaser.yml"))
	if err != nil {
		return nil, err
	}
	known, err := distTargets()
	if err != nil {
		return nil, err
	}
	m, err := parseReleaseMatrix(string(body), known)
	if err != nil {
		return nil, fmt.Errorf(".goreleaser.yml: %w", err)
	}
	out := map[releaseTarget][]listedPackage{}
	for _, target := range m.targets {
		cmd := exec.Command("go", "list", "-deps", "-f", listFormat, "./cmd/rashomon")
		cmd.Dir = moduleRoot
		cmd.Env = append(append(os.Environ(), m.env...), "GOOS="+target.goos, "GOARCH="+target.goarch)
		listing, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("listing %s: %w", target, err)
		}
		pkgs, err := parseListing(string(listing), target)
		if err != nil {
			return nil, fmt.Errorf("listing %s: %w", target, err)
		}
		if err := listingPremises(target, pkgs); err != nil {
			return nil, err
		}
		out[target] = pkgs
	}
	// Every target, or the union is not the release's: a listing that left
	// one out would judge the archives it did list and pass the one it did not.
	if len(out) != len(m.targets) {
		return nil, fmt.Errorf("listed %d of the %d release targets", len(out), len(m.targets))
	}
	return out, nil
}

// releaseDeps is every package any release build links, sorted.
func releaseDeps(t *testing.T) []string {
	t.Helper()
	return unionImports(releaseListing(t))
}

// unionImports is the union of every target's packages: a package one target
// links ships in that target's archive.
func unionImports(byTarget map[releaseTarget][]listedPackage) []string {
	set := map[string]bool{}
	for _, pkgs := range byTarget {
		for _, p := range pkgs {
			set[p.path] = true
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// TestTRU40_ReleaseTargetsAreReadFromGoreleaser: the reader takes every goos
// against every goarch of the one build, and nothing from elsewhere in the
// file; and it refuses every shape it was not written for rather than reading
// fewer targets than ship, or targets that are not builds.
func TestTRU40_ReleaseTargetsAreReadFromGoreleaser(t *testing.T) {
	known := map[releaseTarget]bool{}
	for _, goos := range []string{"linux", "darwin", "freebsd", "windows"} {
		known[releaseTarget{goos, "amd64"}], known[releaseTarget{goos, "arm64"}] = true, true
	}
	build := "builds:\n  - id: rashomon\n    main: ./cmd/rashomon\n    env:\n      - CGO_ENABLED=0\n" +
		"    goos:\n      - linux\n      - darwin\n      - freebsd\n    goarch:\n      - amd64\n      - arm64\n" +
		"    flags:\n      # a \"quoted\" comment\n      - -trimpath\n    ldflags:\n      - -s -w -X main.version={{.Version}}\n"
	archives := "\narchives:\n  - id: rashomon\n    format_overrides:\n      - goos: windows\n    goos:\n      - plan9\n"
	root := "version: 2\n\nproject_name: rashomon\n\n"
	rest := "\nchecksum:\n  algorithm: sha256\n\nchangelog:\n  use: git\n"
	m, err := parseReleaseMatrix(root+build+archives+rest, known)
	if err != nil {
		t.Fatal(err)
	}
	want := []releaseTarget{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"},
		{"freebsd", "amd64"}, {"freebsd", "arm64"}}
	if !reflect.DeepEqual(m.targets, want) {
		t.Errorf("targets = %v, want %v", m.targets, want)
	}
	if !reflect.DeepEqual(m.env, []string{"CGO_ENABLED=0"}) {
		t.Errorf("env = %v, want the build's own", m.env)
	}

	for _, tc := range []struct{ name, yml string }{
		{"no builds block", archives},
		{"no goarch", strings.Replace(build, "    goarch:\n      - amd64\n      - arm64\n", "", 1)},
		{"cgo not off", strings.Replace(build, "CGO_ENABLED=0", "CGO_ENABLED=1", 1)},
		{"an ignore list", build + "    ignore:\n      - goos: darwin\n"},
		{"a targets list", build + "    targets:\n      - linux_amd64\n"},
		{"a second build", build + "  - id: other\n    main: ./cmd/rashomon\n"},
		{"another main", strings.Replace(build, "./cmd/rashomon", "./cmd/other", 1)},
		{"a quoted goos", strings.Replace(build, "      - darwin\n", "      - \"darwin\"\n", 1)},
		{"a single-quoted goos", strings.Replace(build, "      - linux\n", "      - 'linux'\n", 1)},
		{"a quoted env entry", strings.Replace(build, "      - CGO_ENABLED=0\n",
			"      - CGO_ENABLED=0\n      - \"GOFLAGS=-tags=withnet\"\n", 1)},
		{"a goos that is no build", strings.Replace(build, "      - freebsd\n", "      - plan10\n", 1)},
		{"a tags list", build + "    tags:\n      - withnet\n"},
		{"a -tags flag", strings.Replace(build, "      - -trimpath\n", "      - -tags=withnet\n", 1)},
		{"an override's env", build + "    overrides:\n      - goos: darwin\n        goarch: arm64\n" +
			"        env:\n          - CGO_ENABLED=1\n"},
		{"a dir", build + "    dir: ./other\n"},
		{"a list item under its key's own item", strings.Replace(build, "      - CGO_ENABLED=0\n",
			"      - CGO_ENABLED=0\n        - GOFLAGS=-tags=withnet\n", 1)},
		{"an inline list", strings.Replace(build, "    goarch:\n      - amd64\n      - arm64\n", "    goarch: [amd64, arm64]\n", 1)},
		{"an alias", strings.Replace(build, "      - CGO_ENABLED=0\n", "      - CGO_ENABLED=0\n      - *flags\n", 1)},
		{"a goos twice", strings.Replace(build, "      - freebsd\n", "      - linux\n", 1)},
		{"a root env", "env:\n  - GOFLAGS=-tags=withnet\n" + build},
		{"a root before hook", "before:\n  hooks:\n    - go generate ./...\n" + build},
	} {
		if _, err := parseReleaseMatrix(tc.yml, known); err == nil {
			t.Errorf("%s: read without an error; the reader must refuse a shape it does not model", tc.name)
		}
	}

	// The real file reads, and every target it names was listed with the
	// premises above holding.
	if got := releaseListing(t); len(got) == 0 {
		t.Fatal("the release matrix read no target")
	}
}

// TestTRU40_EveryTargetIsListedAsItShips: a package one target links is in
// the union the checks judge; a listing that did not run as its target, or
// ran with cgo on, is refused; and a listing that is not the build it names
// fails its premises.
func TestTRU40_EveryTargetIsListedAsItShips(t *testing.T) {
	linux, windows := releaseTarget{"linux", "amd64"}, releaseTarget{"windows", "amd64"}
	ours := modulePath + "/internal/report"
	union := unionImports(map[releaseTarget][]listedPackage{
		linux:   {{path: "os"}, {path: ours, module: modulePath}},
		windows: {{path: "os"}, {path: "net"}, {path: ours, module: modulePath}},
	})
	if !reflect.DeepEqual(union, []string{ours, "net", "os"}) {
		t.Errorf("union = %v, want the windows-only net in it", union)
	}

	pkgs, err := parseListing("windows/amd64 false net\nwindows/amd64 false example.com/x/y example.com/x v1.2.3\n"+
		"windows/amd64 false "+ours+" "+modulePath+" \n", windows)
	want := []listedPackage{{path: "net"}, {path: "example.com/x/y", module: "example.com/x", version: "v1.2.3"},
		{path: ours, module: modulePath}}
	if err != nil || !reflect.DeepEqual(pkgs, want) {
		t.Errorf("parseListing = %v, %v; want %v", pkgs, err, want)
	}
	for _, line := range []string{"linux/amd64 false os", "windows/amd64 true os", "windows/amd64"} {
		if _, err := parseListing(line+"\n", windows); err == nil {
			t.Errorf("parseListing accepted %q as the windows/amd64 build with cgo off", line)
		}
	}

	var five, unmoduled []listedPackage
	for i := 0; i < 5; i++ {
		path := fmt.Sprintf("%s/internal/p%d", modulePath, i)
		five = append(five, listedPackage{path: path, module: modulePath})
		unmoduled = append(unmoduled, listedPackage{path: path})
	}
	with := func(pkgs []listedPackage, syscalls ...string) []listedPackage {
		out := append([]listedPackage{}, pkgs...)
		for _, s := range syscalls {
			out = append(out, listedPackage{path: "internal/syscall/" + s})
		}
		return out
	}
	for _, tc := range []struct {
		name   string
		target releaseTarget
		pkgs   []listedPackage
		ok     bool
	}{
		{"windows, with its syscall package", windows, with(five, "windows"), true},
		{"linux, with its syscall package", linux, with(five, "unix"), true},
		{"windows, without its", windows, five, false},
		{"linux, without its", linux, five, false},
		{"linux, with the windows one", linux, with(five, "windows"), false},
		{"windows, with the unix one", windows, with(five, "unix"), false},
		{"windows, with both", windows, with(five, "windows", "unix"), false},
		{"too few of ours", linux, with(five[:4], "unix"), false},
		{"ours without the module column", linux, with(unmoduled, "unix"), false},
	} {
		if err := listingPremises(tc.target, tc.pkgs); (err == nil) != tc.ok {
			t.Errorf("%s: premises error = %v, want ok = %v", tc.name, err, tc.ok)
		}
	}
}
