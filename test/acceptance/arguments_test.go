package acceptance

// A command refuses an argument it does not know, and does nothing else.
//
// watch, pause, resume, status and version once dropped whatever followed
// them, so the first thing a person types to learn what a command does ran
// the command: `rashomon watch --help` wrote eight hook entries into
// settings.json and minted a store, `pause --help` paused recording, `resume
// --help` resumed it, and `status --help` printed the status. The commands
// that take flags -- detach, report, digest, forget, spend -- already refused
// an argument they did not know with "unknown argument" and exit 1, and these
// now answer the same way.
//
// --help is not special. No command the README's table or the usage screen
// lists takes it in this release; `rashomon --help` is where the usage lives,
// and the README says so.
// TestArguments_EveryListedCommandRefusesHelp holds that sentence to the
// README's table and the usage screen themselves, so a command added to
// either inherits the rule.

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// strayArguments are what each command is tried with: the help request in
// both spellings a person types first, and arguments nobody defined, shaped
// like a flag and not.
var strayArguments = [][]string{{"--help"}, {"-h"}, {"--not-a-flag"}, {"now"}}

// refusesStrayArguments runs cmd once per stray argument on the machine e
// already is, and fails unless every run exits 1, names the argument it
// refused, prints nothing on stdout, and leaves the store and the settings
// file exactly as they were -- the whole of what any of these commands could
// change. That comparison is a measurement only if the command, run plainly,
// would have changed something; each caller shows so afterwards, on the same
// machine.
func refusesStrayArguments(t *testing.T, e *env, cmd string) {
	t.Helper()
	for _, stray := range strayArguments {
		args := append([]string{cmd}, stray...)
		storeBefore, settingsBefore := walkStore(t, e.home), e.settingsBytes()

		res := e.run("", nil, args...)

		if res.exitCode != 1 {
			t.Errorf("rashomon %s: exit %d, want 1", strings.Join(args, " "), res.exitCode)
		}
		if want := "unknown argument " + strconv.Quote(stray[0]); !strings.Contains(res.stderr, want) {
			t.Errorf("rashomon %s: stderr does not say %s: %q", strings.Join(args, " "), want, res.stderr)
		}
		if res.stdout != "" {
			t.Errorf("rashomon %s printed on stdout, as though it ran:\n%s", strings.Join(args, " "), res.stdout)
		}
		if !reflect.DeepEqual(walkStore(t, e.home), storeBefore) {
			t.Errorf("rashomon %s changed the store", strings.Join(args, " "))
		}
		if !bytes.Equal(e.settingsBytes(), settingsBefore) {
			t.Errorf("rashomon %s changed %s:\n%s", strings.Join(args, " "), e.settingsPath(), e.settingsBytes())
		}
	}
}

// TestArguments_WatchInstallsNothingWhenGivenOne: on a clean machine no stray
// argument writes a settings file or a store. Plain watch, on the same
// machine, then writes both.
func TestArguments_WatchInstallsNothingWhenGivenOne(t *testing.T) {
	e := newEnv(t)
	refusesStrayArguments(t, e, "watch")

	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("premise: plain watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if e.settingsBytes() == nil || len(walkStore(t, e.home)) <= 1 {
		t.Fatal("premise: plain watch wrote no settings file or no store, so the absence above proves nothing")
	}
}

// TestArguments_PauseDoesNotPauseWhenGivenOne runs on a watched machine, so a
// pause would leave both of its traces: the marker every hook reads, and the
// zero-width gap record. Plain pause then leaves them.
func TestArguments_PauseDoesNotPauseWhenGivenOne(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	refusesStrayArguments(t, e, "pause")
	if _, err := os.Stat(filepath.Join(e.home, "pause")); err == nil {
		t.Error("recording is paused after pause was given only arguments it does not know")
	}

	before := walkStore(t, e.home)
	if res := e.pause(); res.exitCode != 0 {
		t.Fatalf("premise: plain pause: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if reflect.DeepEqual(walkStore(t, e.home), before) {
		t.Fatal("premise: plain pause changed nothing in the store either, so the comparison above proves nothing")
	}
}

// TestArguments_ResumeDoesNotResumeWhenGivenOne runs on a watched, paused
// machine: no stray argument lifts the pause or closes its window with a gap
// record. Plain resume then does both.
func TestArguments_ResumeDoesNotResumeWhenGivenOne(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.pause(); res.exitCode != 0 {
		t.Fatalf("pause: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	refusesStrayArguments(t, e, "resume")
	if _, err := os.Stat(filepath.Join(e.home, "pause")); err != nil {
		t.Errorf("recording is no longer paused after resume was given only arguments it does not know (stat: %v)", err)
	}

	before := walkStore(t, e.home)
	if res := e.resume(); res.exitCode != 0 {
		t.Fatalf("premise: plain resume: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if reflect.DeepEqual(walkStore(t, e.home), before) {
		t.Fatal("premise: plain resume changed nothing in the store either, so the comparison above proves nothing")
	}
}

// TestArguments_StatusPrintsNoStatusWhenGivenOne: status writes nothing
// whatever it is given, so the output is where running anyway would show.
// Plain status then prints it.
func TestArguments_StatusPrintsNoStatusWhenGivenOne(t *testing.T) {
	e := newEnv(t)
	refusesStrayArguments(t, e, "status")

	res := e.status()
	if res.exitCode != 0 || !strings.Contains(res.stdout, "store: "+e.home) {
		t.Fatalf("premise: plain status: exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
	}
}

// TestArguments_VersionPrintsNoVersionWhenGivenOne, in all three spellings:
// `version --json` printing plain text would be a flag silently dropped,
// which is what every other command refuses to do.
func TestArguments_VersionPrintsNoVersionWhenGivenOne(t *testing.T) {
	for _, spelling := range []string{"version", "--version", "-v"} {
		t.Run(spelling, func(t *testing.T) {
			e := newEnv(t)
			refusesStrayArguments(t, e, spelling)

			res := e.run("", nil, spelling)
			if res.exitCode != 0 || strings.TrimSpace(res.stdout) == "" {
				t.Fatalf("premise: plain %s: exit %d, stdout %q, stderr %q", spelling, res.exitCode, res.stdout, res.stderr)
			}
		})
	}
}

// readmeTableCommand is a command named in the first cell of a README table
// row; usageCommand one the usage screen offers a person.
var (
	readmeTableCommand = regexp.MustCompile("`rashomon ([a-z]+)")
	usageCommand       = regexp.MustCompile(`^  rashomon ([a-z]+)`)
)

// listedCommands is every command the README's table or the usage screen
// offers a person, read from the two texts rather than restated here. The
// usage screen's own "invoked by Claude Code" section ends the read: those
// entries always exit 0, whatever they are given, because a hook that exits
// non-zero is rendered as an error in the user's session.
func listedCommands(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}

	readme, err := os.ReadFile(filepath.Join(moduleRoot, "README.md"))
	if err != nil {
		t.Fatalf("reading README: %v", err)
	}
	for _, line := range strings.Split(string(readme), "\n") {
		if !strings.HasPrefix(line, "| `rashomon ") {
			continue
		}
		firstCell, _, _ := strings.Cut(strings.TrimPrefix(line, "| "), " | ")
		for _, m := range readmeTableCommand.FindAllStringSubmatch(firstCell, -1) {
			seen[m[1]] = true
		}
	}

	e := newEnv(t)
	res := e.run("", nil, "--help")
	if res.exitCode != 0 {
		t.Fatalf("rashomon --help: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	for _, line := range strings.Split(res.stdout, "\n") {
		if strings.HasPrefix(line, "invoked by Claude Code") {
			break
		}
		if m := usageCommand.FindStringSubmatch(line); m != nil {
			seen[m[1]] = true
		}
	}

	var out []string
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// TestArguments_EveryListedCommandRefusesHelp holds the README's sentence --
// no command takes --help in this release, each refuses it like any argument
// it does not know -- to every command the README or the usage screen lists.
func TestArguments_EveryListedCommandRefusesHelp(t *testing.T) {
	cmds := listedCommands(t)
	// The read first, both ways: a parse that found nothing would make every
	// assertion below pass, and one that reached the hook-invoked section
	// would demand an exit 1 those entries must never give.
	for _, want := range []string{"watch", "detach", "pause", "resume", "status", "report", "digest", "spend", "forget", "version"} {
		if !slices.Contains(cmds, want) {
			t.Errorf("the README table and usage screen no longer list %q (read %v), so this test no longer covers it", want, cmds)
		}
	}
	for _, never := range []string{"hook", "post", "probe", "recap", "env", "run"} {
		if slices.Contains(cmds, never) {
			t.Errorf("listed commands include %q, which is not offered to a person (read %v)", never, cmds)
		}
	}

	for _, cmd := range cmds {
		for _, help := range []string{"--help", "-h"} {
			t.Run(cmd+" "+help, func(t *testing.T) {
				e := newEnv(t)
				res := e.run("", nil, cmd, help)
				if res.exitCode != 1 || res.stdout != "" {
					t.Errorf("rashomon %s %s: exit %d, stdout %q; want exit 1 and nothing on stdout", cmd, help, res.exitCode, res.stdout)
				}
				if want := "rashomon: unknown argument " + strconv.Quote(help) + "\n"; res.stderr != want {
					t.Errorf("rashomon %s %s: stderr %q, want %q", cmd, help, res.stderr, want)
				}
			})
		}
	}
}
