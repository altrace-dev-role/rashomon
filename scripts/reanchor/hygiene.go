package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/altrace-dev-role/rashomon/test/tools/hygiene"
)

// hygieneReason is the reason every fingerprint this tool adds carries.
const hygieneReason = "benchmark provenance prose the re-anchor tool wrote (run, runner, toolchain, old and new figures): it holds no credential and no account value"

// hygienePlan is the change to test/tools/hygiene/reviewed.txt that goes with
// one rewrite of budgets.json.
type hygienePlan struct {
	path       string // reviewed.txt
	file       string // budgets.json as H-113 names it
	text       string // reviewed.txt after the change
	stale, add []string
	changed    bool
}

// planHygiene works out the reviewed.txt that H-113 needs once out replaces
// src, and writes nothing. It drops the file's entries that no finding
// matches any more. It adds an entry only for a finding that vanishes when
// the text this run composed (each basis, and set_from) is taken out of the
// file: such a finding is in that text. A finding that survives that is in
// text the tool did not write, and unless reviewed.txt already excuses it the
// plan refuses with it, named by line and shape as scrub -review does and
// never by value.
func planHygiene(o options, p *planned, src, out []byte) (*hygienePlan, error) {
	rel, err := filepath.Rel(o.root, o.budgets)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("%s is outside the module root %s: pass --no-hygiene", o.budgets, o.root)
	}
	h := &hygienePlan{file: filepath.ToSlash(rel), path: filepath.Join(o.root, filepath.FromSlash(hygiene.Reviewed))}
	raw, err := os.ReadFile(h.path)
	if err != nil {
		return nil, fmt.Errorf("%v (pass --no-hygiene to leave reviewed.txt alone)", err)
	}
	now := hygiene.Scan(h.file, out)
	elsewhere := map[string]bool{}
	for _, f := range hygiene.Scan(h.file, p.applyWith(src, true)) {
		elsewhere[f.Fingerprint] = true
	}
	live := map[string]bool{}
	for _, f := range now {
		live[f.Fingerprint] = true
	}
	excused := map[string]bool{}
	var kept []string
	for _, ln := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if f := strings.Fields(ln); len(f) >= 3 && f[0] == h.file {
			if !live[f[1]] {
				h.stale = append(h.stale, f[1])
				continue
			}
			excused[f[1]] = true
		}
		kept = append(kept, ln)
	}
	var outside []string
	for _, f := range now {
		if elsewhere[f.Fingerprint] && !excused[f.Fingerprint] {
			outside = append(outside, f.String())
		}
	}
	if len(outside) > 0 {
		return nil, fmt.Errorf("H-113 findings outside the text this run rewrote (nothing written; reanchor records only its own prose, so read these and judge them with go run ./test/tools/scrub -review %s):\n  %s",
			h.file, strings.Join(outside, "\n  "))
	}
	for _, f := range now {
		if excused[f.Fingerprint] {
			continue
		}
		excused[f.Fingerprint] = true
		h.add = append(h.add, f.Fingerprint)
		kept = append(kept, h.file+" "+f.Fingerprint+" "+hygieneReason)
	}
	h.changed = len(h.stale)+len(h.add) > 0
	h.text = strings.Join(kept, "\n") + "\n"
	return h, nil
}

func (h *hygienePlan) write(stdout io.Writer) error {
	if !h.changed {
		fmt.Fprintf(stdout, "reanchor: %s needs no change\n", hygiene.Reviewed)
		return nil
	}
	if err := os.WriteFile(h.path, []byte(h.text), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "reanchor: %s: %d stale %s entries dropped, %d added\n", hygiene.Reviewed, len(h.stale), h.file, len(h.add))
	return nil
}
