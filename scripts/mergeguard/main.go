// Command mergeguard is the report-only merge guard's planner (6.4, v0).
//
// mergeguard.sh reads a commit's Rashomon-Receipt trailer and its note in
// refs/notes/rashomon and hands both here: the trailer's value as -trailer,
// the note on stdin. This program checks that the note is the record the
// trailer names, reads the checkpoint's gates, and prints a POSIX sh script
// that reruns, in the repository at -root, each gate the repository itself
// declares in its spec (-spec): a "spec:<id>" gate by that gate's argv in its
// dir, a "ci:<n>" gate by the n-th ci_commands entry. The commands come from
// the repository and never from the note: the verifier does not run what the
// claim tells it to (PF-RG2, the verifier is not the writer). A learned gate
// declares no command and is not rerun.
//
// The script prints one line a gate, "<gate>: checkpoint <state>, rerun
// <state>" (green when the command exits 0, red otherwise, or "not declared"
// and "not runnable"), then "mergeguard: <n> agree, <m> differ, <k> not
// rerun". It never fails a build in v0: the script exits 0, and so does this
// program on anything but a usage error. Every datum the note or the spec
// carries reaches the script single-quoted.
//
// This program runs nothing (H-17: no package of this module imports
// os/exec); mergeguard.sh runs git and the script.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/receipt"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// maxNote bounds the note read: a checkpoint is at most
// verify.CheckpointMaxBytes, and a note far past that is not one.
const maxNote = 16 * verify.CheckpointMaxBytes

func main() {
	os.Exit(guard(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// guard is the program: flags, the note, the spec, and the script on stdout.
func guard(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("mergeguard", flag.ContinueOnError)
	set.SetOutput(stderr)
	trailer := set.String("trailer", "", "the commit's Rashomon-Receipt value")
	root := set.String("root", ".", "the repository's top directory")
	specPath := set.String("spec", "", "the spec (default <root>/.claude/rashomon/spec.json)")
	if err := set.Parse(args); err != nil || set.NArg() > 0 {
		return 2
	}
	if *specPath == "" {
		*specPath = filepath.Join(*root, ".claude", "rashomon", "spec.json")
	}
	note, err := io.ReadAll(io.LimitReader(stdin, maxNote+1))
	var p planner
	switch {
	case err != nil:
		p.say("mergeguard: the note could not be read")
	case len(note) > maxNote:
		p.say(fmt.Sprintf("mergeguard: the note is over %d bytes; not a checkpoint", maxNote))
	default:
		p.plan(*trailer, note, *root, *specPath)
	}
	io.WriteString(stdout, p.script()) //nolint:errcheck // mergeguard.sh reports a short script by its summary line
	return 0
}

// planner builds the script.
type planner struct {
	body strings.Builder
}

// q is s single-quoted for sh: every byte stands for itself.
func q(s string) string { return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'" }

// say prints line from the script.
func (p *planner) say(line string) { fmt.Fprintf(&p.body, "printf '%%s\\n' %s\n", q(line)) }

// script is the body between the counters and the summary.
func (p *planner) script() string {
	return "#!/bin/sh\n# mergeguard's rerun script (6.4, v0)\na=0 d=0 n=0\n" + p.body.String() +
		"printf 'mergeguard: %d agree, %d differ, %d not rerun\\n' \"$a\" \"$d\" \"$n\"\nexit 0\n"
}

// plan checks the note against the trailer and plans each gate.
func (p *planner) plan(trailer string, note []byte, root, specPath string) {
	record := strings.TrimSuffix(string(note), "\n")
	if got := receipt.Digest([]byte(record)); got != trailer {
		p.say("mergeguard: the note's digest " + got + " is not the trailer's " + trailer)
		return
	}
	var cp verify.Checkpoint
	if err := json.Unmarshal([]byte(record), &cp); err != nil || cp.Type != store.TypeVerify {
		p.say("mergeguard: the note is not a checkpoint record")
		return
	}
	declared := p.readSpec(specPath)
	if cp.GatesOmitted > 0 {
		p.say(fmt.Sprintf("mergeguard: the checkpoint omits %d gates (cut to its budget)", cp.GatesOmitted))
	}
	for _, g := range cp.Gates {
		p.gate(g.Gate, string(g.State), root, declared)
	}
}

// command is a declared gate's argv and the directory it runs in.
type command struct {
	dir  string
	argv []string
}

// readSpec is the spec's declared commands by gate id; a spec that is absent
// or does not parse declares none, and the script says which.
func (p *planner) readSpec(path string) map[string]command {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		p.say("mergeguard: no spec at " + path + "; no gate is declared")
		return nil
	}
	if err != nil {
		p.say("mergeguard: the spec could not be read; no gate is declared")
		return nil
	}
	f, _, err := spec.Parse(data)
	if err != nil {
		p.say("mergeguard: the spec does not parse (" + err.Error() + "); no gate is declared")
		return nil
	}
	declared := make(map[string]command, len(f.Gates)+len(f.CICommands))
	for _, g := range f.Gates {
		declared["spec:"+g.ID] = command{g.Dir, g.Argv}
	}
	for i, argv := range f.CICommands {
		declared["ci:"+strconv.Itoa(i)] = command{"", argv}
	}
	return declared
}

// gate plans one gate: its rerun in a subshell, from the repository's root,
// when declared and runnable, its line, and its count.
func (p *planner) gate(id, state, root string, declared map[string]command) {
	c, ok := declared[id]
	if !ok || !runnable(c) {
		rerun := "not declared"
		if ok {
			rerun = "not runnable"
		}
		fmt.Fprintf(&p.body, "n=$((n + 1))\nprintf '%%s: checkpoint %%s, rerun %%s\\n' %s %s %s\n", q(id), q(state), q(rerun))
		return
	}
	words := make([]string, len(c.argv))
	for i, w := range c.argv {
		words[i] = q(w)
	}
	cd := "cd " + q(root)
	if c.dir != "" {
		cd += " && cd " + q(c.dir)
	}
	fmt.Fprintf(&p.body, "if (%s && exec %s) 1>&2; then r=green; else r=red; fi\n", cd, strings.Join(words, " "))
	fmt.Fprintf(&p.body, "if [ \"$r\" = %s ]; then a=$((a + 1)); else d=$((d + 1)); fi\n", q(state))
	fmt.Fprintf(&p.body, "printf '%%s: checkpoint %%s, rerun %%s\\n' %s %s \"$r\"\n", q(id), q(state))
}

// runnable is whether sh can carry the command: an argv to run, and no NUL,
// which no shell word can hold.
func runnable(c command) bool {
	if len(c.argv) == 0 || strings.ContainsRune(c.dir, 0) {
		return false
	}
	for _, w := range c.argv {
		if strings.ContainsRune(w, 0) {
			return false
		}
	}
	return true
}
