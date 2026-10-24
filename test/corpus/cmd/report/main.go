// Command report writes the four disagreement-report files of test/corpus.
//
//	go run ./test/corpus/cmd/report -commit 323e4075c6d5725741747b7ffc70cf8ae875fd8d -out test/corpus/reports
//
// The output depends on the data files and the classifier code alone: no
// clock, no map order, no environment. -commit only labels the report with
// the commit whose classifiers were read.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/test/corpus"
)

func main() {
	commit := flag.String("commit", "", "commit the classifiers were read at (required)")
	out := flag.String("out", "", "directory to write paths.txt, paths.json, shell.txt, shell.json into (required)")
	flag.Parse()
	if *commit == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: report -commit SHA -out DIR")
		os.Exit(2)
	}
	files, err := corpus.Build(*commit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "report:", err)
		os.Exit(1)
	}
	for _, name := range []string{"paths.json", "paths.txt", "shell.json", "shell.txt"} {
		prefix := map[string]string{"paths": "path", "shell": "shell"}[name[:5]]
		dst := filepath.Join(*out, prefix+"-disagreements"+filepath.Ext(name))
		if err := os.WriteFile(dst, files[name], 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "report:", err)
			os.Exit(1)
		}
	}
}
