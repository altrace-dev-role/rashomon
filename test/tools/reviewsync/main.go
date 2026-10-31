// Command reviewsync keeps test/tools/hygiene/reviewed.txt in step with a
// budgets.json that scripts/reanchor has just rewritten. Run it from the
// module root, which scripts/reanchor.sh does:
//
//	go run ./test/tools/reviewsync -neutral NEUTRAL.json [test/perf/budgets.json]
//
// FILE is the rewritten budgets.json. NEUTRAL is the same file with each
// string the re-anchor composed (a row's basis, a platform's set_from)
// replaced by "x", which reanchor writes under --neutral. The command drops
// FILE's entries in reviewed.txt that no finding matches any more, and adds
// one for each finding that is in FILE and not in NEUTRAL: such a finding
// lies inside the composed text, which is benchmark provenance prose, and
// the fixed reason says so. A finding that is in NEUTRAL too lies somewhere
// the re-anchor did not write; unless reviewed.txt already excuses it, the
// command refuses (exit 2, nothing written) and prints it by line and shape,
// never by value. It never records a finding in any other file.
//
// It is not the shipped binary and is not linked into it (H-17).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/altrace-dev-role/rashomon/test/tools/hygiene"
)

// reason is the reason every fingerprint this command adds carries.
const reason = "benchmark provenance prose the re-anchor tool wrote (run, runner, toolchain, old and new figures): it holds no credential and no account value"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reviewsync", flag.ContinueOnError)
	fs.SetOutput(stderr)
	neutral := fs.String("neutral", "", "the rewritten file with each composed string replaced by \"x\" (reanchor --neutral)")
	root := fs.String("root", ".", "module root")
	if fs.Parse(args) != nil {
		return 2
	}
	file := "test/perf/budgets.json"
	if fs.NArg() > 1 || *neutral == "" {
		fmt.Fprintln(stderr, "usage: reviewsync -neutral NEUTRAL.json [BUDGETS.json, relative to the module root]")
		return 2
	}
	if fs.NArg() == 1 {
		file = filepath.ToSlash(fs.Arg(0))
	}
	if err := sync(*root, file, *neutral, stdout); err != nil {
		fmt.Fprintln(stderr, "reviewsync:", err)
		return 2
	}
	return 0
}

func sync(root, file, neutralPath string, stdout io.Writer) error {
	rev := filepath.Join(root, filepath.FromSlash(hygiene.Reviewed))
	raw, err := os.ReadFile(rev)
	if err != nil {
		return err
	}
	now, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		return err
	}
	neutral, err := os.ReadFile(neutralPath)
	if err != nil {
		return err
	}
	found := hygiene.Scan(file, now)
	elsewhere := map[string]bool{}
	for _, f := range hygiene.Scan(file, neutral) {
		elsewhere[f.Fingerprint] = true
	}
	live := map[string]bool{}
	for _, f := range found {
		live[f.Fingerprint] = true
	}
	excused := map[string]bool{}
	var kept []string
	stale := 0
	for _, ln := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if f := strings.Fields(ln); len(f) >= 3 && f[0] == file {
			if !live[f[1]] {
				stale++
				continue
			}
			excused[f[1]] = true
		}
		kept = append(kept, ln)
	}
	var outside []string
	for _, f := range found {
		if elsewhere[f.Fingerprint] && !excused[f.Fingerprint] {
			outside = append(outside, f.String())
		}
	}
	if len(outside) > 0 {
		return fmt.Errorf("H-113 findings outside the text the re-anchor rewrote (nothing written; this command records only that text, so read these and judge them with go run ./test/tools/scrub -review %s):\n  %s",
			file, strings.Join(outside, "\n  "))
	}
	added := 0
	for _, f := range found {
		if excused[f.Fingerprint] {
			continue
		}
		excused[f.Fingerprint] = true
		added++
		kept = append(kept, file+" "+f.Fingerprint+" "+reason)
	}
	if stale+added == 0 {
		fmt.Fprintf(stdout, "reviewsync: %s needs no change\n", hygiene.Reviewed)
		return nil
	}
	if err := os.WriteFile(rev, []byte(strings.Join(kept, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "reviewsync: %s: %d stale %s entries dropped, %d added\n", hygiene.Reviewed, stale, file, added)
	return nil
}
