package acceptance

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

// linkedModules is what ends up in the binaries: the union over every target
// the release builds, from the listings release_targets_test.go takes.
//
// THE UNION IS THE WHOLE POINT, and it was learned from this check failing in
// CI after passing locally. The linked set is platform-dependent -- when the
// SQLite driver was linked, go-isatty was compiled in on darwin and windows
// and not on linux, google/uuid on darwin and linux and not on windows -- so
// no single platform's dependency list describes the archives a release
// publishes. One notices file ships in all of them; it has to cover the
// union, and only a module linked on no shipped target is stale.
func linkedModules(t *testing.T) map[string]string {
	t.Helper()
	return modulesIn(releaseListing(t))
}

// modulesIn is every third-party module any target links, at the version it
// links. This module's own packages and the standard library's carry no
// third-party module, so they are not in it.
func modulesIn(byTarget map[releaseTarget][]listedPackage) map[string]string {
	mods := map[string]string{}
	for _, pkgs := range byTarget {
		for _, p := range pkgs {
			if p.module == "" || p.module == modulePath {
				continue
			}
			mods[p.module] = p.version
		}
	}
	return mods
}

// TestThirdPartyNotices_ReadsEveryLinkedModuleOffTheListings feeds the reader
// listings of its own, because the tree links no third-party module and a
// reader that dropped every line would read exactly as a correct one does.
func TestThirdPartyNotices_ReadsEveryLinkedModuleOffTheListings(t *testing.T) {
	ours := listedPackage{path: modulePath + "/internal/report", module: modulePath}
	got := modulesIn(map[releaseTarget][]listedPackage{
		{"linux", "amd64"}: {{path: "os"}, ours, {path: "example.com/x/a", module: "example.com/x", version: "v1.2.3"}},
		{"windows", "amd64"}: {{path: "os"}, ours, {path: "example.com/x/b", module: "example.com/x", version: "v1.2.3"},
			{path: "example.com/w", module: "example.com/w", version: "v0.1.0"}},
	})
	want := map[string]string{"example.com/x": "v1.2.3", "example.com/w": "v0.1.0"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("modules = %v, want %v: every module any target links, the windows-only one too, "+
			"and neither this module nor the standard library", got, want)
	}
}

// noticesProblems is the whole check as a function of the file's text and the
// linked modules, so that fixtures can run it against a dependency list of
// their own: with nothing linked in this tree, the clauses that need a module
// would otherwise never execute, and a clause that never executes cannot be
// shown to catch anything.
//
// It fails in BOTH directions. A missing module is the licence gap; a stale
// one is a notice for code that does not ship, a false statement about the
// artifact. Once any module is linked the file must carry licence text, since
// a bare inventory is not a notice; for modernc.org/libc the musl and TRE
// notices inside it too. The text is looked for anywhere in the file, not per
// module: writing each module's licence beside it is the author's job, and
// this check sees only that some licence is there. With nothing linked, the
// file has to say so in so many words rather than go quiet: a reader who
// finds an empty list cannot tell "nothing ships" from "nobody looked".
func noticesProblems(text string, mods map[string]string) []string {
	var problems []string
	for path, ver := range mods {
		if !strings.Contains(text, path+" "+ver) {
			problems = append(problems, fmt.Sprintf("%s %s is linked into the binary and is not listed at that version", path, ver))
		}
	}
	for _, line := range strings.Split(text, "\n") {
		// Only the summary list has the "path version" shape with two fields
		// and a dot in the first.
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) != 2 || !strings.Contains(f[0], ".") || !strings.HasPrefix(f[1], "v") {
			continue
		}
		if ver, ok := mods[f[0]]; !ok {
			problems = append(problems, fmt.Sprintf("%s is listed and not linked into the binary", f[0]))
		} else if ver != f[1] {
			problems = append(problems, fmt.Sprintf("%s is listed at %s; the build links %s", f[0], f[1], ver))
		}
	}
	if len(mods) == 0 {
		if !strings.Contains(text, noNoticesSentence) {
			problems = append(problems, "no module is linked and the file does not say so")
		}
		return problems
	}
	// Some licence text, not a bare inventory. Any one of the permissive
	// licences a dependency here could carry; which one is the dependency's.
	licensed := false
	for _, want := range []string{
		"Redistribution and use in source and binary forms", // BSD
		"Permission is hereby granted, free of charge",      // MIT
		"Licensed under the Apache License",                 // Apache-2.0
	} {
		if strings.Contains(text, want) {
			licensed = true
		}
	}
	if !licensed {
		problems = append(problems, "no licence text anywhere; an inventory rather than a notice")
	}
	if _, libc := mods["modernc.org/libc"]; libc && !strings.Contains(text, "LICENSE-3RD-PARTY") {
		problems = append(problems, "modernc.org/libc is linked and its own third-party notices are not reproduced")
	}
	return problems
}

// noNoticesSentence is what the file says when nothing third-party ships.
const noNoticesSentence = "No third-party module is linked into ./cmd/rashomon"

func noticesText(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(moduleRoot, "THIRD_PARTY_NOTICES"))
	if err != nil {
		t.Fatalf("THIRD_PARTY_NOTICES: %v. The released archive ships it beside a statically "+
			"linked binary, so it has to exist and has to be right.", err)
	}
	return string(body)
}

// TestThirdPartyNotices_CoversEveryLinkedModule runs the check against the
// real file and the real union of linked modules.
func TestThirdPartyNotices_CoversEveryLinkedModule(t *testing.T) {
	for _, p := range noticesProblems(noticesText(t), linkedModules(t)) {
		t.Error("THIRD_PARTY_NOTICES: " + p)
	}
}

// TestThirdPartyNotices_CheckerFailsClosed is the check against dependency
// lists of its own, which is the only way its module clauses run in a tree
// that links nothing.
func TestThirdPartyNotices_CheckerFailsClosed(t *testing.T) {
	const bsd = "Redistribution and use in source and binary forms, with or without modification"
	const mit = "Permission is hereby granted, free of charge, to any person"
	cases := []struct {
		name string
		text string
		mods map[string]string
		red  bool
	}{
		{"nothing linked, said", noNoticesSentence + "\n\n  (none)\n", nil, false},
		{"nothing linked, not said", "THIRD-PARTY NOTICES\n\n  (none)\n", nil, true},
		{"linked and listed with its licence", "  example.com/x v1.2.3\n\n" + bsd + "\n", map[string]string{"example.com/x": "v1.2.3"}, false},
		{"linked and missing", noNoticesSentence + "\n", map[string]string{"example.com/x": "v1.2.3"}, true},
		// Only the missing-module clause can turn this one red: the licence is
		// there and the module that is listed is linked at its version.
		{"two linked, licence present, one not listed", "  example.com/x v1.2.3\n" + bsd + "\n",
			map[string]string{"example.com/x": "v1.2.3", "example.com/y": "v1.0.0"}, true},
		{"linked, listed, no licence text", "  example.com/x v1.2.3\n", map[string]string{"example.com/x": "v1.2.3"}, true},
		{"listed and not linked", "  example.com/x v1.2.3\n" + mit + "\n", map[string]string{"example.com/y": "v1.0.0"}, true},
		{"listed at another version", "  example.com/x v1.2.4\n" + bsd + "\n", map[string]string{"example.com/x": "v1.2.3"}, true},
		{"libc without its own notices", "  modernc.org/libc v1.75.7\n" + bsd + "\n", map[string]string{"modernc.org/libc": "v1.75.7"}, true},
		{"libc with its own notices", "  modernc.org/libc v1.75.7\n" + bsd + "\nLICENSE-3RD-PARTY\n", map[string]string{"modernc.org/libc": "v1.75.7"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems := noticesProblems(tc.text, tc.mods)
			if (len(problems) > 0) != tc.red {
				t.Errorf("problems = %v, want red: %v", problems, tc.red)
			}
		})
	}
}
