package verify

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestRC3c_PathClassTable(t *testing.T) {
	cases := []struct {
		path string
		want PathClass
		rule string
	}{
		{"internal/x/y.go", ClassSource, "source ext"},
		{"internal/x/y_test.go", ClassTest, "test name"},
		{"tests/test_a.py", ClassTest, "test segment"},
		{"src/a.spec.ts", ClassTest, "spec name"},
		{"internal/testdata/x.golden", ClassTest, "testdata segment, unknown ext"},
		{".github/workflows/ci.yaml", ClassCI, "CI segment beats config ext"},
		{"Jenkinsfile", ClassCI, "CI basename"},
		{"README.md", ClassDocs, "docs ext"},
		{"LICENSE", ClassDocs, "docs basename"},
		{"docs/example/main.go", ClassDocs, "docs root segment beats source ext"},
		{"go.mod", ClassConfig, "config basename"},
		{".env.local", ClassConfig, "dotfile"},
		{"deploy/app.yaml", ClassConfig, "config ext"},
		{"tests/fixtures/a.json", ClassTest, "test segment beats config ext"},
		{"vendor/a/b.go", ClassGenerated, "generated segment"},
		{"vendor/x_test.go", ClassGenerated, "generated beats test"},
		{"vendor/go.work", ClassGenerated, "generated beats config"},
		{"dist/app.min.js", ClassGenerated, "generated root segment and suffix together (web/app.min.js tests the suffix alone)"},
		{"build/report.md", ClassDocs, "build is not generated (D10); docs ext"},
		{"image.png", ClassSource, "fallback is source (D7)"},
		{"", ClassSource, "empty path falls back"},
		{"src/", ClassSource, "trailing slash, a directory no rule names (dist/ tests the directory rule)"},
		{"cmd/Main.GO", ClassSource, "upper-case ext, source either way under D7 (LICENSE and Jenkinsfile test lower-casing)"},
		{`src\pkg\a_test.go`, ClassTest, "backslashes and a test name, which matches either way (vendor\\a\\b.go tests the split)"},
		// Rows the mutation sweep found missing: each one kills a mutant every
		// row above lets live.
		{"web/app.min.js", ClassGenerated, "generated suffix, no generated segment"},
		{"pkg/test_a.py", ClassTest, "test_*.py, no test segment"},
		{"conftest.py", ClassTest, "conftest.py"},
		{"node_modules/pkg/.travis.yml", ClassGenerated, "generated beats CI"},
		{".github/actions/lint/main_test.go", ClassCI, "CI beats test"},
		{"docs/a_test.go", ClassTest, "test beats docs: the order is by class, not segment first"},
		{"docs/mkdocs.yml", ClassDocs, "docs beats config"},
		{".eslintrc.js", ClassConfig, "config beats source"},
		{`vendor\a\b.go`, ClassGenerated, "backslashes split segments"},
		{"dist/", ClassGenerated, "trailing slash: the last element is a directory"},
		{"docs", ClassSource, "no trailing slash: a basename, not a segment"},
		// D10 (B28): words that name hand-written source deeper in a tree
		// match at the root only, build matches nowhere, and .txt is no
		// longer docs.
		{"cmd/dist/build.go", ClassSource, "dist below the root is source (Go's cmd/dist)"},
		{"go/doc/comment.go", ClassSource, "doc below the root is source (Go's go/doc)"},
		{"dist/app.js", ClassGenerated, "dist at the root is generated"},
		{"target/debug/x", ClassGenerated, "target at the root is generated"},
		{"pkg/target/x.go", ClassSource, "target below the root is source"},
		{"build/ci.sh", ClassSource, "build is not generated"},
		{"requirements.txt", ClassConfig, "requirements*.txt is config"},
		{"requirements-dev.txt", ClassConfig, "requirements*.txt with a suffix"},
		{"constraints.txt", ClassConfig, "constraints*.txt is config"},
		{"CMakeLists.txt", ClassConfig, "CMakeLists.txt is config"},
		{"notes.txt", ClassSource, ".txt is not docs: fallback (D7)"},
		{"docs/README.md", ClassDocs, "docs root segment and docs ext"},
		{"pkg/docs/x.go", ClassSource, "docs below the root is source"},
		// B29: the Java test names match the basename as written.
		{"Latest.java", ClassSource, "Latest is not *Test"},
		{"Contest.java", ClassSource, "Contest is not *Test"},
		{"FooTest.java", ClassTest, "*Test.java"},
		{"FooTests.java", ClassTest, "*Tests.java"},
		{"src/Main.JAVA", ClassSource, "upper-case Java ext, not a test name"},
		{"FooTest.JAVA", ClassTest, "the Java ext compares without case"},
	}
	for _, tc := range cases {
		t.Run(tc.path+" "+tc.rule, func(t *testing.T) {
			if got := ClassOf(tc.path); got != tc.want {
				t.Errorf("ClassOf(%q) = %s, want %s (%s)", tc.path, got, tc.want, tc.rule)
			}
		})
	}
}

func TestRC3d_RelevanceIsSourceTestConfig(t *testing.T) {
	want := map[PathClass]bool{ClassSource: true, ClassTest: true, ClassConfig: true, ClassCI: false, ClassDocs: false, ClassGenerated: false}
	for _, c := range PathClasses() {
		if Relevant(c) != want[c] {
			t.Errorf("Relevant(%s) = %t, want %t", c, Relevant(c), want[c])
		}
	}
	if len(PathClasses()) != 6 {
		t.Fatalf("PF-RC3 names six classes, got %d", len(PathClasses()))
	}
	if got, want := PathClasses(), []PathClass{"source", "test", "config", "ci", "docs", "generated"}; !slices.Equal(got, want) {
		t.Fatalf("PathClasses() = %q, want %q: the words are stored and read back, so their spelling is pinned", got, want)
	}
	if !Relevant(PathClass("asset")) {
		t.Error(`Relevant("asset") = false: a class this binary does not know must stale the gate, not hide the write`)
	}
}

// TestRC3e_GOROOTNonTestGoIsRelevant checks the tables against a tree they
// were not written from: the Go source every go test run has (D10, B28). It
// walks every non-test .go file under GOROOT/src outside vendor and testdata,
// generated files included -- the zerrors_* and ztypes_* families and every
// file with a "Code generated ... DO NOT EDIT." header -- and requires all but
// one in a thousand to read as relevant: a source file classed not relevant
// is an edit that hides behind a green gate, the miss D7 exists to prevent.
// It does not show that generated files are recognised: a path classifier
// cannot tell a generated file from a hand-written one when its name carries
// no marker the tables know.
func TestRC3e_GOROOTNonTestGoIsRelevant(t *testing.T) {
	// runtime.GOROOT's deprecation is about a binary copied to another
	// machine; a test binary runs where the toolchain that built it lives.
	root := filepath.Join(runtime.GOROOT(), "src")
	var total int
	var hidden []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		total++
		if !Relevant(ClassOf(rel)) {
			hidden = append(hidden, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if total == 0 {
		t.Fatalf("no non-test .go files under %s: the walk proves nothing", root)
	}
	ratio := float64(len(hidden)) / float64(total)
	t.Logf("%d of %d non-test .go files under %s are not relevant (%.4f)", len(hidden), total, root, ratio)
	if ratio > 0.001 {
		t.Fatalf("%d of %d (%.4f) non-test .go files are not relevant, over 0.001; first ten: %v", len(hidden), total, ratio, hidden[:min(10, len(hidden))])
	}
}

// TestRC3f_EcosystemTable checks the tables against the conventions of stacks
// the GOROOT walk cannot see, each row the class a maintainer of that stack
// would give the path. Each table entry or guard added for these rows has two
// rows of distinct values, so no one hard-coded path passes it.
func TestRC3f_EcosystemTable(t *testing.T) {
	cases := []struct {
		path string
		want PathClass
	}{
		// ACCEPTED GAP: setuptools writes build/lib as output, but build names
		// hand-written scripts in other trees, so it matches nowhere and an
		// ambiguous word errs toward relevance: this reads as source.
		{"build/lib/pkg/mod.py", ClassSource},
		{"target/classes/Foo.class", ClassGenerated},
		{"src/docs/guide.md", ClassDocs},
		{"notes.txt", ClassSource},
		{"docs/notes.txt", ClassDocs},
		{"requirements-dev.txt", ClassConfig},
		{"constraints.txt", ClassConfig},
		{"CMakeLists.txt", ClassConfig},
		{"FooTest.java", ClassTest},
		{"Testing.java", ClassSource},
		{"TestUtils.java", ClassSource},
		{"ContestResult.java", ClassSource},
		{"FooTests.cs", ClassTest},
		{"foo.spec.ts", ClassTest},
		{"foo.test.tsx", ClassTest},
		{"__tests__/foo.js", ClassTest},
		{"test_foo.py", ClassTest},
		{"conftest.py", ClassTest},
		{"foo_test.go", ClassTest},
		{"FooTest.kt", ClassTest},
		{"spec/foo_spec.rb", ClassTest},
		{".github/workflows/ci.yaml", ClassCI},
		{"Jenkinsfile", ClassCI},
		{".gitlab-ci.yml", ClassCI},
		{"Makefile", ClassConfig},
		{"go.sum", ClassConfig},
		// Machine-written, but it changes what is installed.
		{"package-lock.json", ClassConfig},
		{"Cargo.lock", ClassConfig},
		{"vendor/x/y.go", ClassGenerated},
		{"node_modules/x/index.js", ClassGenerated},
		{"foo.pb.go", ClassGenerated},
		{"zz_generated.deepcopy.go", ClassGenerated},
		{"foo_generated.go", ClassGenerated},
		{"foo.min.js", ClassGenerated},
		{"dist/app.js", ClassGenerated},
		{"foo.gen.ts", ClassGenerated},
		{"Dockerfile", ClassConfig},
		{"main.tf", ClassSource},
		{"BUILD.bazel", ClassConfig},
		{"WORKSPACE", ClassConfig},
		{"Package.swift", ClassConfig},
		{"App.xcodeproj/project.pbxproj", ClassConfig},
		{"src/main.rs", ClassSource},
		{"tests/integration_test.rs", ClassTest},
		{".cargo/config.toml", ClassConfig},

		// A basename of dots alone is path syntax, not a hidden file.
		{".", ClassSource},
		{"..", ClassSource},
		{"...", ClassSource},
		{".env", ClassConfig},
		// The CamelCase test names: the extension compares without case, the
		// stem with it.
		{"FooTest.KT", ClassTest},
		{"FooTests.swift", ClassTest},
		{"FooTest.scala", ClassTest},
		{"Latest.kt", ClassSource},
		{"Contest.cs", ClassSource},
		// The Kelvin sign folds to a one-byte k, so this extension is .kt and
		// two bytes shorter folded than as written: the stem is cut by the
		// extension as written.
		{"FooTest.\u212At", ClassTest},
		// zz_generated.go is generated by the _generated.go suffix, not by the
		// prefix, which needs its dot: zz_generated_test.go is a test.
		{"zz_generated.go", ClassGenerated},
		{"zz_generated_test.go", ClassTest},
		{"api/v1/zz_generated.conversion.go", ClassGenerated},
		{"api.gen.ts", ClassGenerated},
		{"client.gen.js", ClassGenerated},
		{"BUILD", ClassConfig},
		{"MODULE.bazel", ClassConfig},
		{"WORKSPACE.bazel", ClassConfig},
		{"ios/Runner.xcodeproj/project.pbxproj", ClassConfig},
		// go.work is go.mod's sibling, the workspace's build input. go.work.sum
		// is config already, by the .sum extension, so the go.work row is the
		// whole fix and not half of one.
		{"go.work", ClassConfig},
		{"go.work.sum", ClassConfig},
		// The generators' own suffixes: graphql-code-generator's .generated.ts
		// and Dart build_runner's .g.dart, beside .dart, which is source.
		{"src/types.generated.ts", ClassGenerated},
		{"lib/user.g.dart", ClassGenerated},
		// The suffix needs its dot, and user.g.dart also ends in the dotless
		// g.dart, so this row alone kills the dotless mutant: keep it.
		{"lib/debug.dart", ClassSource},
		// A dotfile named by the suffix alone: rule 1 is checked before the
		// dotfile rule, so it reads as generated, not config.
		{".g.dart", ClassGenerated},
		// freezed's part files, and the Dart protoc plugin's outputs, the last
		// under its grpc option.
		{"lib/model.freezed.dart", ClassGenerated},
		{"lib/proto/msg.pb.dart", ClassGenerated},
		{"lib/proto/msg.pbenum.dart", ClassGenerated},
		{"lib/proto/msg.pbjson.dart", ClassGenerated},
		{"lib/proto/msg.pbserver.dart", ClassGenerated},
		{"lib/proto/msg.pbgrpc.dart", ClassGenerated},
		// The dotless controls, hand-written files named after the package:
		// each suffix needs its dot, and each row alone kills its dotless
		// mutant.
		{"lib/freezed.dart", ClassSource},
		{"lib/pb.dart", ClassSource},
		// The Relay compiler writes into a __generated__ directory beside its
		// input when no artifactDirectory is set.
		{"src/__generated__/UserQuery.graphql.ts", ClassGenerated},
		// A bare script named build reads as config, a deliberate label: config
		// and source are both relevant, and Bazel's BUILD needs the basename.
		{"build", ClassConfig},

		// ACCEPTED GAP: an absolute Windows path's first element is the drive
		// letter, so no root-segment rule fires on one and dist here reads as
		// source, not generated; the basename rules still fire, so a_test.go
		// is a test. The caller relativises a path before classifying it.
		{`C:\dist\app.js`, ClassSource},
		{`C:\repo\a_test.go`, ClassTest},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := ClassOf(tc.path); got != tc.want {
				// %+q, so the Kelvin row reads as K and not as a K.
				t.Errorf("ClassOf(%+q) = %s, want %s", tc.path, got, tc.want)
			}
		})
	}
}

func FuzzRC3_ClassOfNeverPanics(f *testing.F) {
	for _, seed := range []string{"", "a.go", "vendor/x_test.go", "\x00", "../../etc/passwd", "a/b/c/d/e/f/g/h.PY"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, p string) {
		got := ClassOf(p)
		for _, c := range PathClasses() {
			if got == c {
				return
			}
		}
		t.Fatalf("ClassOf(%q) = %q, not a closed class", p, got)
	})
}
