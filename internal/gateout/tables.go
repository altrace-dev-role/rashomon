package gateout

import (
	"regexp"
	"strings"
	"sync"
)

type table struct {
	program                     string
	pass, fail, failLine, empty []*regexp.Regexp
	delegate                    bool
}

// tables returns the runner tables, compiled on the first call and never at
// init (RES-2): the post hook links this package and runs on every tool call,
// and only a gate run's output is classified. Source patterns are fixed and
// are validated against spec's RE2 bounds by TestGO10_TablesPinned.
//
// pytest's fail forms are its summary line, as its pass forms are, with a
// count of at least 1: a delegated run is matched against every table, so a
// bare count read another tool's "0 failed" or "0 errors", or a test's own log
// line, as a failure, and the gate went red over a run that passed
// (GATEOUT-ZERO1).
func tables() []table {
	rowsOnce.Do(func() {
		rowsOnce.v = []table{
			{program: "go", pass: compile(`^ok\s`), fail: compile(`^FAIL\s`, `^FAIL$`), failLine: compile(`^--- FAIL`), empty: compile(`\[no test files\]`, `\[no tests to run\]`)},
			{program: "jest", pass: compile(`^Tests:.*\d+ passed`), fail: compile(`^Tests:.*\d+ failed`)},
			{program: "vitest", pass: compile(`^Tests\s+.*\d+ passed`), fail: compile(`^Tests\s+.*\d+ failed`)},
			{program: "pytest", pass: compile(`^(\d+ \w+, )*\d+ passed(, [^=]*)? in \d`, `^=+ .*\d+ passed.* in \d[\d.]*s.* =+$`), fail: compile(`^(\d+ \w+, )*[1-9]\d* (failed|errors?)(, [^=]*)? in \d`, `^=+ .*[1-9]\d* (failed|errors?).* in \d[\d.]*s.* =+$`, `Fatal Python error`, `^Killed`), empty: compile(`no tests ran`)},
			{program: "cargo", pass: compile(`^test result: ok`), fail: compile(`^test result: FAILED`, `^error: test failed`, `^error: \d+ targets? failed`)},
			{program: "make", delegate: true, fail: compile(`^make(\[\d+\])?: \*\*\*`)},
			{program: "npm", delegate: true, fail: compile(`^npm (ERR!|error)`, `ELIFECYCLE`)},
			{program: "yarn", delegate: true, fail: compile(`^error Command failed`)},
			{program: "pnpm", delegate: true, fail: compile(`^ERR_PNPM`)},
		}
	})
	return rowsOnce.v
}

// rowsOnce holds what tables returns.
var rowsOnce struct {
	sync.Once
	v []table
}

func compile(sources ...string) []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(sources))
	for _, source := range sources {
		patterns = append(patterns, regexp.MustCompile(source))
	}
	return patterns
}

func findProgram(program string) *table {
	rows := tables()
	for i := range rows {
		if rows[i].program == program {
			return &rows[i]
		}
	}
	return nil
}

func matches(patterns []*regexp.Regexp, line []byte) bool {
	for _, pattern := range patterns {
		if pattern.Match(line) {
			return true
		}
	}
	return false
}

// ProgramFor is the program word Classify's tables are keyed by for a run
// whose line launched runner: the table's own word, found among the words of
// the runner's row -- "pytest" for "python3 -m pytest", "go" for "go test",
// "jest" for "npx jest" -- else program, the line's first word, which is what
// the tables were keyed by before the line's runner was recorded. runner is
// the shape's runner_name (a row of the binary's runner lists, never a word of
// the command), so the word returned is a table's and not the line's. A
// runner no table names, `python3 -m unittest` or `bun test`, returns program
// as it was, and Classify answers unattributable for it as before.
func ProgramFor(program, runner string) string {
	// The words are walked in place: strings.Fields would allocate a slice for
	// every run that names a runner.
	for rest := runner; rest != ""; {
		w := rest
		if i := strings.IndexByte(rest, ' '); i >= 0 {
			w, rest = rest[:i], rest[i+1:]
		} else {
			rest = ""
		}
		if w != "" && findProgram(w) != nil {
			return w
		}
	}
	return program
}

// Delegated reports whether program is a delegate row: a runner that runs
// other runners (make, npm, yarn, pnpm), whose output holds any step's lines.
// A pass line in it is one step's, not the run's, so a reader that takes an
// outcome from the output takes only a failure from a delegated run.
func Delegated(program string) bool {
	row := findProgram(program)
	return row != nil && row.delegate
}

// FailLineDecides reports whether a bare fail line is all the table has to
// say a run failed: true only for a program whose row has no summary form of
// its own. A reader that steers on a failure rests on summary_fail where the
// table has one, since a fail line is a substring a pipe may have kept without
// the run's verdict (`--- FAIL` out of a test a retry then passed). Every row
// now has a summary form, so this is false for each of them and for a program
// no table names.
func FailLineDecides(program string) bool {
	row := findProgram(program)
	return row != nil && !row.delegate && len(row.fail) == 0
}
