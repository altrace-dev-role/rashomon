package acceptance

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TRU-40: no networking package anywhere in the binary's dependency graph, its
// own or a dependency's, on any release target, and os/exec only through the
// importers h17_no_egress_test.go sanctions, of which there are none until
// the supervisor lands. "Any release target" is every GOOS/GOARCH pair
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
// checks the binary's whole graph on every release target, so that a
// dependency bringing os/exec in with nobody here having written the import
// still fails. While the map has no entry for it, os/exec must be absent
// altogether.
func TestTRU40_OsExecEntersOnlyThroughSanctionedImporters(t *testing.T) {
	for _, pkg := range releaseDeps(t) {
		if pkg == "os/exec" && sanctionedSource["os/exec"] == "" {
			t.Errorf("the binary depends on os/exec and nothing sanctions it")
		}
	}
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

// networkSentence is the README's claim, rendered from the banned set.
func networkSentence() string {
	pkgs := make([]string, 0, len(forbiddenImports))
	for pkg := range forbiddenImports {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	last := pkgs[len(pkgs)-1]
	return "No package in the rashomon binary, its own or a dependency's, imports " +
		strings.Join(pkgs[:len(pkgs)-1], ", ") + " or " + last +
		", and CI checks that on every pull request, every push to main and every release tag."
}
