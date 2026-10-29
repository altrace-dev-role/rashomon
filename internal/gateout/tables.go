package gateout

import (
	"regexp"
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
func tables() []table {
	rowsOnce.Do(func() {
		rowsOnce.v = []table{
			{program: "go", pass: compile(`^ok\s`), fail: compile(`^FAIL\s`), failLine: compile(`^--- FAIL`), empty: compile(`\[no test files\]`, `\[no tests to run\]`)},
			{program: "jest", pass: compile(`^Tests:.*\d+ passed`), fail: compile(`^Tests:.*\d+ failed`)},
			{program: "vitest", pass: compile(`^Tests\s+.*\d+ passed`), fail: compile(`^Tests\s+.*\d+ failed`)},
			{program: "pytest", pass: compile(`\d+ passed`), fail: compile(`\d+ (failed|error)`), empty: compile(`no tests ran`)},
			{program: "cargo", pass: compile(`^test result: ok`), fail: compile(`^test result: FAILED`)},
			{program: "make", delegate: true},
			{program: "npm", delegate: true},
			{program: "yarn", delegate: true},
			{program: "pnpm", delegate: true},
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
