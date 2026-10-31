package shape

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"
)

// runnerListPins is, per RulesVersion, the sha256 of the runner lists that
// version decides verb_class and status_masked with (runnerListsDigest). A
// bump adds a pin; a pin is never edited. The old pins stay by design, as
// history of what each version decided, though only RulesVersion's own is
// consulted: a version that changed nothing here repeats the digest above it.
var runnerListPins = map[int]string{
	1:  "522cdda1bbddd626a4bb17c200854d3b31ab80bfb1c87d356bf638c9ee2d2aea",
	2:  "57769de023e65afb569bcd71d1c3ccc7fe5a4b33b9b2b3a8e50e5919b8b6467e", // rake, composer, mix, bazel test as test runners (2026-10-23)
	3:  "57769de023e65afb569bcd71d1c3ccc7fe5a4b33b9b2b3a8e50e5919b8b6467e", // RulesVersion 3 changed the in-place tables only; the runner lists are unchanged
	4:  "57769de023e65afb569bcd71d1c3ccc7fe5a4b33b9b2b3a8e50e5919b8b6467e", // RulesVersion 4 changed how the lists are matched (matcherPins); the lists are unchanged
	5:  "57769de023e65afb569bcd71d1c3ccc7fe5a4b33b9b2b3a8e50e5919b8b6467e", // RulesVersion 5 changed the leading-cd fold and the location rule; the lists are unchanged
	6:  "57769de023e65afb569bcd71d1c3ccc7fe5a4b33b9b2b3a8e50e5919b8b6467e", // RulesVersion 6 dropped -l from checkOnly, made make's -j a digits entry and put the flag tables under a pin (flagTablePins); the lists are unchanged
	7:  "57769de023e65afb569bcd71d1c3ccc7fe5a4b33b9b2b3a8e50e5919b8b6467e", // RulesVersion 7 changed when a runner's line is whole (closesGroups) and perl's -0 in clusterHas; the lists are unchanged
	8:  "57769de023e65afb569bcd71d1c3ccc7fe5a4b33b9b2b3a8e50e5919b8b6467e", // RulesVersion 8 added the hiding flags and folded pushd; the lists are unchanged
	9:  "57769de023e65afb569bcd71d1c3ccc7fe5a4b33b9b2b3a8e50e5919b8b6467e", // RulesVersion 9 ended a comment at its newline in the program search; the lists are unchanged
	10: "57769de023e65afb569bcd71d1c3ccc7fe5a4b33b9b2b3a8e50e5919b8b6467e", // RulesVersion 10 named a Bash line's write targets (writeTargetPins); the lists are unchanged
}

// runnerListsDigest digests testCommands, then buildCommands: each row's
// words, each row, and the two lists apart.
func runnerListsDigest() string {
	var b strings.Builder
	for _, list := range [][][]string{testCommands, buildCommands} {
		for _, row := range list {
			b.WriteString(strings.Join(row, "\x1f"))
			b.WriteByte('\x1e')
		}
		b.WriteByte('\x1d')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// TestRulesVersion_RunnerLists: the runner lists cannot change without a
// RulesVersion bump, or records written before and after the change carry
// the same rules_version and read as decided by the same lists. Break: add,
// drop or edit a row of testCommands or buildCommands and leave RulesVersion
// as it was.
func TestRulesVersion_RunnerLists(t *testing.T) {
	got := runnerListsDigest()
	want, ok := runnerListPins[RulesVersion]
	if !ok || want != got {
		t.Fatalf("the runner lists digest to %s, and RulesVersion %d pins %q: a list changed without a bump. "+
			"Bump RulesVersion and add its pin here (and the write-table, matcher, report's pass-list and hook's location pins).", got, RulesVersion, want)
	}
}

// writeTablePins is, per RulesVersion, the sha256 of the in-place tables that
// version decides may_write with (writeTablesDigest). A bump adds a pin; a
// pin is never edited. Version 1 had no in-place tables, and version 2's pin
// was computed from its tables when this test was added, with shortClusters
// empty, not read from a version-2 build.
var writeTablePins = map[int]string{
	2:  "9c73ce1ce2a8d00e67481165c1adbecf0e55f0e1dfd7a1f314d62e2ba05168c5",
	3:  "71c7a0e23ac2a79834dbf5d78e18ef67f4f5bc3ea663cac77aec017c38f0f7f7", // short-option clusters for sed and perl, BSD sed's -I; help and version words clear an editor (2026-10-24)
	4:  "71c7a0e23ac2a79834dbf5d78e18ef67f4f5bc3ea663cac77aec017c38f0f7f7", // RulesVersion 4 changed how the runner lists are matched; the in-place tables are unchanged
	5:  "71c7a0e23ac2a79834dbf5d78e18ef67f4f5bc3ea663cac77aec017c38f0f7f7", // RulesVersion 5 changed the leading-cd fold and the location rule; the in-place tables are unchanged
	6:  "dfb8010e06386af079d0761108eb40c292869806f9321f297ed7ae10a115528b", // -l no longer clears an editor: gofmt -l -w, sed -l N -i, perl -l -pi, black -l N and isort -l N write (2026-10-25)
	7:  "dfb8010e06386af079d0761108eb40c292869806f9321f297ed7ae10a115528b", // RulesVersion 7 reads perl's -0 digit run in clusterHas, code and not a table; the in-place tables are unchanged
	8:  "dfb8010e06386af079d0761108eb40c292869806f9321f297ed7ae10a115528b", // RulesVersion 8 added the hiding flags and folded pushd; the in-place tables are unchanged
	9:  "dfb8010e06386af079d0761108eb40c292869806f9321f297ed7ae10a115528b", // RulesVersion 9 ended a comment at its newline in the program search; the in-place tables are unchanged
	10: "dfb8010e06386af079d0761108eb40c292869806f9321f297ed7ae10a115528b", // RulesVersion 10 named a Bash line's write targets (writeTargetPins); the in-place tables are unchanged
}

// writeTablesDigest digests inPlaceEditors (each editor in sorted order, then
// its words), then checkOnly, then shortClusters (each editor in sorted
// order, then its in-place letters and the letters that consume), with
// runnerListsDigest's separators.
func writeTablesDigest() string {
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(inPlaceEditors)) {
		b.WriteString(strings.Join(append([]string{k}, inPlaceEditors[k]...), "\x1f"))
		b.WriteByte('\x1e')
	}
	b.WriteByte('\x1d')
	b.WriteString(strings.Join(checkOnly, "\x1f"))
	b.WriteByte('\x1d')
	for _, k := range slices.Sorted(maps.Keys(shortClusters)) {
		c := shortClusters[k]
		b.WriteString(strings.Join([]string{k, c.letters, c.consumes}, "\x1f"))
		b.WriteByte('\x1e')
	}
	b.WriteByte('\x1d')
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// TestRulesVersion_WriteTables: the in-place tables cannot change without a
// RulesVersion bump, or records written before and after the change carry
// the same rules_version and read as decided by the same tables. Break: add,
// drop or edit an editor, its words, a check word or a cluster and leave
// RulesVersion as it was.
func TestRulesVersion_WriteTables(t *testing.T) {
	got := writeTablesDigest()
	want, ok := writeTablePins[RulesVersion]
	if !ok || want != got {
		t.Fatalf("the in-place tables digest to %s, and RulesVersion %d pins %q: a table changed without a bump. "+
			"Bump RulesVersion and add its pin here (and the runner-list, matcher, pass-list and location pins).", got, RulesVersion, want)
	}
}

// matcherPins is, per RulesVersion, the sha256 of the shape fields Derive
// decides for the lines of matcherCorpus (matcherDigest). The pins above see
// a table change; this one sees a change in the code that reads the tables
// -- runnerOn, refusesRun, the program search, the masked parser -- on these
// lines only, so a matcher change that moves none of them still needs its
// bump made by hand. A bump adds a pin; a pin is never edited, and the corpus
// grows only with a bump. Version 3's pin was computed with version 3's
// verb.go and masked.go when this test was added.
var matcherPins = map[int]string{
	3:  "3b37daf2498f1bfed509f7f0bcc4b9b44381e6a56ab537abfb3b553d4b90ea9a",
	4:  "d8ce2f1af01d7dbec051b928d5ac03ccf16830cb432ab1f36c972466f6ccebb0", // a listed flag may stand before a runner's row words (leadingFlags); a flag that hides the runner's failure refuses it (2026-10-24)
	5:  "a6b4f738864bca26e64502c754ade947cef3f0d4eca78ab34c5a3f4fff89fb38", // the masked parser passes over every leading cd step the program search does where both readings fold alike (agreedFold): `(cd /tmp && go test ./...)` gains its runner (2026-10-24)
	6:  "8c69e44656ed2328c904397bcb2182e6937a22205c22fc0508167d4a5b554d35", // make's -j takes digits only (a `-j*` prefix admitted `make -j=4 test` and `make -j4x test`, which make rejects): their lines lose the runner (2026-10-25)
	7:  "165e06eef4489b033a8484ee4800cfe74f08a98cd935e6005c6dd49ed21865d5", // a runner's line is whole only where the groups opened before the runner close after it: `go test ./... )` and `(go test ./...` lose class test, `{ go test ./...; }` gains it, and a `#` ends the count (2026-10-28)
	8:  "f867982fb3673009fa5854a8fe5d862ff455e6e0d0be54fb334739db2dcb67b3", // a hiding flag records its runner masked with the runner's kind, not as no run; pushd folds as a cd step in both readings (2026-10-28)
	9:  "287971f971e93b09a7f1cd5f2a2cf7368d907bd9d4b788eaa5e625668d689a30", // a comment ends at its newline in the program search as in the masked parser: the cd steps after `# c\` fold and the runner is go test (2026-10-28)
	10: "287971f971e93b09a7f1cd5f2a2cf7368d907bd9d4b788eaa5e625668d689a30", // RulesVersion 10 named a Bash line's write targets (writeTargetPins) and factored stageMayWrite out of mayWrite; no matcher field moves (2026-10-28)
}

// matcherCorpus is the lines matcherPins digests, a frozen copy: the lines
// whose runner the verb class and the masked parser read apart when version
// 4 was made, then the rows that version's tests added. A row a test gains
// later is not copied here without a bump, so it cannot move a pin.
var matcherCorpus = []string{
	"make -j4 test", "mvn -B test", "bazel --output_base=/tmp test //...", "cargo +nightly test",
	"! go test ./...", "if go test ./...; then :; fi", "(cd /tmp && go test ./...)",

	"mvn -B -q test", "gradle --no-daemon test", "python -u -m pytest", "npx --yes jest", "npx -y jest",
	"python3 -u -m pytest", "./mvnw -B test", "./gradlew --no-daemon test",
	"python -m pytest", "cmake --build .", "make -C sub test", "go -C sub test ./...", "go test ./...", "make test",

	"make -n test", "make -j4 -n test", "mvn -B -DskipTests test", "make -Cdir test", "make --directory=sub test",
	"go -C=sub test ./...", "python -W ignore -m pytest", "bazel --bazelrc=ci.bazelrc test //...", "make +x test",
	"mvn -B=true test", "make -j$N test", `make "-j4" test`, "make -j 4 test", "make -j4x test",
	"cargo +nonesuch test",
	"make -C test test", "go -C test test ./...", "gradle -p test test", "mvn -P test compile", "mvn -pl test compile",
	"make -i test", "make test -i", "make --ignore-errors test", "make -i build", "mvn -fn test", "mvn test -fn",
	"mvn --fail-never test", "npm --if-present test", "npm test --if-present",
	"mvn test -Dmaven.test.failure.ignore", "mvn -Dmaven.test.failure.ignore=true test",
	"mvn -Dmaven.test.failure.ignore=true package", "mvn package -Dmaven.test.failure.ignore=true",
	"mvn test -Dmaven.test.failure.ignore=false", "npm test --if-present=false",
	"bazel --output_base /tmp test //...", "npm --prefix web test",
	"make test -C sub", "cargo test --manifest-path sub/Cargo.toml", "npm test --prefix web", "go test -C sub ./...",

	"echo hi && go test ./...", "true && go test ./...", "go build ./... && go test ./...", "make lint; go test ./...",
	"if [[ -d x ]]; then go test ./...; fi", "cd /tmp && echo hi && go test ./...",
	strings.Repeat("cd a && ", 64) + "go test ./...", strings.Repeat("cd a && ", 65) + "go test ./...",

	"make -j test", "make -j16 test", "make -j=4 test", "make -jx test", "make -j 4x test", "make -j $N test",

	"sed -l -i.bak s/x/y/ a.go", "perl -l -pi -e s/x/y/ a.go", "perl -l -i -pe s/x/y/ a.go", "perl -0777pi -e s/x/y/ a.go",
	"gofmt -l .", "npx prettier -l .",
	"go test ./... )", "(go test ./...", "(go test ./...))", "{ go test ./...; }", "go test ./... # )", "go test ./... #)",
	"cd /a) && go test ./...", "cd /a # ) && go test ./...",

	"make test --ignore-errors", "mvn test --fail-never", "mvn -Dmaven.test.failure.ignore test", "mvn -fn package",
	"mvn test -Dmaven.test.failure.ignore=1", "make -i test -n",
	"cd /a && pushd /b && go test ./...", "pushd /b && go test ./...", "pushd /b; go test ./...", "pushd +1 && go test ./...",
	"popd && go test ./...", "cd /a && popd && go test ./...",
	"cd a && # c\\\ncd b && go test ./...", "# c\\\ncd a && go test ./...", "cd a && #\\\n cd b && go test ./...",
}

// matcherDigest digests, for each line of matcherCorpus in order, the line,
// its verb_class, status_masked and runner_kind (a NUL byte for null) and
// whether runner_digest is set, with runnerListsDigest's separators.
func matcherDigest() string {
	orNull := func(p *string) string {
		if p == nil {
			return "\x00"
		}
		return *p
	}
	var b strings.Builder
	for _, cmd := range matcherCorpus {
		raw, _ := json.Marshal(map[string]string{"command": cmd})
		s := Derive("Bash", raw, []byte("key"))
		digest := "nil"
		if s.RunnerDigest != nil {
			digest = "set"
		}
		b.WriteString(strings.Join([]string{cmd, s.VerbClass, orNull(s.StatusMasked), orNull(s.RunnerKind), digest}, "\x1f"))
		b.WriteByte('\x1e')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// TestRulesVersion_Matchers: a change in how the tables are read that moves
// a recorded field on a corpus line cannot ship without a RulesVersion bump,
// or records written before and after it carry the same rules_version. Break:
// pass over, or stop passing over, a flag before a runner's row words and
// leave RulesVersion as it was.
func TestRulesVersion_Matchers(t *testing.T) {
	got := matcherDigest()
	want, ok := matcherPins[RulesVersion]
	if !ok || want != got {
		t.Fatalf("the matcher corpus digests to %s, and RulesVersion %d pins %q: a matcher changed without a bump. "+
			"Bump RulesVersion and add its pin here (and the runner-list, write-table, pass-list and location pins).", got, RulesVersion, want)
	}
}

// flagTablePins is, per RulesVersion, the sha256 of the matcher flag tables
// and the inline writer words (flagTablesDigest): the arguments that refuse a
// runner (notARun, notARunAny, mavenSkipProperties), the flags that may stand
// before its words (leadingFlags), and the words mayWrite reads wherever they
// stand (inlineWriters, findWriteWords, curlLongOutputs, curlArgumentOptions).
// Versions 1 to 5 held these under the matcher pin's corpus lines alone and
// have no entry here; version 6's pin was computed from its tables when this
// test was added. A bump adds a pin; a pin is never edited.
var flagTablePins = map[int]string{
	6:  "c03726242fe0dd64c144087e62bc5dfdcbf229fae18f0b9a1b5fb69d2f13063f",
	7:  "c03726242fe0dd64c144087e62bc5dfdcbf229fae18f0b9a1b5fb69d2f13063f", // RulesVersion 7 changed when a runner's line is whole and perl's -0 in clusterHas; the flag tables are unchanged
	8:  "653466313cb604b05497c12430594ce777ba4507d3e4417640fd0836e5572427", // the hiding flags (make -i and --ignore-errors, mvn -fn, --fail-never and -Dmaven.test.failure.ignore, npm --if-present) leave notARun and mavenSkipProperties for leadingFlags, marked ! (2026-10-28)
	9:  "653466313cb604b05497c12430594ce777ba4507d3e4417640fd0836e5572427", // RulesVersion 9 ended a comment at its newline in the program search; the flag tables are unchanged
	10: "653466313cb604b05497c12430594ce777ba4507d3e4417640fd0836e5572427", // RulesVersion 10 named a Bash line's write targets (writeTargetPins); the flag tables are unchanged
}

// flagTablesDigest digests notARun (each runner in sorted order, then its
// words), notARunAny, mavenSkipProperties, leadingFlags (each program in
// sorted order, then its words), and the writer words, with
// runnerListsDigest's separators.
func flagTablesDigest() string {
	var b strings.Builder
	sorted := func(m map[string][]string) {
		for _, k := range slices.Sorted(maps.Keys(m)) {
			b.WriteString(strings.Join(append([]string{k}, m[k]...), "\x1f"))
			b.WriteByte('\x1e')
		}
		b.WriteByte('\x1d')
	}
	sorted(notARun)
	b.WriteString(strings.Join(notARunAny, "\x1f"))
	b.WriteByte('\x1d')
	b.WriteString(strings.Join(mavenSkipProperties, "\x1f"))
	b.WriteByte('\x1d')
	sorted(leadingFlags)
	for _, list := range [][]string{inlineWriters, findWriteWords, curlLongOutputs, {curlArgumentOptions}} {
		b.WriteString(strings.Join(list, "\x1f"))
		b.WriteByte('\x1d')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// TestRulesVersion_FlagTables: the matcher flag tables and the inline writer
// words cannot change without a RulesVersion bump, or records written before
// and after the change carry the same rules_version and read as decided by
// the same tables. Break: add or drop a word of notARun, notARunAny,
// leadingFlags or mayWrite's inline writers and leave RulesVersion as it was.
func TestRulesVersion_FlagTables(t *testing.T) {
	got := flagTablesDigest()
	want, ok := flagTablePins[RulesVersion]
	if !ok || want != got {
		t.Fatalf("the flag tables digest to %s, and RulesVersion %d pins %q: a table changed without a bump. "+
			"Bump RulesVersion and add its pin here (and the runner-list, write-table, matcher, pass-list and location pins).", got, RulesVersion, want)
	}
}

// matcherMayWrite is may_write on each line of matcherCorpus, read with
// version 9's mayWrite, before stageMayWrite was factored out of it: the
// indices of the lines that read true. matcherDigest holds no may_write, so
// this is what proves the factoring moved no answer. A bump that moves one
// edits this table and says so.
var matcherMayWrite = []int{5, 62, 63, 64, 65, 66, 67, 76, 77, 78, 79, 85, 100, 101}

// TestWT10_MayWriteUnchanged: may_write on every matcher corpus line is what
// version 9's mayWrite gave it. Break: let stageMayWrite answer true for a
// stage no word of which writes, or drop a source from it.
func TestWT10_MayWriteUnchanged(t *testing.T) {
	want := make(map[int]bool, len(matcherMayWrite))
	for _, k := range matcherMayWrite {
		want[k] = true
	}
	for k, cmd := range matcherCorpus {
		raw, _ := json.Marshal(map[string]string{"command": cmd})
		if got := Derive("Bash", raw, []byte("key")).MayWrite; got != want[k] {
			t.Errorf("matcherCorpus[%d] %q: may_write %t, version 9 read %t", k, cmd, got, want[k])
		}
	}
}

// writeTargetPins is, per RulesVersion, the sha256 of what WrittenPaths names
// for the lines of writeTargetCorpus (writeTargetDigest). Version 10 is the
// first to record written_paths. A bump adds a pin; a pin is never edited, and
// the corpus grows only with a bump.
var writeTargetPins = map[int]string{
	10: "eb0c049a1042ccd3443eb5d9ca7ca264639037081132ac604b25f4da717ecc2b",
}

// writeTargetCorpus is the lines writeTargetPins digests, a frozen copy of the
// rows TestWT1 to TestWT13 held when version 10 was made, one or more per
// rule. A row a test gains later is not copied here without a bump.
var writeTargetCorpus = []string{
	"cat a.txt > b.log", "cat a.txt >| b.log", "cat a.txt &> b.log", "cat a.txt 2> err.log", "cat <> a.log",
	"cat a.txt >&2", "cat < a.txt", "cat a.txt > $out", "echo x > a.log", "make > build.log",
	"sed -i -e s/x/y/ a.go", "sed -i s/x/y/ a.go b.go", "sed -i -f fix.sed a.go", "sed -i a.go -e s/x/y/",
	"sed -i '' s/x/y/ a.go", "perl -i -pe s/x/y/ a.go", "perl -pi fix.pl a.go", "gofmt -w a.go b.go",
	"go fmt ./...", "ruff format a.py", "prettier --write a.ts", "black a.py", "gofmt -w --diff a.go",
	"cat a | tee -a a.log", "go test ./... | tee test.log", "cp a.go b.go", "install -m 755 a b",
	"cp -t dir a.go b.go", "cp --target-directory=dir a.go", "cp a.go dir/", "truncate -s 0 a.log",
	"rm -rf", "git checkout -- a.go", "ls | xargs rm", "sudo touch a.go", "/bin/rm a.go",
	"rm $f", "touch {a,b}.go", "rm -- -f.txt", "rm a.go $f", "rm -r a/.", "cat a | tee /dev/null",
	"touch a.go; sed -i s/x/y/ b.go", "touch a.go\nrm b.go", "cp a b 2> err.log", "echo $(touch a.go) > b.log",
	"cd sub && go run gen.go > out.go", "cd a && cd /b && touch x", "cd sub; touch x", "cd ~ && touch x",
	"touch a && cd sub && touch b", "cat curl | cat -o",
}

// writeTargetDigest digests, for each line of writeTargetCorpus in order, the
// line, its targets and whether they are complete, with runnerListsDigest's
// separators.
func writeTargetDigest() string {
	var b strings.Builder
	for _, cmd := range writeTargetCorpus {
		raw, _ := json.Marshal(map[string]string{"command": cmd})
		targets, complete := WrittenPaths("Bash", raw)
		b.WriteString(strings.Join(append([]string{cmd}, targets...), "\x1f"))
		if complete {
			b.WriteString("\x1fcomplete")
		}
		b.WriteByte('\x1e')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// TestRulesVersion_WriteTargets: a change in how WrittenPaths reads a line
// that moves a target or the complete bit on a corpus line cannot ship
// without a RulesVersion bump, or records written before and after it carry
// the same rules_version and read as named by the same rule. Break: name sed's
// script as a file and leave RulesVersion as it was.
func TestRulesVersion_WriteTargets(t *testing.T) {
	got := writeTargetDigest()
	want, ok := writeTargetPins[RulesVersion]
	if !ok || want != got {
		t.Fatalf("the write-target corpus digests to %s, and RulesVersion %d pins %q: the write-target rule changed without a bump. "+
			"Bump RulesVersion and add its pin here (and the runner-list, write-table, matcher, flag-table, pass-list and location pins).", got, RulesVersion, want)
	}
}
