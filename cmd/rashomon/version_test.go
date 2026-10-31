package main

import (
	"runtime/debug"
	"testing"
)

// TestVersion_ResolveVersion pins resolveVersion's three branches (A1): a
// linker-set value wins, a tagged module version comes next with its v
// dropped, and anything else is dev.
func TestVersion_ResolveVersion(t *testing.T) {
	bi := func(v string) *debug.BuildInfo { return &debug.BuildInfo{Main: debug.Module{Version: v}} }
	for _, tc := range []struct {
		name    string
		ldflags string
		bi      *debug.BuildInfo
		ok      bool
		want    string
	}{
		{"ldflags beat a tagged module", "1.2.0-alpha.1", bi("v9.9.9"), true, "1.2.0-alpha.1"},
		{"ldflags with no build info", "812eebfb", nil, false, "812eebfb"},
		{"a go install of a tag", "dev", bi("v1.2.0-alpha.1"), true, "1.2.0-alpha.1"},
		{"an empty ldflags is unset", "", bi("v1.2.0"), true, "1.2.0"},
		{"a module version without a v is kept", "dev", bi("1.2.0"), true, "1.2.0"},
		{"a local build says (devel)", "dev", bi("(devel)"), true, "dev"},
		{"an empty module version", "dev", bi(""), true, "dev"},
		{"build info unreadable", "dev", bi("v1.2.0"), false, "dev"},
		{"build info absent", "dev", nil, true, "dev"},
	} {
		if got := resolveVersion(tc.ldflags, tc.bi, tc.ok); got != tc.want {
			t.Errorf("%s: resolveVersion(%q) = %q, want %q", tc.name, tc.ldflags, got, tc.want)
		}
	}
}

// TestVersion_BuildVersionIsTheLinkerValueOrResolved pins that buildVersion
// reads the same decision: this test binary has no ldflags, so it is what
// resolveVersion says of the test binary's own build info.
func TestVersion_BuildVersionIsTheLinkerValueOrResolved(t *testing.T) {
	bi, ok := debug.ReadBuildInfo()
	if want := resolveVersion(version, bi, ok); buildVersion() != want {
		t.Errorf("buildVersion() = %q, want %q", buildVersion(), want)
	}
}
