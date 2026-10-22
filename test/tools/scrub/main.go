// Command scrub rewrites a captured hook payload or session transcript so it
// can be committed as a fixture. Run it from the module root:
//
//	go run ./test/tools/scrub capture.ndjson > test/fixtures/x/capture.ndjson
//	go run ./test/tools/scrub -w test/fixtures/x/payload.json test/fixtures/x/transcript.jsonl
//
// Secret shapes become [redacted], email addresses and UUIDs stable
// placeholders, home, temp and scratchpad paths ~, $TMPDIR and $SCRATCHPAD,
// and each inventory list of an init event its count. Every file named in one
// run shares the placeholders, so a session id in a payload and in the
// transcript beside it reads the same in both.
//
// Afterwards each output is scanned as H-113 scans the repository. Anything
// left -- an inventory list inside text, a shape the patterns do not know --
// is printed by file and line, never by value, and the exit status is 1: the
// file still needs a person. A usage or I/O error is exit 2.
//
//	go run ./test/tools/scrub -review test/fixtures/nono/audit-events.ndjson
//
// rewrites nothing and prints each finding with its fingerprint, for a
// person who has read the value and judged it neither a secret nor an
// account's shape to record in test/tools/hygiene/reviewed.txt.
//
// It is not the shipped binary and is not linked into it, so a hook never
// runs it and its exit status binds nothing but this command line.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/test/tools/hygiene"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("scrub", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inPlace := flags.Bool("w", false, "rewrite each file in place instead of printing one to stdout")
	review := flags.Bool("review", false, "rewrite nothing; print each finding with the fingerprint "+hygiene.Reviewed+" takes")
	if flags.Parse(args) != nil {
		return 2
	}
	files := flags.Args()
	if len(files) == 0 || !*inPlace && !*review && len(files) > 1 {
		fmt.Fprintln(stderr, "usage: scrub FILE > OUT, scrub -w FILE..., or scrub -review FILE...")
		return 2
	}
	if *review {
		return reviewFiles(files, stdout, stderr)
	}
	home, _ := os.UserHomeDir()
	z := hygiene.NewScrubber(home)
	status := 0
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			fmt.Fprintln(stderr, "scrub:", err)
			return 2
		}
		out, err := z.Scrub(name, data)
		if err != nil {
			fmt.Fprintf(stderr, "scrub: %s: %v\n", name, err)
			return 2
		}
		for _, f := range hygiene.Scan(filepath.ToSlash(name), out) {
			fmt.Fprintln(stderr, "scrub: still there:", f)
			status = 1
		}
		if *inPlace {
			err = os.WriteFile(name, out, 0o644)
		} else {
			_, err = stdout.Write(out)
		}
		if err != nil {
			fmt.Fprintln(stderr, "scrub:", err)
			return 2
		}
	}
	return status
}

// reviewFiles prints what H-113 would find in each file, with the
// fingerprint an entry in the reviewed list takes. The fingerprint covers the
// path as given, so name the file from the module root, as the walk does.
// It is for the person deciding, on their own machine: CI never prints one.
func reviewFiles(files []string, stdout, stderr io.Writer) int {
	status := 0
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			fmt.Fprintln(stderr, "scrub:", err)
			return 2
		}
		for _, f := range hygiene.Scan(filepath.ToSlash(name), data) {
			fmt.Fprintf(stdout, "%s %s\n", f.Fingerprint, f)
			status = 1
		}
	}
	return status
}
