package acceptance

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// THIRD_PARTY_NOTICES has to stay true, and a notices file is the classic
// artifact that is written once and then silently wrong.
//
// The released binaries are statically linked and pure Go, so every module in
// the build is compiled INTO the artifact a user downloads. A dependency added
// later ships its code to those users with no licence beside it, and nothing
// about that failure is visible: the build is green, the release publishes, and
// the only symptom is a legal one nobody sees until somebody audits.
//
// So the file is checked against `go list -deps` rather than trusted. The set
// is deliberately the LINKED modules and not go.sum, which pins the whole
// module graph including packages the build never reaches -- writing notices
// for those would overstate what is in the binary, which is its own kind of
// wrong.

// releasePlatforms is the matrix .goreleaser.yml builds: three operating
// systems by two architectures.
//
// It is here rather than parsed out of the yaml because a wrong list fails
// LOUDLY -- TestThirdPartyNotices_MatchesTheReleaseMatrix compares it against
// the file and goes red on any drift -- whereas a yaml parser that silently
// read nothing would make this whole check pass by testing an empty set.
var releasePlatforms = []struct{ goos, goarch string }{
	{"linux", "amd64"}, {"linux", "arm64"},
	{"darwin", "amd64"}, {"darwin", "arm64"},
	{"windows", "amd64"}, {"windows", "arm64"},
}

// linkedModules is what ends up in the binaries -- the UNION over every
// platform the release ships, not the one the tests happen to run on.
//
// THE UNION IS THE WHOLE POINT, and it was learned from this check failing in
// CI after passing locally. The linked set is platform-dependent: go-isatty
// and go-strftime are compiled in on darwin and windows and not on linux;
// google/uuid on darwin and linux and not on windows. So NO single platform's
// dependency list describes the set of archives a release publishes, and a
// host-only check is wrong in the stale direction on every platform -- it
// demanded go-isatty be listed when run on a Mac and demanded it be absent
// when run on Linux, for one file that has to satisfy both.
//
// One notices file ships in all six archives. It therefore has to cover the
// union, and only a module linked on NO shipped platform is genuinely stale.
func linkedModules(t *testing.T) map[string]string {
	t.Helper()
	mods := map[string]string{}
	for _, p := range releasePlatforms {
		cmd := exec.Command("go", "list", "-deps", "-f",
			"{{if .Module}}{{.Module.Path}} {{.Module.Version}}{{end}}", "./cmd/rashomon")
		cmd.Dir = moduleRoot
		cmd.Env = append(os.Environ(), "GOOS="+p.goos, "GOARCH="+p.goarch)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("listing linked modules for %s/%s: %v", p.goos, p.goarch, err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			f := strings.Fields(line)
			if len(f) != 2 || strings.HasPrefix(f[0], "github.com/altrace-dev-role/rashomon") {
				continue
			}
			mods[f[0]] = f[1]
		}
	}
	if len(mods) == 0 {
		t.Fatal("no linked modules found; this check is not looking at the build any more")
	}
	return mods
}

// TestThirdPartyNotices_MatchesTheReleaseMatrix keeps releasePlatforms honest.
//
// The union above is only the right answer if it is a union over the platforms
// actually shipped. If .goreleaser.yml gains an operating system and this list
// does not, the notices file silently stops covering an archive that ships --
// and it fails in the direction where nobody notices, because adding a platform
// can only ADD modules and the existing assertions would stay green.
func TestThirdPartyNotices_MatchesTheReleaseMatrix(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(moduleRoot, ".goreleaser.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(body), "\r\n", "\n")

	seen := map[string]bool{}
	for _, p := range releasePlatforms {
		seen[p.goos] = true
		if !strings.Contains(text, "      - "+p.goos+"\n") {
			t.Errorf("releasePlatforms builds %s and .goreleaser.yml does not list it", p.goos)
		}
		if !strings.Contains(text, "      - "+p.goarch+"\n") {
			t.Errorf("releasePlatforms builds %s and .goreleaser.yml does not list it", p.goarch)
		}
	}
	for _, goos := range []string{"linux", "darwin", "windows", "freebsd", "openbsd"} {
		if strings.Contains(text, "      - "+goos+"\n") && !seen[goos] {
			t.Errorf(".goreleaser.yml ships %s and releasePlatforms does not cover it, so "+
				"THIRD_PARTY_NOTICES is not checked against that archive's dependency set",
				goos)
		}
	}
}

// TestThirdPartyNotices_CoversEveryLinkedModule fails in BOTH directions. A
// missing module is the licence gap; a stale one is a notice for code that is
// no longer shipped, which is a different false statement about the artifact.
func TestThirdPartyNotices_CoversEveryLinkedModule(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(moduleRoot, "THIRD_PARTY_NOTICES"))
	if err != nil {
		t.Fatalf("THIRD_PARTY_NOTICES: %v. The released archive ships it beside a statically "+
			"linked binary; without it the artifact carries third-party code with no "+
			"licence.", err)
	}
	text := string(body)

	mods := linkedModules(t)
	for path, ver := range mods {
		if !strings.Contains(text, path+" "+ver) {
			t.Errorf("%s %s is linked into the binary and is not listed in "+
				"THIRD_PARTY_NOTICES at that version. Regenerate the file: its code ships "+
				"to every user who downloads a release.", path, ver)
		}
	}

	// The other direction: a module listed but no longer linked.
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		// Only the summary list at the top has the "path version" shape with
		// two fields and a dot in the first.
		f := strings.Fields(line)
		if len(f) != 2 || !strings.Contains(f[0], ".") || !strings.HasPrefix(f[1], "v") {
			continue
		}
		if ver, ok := mods[f[0]]; !ok {
			t.Errorf("THIRD_PARTY_NOTICES lists %s, which is not linked into the binary. A "+
				"notice for code that does not ship is a false statement about the "+
				"artifact.", f[0])
		} else if ver != f[1] {
			t.Errorf("THIRD_PARTY_NOTICES lists %s at %s; the build links %s. A licence can "+
				"change between versions.", f[0], f[1], ver)
		}
	}
}

// TestThirdPartyNotices_CarriesActualLicenceText guards against the file
// degrading into a list of names. A bare inventory is not a notice: the
// licences themselves require their text to travel with the distribution.
func TestThirdPartyNotices_CarriesActualLicenceText(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(moduleRoot, "THIRD_PARTY_NOTICES"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)

	for _, want := range []string{
		"Redistribution and use in source and binary forms", // BSD
		"Permission is hereby granted, free of charge",      // MIT
	} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q anywhere in THIRD_PARTY_NOTICES; it has become an inventory "+
				"rather than a notice", want)
		}
	}

	// modernc.org/libc is a translation of musl, and its own third-party
	// notices are the ones most easily lost, being a file inside a file.
	if !strings.Contains(text, "LICENSE-3RD-PARTY") {
		t.Error("the modernc.org/libc third-party notices are not reproduced; they cover " +
			"the musl and TRE derived sources inside the libc translation, which is the " +
			"part of this binary least likely to be noticed and most likely to matter")
	}
}
