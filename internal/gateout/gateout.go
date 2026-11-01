// Package gateout classifies bounded test-runner output for gate records.
// It is a pure function: the caller supplies the runner kind, normalized
// program and bytes, and no output text is retained (archive 2.2.10, D22).
package gateout

import (
	"bytes"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// MaxScanBytes bounds the runner output scanned for a gate result (D22).
const MaxScanBytes = 1 << 20

// Wrapper is the program Classify reads a call's output as where its line
// names no runner: a script, a shell, a program no list knows (WRAPPER-RUN1).
// It is read as a delegate's output is, by every runner row, and a row counts
// only where its signature is in the output; no delegate's marker counts, and
// no table is keyed by the word. A result no row decided names Wrapper as its
// parser, or the first row signed in it.
const Wrapper = "wrapper"

// Classify reports the table-driven result of test runner output. Build
// runners and unknown programs do not scan text. The scan is O(len(text))
// with no per-line allocation; inputs over MaxScanBytes are unread. A
// delegate's or a wrapper's output is scanned twice: once for the rows whose
// signature it holds, then by those rows.
func Classify(kind, program string, text []byte) store.GateOutput {
	if kind != "test" && kind != "build" {
		return store.GateOutput{Kind: "unattributable"}
	}
	if kind == "build" {
		return store.GateOutput{Kind: "no_summary", Parser: program}
	}
	if len(text) > MaxScanBytes {
		reason := "too_large"
		return store.GateOutput{Kind: "unread", Reason: &reason}
	}

	wrapper := program == Wrapper
	row := findProgram(program)
	if row == nil && !wrapper {
		return store.GateOutput{Kind: "unattributable"}
	}
	var counted uint32 // the rows a delegated line may match, by index
	if wrapper || row.delegate {
		row, counted = nil, signedRows(text, wrapper)
		if wrapper && counted == 0 {
			return store.GateOutput{Kind: "no_summary", Parser: program}
		}
	}

	var pass, fail, failLine, empty bool
	var passParser, failParser, failLineParser, emptyParser string
	for len(text) > 0 {
		line := text
		if i := bytes.IndexByte(text, '\n'); i >= 0 {
			line, text = text[:i], text[i+1:]
		} else {
			text = nil
		}
		parser, result := matchLine(line, row)
		if row == nil {
			parser, result = matchDelegatedLine(line, counted)
		}
		switch result {
		case "empty":
			empty = true
			if emptyParser == "" {
				emptyParser = parser
			}
		case "fail":
			fail = true
			if failParser == "" {
				failParser = parser
			}
		case "fail_line":
			failLine = true
			if failLineParser == "" {
				failLineParser = parser
			}
		case "pass":
			pass = true
			if passParser == "" {
				passParser = parser
			}
		}
	}

	if fail {
		return store.GateOutput{Kind: "summary_fail", Parser: fallback(failParser, program)}
	}
	if failLine {
		return store.GateOutput{Kind: "fail_line", Parser: fallback(failLineParser, program)}
	}
	if pass {
		return store.GateOutput{Kind: "summary_pass", Parser: fallback(passParser, program)}
	}
	if empty {
		return store.GateOutput{Kind: "empty_run", Parser: fallback(emptyParser, program)}
	}
	if wrapper {
		// A runner signed the output and printed no outcome line: it ran here,
		// and how it ended is not in what the wrapper kept.
		rows := tables()
		for i := range rows {
			if counted&(1<<i) != 0 {
				return store.GateOutput{Kind: "no_summary", Parser: rows[i].program}
			}
		}
	}
	return store.GateOutput{Kind: "no_summary", Parser: program}
}

// signedRows is the set of rows, as bits by index, whose lines count in a
// delegate's or, with wrapper, a wrapper's output: each runner row with a
// signature line in text; for a delegate, also every row with no signature,
// its own markers among them, as before signatures (WRAPPER-RUN1, D3). One
// pass, which ends once every signed row is found.
func signedRows(text []byte, wrapper bool) uint32 {
	rows := tables()
	var counted, open uint32
	for i := range rows {
		switch {
		case len(rows[i].sig) > 0:
			open |= 1 << i
		case !wrapper:
			counted |= 1 << i
		}
	}
	for len(text) > 0 && open != 0 {
		line := text
		if i := bytes.IndexByte(text, '\n'); i >= 0 {
			line, text = text[:i], text[i+1:]
		} else {
			text = nil
		}
		for i := range rows {
			if open&(1<<i) != 0 && matches(rows[i].sig, line) {
				counted, open = counted|1<<i, open&^(1<<i)
			}
		}
	}
	return counted
}

func fallback(parser, program string) string {
	if parser != "" {
		return parser
	}
	return program
}

func matchLine(line []byte, row *table) (string, string) {
	if row == nil {
		return "", ""
	}
	if matches(row.empty, line) {
		return row.program, "empty"
	}
	if matches(row.fail, line) {
		return row.program, "fail"
	}
	if matches(row.failLine, line) {
		return row.program, "fail_line"
	}
	if matches(row.pass, line) {
		return row.program, "pass"
	}
	return "", ""
}

// matchDelegatedLine is the first row of counted (signedRows) that matches
// line, and its result.
func matchDelegatedLine(line []byte, counted uint32) (string, string) {
	rows := tables()
	for i := range rows {
		if counted&(1<<i) == 0 {
			continue
		}
		candidate := &rows[i]
		// A delegate row has only fail patterns: the delegate's own failure
		// markers, which apply to a run of any delegate (make runs npm).
		if parser, result := matchLine(line, candidate); result != "" {
			return parser, result
		}
	}
	return "", ""
}
