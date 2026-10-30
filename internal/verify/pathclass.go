package verify

import (
	"path"
	"slices"
	"strings"
)

// PathClass is what a written path is, for staleness (PF-RC3: "a closed path
// class: source, test, config, CI, docs or generated"). The list is closed at
// six: a path no rule matches is source, so an unknown write stales the gates
// rather than taking a seventh class. Invariant: ClassOf maps every string to
// exactly one of the six, and is total and pure -- it reads the path and
// nothing else, so storing the class on the record later changes nothing it
// computes.
type PathClass string

const (
	ClassSource    PathClass = "source"
	ClassTest      PathClass = "test"
	ClassConfig    PathClass = "config"
	ClassCI        PathClass = "ci"
	ClassDocs      PathClass = "docs"
	ClassGenerated PathClass = "generated"
)

// PathClasses is the closed list in PF-RC3's order, for tests and the fuzz
// target. A function, not a variable, so no caller can change the list.
func PathClasses() []PathClass {
	return []PathClass{ClassSource, ClassTest, ClassConfig, ClassCI, ClassDocs, ClassGenerated}
}

// Relevant reports whether a write of class c can change a gate's outcome and
// so stales it: source, test and config. A dependency or build-config
// edit changes outcomes exactly as a source edit does; CI config, docs and
// generated output do not change the local gate. It names the three classes
// that are not relevant rather than the three that are, so a class this
// binary does not know -- a seventh one read from a newer record, say --
// stales the gate instead of hiding the write: an unknown fails closed, as
// Rank's does.
func Relevant(c PathClass) bool {
	return c != ClassCI && c != ClassDocs && c != ClassGenerated
}

// The tables, lower case, one per rule of ClassOf's precedence. Segments are
// directory elements; root segments match the first element only; basenames
// and suffixes are matched against the last element; extensions are path.Ext
// of it. Each is a slice of constants, laid out in the binary with no code
// run at init (RES-2), and searched in order; the longest, generatedSuffixes,
// holds fifteen.
var (
	// Rule 1, generated. These words name build output wherever they appear.
	generatedSegments = []string{"vendor", "node_modules", "__pycache__", ".next", "__generated__"}
	// dist and target name build output at the project root only: deeper in a
	// tree they are hand-written source, as Go's own cmd/dist and a package
	// named target are, so they classify only as the first path segment. build
	// is in neither list: Gradle output in one repository, hand-written scripts
	// in another, and where two readings are possible the class that stales a
	// gate is chosen, since an edit hidden behind a green gate is the miss to
	// prevent.
	generatedRootSegments = []string{"dist", "target"}
	generatedSuffixes     = []string{".pb.go", ".gen.go", ".gen.ts", ".gen.js", "_generated.go", ".min.js", ".min.css", ".generated.ts", ".g.dart", ".freezed.dart", ".pb.dart", ".pbenum.dart", ".pbjson.dart", ".pbserver.dart", ".pbgrpc.dart"}
	// Kubernetes and operator-sdk codegen writes zz_generated.<kind>.go, a
	// basename prefix no segment, basename, suffix or extension expresses.
	generatedPrefixes = []string{"zz_generated."}

	// Rule 2, CI.
	ciSegments  = []string{".github", ".gitlab", ".circleci", ".buildkite"}
	ciBasenames = []string{".gitlab-ci.yml", ".travis.yml", "azure-pipelines.yml", "jenkinsfile"}

	// Rule 3, test. With test_*.py, conftest.py and the CamelCase names, in
	// isTestName.
	testSegments = []string{"test", "tests", "spec", "__tests__", "testdata", "e2e"}
	testSuffixes = []string{
		"_test.go", "_test.py", "_spec.rb", "_test.rb", "_test.exs",
		".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx", ".test.ts", ".test.tsx", ".test.js", ".test.jsx",
	}
	// The extensions whose test classes are named <Name>Test or <Name>Tests:
	// JUnit's Java, Kotlin, xUnit's and NUnit's C#, XCTest's Swift and
	// ScalaTest's Scala.
	camelTestExtensions = []string{".java", ".kt", ".cs", ".swift", ".scala"}

	// Rule 4, docs. docs and doc at the project root only, for dist's reason:
	// deeper in a tree a directory word may be hand-written source, as go/doc
	// is a Go package. .txt is not a docs extension, since requirements.txt
	// and CMakeLists.txt are build inputs, which are config, never docs.
	docsRootSegments = []string{"docs", "doc"}
	docsExtensions   = []string{".md", ".rst", ".adoc"}
	docsBasenames    = []string{"license", "changelog", "authors", "notice"}

	// Rule 5, config. With every dotfile, and requirements*.txt and
	// constraints*.txt in isConfigName. The extension list already covers
	// every config basename that ends in one of its extensions (package.json,
	// go.sum, Cargo.toml and the rest); the basenames are the ones it cannot.
	// Bazel's build and workspace files and the Swift package manifest are
	// build inputs with no telling extension; a script named build reads as
	// config too, which is relevant either way. .pbxproj is Xcode's project
	// file, a build input.
	configExtensions = []string{".json", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".lock", ".sum", ".properties", ".xml", ".pbxproj"}
	configBasenames  = []string{"makefile", "dockerfile", "go.mod", "go.work", "cmakelists.txt",
		"build", "build.bazel", "workspace", "workspace.bazel", "module.bazel", "package.swift"}
)

// ClassOf is the class of a path as the record stores it (PF-RC3), by a total
// precedence in which the first rule that matches wins: generated, CI, test,
// docs, config, and source for a path no rule matches, so an unknown write
// stales the gates. The path is normalised first, so the class does not
// depend on the OS that wrote it or on a shouting extension.
//
// Segment rules match directory elements only. The last element is a
// basename unless the path ends in a slash: "docs" is a file named docs and
// falls back to source, "docs/" is a directory and is docs. Root segments
// match the first element of the path as given, so they need a path relative
// to the project: an absolute path never matches one and falls through, which
// errs toward relevance.
//
// Complexity: four walks over the k directory elements, each element and the
// basename and extension looked up in tables of at most twelve words, so
// O(k); no allocation when the path is already lower case with forward
// slashes.
//
// Input is bounded upstream: the hook caps every path field at MaxPathBytes
// and written_paths at MaxWrittenPathsBytes with store.CapDeclaration, called
// from the handler's capture in internal/hook/handle.go before the record is
// written; the store's own write path checks the whole line against
// MaxRecordBytes only. ClassOf has no cap of its own and trusts that every
// path it classifies came through the hook; a Declaration built directly can
// hand it any length, which costs O(len) and nothing else.
func ClassOf(p string) PathClass {
	n := normalise(p)
	switch {
	case hasSegment(n.dir, generatedSegments) || inSet(n.first, generatedRootSegments) || hasAnySuffix(n.base, generatedSuffixes) || hasAnyPrefix(n.base, generatedPrefixes):
		return ClassGenerated
	case hasSegment(n.dir, ciSegments) || inSet(n.base, ciBasenames):
		return ClassCI
	case hasSegment(n.dir, testSegments) || isTestName(n):
		return ClassTest
	case inSet(n.first, docsRootSegments) || inSet(n.ext, docsExtensions) || inSet(n.base, docsBasenames):
		return ClassDocs
	case inSet(n.ext, configExtensions) || isConfigName(n.base) || isDotfile(n.base):
		return ClassConfig
	}
	return ClassSource
}

// normalised is a path split for ClassOf: dir, first, base and ext lower case
// with slashes for separators; raw is the basename as written.
type normalised struct {
	dir   string // everything before the basename, with its trailing slash
	first string // dir's first element; empty for a basename alone or an absolute path
	base  string
	ext   string
	raw   string
}

// normalise turns backslashes into slashes and lower-cases the path, then
// splits it at its last slash into the directory part and the basename. A
// trailing slash leaves the basename empty, so every element of a directory
// path is a segment; no separate trim is needed.
func normalise(p string) normalised {
	p = strings.ReplaceAll(p, `\`, "/")
	i := strings.LastIndexByte(p, '/')
	raw := p[i+1:]
	// Lower-casing can change a non-ASCII path's length in bytes (İ is two
	// bytes and its lower case three), so the split is found again in the
	// lower-case copy rather than reused.
	lower := strings.ToLower(p)
	j := strings.LastIndexByte(lower, '/')
	dir, base := lower[:j+1], lower[j+1:]
	first, _, _ := strings.Cut(dir, "/")
	return normalised{dir: dir, first: first, base: base, ext: path.Ext(base), raw: raw}
}

// hasSegment reports whether an element of dir is in s. Empty elements, from
// "a//b" or a leading slash, match nothing.
func hasSegment(dir string, s []string) bool {
	for dir != "" {
		var seg string
		seg, dir, _ = strings.Cut(dir, "/")
		if inSet(seg, s) {
			return true
		}
	}
	return false
}

// isTestName is rule 3's basename half: the suffix table, test_*.py,
// conftest.py, and the CamelCase names *Test and *Tests in .java, .kt, .cs,
// .swift and .scala. The CamelCase names are matched on the basename as
// written, because the convention is CamelCase: folded to lower case, Test
// would also match Latest.kt and Contest.cs. Only their extension is
// compared without case, as n.ext; the stem is cut by the extension as
// written, since folding can change a length in bytes (the Kelvin sign folds
// to a one-byte k), and a length taken from n.raw itself never cuts past its
// start.
func isTestName(n normalised) bool {
	if n.base == "conftest.py" || (strings.HasPrefix(n.base, "test_") && strings.HasSuffix(n.base, ".py")) {
		return true
	}
	if ext := path.Ext(n.raw); inSet(n.ext, camelTestExtensions) {
		stem := n.raw[:len(n.raw)-len(ext)]
		if strings.HasSuffix(stem, "Test") || strings.HasSuffix(stem, "Tests") {
			return true
		}
	}
	return hasAnySuffix(n.base, testSuffixes)
}

// isConfigName is rule 5's basename half: the basename table, and the Python
// dependency lists requirements*.txt and constraints*.txt, build inputs, which
// are config, never docs.
func isConfigName(base string) bool {
	if inSet(base, configBasenames) {
		return true
	}
	return strings.HasSuffix(base, ".txt") && (strings.HasPrefix(base, "requirements") || strings.HasPrefix(base, "constraints"))
}

// isDotfile reports whether base names a hidden file. "." and ".." are path
// syntax, not hidden files, so a basename of dots alone is not one and falls
// back to source.
func isDotfile(base string) bool {
	return strings.HasPrefix(base, ".") && strings.TrimLeft(base, ".") != ""
}

func hasAnySuffix(s string, suffixes []string) bool {
	for _, x := range suffixes {
		if strings.HasSuffix(s, x) {
			return true
		}
	}
	return false
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, x := range prefixes {
		if strings.HasPrefix(s, x) {
			return true
		}
	}
	return false
}

func inSet(s string, words []string) bool {
	return slices.Contains(words, s)
}

// inStore reports whether the slash-separated path p, cleaned, carries a
// .claude/rashomon/ segment (card 3.2, D131): the store's directory
// itself or anything under it, at the root or below any directory.
// integrity's store_touched check reads it.
func inStore(p string) bool {
	return strings.Contains("/"+path.Clean(p)+"/", "/.claude/rashomon/")
}

// underStore reports whether p, a path as a call named it (slashed), is
// root, rashomon's store root, or under it once cleaned (D173). A root of
// "" is no store.
func underStore(p, root string) bool {
	if root == "" || p == "" {
		return false
	}
	p, r := path.Clean(p), path.Clean(slashed(root))
	_, in := under(p, r)
	return in || p == r
}
