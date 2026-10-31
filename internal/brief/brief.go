// Package brief is the project brief at session start (card BRIEF1): what
// the project's own files say about it, read at the moment the session
// starts, every line naming the file or record it came from. A source that
// is missing or cannot be read gives that line's none form, never an
// inference: no manifest is guessed from file extensions, no test command
// from a directory's contents. probe start prints it to the user and the
// `rashomon brief` verb prints the same text.
package brief

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The kinds of a brief's facts, one line each, in the order Build gives them;
// the spec trust line only under an unsigned bindings file (trustFact).
const (
	KindInstructions = "instructions"
	KindBuild        = "build"
	KindTests        = "tests"
	KindSpec         = "spec"
	KindSpecTrust    = "spec trust"
	KindTask         = "task"
	KindPackages     = "packages"
	KindVerify       = "verify"
	// KindLastSession is the line that replaces the verify line at startup
	// when the project's previous session was looked for (Brief.WithVerify).
	KindLastSession = "last session"
	// LastSessionLead is the text receipt.PreviousLine opens with, which the
	// fact's kind says already.
	LastSessionLead = "last session: "
)

// The walk's bounds: MaxWalkEntries directory entries read in all and
// WalkBudget of its own time, past either of which it stops and the packages
// line says so; directories deeper than maxDepth below the root are not read.
const (
	MaxWalkEntries = 2000
	WalkBudget     = 20 * time.Millisecond
	maxDepth       = 4
	walkChunk      = 256
)

// Width is the column bound of every rendered line, and header the first
// line's text before the root.
const (
	Width  = 80
	header = "rashomon brief: "
)

// modHead is how much of go.mod is read for its module line.
const modHead = 4 << 10

// manifests is the closed list of build manifests looked for at the root, in
// the order the build line names them.
var manifests = []string{"go.mod", "package.json", "pyproject.toml", "setup.py", "Cargo.toml", "pom.xml",
	"build.gradle", "build.gradle.kts", "Makefile", "CMakeLists.txt", "Gemfile", "mix.exs", "composer.json"}

// Fact is one line of the brief: its kind, its text and the file or record
// the text was read from; Next, when not "", is a line under it, the
// command the spec trust line names (D189).
type Fact struct {
	Kind, Text, Source string
	Next               string
}

// Brief is a project's facts, one per kind, and the root they were read at,
// with what the facts were read from as fields, for a reader that cannot take
// a fact's text as it stands (PREFLIGHT1 renders its own): the instruction
// files and manifests found, as paths relative to the root (a file outside
// it, the user's CLAUDE.md, as its own path), go.mod's module ("" for none),
// and the packages walk's count of directories with Go files, whether it was
// capped, and those directories relative to the root, sorted; the walk's cap
// bounds them.
type Brief struct {
	Root         string
	Facts        []Fact
	Instructions []string
	Manifests    []string
	Module       string
	GoDirs       int
	WalkCapped   bool
	Dirs         []string
}

// PluginReport is the verify line's command inside the plugin: the slash
// command is the plugin's name (install.PluginName, what its manifest
// declares) and its report skill, and it reads the last session.
const PluginReport = "/" + install.PluginName + ":report last"

// Pointer is the report command a brief's verify line names: PluginReport
// when Plugin, probe start running as the plugin's entry, the one origin
// where that command exists; else the CLI's report of SessionID, the
// payload's session ("" at a terminal, where the verb has none).
type Pointer struct {
	Plugin    bool
	SessionID string
}

// Build reads the brief of the project at root: its instruction files, its
// manifests, its test command (the spec's declared test gate, else what the
// project index under storeRoot has seen; "" reads no index), the spec sp
// (nil for none), whether the trust bindings under storeRoot are unsigned,
// the spec's task, and its Go package directories. It reads names, sizes and
// go.mod's module line; it never reads an instruction file's content. The
// verify line names the report command report points at.
func Build(root, storeRoot string, report Pointer, sp *spec.Compiled) Brief {
	b := Brief{Root: root}
	instructions := b.instructionsFact()
	build := b.buildFact()
	packages := b.packagesFact()
	b.Facts = []Fact{instructions, build, testsFact(root, storeRoot, sp), specFact(root, sp)}
	if f, ok := trustFact(storeRoot, sp); ok {
		b.Facts = append(b.Facts, f)
	}
	b.Facts = append(b.Facts, taskFact(root, sp), packages, verifyFact(report))
	return b
}

// WithVerify is b with its verify line, the last fact, replaced by f; a brief
// whose last fact is not the verify line is returned as it is.
func (b Brief) WithVerify(f Fact) Brief {
	if n := len(b.Facts); n == 0 || b.Facts[n-1].Kind != KindVerify {
		return b
	}
	facts := append([]Fact{}, b.Facts...)
	facts[len(facts)-1] = f
	b.Facts = facts
	return b
}

// verifyFact points at this session's report: the plugin's command, which
// takes no id, or the CLI's. An empty --session renders every session
// (report/gather), so the CLI line names the id or none: a session id is 36
// columns, which leaves the line no room for more.
func verifyFact(report Pointer) Fact {
	switch {
	case report.Plugin:
		return newFact(KindVerify, PluginReport, "the plugin's report command")
	case report.SessionID == "":
		return newFact(KindVerify, "rashomon status for coverage; rashomon report --session ID", "the rashomon command")
	}
	return newFact(KindVerify, "rashomon report --session "+report.SessionID, "the payload's session_id")
}

// Render is the brief as the user reads it: a first line naming the root,
// then one line per fact, "  <kind>: <text>", and a fact's Next under it,
// "    <next>". A fact's line past Width columns ends in "..." at Width,
// which Build's texts are sized not to need; Next is whole, since a command
// cut short runs nothing, and the binary's path in it can pass Width.
func Render(b Brief) string {
	lines := []string{header + shown(b.Root, Width-len(header))}
	for _, f := range b.Facts {
		lines = append(lines, cut("  "+f.Kind+": "+f.Text, Width))
		if f.Next != "" {
			lines = append(lines, "    "+f.Next)
		}
	}
	return strings.Join(lines, "\n")
}

// Resumed is the one line printed at resume in place of the brief, naming
// the verb by this binary (self).
func Resumed(root string) string {
	return header + "resumed " + shown(root, Width-len(header)) + "; " + printable(self()) + " brief for the full brief"
}

// executable is the path this process runs as; a test replaces it.
var executable = os.Executable

// self is the command that runs this binary at a terminal (D189): the path
// the process runs as, true for both installs, since a plugin's binary is
// off PATH and a settings install may be too; quoted for a shell when it
// holds a character a shell reads; rashomon when there is no path.
func self() string {
	exe, err := executable()
	if err != nil || exe == "" {
		return "rashomon"
	}
	if strings.ContainsAny(exe, " \t'\"\\$`!*?[](){}<>|&;#~") {
		return "'" + strings.ReplaceAll(exe, "'", `'\''`) + "'"
	}
	return exe
}

// room is the columns a fact of kind k has for its text on a rendered line.
func room(k string) int { return Width - len("  "+k+": ") }

// newFact is a fact whose text has every rune that does not print replaced:
// a newline in a task goal or a path would add a line to the brief, and an
// escape byte would reach the user's terminal.
func newFact(kind, text, source string) Fact {
	return Fact{Kind: kind, Text: printable(text), Source: source}
}

func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return '?'
	}, s)
}

// cut is s at most n runes long, its tail replaced by "..." when it was over.
func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-3]) + "..."
}

// shown is root at most n runes long: whole when it fits, else "..." and
// its tail, from a separator when the tail holds one.
func shown(root string, n int) string {
	root = printable(root)
	r := []rune(root)
	if len(r) <= n {
		return root
	}
	tail := string(r[len(r)-(n-3):])
	if i := strings.IndexRune(tail, filepath.Separator); i > 0 {
		tail = tail[i:]
	}
	return "..." + tail
}

// rel is p relative to root, slash-separated, when p is under root; else p.
func rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return p
	}
	return filepath.ToSlash(r)
}

// size is n bytes as the instructions line prints it: "812 B", "2.1 KiB",
// "12 KiB", "1.5 MiB".
func size(n int64) string {
	if n < 1024 {
		return strconv.FormatInt(n, 10) + " B"
	}
	f, unit := float64(n)/1024, "KiB"
	if f >= 1024 {
		f, unit = f/1024, "MiB"
	}
	if f < 10 {
		return fmt.Sprintf("%.1f %s", f, unit)
	}
	return fmt.Sprintf("%.0f %s", f, unit)
}

// instructionsFact names each instruction file present with its size: the
// root's CLAUDE.md, .claude/CLAUDE.md and AGENTS.md, and the user's
// CLAUDE.md in Claude Code's configuration directory. A symlink is named
// "(link)" and not followed. When the sized list does not fit its line, the
// names alone do.
func (b *Brief) instructionsFact() Fact {
	root := b.Root
	type file struct{ name, path string }
	files := []file{{"CLAUDE.md", filepath.Join(root, "CLAUDE.md")},
		{".claude/CLAUDE.md", filepath.Join(root, ".claude", "CLAUDE.md")}, {"AGENTS.md", filepath.Join(root, "AGENTS.md")}}
	// The user's directory is the project's own .claude when the project is
	// the home directory; that file is already named.
	if dir, err := settings.ConfigDir(); err == nil && filepath.Join(dir, "CLAUDE.md") != files[1].path {
		files = append(files, file{"user CLAUDE.md", filepath.Join(dir, "CLAUDE.md")})
	}
	var sized, names, paths, all []string
	for _, f := range files {
		all = append(all, f.path)
		fi, err := os.Lstat(f.path)
		switch {
		case err != nil:
			continue
		case fi.Mode()&fs.ModeSymlink != 0:
			sized = append(sized, f.name+" (link)")
		case fi.Mode().IsRegular():
			sized = append(sized, f.name+" "+size(fi.Size()))
		default:
			continue
		}
		names, paths = append(names, f.name), append(paths, f.path)
		b.Instructions = append(b.Instructions, rel(root, f.path))
	}
	if len(names) == 0 {
		var looked []string
		for _, f := range files {
			looked = append(looked, f.name)
		}
		return newFact(KindInstructions, "none ("+strings.Join(looked, ", ")+")", strings.Join(all, ", "))
	}
	text := strings.Join(sized, ", ")
	if utf8.RuneCountInString(text) > room(KindInstructions) {
		text = strings.Join(names, ", ")
	}
	return newFact(KindInstructions, text, strings.Join(paths, ", "))
}

// buildFact names each manifest of the closed list present at the root, and
// go.mod's module.
func (b *Brief) buildFact() Fact {
	var names, paths []string
	for _, m := range manifests {
		p := filepath.Join(b.Root, m)
		fi, err := os.Lstat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		name := m
		switch {
		case fi.Mode()&fs.ModeSymlink != 0:
			name += " (link)"
		case m == "go.mod":
			if b.Module = module(p); b.Module != "" {
				name += " (module " + b.Module + ")"
			}
		}
		b.Manifests = append(b.Manifests, m)
		names, paths = append(names, name), append(paths, p)
	}
	if len(names) == 0 {
		return newFact(KindBuild, "no manifest at the root", b.Root)
	}
	return newFact(KindBuild, strings.Join(names, ", "), strings.Join(paths, ", "))
}

// module is go.mod's module path from its first modHead bytes, "" when no
// whole line there declares one.
func module(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck // read-only
	b, err := io.ReadAll(io.LimitReader(f, modHead))
	if err != nil {
		return ""
	}
	// A line the head cuts is not read: its module path could be cut too.
	if len(b) == modHead {
		b = b[:max(bytes.LastIndexByte(b, '\n'), 0)]
	}
	for _, line := range strings.Split(string(b), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module")
		if !ok || rest == "" || rest[0] != ' ' && rest[0] != '\t' {
			continue
		}
		rest, _, _ = strings.Cut(rest, "//")
		rest = strings.TrimSpace(rest)
		if u, err := strconv.Unquote(rest); err == nil {
			rest = u
		}
		return rest
	}
	return ""
}

// testsFact is the spec's declared test gate, quoted with its trust; with
// none, the test-kind families the project index has seen (B2: the index
// counts runs, not greens), how many sessions the most-seen one ran in, and
// how many are required.
func testsFact(root, storeRoot string, sp *spec.Compiled) Fact {
	if sp != nil {
		var tests []spec.Declared
		for _, d := range sp.Gates {
			if d.Gate.Kind == "test" {
				tests = append(tests, d)
			}
		}
		if len(tests) > 0 {
			d := tests[0]
			trust, source := sp.Trust, ""
			if d.Source >= 0 && d.Source < len(sp.Sources) {
				trust, source = sp.Sources[d.Source].Trust, sp.Sources[d.Source].Path
			}
			tail := " (" + string(trust) + ")"
			if len(tests) > 1 {
				tail += fmt.Sprintf(" +%d more", len(tests)-1)
			}
			const lead = "spec declares "
			argv := cut(strings.Join(d.Gate.Argv, " "), room(KindTests)-len(lead)-len(tail)-2)
			return newFact(KindTests, lead+strconv.Quote(argv)+tail, source)
		}
	}
	source := "no store"
	var idx *verify.ProjectIndex
	if storeRoot != "" {
		source = verify.IndexPath(storeRoot, root)
		idx, _ = verify.ReadIndex(source)
	}
	runners, most, required := 0, 0, 0
	if idx != nil {
		now := time.Now().UnixMilli()
		for _, e := range idx.Entries {
			if e.Kind != "test" {
				continue
			}
			runners, most = runners+1, max(most, e.Sessions)
			if idx.Required(e.Family, now) {
				required++
			}
		}
	}
	if runners == 0 {
		return newFact(KindTests, "no spec gate; no test runner seen in this project yet", source)
	}
	return newFact(KindTests, fmt.Sprintf("no spec gate; %s seen in up to %s (%d required)",
		count(runners, "test runner"), count(most, "session"), required), source)
}

// count is n and noun, plural unless n is one.
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// specFact names each spec file that applies with its trust, a trusted or
// managed one with its digest's head, an untrusted one with the verb that
// trusts it, and how many files discovery refused.
func specFact(root string, sp *spec.Compiled) Fact {
	none := filepath.Join(root, ".claude", "rashomon", "spec.json")
	if sp == nil || len(sp.Sources) == 0 && len(sp.Errors) == 0 {
		return newFact(KindSpec, "none", none)
	}
	var parts, paths []string
	for _, s := range sp.Sources {
		part := rel(root, s.Path) + " " + string(s.Trust)
		if s.Trust == spec.Untrusted {
			part += ": run rashomon spec trust"
		} else {
			part += " (" + s.Digest[:min(len(s.Digest), len("sha256:")+8)] + ")"
		}
		parts, paths = append(parts, part), append(paths, s.Path)
	}
	text := strings.Join(parts, ", ")
	if len(parts) == 0 {
		text, paths = "none", []string{none}
	}
	if n := len(sp.Errors); n > 0 {
		text += " (" + count(n, "error") + ")"
	}
	return newFact(KindSpec, text, strings.Join(paths, ", "))
}

// trustFact is the spec trust line (D173): the bindings file under storeRoot
// is spec.ErrBindingsUnsigned, written by something other than `rashomon
// spec trust`, so it binds nothing and a spec the user trusted reads
// untrusted until trusted again; since D173b that includes a file over its
// size limit, not its shape, or beside an unreadable key, which the verb
// replaces, while a file that cannot be read is not named. The file is read
// only when a source reads untrusted, as discovery reads it only for a
// repository spec; storeRoot ""
// has no file. Its Next is the trust verb by this binary's path (self, D189):
// run with no PATH, it trusts the spec nearest its working directory, and the
// spec line above names that spec's path.
func trustFact(storeRoot string, sp *spec.Compiled) (Fact, bool) {
	if storeRoot == "" || sp == nil || !slices.ContainsFunc(sp.Sources, func(s spec.Source) bool { return s.Trust == spec.Untrusted }) {
		return Fact{}, false
	}
	if _, err := spec.ReadBindings(spec.OSFS{}, storeRoot); !errors.Is(err, spec.ErrBindingsUnsigned) {
		return Fact{}, false
	}
	f := newFact(KindSpecTrust, "bindings file unsigned; re-trust from a terminal:", filepath.Join(storeRoot, spec.TrustFile))
	f.Next = printable(self() + " spec trust")
	return f, true
}

// taskFact is the active task's goal, cut to its line. The task file sits
// beside the repository spec, else at the project root (spec.Discover).
func taskFact(root string, sp *spec.Compiled) Fact {
	path := filepath.Join(root, filepath.FromSlash(spec.TaskFile))
	if sp != nil {
		for _, s := range sp.Sources {
			if s.Trust != spec.Managed {
				path = filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(s.Path))), filepath.FromSlash(spec.TaskFile))
			}
		}
	}
	if sp == nil || sp.Task == nil {
		return newFact(KindTask, "none", path)
	}
	const lead = "task/current.json: "
	goal := cut(strings.Join(strings.Fields(sp.Task.Goal), " "), room(KindTask)-len(lead)-2)
	return newFact(KindTask, lead+strconv.Quote(goal), path)
}

// packagesFact counts the directories holding Go files when go.mod is at the
// root (buildFact's Manifests); it reads nothing for any other language.
func (b *Brief) packagesFact() Fact {
	switch {
	case !slices.Contains(b.Manifests, "go.mod") && len(b.Manifests) > 0:
		return newFact(KindPackages, "not read for this language", b.Root)
	case !slices.Contains(b.Manifests, "go.mod"):
		return newFact(KindPackages, "not read (no go.mod)", b.Root)
	}
	dirs, _, capped := walkGo(b.Root, time.Now)
	for _, d := range dirs {
		b.Dirs = append(b.Dirs, rel(b.Root, d))
	}
	slices.Sort(b.Dirs)
	b.GoDirs, b.WalkCapped = len(dirs), capped
	n := count(len(dirs), "directory")
	if capped {
		return newFact(KindPackages, "walk capped: at least "+n+" with Go files", b.Root)
	}
	return newFact(KindPackages, n+" with Go files (bounded walk)", b.Root)
}

// walkGo is the directories at most maxDepth below root that hold a .go
// file, found breadth first, skipping what the go tool skips (names starting
// with "." or "_", testdata) and vendor, and never following a symlink. It
// stops, capped, at MaxWalkEntries entries read or once clock reads
// WalkBudget past its first reading, checked before each read.
func walkGo(root string, clock func() time.Time) (dirs []string, entries int, capped bool) {
	start := clock()
	over := func() bool { return clock().Sub(start) >= WalkBudget }
	level := []string{root}
	for depth := 0; depth <= maxDepth && len(level) > 0; depth++ {
		var next []string
		for _, dir := range level {
			subs, hasGo, n, short := readGoDir(dir, MaxWalkEntries-entries, over)
			entries += n
			if hasGo {
				dirs = append(dirs, dir)
			}
			if short {
				return dirs, entries, true
			}
			next = append(next, subs...)
		}
		level = next
	}
	return dirs, entries, false
}

// readGoDir reads dir in chunks, at most left entries in all, asking over
// before each chunk: dir's subdirectories to walk, whether it holds a .go
// file, how many entries it read, and whether it stopped short of its end.
// A directory that cannot be opened or read is read as far as it can be.
func readGoDir(dir string, left int, over func() bool) (subs []string, hasGo bool, n int, short bool) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, false, 0, false
	}
	defer f.Close() //nolint:errcheck // read-only
	for {
		if n >= left || over() {
			return subs, hasGo, n, true
		}
		ents, err := f.ReadDir(min(walkChunk, left-n))
		n += len(ents)
		for _, e := range ents {
			name := e.Name()
			switch {
			case name[0] == '.' || name[0] == '_' || name == "testdata" || name == "vendor":
			case e.IsDir():
				subs = append(subs, filepath.Join(dir, name))
			case e.Type().IsRegular() && strings.HasSuffix(name, ".go"):
				hasGo = true
			}
		}
		if err != nil {
			return subs, hasGo, n, false
		}
	}
}
