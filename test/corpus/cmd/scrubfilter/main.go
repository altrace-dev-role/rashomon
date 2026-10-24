// Command scrubfilter turns the unscrubbed candidates that tools/build_shell.py
// and tools/build_paths.py write into the committed data files: it drops every
// row whose subject the secret shape scrubber flags (shell lines only: for a
// path the fixture scanner below already applies the same entropy test, and a
// second pass would only count the same drops twice), or whose whole serialised row the fixture scanner flags
// (that scan is the one H-113 runs over every JSON Lines file), and keeps the
// rest byte for byte.
//
//	go run ./test/corpus/cmd/scrubfilter -field line CANDIDATES.jsonl > test/corpus/shell.jsonl
//	go run ./test/corpus/cmd/scrubfilter -field path CANDIDATES.jsonl > test/corpus/paths.jsonl
//
// A flagged row is dropped, never rewritten, so no row is a redaction of a
// secret and no placeholder stands in for one. The number dropped, and by
// which check, goes to stderr (counts only, never a line).
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/altrace-dev-role/rashomon/internal/secretshape"
	"github.com/altrace-dev-role/rashomon/test/corpus"
	"github.com/altrace-dev-role/rashomon/test/tools/hygiene"
)

func main() {
	field := flag.String("field", "", "row field holding the subject: line or path (required)")
	flag.Parse()
	if flag.NArg() != 1 || (*field != "line" && *field != "path") {
		fmt.Fprintln(os.Stderr, "usage: scrubfilter -field line|path CANDIDATES.jsonl > OUT.jsonl")
		os.Exit(2)
	}
	data, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "scrubfilter:", err)
		os.Exit(2)
	}
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	var kept, bySecret, byScan int
	for _, raw := range bytes.Split(data, []byte("\n")) {
		if len(raw) == 0 {
			continue
		}
		subject, report, err := judge(*field, raw)
		if err != nil {
			fmt.Fprintln(os.Stderr, "scrubfilter:", err)
			os.Exit(2)
		}
		switch {
		case *field == "line" && secretshape.Secretish(subject):
			bySecret++
		case len(hygiene.Scan("test/corpus/"+*field+".jsonl", append(append([]byte{}, raw...), '\n'))) > 0 ||
			len(hygiene.Scan("test/corpus/reports/"+*field+".txt", []byte(report))) > 0:
			byScan++
		default:
			kept++
			w.Write(raw)
			w.WriteByte('\n')
		}
	}
	fmt.Fprintf(os.Stderr, "scrubfilter: kept %d, dropped %d by secretshape, %d by hygiene.Scan\n", kept, bySecret, byScan)
}

// judge decodes one candidate row and returns its subject and the line the
// text report would print for it were it a disagreement, so the scan covers
// the worst case: the report line of a row that disagrees.
func judge(field string, raw []byte) (subject, report string, err error) {
	var j []corpus.Judged
	switch field {
	case "line":
		var r corpus.ShellRow
		if err = json.Unmarshal(raw, &r); err == nil {
			subject = r.Line
			j, err = corpus.JudgeShell([]corpus.ShellRow{r})
		}
	default:
		var r corpus.PathRow
		if err = json.Unmarshal(raw, &r); err == nil {
			subject = r.Path
			j, err = corpus.JudgePaths([]corpus.PathRow{r})
		}
	}
	if err != nil {
		return "", "", err
	}
	// Force the row to read as a disagreement whatever the classifier said.
	j[0].Want, j[0].Got = "want", "got"
	return subject, corpus.ReportLine(j[0]), nil
}
