package gateinfer

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// project writes files, a map of slash paths to contents, under a new root;
// a path ending in "/" is a directory.
func project(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, body := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if strings.HasSuffix(p, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const goMod = "module example.com/m\n\ngo 1.25\n"

// TestGI1_EachEcosystemItsOwnTestCommand (GATE-INFER1): a fresh project in
// each ecosystem names its test command and the file it came from, and a
// file that is present but declares no test (the decoys) names none, so
// unknown prints as unknown. Break: guess from a manifest's presence alone.
func TestGI1_EachEcosystemItsOwnTestCommand(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		argv  string // "" for none
		from  string
	}{
		{"go.mod", map[string]string{"go.mod": goMod}, "go test ./...", "go.mod"},
		{"pytest.ini", map[string]string{"pytest.ini": "[pytest]\n"}, "pytest", "pytest.ini"},
		{"conftest.py", map[string]string{"conftest.py": "import pytest\n"}, "pytest", "conftest.py"},
		{"pyproject with pytest", map[string]string{"pyproject.toml": "[project]\nname = \"a\"\n\n[tool.pytest.ini_options]\naddopts = \"-q\"\n"}, "pytest", "pyproject.toml"},
		{"decoy: pyproject with no pytest", map[string]string{"pyproject.toml": "[project]\nname = \"a\"\n"}, "", ""},
		{"setup.cfg with pytest", map[string]string{"setup.cfg": "[metadata]\nname = a\n\n[tool:pytest]\ntestpaths = tests\n"}, "pytest", "setup.cfg"},
		{"decoy: setup.cfg with no pytest", map[string]string{"setup.cfg": "[metadata]\nname = a\n"}, "", ""},
		{"package.json npm", map[string]string{"package.json": `{"name":"a","scripts":{"test":"jest"}}`}, "npm test", "package.json"},
		{"package.json yarn", map[string]string{"package.json": `{"scripts":{"test":"vitest run"}}`, "yarn.lock": ""}, "yarn test", "package.json"},
		{"package.json pnpm", map[string]string{"package.json": `{"scripts":{"test":"jest"}}`, "pnpm-lock.yaml": ""}, "pnpm test", "package.json"},
		{"decoy: npm's placeholder test", map[string]string{"package.json": `{"scripts":{"test":"echo \"Error: no test specified\" && exit 1"}}`}, "", ""},
		{"decoy: package.json with no test", map[string]string{"package.json": `{"scripts":{"build":"tsc"}}`}, "", ""},
		{"Cargo.toml", map[string]string{"Cargo.toml": "[package]\nname = \"a\"\n"}, "cargo test", "Cargo.toml"},
		{"pom.xml", map[string]string{"pom.xml": "<project/>\n"}, "mvn test", "pom.xml"},
		{"pom.xml with mvnw", map[string]string{"pom.xml": "<project/>\n", "mvnw": "#!/bin/sh\n"}, "./mvnw test", "pom.xml"},
		{"build.gradle", map[string]string{"build.gradle": "plugins { id 'java' }\n"}, "gradle test", "build.gradle"},
		{"build.gradle.kts with gradlew", map[string]string{"build.gradle.kts": "plugins { java }\n", "gradlew": "#!/bin/sh\n"}, "./gradlew test", "build.gradle.kts"},
		{"CMakeLists.txt with tests", map[string]string{"CMakeLists.txt": "project(a)\nenable_testing()\nadd_test(NAME t COMMAND t)\n"}, "ctest", "CMakeLists.txt"},
		{"decoy: CMakeLists.txt with no tests", map[string]string{"CMakeLists.txt": "project(a)\nadd_executable(a a.c)\n"}, "", ""},
		{"a .csproj", map[string]string{"App.csproj": "<Project/>\n"}, "dotnet test", "App.csproj"},
		{"a .sln", map[string]string{"App.sln": "\n"}, "dotnet test", "App.sln"},
		{"composer.json test script", map[string]string{"composer.json": `{"scripts":{"test":"phpunit"}}`}, "composer test", "composer.json"},
		{"composer.json and phpunit.xml.dist", map[string]string{"composer.json": `{"name":"a/b"}`, "phpunit.xml.dist": "<phpunit/>\n"}, "vendor/bin/phpunit", "phpunit.xml.dist"},
		{"decoy: composer.json alone", map[string]string{"composer.json": `{"name":"a/b"}`}, "", ""},
		{"Gemfile and spec/", map[string]string{"Gemfile": "source 'https://rubygems.org'\n", "spec/": ""}, "bundle exec rspec", "Gemfile"},
		{"decoy: Gemfile alone", map[string]string{"Gemfile": "source 'https://rubygems.org'\n"}, "", ""},
		{"Package.swift", map[string]string{"Package.swift": "// swift-tools-version:5.9\n"}, "swift test", "Package.swift"},
		{"pubspec.yaml flutter", map[string]string{"pubspec.yaml": "name: a\ndependencies:\n  flutter:\n    sdk: flutter\n"}, "flutter test", "pubspec.yaml"},
		{"pubspec.yaml dart", map[string]string{"pubspec.yaml": "name: a\n"}, "dart test", "pubspec.yaml"},
		{"Makefile test target", map[string]string{"Makefile": "build:\n\tgo build ./...\n\ntest: build\n\tgo test ./...\n"}, "make test", "Makefile"},
		{"decoy: Makefile with no test target", map[string]string{"Makefile": "build:\n\tcc -o a a.c\n"}, "", ""},
		{"decoy: nothing at all", map[string]string{"README": "a\n"}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Infer(project(t, tc.files))
			if tc.argv == "" {
				if ok {
					t.Errorf("Infer = %q from %s, want none: nothing here declares a test", got.Argv, got.From)
				}
				return
			}
			if !ok || strings.Join(got.Argv, " ") != tc.argv || got.From != tc.from {
				t.Errorf("Infer = %q from %q (ok %v), want %q from %q", got.Argv, got.From, ok, tc.argv, tc.from)
			}
		})
	}
}

// TestGI1_CIFirst (GATE-INFER1, R52(2)): the project's CI is its own
// declaration of what must pass, so a workflow's test step wins over the
// Makefile, package.json and the manifests, word for word as the step runs
// it. A step whose line needs a guess (a pipe) names nothing, and the
// manifest stands. A line that merely mentions a test command outside a run
// step is not one.
// Break: read go.mod first, and `go test -race ./...` reads as plain
// `go test ./...`.
func TestGI1_CIFirst(t *testing.T) {
	const wf = ".github/workflows/ci.yml"
	for _, tc := range []struct {
		name  string
		files map[string]string
		argv  string
		from  string
		more  int
	}{
		{"a run step", map[string]string{wf: "jobs:\n  t:\n    steps:\n      - uses: actions/checkout@v4\n      - run: go test -race ./...\n", "go.mod": goMod},
			"go test -race ./...", wf, 0},
		{"a run block", map[string]string{wf: "jobs:\n  t:\n    steps:\n      - name: Test\n        run: |\n          go vet ./...\n          go test -count=1 ./...\n", "go.mod": goMod},
			"go test -count=1 ./...", wf, 0},
		{"over the Makefile and package.json", map[string]string{wf: "steps:\n  - run: npm test\n", "Makefile": "test:\n\tnpm test\n", "package.json": `{"scripts":{"test":"jest"}}`},
			"npm test", wf, 0},
		{"two test steps", map[string]string{wf: "steps:\n  - run: pytest -q\n  - run: python -m pytest tests/slow\n"},
			"pytest -q", wf, 1},
		{"gitlab script", map[string]string{".gitlab-ci.yml": "test:\n  script:\n    - cargo test --all\n"},
			"cargo test --all", ".gitlab-ci.yml", 0},
		{"a cd and a test", map[string]string{wf: "steps:\n  - run: cd sub && go test ./...\n", "go.mod": goMod},
			"go test ./...", wf, 0},
		{"a piped line falls to the manifest", map[string]string{wf: "steps:\n  - run: go test ./... | tee out.txt\n", "go.mod": goMod},
			"go test ./...", "go.mod", 0},
		{"a step name is not a run", map[string]string{wf: "steps:\n  - name: go test ./pkg\n    uses: some/action@v1\n", "go.mod": goMod},
			"go test ./...", "go.mod", 0},
		{"the Makefile over a manifest", map[string]string{"Makefile": "test:\n\tgo test ./...\n", "go.mod": goMod},
			"make test", "Makefile", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Infer(project(t, tc.files))
			if !ok || strings.Join(got.Argv, " ") != tc.argv || got.From != tc.from || got.More != tc.more {
				t.Errorf("Infer = %q from %q, %d more (ok %v), want %q from %q, %d more", got.Argv, got.From, got.More, ok, tc.argv, tc.from, tc.more)
			}
		})
	}
}

// TestGI1_EveryCommandIsInTheClosedTable: every command Infer can name
// starts with a row of the one table the CI scan reads, so a manifest's
// command is never a command the CI scan would refuse.
func TestGI1_EveryCommandIsInTheClosedTable(t *testing.T) {
	for _, m := range manifests() {
		if !slices.ContainsFunc(commands, func(c []string) bool { return hasPrefix(m.argv, c) }) {
			t.Errorf("manifest %s names %q, which no row of the command table starts", m.file, m.argv)
		}
	}
}

// TestGI1_DeclaredOnlyWhereTheProjectSaysSo (GATE-INFER1, R55): a command
// the project names itself (a CI step, a Makefile test target, a test
// script, a test framework's config) is declared, and only that may reach
// the model; a manifest's presence alone (go.mod means go test ./...) is a
// convention for the user's eyes. Break: mark every inference declared, and
// a guess costs the model a line every Stop.
func TestGI1_DeclaredOnlyWhereTheProjectSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    map[string]string
		declared bool
	}{
		{"a CI step", map[string]string{".github/workflows/ci.yml": "steps:\n  - run: go test ./...\n"}, true},
		{"a Makefile test target", map[string]string{"Makefile": "test:\n\tgo test ./...\n"}, true},
		{"a test script", map[string]string{"package.json": `{"scripts":{"test":"jest"}}`}, true},
		{"pytest's config", map[string]string{"pytest.ini": "[pytest]\n"}, true},
		{"CMake's tests", map[string]string{"CMakeLists.txt": "enable_testing()\n"}, true},
		{"go.mod alone", map[string]string{"go.mod": goMod}, false},
		{"Cargo.toml alone", map[string]string{"Cargo.toml": "[package]\n"}, false},
		{"a .csproj alone", map[string]string{"App.csproj": "<Project/>\n"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Infer(project(t, tc.files))
			if !ok || got.Declared != tc.declared {
				t.Errorf("Infer = %+v (ok %v), want Declared %v", got, ok, tc.declared)
			}
		})
	}
}

// TestGI1_CILinesReadByAClosedGrammar (GATE-INFER1, manager 01:5x): the
// common real forms of a CI test line name the command the project runs --
// a leading cd as its directory, quoted arguments as words, a trailing
// comment dropped, a vet before it skipped, a glob kept as written -- and
// every form that would need a guess is unknown: a pipe, ||, a redirection,
// a heredoc, a subshell, a substitution, a variable as the command, a cd
// out of the checkout. Break: refuse every quote, and `pytest -k 'not
// slow'`, the usual form, falls to convention.
func TestGI1_CILinesReadByAClosedGrammar(t *testing.T) {
	for _, tc := range []struct {
		line string
		argv []string // nil for unknown
		dir  string
	}{
		{"cd backend && pytest", []string{"pytest"}, "backend"},
		{"cd a && cd b && go test ./...", []string{"go", "test", "./..."}, "a/b"},
		{"go test -race ./... # the race build", []string{"go", "test", "-race", "./..."}, ""},
		{"pytest -k 'not slow'", []string{"pytest", "-k", "not slow"}, ""},
		{`pytest -k "not slow and not db"`, []string{"pytest", "-k", "not slow and not db"}, ""},
		{"go vet ./...; go test ./...", []string{"go", "test", "./..."}, ""},
		{"pytest tests/*_test.py", []string{"pytest", "tests/*_test.py"}, ""},
		{"go test ./... | tee out.txt", nil, ""},
		{"go test ./... || true", nil, ""},
		{"go test ./... > out.txt", nil, ""},
		{"python - <<EOF", nil, ""},
		{"(cd sub && go test ./...)", nil, ""},
		{"go test $(go list ./...)", nil, ""},
		{"go test `go list ./...`", nil, ""},
		{"$GO test ./...", nil, ""},
		{"cd .. && go test ./...", nil, ""},
		{"echo 'unterminated", nil, ""},
	} {
		got, ok := command(tc.line)
		if tc.argv == nil {
			if ok {
				t.Errorf("%q names %q in %q, want unknown", tc.line, got.argv, got.dir)
			}
			continue
		}
		if !ok || !slices.Equal(got.argv, tc.argv) || got.dir != tc.dir {
			t.Errorf("%q names %q in %q (ok %v), want %q in %q", tc.line, got.argv, got.dir, ok, tc.argv, tc.dir)
		}
	}
}
