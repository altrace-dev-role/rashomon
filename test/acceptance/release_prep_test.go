package acceptance

// A release publishes the plugin archive under the version the plugin's
// manifest names. Claude Code caches an installed plugin under a directory
// named by the version plugin/.claude-plugin/plugin.json names, and the
// marketplace installs releases/latest/download/rashomon-plugin.zip. So the
// build refuses any other version, the release stays a draft until the
// archive is on it, and a dispatch of ci.yaml rehearses both with nothing
// published.

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// releaseYAMLLines reads a file of this repository and returns its lines
// that are not YAML comments.
func releaseYAMLLines(t *testing.T, rel string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(moduleRoot, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, l := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			lines = append(lines, l)
		}
	}
	return lines
}

// TestREL1_ThePluginBuildRefusesAVersionThePluginManifestDoesNotName runs a
// copy of scripts/build-plugin.sh beside a copy of the manifest, with a go on
// PATH that exits 42. A refused version exits 1 before go is reached; an
// accepted one reaches go, so nothing is built and the real tree is never
// touched.
func TestREL1_ThePluginBuildRefusesAVersionThePluginManifestDoesNotName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("build-plugin.sh is a POSIX shell script and Windows has no /bin/sh")
	}
	tmp := t.TempDir()
	for _, rel := range []string{"scripts/build-plugin.sh", "plugin/.claude-plugin/plugin.json"} {
		b, err := os.ReadFile(filepath.Join(moduleRoot, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(tmp, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fakeBin := filepath.Join(tmp, "fakebin")
	if err := os.Mkdir(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fakeBin, "go"), []byte("#!/bin/sh\nexit 42\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(tmp, "plugin", ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &manifest); err != nil || manifest.Version == "" {
		t.Fatalf("plugin.json names no version: %v", err)
	}
	run := func(version string) (int, string) {
		cmd := exec.Command("/bin/sh", filepath.Join(tmp, "scripts", "build-plugin.sh"), version)
		cmd.Env = []string{"PATH=" + fakeBin + ":/usr/bin:/bin", "HOME=" + tmp}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		var exit *exec.ExitError
		switch {
		case err == nil:
			return 0, stderr.String()
		case errors.As(err, &exit):
			return exit.ExitCode(), stderr.String()
		}
		t.Fatalf("build-plugin.sh %s did not run: %v", version, err)
		return -1, ""
	}
	for _, c := range []struct {
		version string
		want    int
	}{
		{"0.0.0-not-this", 1},
		{manifest.Version, 42},
		{"dev", 42},
	} {
		code, stderr := run(c.version)
		if code != c.want {
			t.Errorf("build-plugin.sh %s: exit %d, want %d (1 refused, 42 reached go); stderr:\n%s", c.version, code, c.want, stderr)
		}
		if c.want == 1 && !strings.Contains(stderr, "plugin.json") {
			t.Errorf("build-plugin.sh %s: the refusal does not name plugin.json:\n%s", c.version, stderr)
		}
	}
}

// TestREL1b_TheReleasePublishesOnlyAfterThePluginArchiveIsAttached reads the
// release job's step lines in order: the archive is built and kept, GoReleaser
// makes the draft, the archive is attached, and only then is the release
// published and read back from releases/latest.
func TestREL1b_TheReleasePublishesOnlyAfterThePluginArchiveIsAttached(t *testing.T) {
	lines := releaseYAMLLines(t, ".github/workflows/release.yaml")
	prev, prevMarker := -1, ""
	for _, marker := range []string{
		"scripts/build-plugin.sh",
		"actions/upload-artifact@",
		"goreleaser/goreleaser-action@",
		"gh release upload",
		"--draft=false",
		"releases/latest/download/rashomon-plugin.zip",
	} {
		i := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, marker) })
		switch {
		case i < 0:
			t.Errorf("release.yaml has no step line with %q", marker)
			continue
		case i <= prev:
			t.Errorf("release.yaml: %q comes before %q", marker, prevMarker)
		}
		prev, prevMarker = i, marker
	}
}

// TestREL1c_GoreleaserMakesADraftAndNoPrerelease: GoReleaser makes the
// release a draft, which the release job publishes after the archive is on
// it, and never marks it a prerelease, which releases/latest never points at.
func TestREL1c_GoreleaserMakesADraftAndNoPrerelease(t *testing.T) {
	lines := releaseYAMLLines(t, ".goreleaser.yml")
	i := slices.Index(lines, "release:")
	if i < 0 || i+1 >= len(lines) || lines[i+1] != "  draft: true" {
		t.Errorf(".goreleaser.yml has no root release block that starts with draft: true")
	}
	for _, l := range lines {
		if strings.Contains(l, "prerelease") {
			t.Errorf(".goreleaser.yml sets prerelease (%q); the marketplace would stay on the last full release", strings.TrimSpace(l))
		}
	}
}

// releaseRootBlock returns the lines under one root key of a YAML file of
// this repository, comments left out, up to the next root key.
func releaseRootBlock(t *testing.T, rel, key string) []string {
	t.Helper()
	lines := releaseYAMLLines(t, rel)
	start := slices.Index(lines, key)
	if start < 0 {
		t.Fatalf("%s has no root %s block", rel, key)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if lines[i] != "" && !strings.HasPrefix(lines[i], " ") {
			end = i
			break
		}
	}
	return lines[start+1 : end]
}

// TestREL1e_TheReleaseBodyListsNoCommits: GitHub caps a release body at
// 125,000 characters and a list of every commit since the previous tag grows
// with each commit, so GoReleaser writes no changelog. The notes are set on
// the release by hand after it is published.
func TestREL1e_TheReleaseBodyListsNoCommits(t *testing.T) {
	block := releaseRootBlock(t, ".goreleaser.yml", "changelog:")
	if !slices.Contains(block, `  disable: "true"`) {
		t.Errorf(".goreleaser.yml's changelog block does not disable the changelog: %q", block)
	}
}

// TestREL1f_TheReleaseHeaderIsAShortFixedText: with no changelog the release
// body is the header until the notes are set, and the header is rendered only
// when the release is published, which no dry run reaches. So it is one
// double-quoted line, not empty, under 400 characters, with no template
// action, and the release has no footer.
func TestREL1f_TheReleaseHeaderIsAShortFixedText(t *testing.T) {
	var headers []string
	for _, l := range releaseRootBlock(t, ".goreleaser.yml", "release:") {
		switch {
		case strings.HasPrefix(l, "  footer:"):
			t.Errorf(".goreleaser.yml's release block has a footer: %q", strings.TrimSpace(l))
		case strings.HasPrefix(l, "  header:"):
			headers = append(headers, l)
		}
	}
	if len(headers) != 1 {
		t.Fatalf(".goreleaser.yml's release block has %d header lines, want 1", len(headers))
	}
	m := regexp.MustCompile(`^  header: "([^"\\]*)"$`).FindStringSubmatch(headers[0])
	if m == nil {
		t.Fatalf("the release header is not one double-quoted string on its line: %q", headers[0])
	}
	text := m[1]
	if strings.TrimSpace(text) == "" {
		t.Errorf("the release header is empty")
	}
	if n := utf8.RuneCountInString(text); n >= 400 {
		t.Errorf("the release header is %d characters long, want under 400", n)
	}
	if strings.Contains(text, "{{") || strings.Contains(text, "}}") {
		t.Errorf("the release header holds a template action: %q", text)
	}
}

// TestREL1d_TheDryRunJobKeepsTheArchive: ci.yaml's release-dry-run job runs
// on a dispatch only, runs GoReleaser with nothing published, builds the
// plugin archive and keeps it.
func TestREL1d_TheDryRunJobKeepsTheArchive(t *testing.T) {
	lines := releaseYAMLLines(t, ".github/workflows/ci.yaml")
	start := slices.Index(lines, "  release-dry-run:")
	if start < 0 {
		t.Fatal("ci.yaml has no release-dry-run job")
	}
	job := regexp.MustCompile(`^  [A-Za-z0-9_-]+:\s*$`)
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if job.MatchString(lines[i]) {
			end = i
			break
		}
	}
	block := strings.Join(lines[start:end], "\n")
	for _, want := range []string{
		"workflow_dispatch",
		"args: release --snapshot --clean",
		"scripts/build-plugin.sh",
		"actions/upload-artifact@",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("ci.yaml's release-dry-run job has no line with %q", want)
		}
	}
}

// TestRelease_PgraphShipsInEveryArchive: .goreleaser.yml builds pgraph from
// its own module with cgo off (the matrix reader refuses any other pgraph
// build), for the same six pairs as rashomon, and the archive takes both
// builds, so every archive a release publishes carries pgraph beside
// rashomon. Break: take pgraph out of the archive's ids, or build it with cgo
// on.
func TestRelease_PgraphShipsInEveryArchive(t *testing.T) {
	builds, err := readReleaseBuilds()
	if err != nil {
		t.Fatal(err)
	}
	pgraph, ok := builds["pgraph"]
	if !ok {
		t.Fatal(".goreleaser.yml builds no pgraph, so no archive carries it")
	}
	rashomon := builds["rashomon"].targets
	if len(rashomon) != 6 || !slices.Equal(pgraph.targets, rashomon) {
		t.Errorf("pgraph is built for %v and rashomon for %v; want the same six pairs", pgraph.targets, rashomon)
	}
	var ids []string
	inIDs := false
	for _, l := range releaseRootBlock(t, ".goreleaser.yml", "archives:") {
		switch {
		case l == "    ids:":
			inIDs = true
		case inIDs && strings.HasPrefix(l, "      - "):
			ids = append(ids, strings.TrimPrefix(l, "      - "))
		default:
			inIDs = false
		}
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"pgraph", "rashomon"}) {
		t.Errorf("the archive takes the builds %v, want pgraph and rashomon", ids)
	}
}
