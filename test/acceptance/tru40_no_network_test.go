package acceptance

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// TRU-40: no networking package anywhere in the binary's dependency graph, its
// own or a dependency's, on any release target, and os/exec only through the
// importers h17_no_egress_test.go sanctions: the supervisor, which starts the
// binary's own child for each hook entry. "Any release target" is every GOOS/GOARCH pair
// .goreleaser.yml builds, with CGO_ENABLED=0 (release_targets_test.go): the
// test machine's own build would let a _windows.go file, or one built only
// without cgo, import net and ship.
//
// H-17 scopes the same ban to the recorder, which is the guarantee that
// matters inside a tool call. This is the whole-binary form, which the alpha
// could not make while the report linked a SQLite driver whose subtree carried
// net and os/exec. With the driver gone the stronger claim is true, and it is
// the one the README makes.
func TestTRU40_BinaryHasNoNetworkingPackage(t *testing.T) {
	binary := releaseDeps(t)
	// The failure mode of every dependency-scanning check is to scan nothing.
	if len(binary) < 10 {
		t.Fatalf("dependency listing returned only %d packages; the command did not run as expected", len(binary))
	}
	for _, pkg := range binary {
		if why, bad := forbiddenImports[pkg]; bad && pkg != "os/exec" {
			t.Errorf("the binary depends on %s, which %s", pkg, why)
		}
	}
}

// TestTRU40_OsExecEntersOnlyThroughSanctionedImporters is the os/exec half.
// H-17 checks our own import statements against the sanctioned map; this
// checks the binary's whole graph on every release target, every package in
// it that imports os/exec directly, so that a dependency bringing os/exec in
// with nobody here having written the import still fails.
func TestTRU40_OsExecEntersOnlyThroughSanctionedImporters(t *testing.T) {
	byTarget := releaseImporters(t, "os/exec")
	if len(byTarget) == 0 {
		t.Fatal("no release target was listed")
	}
	for target, importers := range byTarget {
		for _, imp := range importers {
			if !slices.Contains(sanctionedSource["os/exec"], imp) {
				t.Errorf("the %s build links %s, which imports os/exec, and nothing sanctions it", target, imp)
			}
		}
	}
}

// releaseImporters lists, for every release target, the packages in the
// binary's graph that import pkg directly.
func releaseImporters(t *testing.T, pkg string) map[releaseTarget][]string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(moduleRoot, ".goreleaser.yml"))
	if err != nil {
		t.Fatal(err)
	}
	known, err := distTargets()
	if err != nil {
		t.Fatal(err)
	}
	m, err := parseReleaseMatrix(string(body), known)
	if err != nil {
		t.Fatal(err)
	}
	out := map[releaseTarget][]string{}
	for _, target := range m.targets {
		cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}} {{join .Imports \",\"}}", "./cmd/rashomon")
		cmd.Dir = moduleRoot
		cmd.Env = releaseListEnv(target, m.env)
		listing, err := cmd.Output()
		if err != nil {
			t.Fatalf("listing %s: %v", target, err)
		}
		lines := strings.Split(strings.TrimSpace(string(listing)), "\n")
		if len(lines) < 10 {
			t.Fatalf("the %s listing names only %d packages; it is not looking at the build", target, len(lines))
		}
		out[target] = []string{}
		for _, line := range lines {
			path, imports, _ := strings.Cut(line, " ")
			if slices.Contains(strings.Split(imports, ","), pkg) {
				out[target] = append(out[target], path)
			}
		}
	}
	return out
}

// TestTRU40_ReadmeNetworkSentenceMatchesTheTest pins the README's networking
// sentence to what the two tests above assert, so the public claim cannot
// drift from the check behind it: the sentence names the banned set, and the
// set is read from forbiddenImports, not typed twice.
func TestTRU40_ReadmeNetworkSentenceMatchesTheTest(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(moduleRoot, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	// The README wraps at 80 columns; the claim is one sentence.
	readme := strings.Join(strings.Fields(string(data)), " ")
	if !strings.Contains(readme, networkSentence()) {
		t.Errorf("README.md does not carry the networking sentence the tests assert:\n%s", networkSentence())
	}
}

// networkSentence is the README's claim, rendered from the banned set and
// the sanctioned importers: the packages nothing imports, then each package
// only its sanctioned importers do, named by their path in this module.
func networkSentence() string {
	var banned, sanctioned []string
	for pkg := range forbiddenImports {
		if _, ok := sanctionedSource[pkg]; ok {
			sanctioned = append(sanctioned, pkg)
			continue
		}
		banned = append(banned, pkg)
	}
	sort.Strings(banned)
	sort.Strings(sanctioned)
	s := "No package in the rashomon binary, its own or a dependency's, imports " + orList(banned)
	for _, pkg := range sanctioned {
		var who []string
		for _, src := range sanctionedSource[pkg] {
			who = append(who, strings.TrimPrefix(src, rashomonModule+"/"))
		}
		sort.Strings(who)
		s += ", and only " + orList(who) + " imports " + pkg
	}
	return s + "; CI checks that on every pull request, every push to main and every release tag."
}

// orList joins a sorted list as prose: "a", "a or b", "a, b or c".
func orList(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}
