package install

import (
	"strings"
	"testing"
)

// TestCommandOnWindowsPathsIsNotRunnable pins a KNOWN LIMITATION rather than a
// property this package keeps.
//
// shellQuote quotes for a POSIX shell. A Windows executable path contains
// backslashes, which are not in its safe set, so every Windows install gets
// POSIX single quotes around the program:
//
//	'C:\Users\alice\rashomon.exe' hook --install 0123...
//
// cmd.exe does not treat the apostrophe as a quote. It looks for a program
// literally named 'C:\Users\alice\rashomon.exe, does not find one, and the
// hook fails on every tool call -- silently, because a hook that cannot run
// is a hook that records nothing rather than one that reports an error.
//
// The project cross-builds and ships windows/amd64 and windows/arm64
// archives, so somebody can download one, run `watch`, and get five dead hook
// entries. That is a release decision -- drop the target, or quote for cmd.exe
// -- and this test exists so the decision is not made by accident: the day
// quoting is fixed, this test fails and points at the README caveat that has
// to come out with it.
//
// It is written this way because the behaviour cannot be observed by running
// the binary here: the quoting is a pure string function with no dependence on
// the runtime OS, so the defect reproduces on any platform, and a Linux VM --
// WSL included -- would exercise the POSIX path that already works.
func TestCommandOnWindowsPathsIsNotRunnable(t *testing.T) {
	for _, exe := range []string{
		`C:\Users\alice\go\bin\rashomon.exe`,
		`C:\Program Files\rashomon\rashomon.exe`,
	} {
		t.Run(exe, func(t *testing.T) {
			got := Spec{Executable: exe, InstallID: "0123456789abcdef"}.Command(EventPreToolUse)

			if !strings.HasPrefix(got, "'") {
				t.Fatalf("Command() = %q; this test pins the POSIX quoting of a Windows "+
					"path. If quoting is now correct for cmd.exe, delete this test and "+
					"the Windows caveat in README.md's Installing section with it.", got)
			}
			if !strings.Contains(got, `\`) {
				t.Errorf("Command() = %q, expected the backslashes to survive quoting", got)
			}
		})
	}
}

// TestCommandOnPosixPathsIsRunnable is the positive twin: the quoting this
// package does is correct for the platform it was written for, and a path
// needing no quoting gets none.
func TestCommandOnPosixPathsIsRunnable(t *testing.T) {
	plain := Spec{Executable: "/usr/local/bin/rashomon", InstallID: "abc"}.Command(EventPreToolUse)
	if strings.Contains(plain, "'") {
		t.Errorf("a path needing no quoting was quoted: %q", plain)
	}
	spaced := Spec{Executable: "/opt/my tools/rashomon", InstallID: "abc"}.Command(EventPreToolUse)
	if !strings.HasPrefix(spaced, `'/opt/my tools/rashomon'`) {
		t.Errorf("a path with a space was not quoted for the shell: %q", spaced)
	}
}
