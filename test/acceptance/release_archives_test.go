package acceptance

// scripts/check-release-archives.sh reads what a release ships before it is
// published: release.yaml runs it between GoReleaser's draft and publishing,
// and ci.yaml's release-dry-run job runs it on a snapshot. Every archive
// .goreleaser.yml makes must carry rashomon, pgraph, LICENSE and
// THIRD_PARTY_NOTICES, and the plugin archive both binaries for each of its
// four platforms beside the same two files. The tests below run the script on
// a release written here, whole and then with one member or one archive gone
// in turn, and read that both workflows run it where a red result keeps the
// release a draft.

import (
	"archive/zip"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// checkArchivesScript is the script, by its path in the repository.
const checkArchivesScript = "scripts/check-release-archives.sh"

// checkArchivesRun is the step line both workflows run the script with.
const checkArchivesRun = "        run: scripts/check-release-archives.sh dist \"$RUNNER_TEMP/rashomon-plugin.zip\""

// fakePlugin is the plugin archive's name in a fakeRelease.
const fakePlugin = "rashomon-plugin.zip"

// fakeRelease is a release as the script reads it: each archive's file name,
// the plugin archive's included, and the names of the members it holds.
type fakeRelease map[string][]string

// fakeArchiveName names a target's archive as .goreleaser.yml's
// name_template does, for the version 0.0.0-test.
func fakeArchiveName(tg releaseTarget) string {
	if tg.goos == "windows" {
		return "rashomon_0.0.0-test_" + tg.goos + "_" + tg.goarch + ".zip"
	}
	return "rashomon_0.0.0-test_" + tg.goos + "_" + tg.goarch + ".tar.gz"
}

// fakeBinaryName is a binary's name in a target's archive.
func fakeBinaryName(tg releaseTarget, binary string) string {
	if tg.goos == "windows" {
		return binary + ".exe"
	}
	return binary
}

// fullRelease is a release with every member the script asks for: one
// archive for each target, and the plugin archive with what build-plugin.sh
// packs.
func fullRelease(targets []releaseTarget) fakeRelease {
	rel := fakeRelease{}
	for _, tg := range targets {
		rel[fakeArchiveName(tg)] = []string{fakeBinaryName(tg, "rashomon"), fakeBinaryName(tg, "pgraph"), "README.md", "LICENSE", "THIRD_PARTY_NOTICES"}
	}
	plugin := []string{"bin/rashomon", "bin/pgraph", "hooks/hooks.json", ".claude-plugin/plugin.json", "LICENSE", "THIRD_PARTY_NOTICES"}
	for _, pair := range []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64"} {
		plugin = append(plugin, "bin/rashomon-"+pair, "bin/pgraph-"+pair)
	}
	rel[fakePlugin] = plugin
	return rel
}

// writeZip writes a .zip holding one small script per member, in the order
// given.
func writeZip(t *testing.T, dest string, members []string) {
	t.Helper()
	f, err := os.Create(dest)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, name := range members {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("#!/bin/sh\nexit 0\n")); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []interface{ Close() error }{zw, f} {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// checkFakeRelease writes rel into a new folder, its archives into dist and
// the plugin archive beside it, runs the script on them, and returns its exit
// code and output.
func checkFakeRelease(t *testing.T, rel fakeRelease) (int, string) {
	t.Helper()
	dir := t.TempDir()
	dist := filepath.Join(dir, "dist")
	if err := os.Mkdir(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(dir, fakePlugin)
	for name, members := range rel {
		switch {
		case name == fakePlugin:
			writeZip(t, plugin, members)
		case strings.HasSuffix(name, ".zip"):
			writeZip(t, filepath.Join(dist, name), members)
		default:
			modes := map[string]int64{}
			for _, member := range members {
				modes[member] = 0o755
			}
			writeArchive(t, filepath.Join(dist, name), modes)
		}
	}
	out, err := exec.Command("sh", filepath.Join(moduleRoot, checkArchivesScript), dist, plugin).CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, string(out)
	case errors.As(err, &exit):
		return exit.ExitCode(), string(out)
	}
	t.Fatalf("%s did not run: %v", checkArchivesScript, err)
	return 0, ""
}

// TestReleaseArchives_CheckScriptRefusesAMissingMember: the script passes a
// release with every member and reads each archive, and refuses, naming what
// is missing, a release with one member or one archive gone. Each target
// .goreleaser.yml builds loses its archive in turn, so a target the script
// does not know fails here, and so does a dist holding two archives for one
// target. Break: drop a member from the script's list.
func TestReleaseArchives_CheckScriptRefusesAMissingMember(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the script is POSIX sh, run by the release jobs on linux")
	}
	builds, err := readReleaseBuilds()
	if err != nil {
		t.Fatal(err)
	}
	targets := builds["rashomon"].targets
	if len(targets) != 6 {
		t.Fatalf(".goreleaser.yml builds rashomon for %v, want six pairs", targets)
	}
	code, out := checkFakeRelease(t, fullRelease(targets))
	if code != 0 {
		t.Fatalf("a release with every member: exit %d, want 0:\n%s", code, out)
	}
	for name := range fullRelease(targets) {
		if !strings.Contains(out, "read "+name+"\n") {
			t.Errorf("the script did not read %s:\n%s", name, out)
		}
	}

	type gone struct{ archive, member, want string }
	cases := []gone{
		{"rashomon_0.0.0-test_linux_arm64.tar.gz", "pgraph", "rashomon_0.0.0-test_linux_arm64.tar.gz has no pgraph\n"},
		{"rashomon_0.0.0-test_linux_amd64.tar.gz", "rashomon", "rashomon_0.0.0-test_linux_amd64.tar.gz has no rashomon\n"},
		{"rashomon_0.0.0-test_darwin_arm64.tar.gz", "THIRD_PARTY_NOTICES", "rashomon_0.0.0-test_darwin_arm64.tar.gz has no THIRD_PARTY_NOTICES\n"},
		{"rashomon_0.0.0-test_darwin_amd64.tar.gz", "LICENSE", "rashomon_0.0.0-test_darwin_amd64.tar.gz has no LICENSE\n"},
		{"rashomon_0.0.0-test_windows_amd64.zip", "pgraph.exe", "rashomon_0.0.0-test_windows_amd64.zip has no pgraph.exe\n"},
		{"rashomon_0.0.0-test_windows_arm64.zip", "rashomon.exe", "rashomon_0.0.0-test_windows_arm64.zip has no rashomon.exe\n"},
		{fakePlugin, "bin/pgraph-linux-amd64", "rashomon-plugin.zip has no bin/pgraph-linux-amd64\n"},
		{fakePlugin, "bin/rashomon-darwin-arm64", "rashomon-plugin.zip has no bin/rashomon-darwin-arm64\n"},
		{fakePlugin, "LICENSE", "rashomon-plugin.zip has no LICENSE\n"},
		{fakePlugin, "THIRD_PARTY_NOTICES", "rashomon-plugin.zip has no THIRD_PARTY_NOTICES\n"},
		{fakePlugin, "", "there is no plugin archive"},
	}
	for _, tg := range targets {
		name := fakeArchiveName(tg)
		ext := strings.TrimPrefix(name, "rashomon_0.0.0-test_"+tg.goos+"_"+tg.goarch)
		cases = append(cases, gone{name, "", "has no single rashomon_*_" + tg.goos + "_" + tg.goarch + ext + " archive\n"})
	}
	for _, tc := range cases {
		rel := fullRelease(targets)
		if tc.member == "" {
			delete(rel, tc.archive)
		} else {
			kept := slices.DeleteFunc(slices.Clone(rel[tc.archive]), func(member string) bool { return member == tc.member })
			if len(kept) != len(rel[tc.archive])-1 {
				t.Fatalf("%s holds no member %s to take out", tc.archive, tc.member)
			}
			rel[tc.archive] = kept
		}
		code, out := checkFakeRelease(t, rel)
		if code != 1 || !strings.Contains(out, tc.want) {
			t.Errorf("%s without %q: exit %d, want 1 and %q:\n%s", tc.archive, tc.member, code, tc.want, out)
		}
	}

	rel := fullRelease(targets)
	rel["rashomon_0.0.0-old_linux_amd64.tar.gz"] = rel["rashomon_0.0.0-test_linux_amd64.tar.gz"]
	code, out = checkFakeRelease(t, rel)
	if want := "has no single rashomon_*_linux_amd64.tar.gz archive\n"; code != 1 || !strings.Contains(out, want) {
		t.Errorf("a dist with two linux/amd64 archives: exit %d, want 1 and %q:\n%s", code, want, out)
	}
}

// ciJobLines is one job of ci.yaml, comments left out, from its key up to the
// next job's.
func ciJobLines(t *testing.T, job string) []string {
	t.Helper()
	lines := releaseYAMLLines(t, ".github/workflows/ci.yaml")
	start := slices.Index(lines, "  "+job+":")
	if start < 0 {
		t.Fatalf("ci.yaml has no %s job", job)
	}
	key := regexp.MustCompile(`^  [A-Za-z0-9_-]+:\s*$`)
	for i := start + 1; i < len(lines); i++ {
		if key.MatchString(lines[i]) {
			return lines[start:i]
		}
	}
	return lines[start:]
}

// TestReleaseArchives_TheReleaseIsCheckedBeforeItIsPublished: release.yaml
// runs the script after GoReleaser makes the draft and before the step that
// attaches the plugin archive and publishes, so a release that fails it stays
// a draft, and the dry run runs it on the snapshot GoReleaser wrote. Both run
// it by its path, so it is executable. Break: move the release job's check
// below the publishing step, or drop either.
func TestReleaseArchives_TheReleaseIsCheckedBeforeItIsPublished(t *testing.T) {
	lines := releaseYAMLLines(t, ".github/workflows/release.yaml")
	first := func(marker string) int {
		return slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, marker) })
	}
	made, checked, published := first("goreleaser/goreleaser-action@"), slices.Index(lines, checkArchivesRun), first("gh release upload")
	if made < 0 || checked < 0 || published < 0 || made > checked || checked > published {
		t.Errorf("release.yaml, lines that are not comments: GoReleaser is line %d, the check %d, the publishing step %d; want the check between the draft and the publishing", made, checked, published)
	}
	job := ciJobLines(t, "release-dry-run")
	snapshot, dryCheck := slices.Index(job, "          args: release --snapshot --clean"), slices.Index(job, checkArchivesRun)
	if snapshot < 0 || dryCheck < snapshot {
		t.Errorf("ci.yaml's release-dry-run job: the snapshot is line %d, the check %d; want the check after the snapshot", snapshot, dryCheck)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(moduleRoot, checkArchivesScript))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&0o100 == 0 {
			t.Errorf("%s is not executable, and both workflows run it by its path", checkArchivesScript)
		}
	}
}
