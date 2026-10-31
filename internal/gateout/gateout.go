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

// Classify reports the table-driven result of test runner output. Build
// runners and unknown programs do not scan text. The scan is O(len(text))
// with no per-line allocation; inputs over MaxScanBytes are unread.
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

	row := findProgram(program)
	if row == nil {
		return store.GateOutput{Kind: "unattributable"}
	}
	delegate := row.delegate
	if delegate {
		row = nil
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
			parser, result = matchDelegatedLine(line)
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
	return store.GateOutput{Kind: "no_summary", Parser: program}
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

func matchDelegatedLine(line []byte) (string, string) {
	rows := tables()
	for i := range rows {
		candidate := &rows[i]
		// A delegate row has only fail patterns: the delegate's own failure
		// markers, which apply to a run of any delegate (make runs npm).
		if parser, result := matchLine(line, candidate); result != "" {
			return parser, result
		}
	}
	return "", ""
}
