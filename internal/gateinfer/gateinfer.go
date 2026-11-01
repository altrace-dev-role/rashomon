// Package gateinfer names a project's test command from its own files when
// nothing better is known (card GATE-INFER1): no spec declares a test gate
// and the project index has learned no test runner. It reads the project's
// CI first, the project's own declaration of what must pass, then a
// Makefile's test target and package.json's test script, then the
// manifests, and names the file it read. What it names is an inference,
// never authoritative: a declared spec gate always wins, and the first
// green run of the command makes it a learned runner.
//
// Cost: one Lstat per candidate file, one ReadDir of .github/workflows and
// one of the root, and a read of at most MaxRead bytes of each file whose
// content decides, line by line through one MaxLine buffer, never the whole
// file at once; a JSON manifest, which jsonscan reads whole, is read only up
// to MaxJSON. It never walks below the root, so a subproject's command is
// unknown, never guessed.
package gateinfer

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
)

// The bounds of one file's read: MaxRead in all, MaxLine for one line (a
// longer line makes the file unknown), and MaxJSON for a JSON manifest
// (a larger one is unknown).
const (
	MaxRead = 256 << 10
	MaxLine = 64 << 10
	MaxJSON = 64 << 10
)

// Inferred is a test command read from the project's own files.
type Inferred struct {
	// Argv is the command, word for word as the file gives it, or the
	// ecosystem's own test command for a manifest.
	Argv []string
	// Dir is the directory, relative to the root, that a CI line's leading
	// `cd DIR` runs the command in; "" for the root.
	Dir string
	// From is the file it came from, relative to the root, slash-separated.
	From string
	// More is how many further test commands the same file names.
	More int
	// Declared is whether the project itself names this command: a CI step,
	// a Makefile test target, a test script, or a test framework's own
	// config. A command read from a manifest's presence alone is a
	// convention, which goes to the user and never to the model.
	Declared bool
}

// commands is the closed table of test commands, by their leading words:
// the only commands a CI line may name, and the rows every manifest's
// command starts with (TestGI1_EveryCommandIsInTheClosedTable).
var commands = [][]string{
	{"go", "test"},
	{"pytest"}, {"python", "-m", "pytest"}, {"python3", "-m", "pytest"},
	{"npm", "test"}, {"npm", "run", "test"}, {"yarn", "test"}, {"pnpm", "test"},
	{"jest"}, {"npx", "jest"}, {"vitest"}, {"npx", "vitest"},
	{"mvn", "test"}, {"./mvnw", "test"}, {"gradle", "test"}, {"./gradlew", "test"},
	{"ctest"}, {"make", "test"},
	{"dotnet", "test"},
	{"vendor/bin/phpunit"}, {"phpunit"}, {"composer", "test"},
	{"bundle", "exec", "rspec"}, {"rspec"},
	{"cargo", "test"},
	{"swift", "test"},
	{"flutter", "test"}, {"dart", "test"},
}

// manifest is one fallback row: the file whose presence (and, where
// decide is set, whose content) names an ecosystem's test command.
type manifest struct {
	file     string
	argv     []string
	declared bool
	decide   func(root, file string) ([]string, string, bool)
}

// manifests is the fallback, in order. A row with decide may read the file
// and refuse it (a pyproject with no pytest) or name another command or
// file (mvnw beside pom.xml). Built on its first call and never at init
// (RES-2): its decide rows are closures over the readers.
func manifests() []manifest {
	manifestsOnce.Do(func() {
		manifestsOnce.v = []manifest{
			{file: "go.mod", argv: []string{"go", "test", "./..."}},
			{file: "pytest.ini", argv: []string{"pytest"}, declared: true},
			{file: "conftest.py", argv: []string{"pytest"}, declared: true},
			{file: "pyproject.toml", argv: []string{"pytest"}, declared: true, decide: contains("[tool.pytest")},
			{file: "setup.cfg", argv: []string{"pytest"}, declared: true, decide: contains("[tool:pytest]")},
			{file: "Cargo.toml", argv: []string{"cargo", "test"}},
			{file: "pom.xml", argv: []string{"mvn", "test"}, decide: wrapper("mvnw", []string{"./mvnw", "test"})},
			{file: "build.gradle.kts", argv: []string{"gradle", "test"}, decide: wrapper("gradlew", []string{"./gradlew", "test"})},
			{file: "build.gradle", argv: []string{"gradle", "test"}, decide: wrapper("gradlew", []string{"./gradlew", "test"})},
			{file: "CMakeLists.txt", argv: []string{"ctest"}, declared: true, decide: contains("enable_testing(", "add_test(")},
			{file: "composer.json", argv: []string{"composer", "test"}, declared: true, decide: composer},
			{file: "Gemfile", argv: []string{"bundle", "exec", "rspec"}, decide: rspec},
			{file: "Package.swift", argv: []string{"swift", "test"}},
			{file: "pubspec.yaml", argv: []string{"dart", "test"}, decide: flutter},
		}
	})
	return manifestsOnce.v
}

// manifestsOnce holds what manifests returns.
var manifestsOnce struct {
	sync.Once
	v []manifest
}

// Infer is the test command of the project at root, and whether one was
// found: the CI's, then the Makefile's test target, then package.json's
// test script, then the first manifest that declares one.
func Infer(root string) (Inferred, bool) {
	if in, ok := fromCI(root); ok {
		return in, true
	}
	if found, _ := scan(root, "Makefile", makeTest()); found {
		return Inferred{Argv: []string{"make", "test"}, From: "Makefile", Declared: true}, true
	}
	if body, ok := readJSON(root, "package.json"); ok && scriptsTest(body) {
		argv := []string{"npm", "test"}
		switch {
		case exists(root, "pnpm-lock.yaml"):
			argv = []string{"pnpm", "test"}
		case exists(root, "yarn.lock"):
			argv = []string{"yarn", "test"}
		}
		return Inferred{Argv: argv, From: "package.json", Declared: true}, true
	}
	for _, m := range manifests() {
		if !exists(root, m.file) {
			continue
		}
		if m.decide == nil {
			return Inferred{Argv: m.argv, From: m.file, Declared: m.declared}, true
		}
		if argv, from, ok := m.decide(root, m.file); ok {
			if argv == nil {
				argv = m.argv
			}
			if from == "" {
				from = m.file
			}
			return Inferred{Argv: argv, From: from, Declared: m.declared}, true
		}
	}
	if f, ok := dotnet(root); ok {
		return Inferred{Argv: []string{"dotnet", "test"}, From: f}, true
	}
	return Inferred{}, false
}

// fromCI is the first test command a GitHub workflow (in name order) or
// .gitlab-ci.yml runs, with how many more the same file runs.
func fromCI(root string) (Inferred, bool) {
	var files []string
	if es, err := os.ReadDir(filepath.Join(root, ".github", "workflows")); err == nil {
		for _, e := range es {
			if n := e.Name(); e.Type().IsRegular() && (strings.HasSuffix(n, ".yml") || strings.HasSuffix(n, ".yaml")) {
				files = append(files, path.Join(".github/workflows", n))
			}
		}
	}
	files = append(files, ".gitlab-ci.yml")
	for _, f := range files {
		var r runLines
		if _, ok := scan(root, f, r.line); ok && len(r.out) > 0 {
			return Inferred{Argv: r.out[0].argv, Dir: r.out[0].dir, From: f, More: len(r.out) - 1, Declared: true}, true
		}
	}
	return Inferred{}, false
}

// runLines reads the test commands a CI file runs, one line at a time, in
// file order: the value of a `run:` key, each line of a `run: |` block, and
// each item of a `script:` list, each read by command's closed grammar.
// block and list are the key's indent while inside its block or list, and
// one more than it (0) outside.
type runLines struct {
	out         []found
	block, list int
}

// line reads one line of the file; it always asks for the next.
func (r *runLines) line(line string) bool {
	text := strings.TrimSpace(line)
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if text == "" || strings.HasPrefix(text, "#") {
		return true
	}
	if r.block > 0 && indent < r.block {
		r.block = 0
	}
	if r.list > 0 && (indent < r.list-1 || indent == r.list-1 && !strings.HasPrefix(text, "-")) {
		r.list = 0
	}
	var cmd string
	item := strings.TrimSpace(strings.TrimPrefix(text, "-"))
	switch {
	case r.block > 0:
		cmd = text
	case r.list > 0 && strings.HasPrefix(text, "-"):
		cmd = item
	case strings.HasPrefix(item, "run:"):
		v := strings.TrimSpace(strings.TrimPrefix(item, "run:"))
		if v == "|" || v == "|-" || v == ">" || v == ">-" {
			r.block = indent + 1
			return true
		}
		cmd = v
	case item == "script:":
		r.list = indent + 1
		return true
	}
	if f, ok := command(unquote(cmd)); ok {
		r.out = append(r.out, f)
	}
	return true
}

// found is one test command a CI line runs, and the directory a leading
// `cd DIR` moved it to ("" for the checkout's root).
type found struct {
	argv []string
	dir  string
}

// command is the test command a CI line runs, read by a closed grammar
// that guesses nothing: POSIX quoting makes words, an unquoted ` #` ends
// the line, unquoted `&&` and `;` split it into simple commands, a `cd DIR`
// among them sets the directory, and the first whose leading words are a
// row of the table is the command. Globs stay literal words. A pipe, `||`,
// a background `&`, a redirection or heredoc, a subshell, a substitution
// ($( ) or backticks) or a variable as a command's first word makes the
// line unknown, never a guess.
func command(line string) (found, bool) {
	segs, ok := segments(line)
	if !ok {
		return found{}, false
	}
	var dir string
	for _, argv := range segs {
		switch {
		case len(argv) == 0:
		case argv[0] == "cd" && len(argv) == 2:
			if dir = path.Join(dir, argv[1]); path.IsAbs(dir) || strings.HasPrefix(dir, "..") || strings.Contains(dir, "$") {
				return found{}, false // outside the checkout, or not known
			}
		case strings.Contains(argv[0], "$"):
			return found{}, false
		case slices.ContainsFunc(commands, func(c []string) bool { return hasPrefix(argv, c) }):
			return found{argv: argv, dir: dir}, true
		}
	}
	return found{}, false
}

// segments is line's simple commands as words, split on unquoted `&&` and
// `;`, or false for any construct command refuses.
func segments(line string) ([][]string, bool) {
	var segs [][]string
	var argv []string
	var word strings.Builder
	inWord := false
	end := func() {
		if inWord {
			argv = append(argv, word.String())
			word.Reset()
			inWord = false
		}
	}
	split := func() {
		end()
		segs = append(segs, argv)
		argv = nil
	}
	r := []rune(line)
	for i := 0; i < len(r); i++ {
		c := r[i]
		switch {
		case c == '\\':
			if i+1 < len(r) {
				i++
				word.WriteRune(r[i])
				inWord = true
			}
		case c == '\'':
			j := i + 1
			for j < len(r) && r[j] != '\'' {
				j++
			}
			if j == len(r) {
				return nil, false
			}
			word.WriteString(string(r[i+1 : j]))
			inWord, i = true, j
		case c == '"':
			j := i + 1
			for ; j < len(r) && r[j] != '"'; j++ {
				switch {
				case r[j] == '`' || r[j] == '$' && j+1 < len(r) && r[j+1] == '(':
					return nil, false
				case r[j] == '\\' && j+1 < len(r) && strings.ContainsRune("$`\"\\", r[j+1]):
					j++
				}
				word.WriteRune(r[j])
			}
			if j == len(r) {
				return nil, false
			}
			inWord, i = true, j
		case c == ' ' || c == '\t':
			end()
		case c == '#' && !inWord:
			split()
			return segs, true
		case c == ';':
			split()
		case c == '&' && i+1 < len(r) && r[i+1] == '&':
			split()
			i++
		case strings.ContainsRune("&|<>()`", c), c == '$' && i+1 < len(r) && r[i+1] == '(':
			return nil, false
		default:
			word.WriteRune(c)
			inWord = true
		}
	}
	split()
	return segs, true
}

// unquote strips one pair of matching quotes around a YAML scalar.
func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// hasPrefix reports whether argv starts with row's words.
func hasPrefix(argv, row []string) bool {
	return len(argv) >= len(row) && slices.Equal(argv[:len(row)], row)
}

// makeTest is a line reader that stops at a Makefile line declaring a test
// target, so scan says stopped only for a file that has one.
func makeTest() func(string) bool {
	return func(line string) bool {
		name, rest, ok := strings.Cut(line, ":")
		return !(ok && strings.TrimRight(name, " \t") == "test" && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(rest, "="))
	}
}

// npmPlaceholder is the test script npm init writes, which tests nothing.
const npmPlaceholder = "echo \"Error: no test specified\" && exit 1"

// scriptsTest reports a package.json whose scripts.test is a string other
// than npm's placeholder. The file is located with jsonscan, never decoded.
func scriptsTest(body []byte) bool {
	s, ok := script(body)
	return ok && strings.TrimSpace(s) != "" && s != npmPlaceholder
}

// script is a JSON manifest's scripts.test, read with jsonscan.
func script(body []byte) (string, bool) {
	if !json.Valid(body) {
		return "", false
	}
	top := jsonscan.ValueSpan(body)
	scripts, err := jsonscan.Lookup(body, top, "scripts")
	if err != nil || !scripts.Present() {
		return "", false
	}
	test, err := jsonscan.Lookup(body, scripts, "test")
	if err != nil || !test.Present() {
		return "", false
	}
	out, _, err := jsonscan.Head(body, test, MaxRead, make([]byte, 0, 256))
	if err != nil {
		return "", false
	}
	return string(out), true
}

// contains is a decide that accepts the manifest when a line of it holds
// any of marks.
func contains(marks ...string) func(string, string) ([]string, string, bool) {
	return func(root, file string) ([]string, string, bool) {
		return nil, "", hasLine(root, file, marks...)
	}
}

// hasLine reports whether a line of root/rel holds any of marks, reading no
// further than the first that does.
func hasLine(root, rel string, marks ...string) bool {
	found, _ := scan(root, rel, func(line string) bool {
		return !slices.ContainsFunc(marks, func(m string) bool { return strings.Contains(line, m) })
	})
	return found
}

// wrapper is a decide that names the project's own wrapper script when it
// sits beside the manifest.
func wrapper(script string, argv []string) func(string, string) ([]string, string, bool) {
	return func(root, _ string) ([]string, string, bool) {
		if exists(root, script) {
			return argv, "", true
		}
		return nil, "", true
	}
}

// composer names `composer test` for a test script, else phpunit when its
// config is there; composer.json alone names nothing.
func composer(root, file string) ([]string, string, bool) {
	if body, ok := readJSON(root, file); ok {
		if s, ok := script(body); ok && strings.TrimSpace(s) != "" {
			return nil, "", true
		}
	}
	for _, f := range []string{"phpunit.xml", "phpunit.xml.dist"} {
		if exists(root, f) {
			return []string{"vendor/bin/phpunit"}, f, true
		}
	}
	return nil, "", false
}

// rspec names `bundle exec rspec` only beside a spec directory.
func rspec(root, _ string) ([]string, string, bool) {
	fi, err := os.Lstat(filepath.Join(root, "spec"))
	return nil, "", err == nil && fi.IsDir()
}

// flutter names `flutter test` for a package on the Flutter SDK, else the
// Dart default.
func flutter(root, file string) ([]string, string, bool) {
	if hasLine(root, file, "sdk: flutter") {
		return []string{"flutter", "test"}, "", true
	}
	return nil, "", true
}

// dotnet is the first .sln or .csproj at the root, in name order.
func dotnet(root string) (string, bool) {
	es, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	for _, e := range es {
		if n := e.Name(); e.Type().IsRegular() && (strings.HasSuffix(n, ".sln") || strings.HasSuffix(n, ".csproj")) {
			return n, true
		}
	}
	return "", false
}

// exists reports a regular file or directory at root/rel, by one Lstat.
func exists(root, rel string) bool {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

// open is the regular file root/rel, by one Lstat and one open.
func open(root, rel string) (*os.File, bool) {
	p := filepath.Join(root, filepath.FromSlash(rel))
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
		return nil, false
	}
	f, err := os.Open(p)
	return f, err == nil
}

// scan feeds each line of the regular file root/rel, of its first MaxRead
// bytes, to each until each returns false: stopped says it did, and ok that
// the lines each saw were read, false for no such file, a read error or a
// line over MaxLine.
func scan(root, rel string, each func(line string) bool) (stopped, ok bool) {
	f, ok := open(root, rel)
	if !ok {
		return false, false
	}
	defer f.Close()
	sc := bufio.NewScanner(io.LimitReader(f, MaxRead))
	sc.Buffer(make([]byte, 0, 4<<10), MaxLine)
	for sc.Scan() {
		if !each(sc.Text()) {
			return true, true
		}
	}
	return false, sc.Err() == nil
}

// readJSON is the regular file root/rel whole when it is at most MaxJSON
// bytes, for jsonscan, which reads a document whole; a larger one is unknown.
func readJSON(root, rel string) ([]byte, bool) {
	f, ok := open(root, rel)
	if !ok {
		return nil, false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxJSON+1))
	return b, err == nil && len(b) <= MaxJSON
}
