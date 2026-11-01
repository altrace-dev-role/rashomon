package shape

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// PrintedStatusRulesVersion is the first RulesVersion at which a masked
// runner's exit status, where the line printed it, is read from the output
// (STATUS-PROBE, FALSE-FIRE1): gate_output printed_pass or printed_fail. A
// reader that takes an outcome from those words asks this of the record first.
const PrintedStatusRulesVersion = 12

// PrintedStatus is the exit status a line printed for the runner whose status
// it hid (STATUS-PROBE): `make check 2>&1; echo "EXIT: $?"` returns echo's 0
// whether make failed or not, and prints make's status on a line of its own.
// ok is false wherever the output does not say it for certain:
//
//   - The line hides exactly one runner's status (statusMasked's reading, on
//     the same parse), no hiding flag stands on it, and every run of it is a
//     command of the line's top-level list, alone or last after &&, so that
//     the status the next command reads is the runner's whenever it ran and
//     some earlier command's non-zero status when it did not.
//   - A run's status is printed by an `echo` with no redirection (a flag
//     is a word of the template, which echo does not print): as $? or ${PIPESTATUS[n]} (bash, from 0) or
//     ${pipestatus[n]} (zsh, from 1) in the command right after the run, the
//     index the runner's stage; or as $NAME where the command right after the
//     run is NAME=$? and nothing else on the line names NAME. Each variable is
//     empty in the other shell, so a reading for the wrong shell prints no
//     digits and reads nothing.
//   - The echo's whole output line is its words joined by single spaces with
//     each status a run of one to three digits: the echo's text is the
//     template, read in memory and never kept, and a line of output reads only
//     where it is that template from its first byte to its end. Any other
//     expansion, a $ the shell prints as it is ('$?', \$?), a backslash or a
//     glob character in an echo's words, and the echo gives no template, so
//     text the runner itself printed, or a log a later tail printed, is never
//     read in the echo's place.
//   - The last run of the runner decides, as a later run of the same runner
//     supersedes an earlier one in the record. Its status must be printed;
//     a run printed on two lines with different digits, or any other run's,
//     reads nothing.
//
// The digits are the status: 0 passed, any other failed. Only that bit is
// recorded (gate_output.kind); neither the digits nor any word of the line is.
func PrintedStatus(toolName string, toolInput json.RawMessage, out []byte) (code int, ok bool) {
	if verbForTool(toolName) != VerbExecute {
		return 0, false
	}
	cmd, isShell := commandField(toolInput)
	if !isShell {
		return 0, false
	}
	pshaped, perr := tokenizeProgram(cmd)
	uncertain := perr == errUncertain || perr == errTokenBound
	p, body, hidden, _, ok := maskedRun(cmd, pshaped[:leadingFold(pshaped, uncertain)])
	if !ok || len(hidden) != 1 {
		return 0, false
	}
	lines, last, ok := p.printedLines(body, hidden[0])
	if !ok || len(lines) == 0 {
		return 0, false
	}
	return readPrinted(lines, last, out)
}

// StatusHidden reports a status_masked that says the line hid a runner's
// status: test or build, not none and not null.
func StatusHidden(masked *string) bool {
	return masked != nil && (*masked == MaskedTest || *masked == MaskedBuild)
}

// probeLine is one echo's output line as a template: lits[0], a status, lits[1],
// ..., lits[n]. runs[k] is the run of the runner whose status the k-th hole
// prints, in line order from 0, or -1 for another command's status, whose
// digits are matched and not read.
type probeLine struct {
	lits []string
	runs []int
}

// printedLines walks the line's top-level list for the runs of runner group g
// and the echoes that print their status: the templates, and the index of the
// last run. ok is false where a run is not where PrintedStatus reads one.
func (p *lineParser) printedLines(body list, g int) (lines []probeLine, last int, ok bool) {
	var (
		runs = 0
		// prev is the run whose status the next command reads as $?, -1 where
		// the item before was not a run; stage and stages are its pipe's.
		prev, stage, stages = -1, 0, 0
		vars                = map[string]int{}
	)
	for _, it := range body.items {
		k, n, holds, ok := p.runIn(it, g)
		if !ok {
			return nil, 0, false
		}
		if holds {
			prev, stage, stages = runs, k, n
			runs++
			continue
		}
		run, rStage, rStages := prev, stage, stages
		prev = -1
		c := soleSimple(it)
		if c == nil {
			continue
		}
		switch {
		case c.op == opAssign && p.statusAssignment(c):
			// NAME=$? right after a run names that run; any other NAME=$? a
			// status of something else.
			name := c.assigns[0].name
			vars[name] = -1
			if run >= 0 && rStage == rStages-1 && p.namedOnce(name) {
				vars[name] = run
			}
		case c.op == opOther:
			if l, ok := p.echoTemplate(c, run, rStage, rStages, vars); ok {
				lines = append(lines, l)
			}
		}
	}
	if runs == 0 || runs != len(p.runners[g].starts) {
		return nil, 0, false
	}
	return lines, runs - 1, true
}

// runIn reports whether the list item runs group g, and where it does, the
// runner's stage in the item's last pipeline and that pipeline's length. ok
// is false where g runs in the item anywhere but as a stage of the last
// pipeline of an && chain, unnegated and in the foreground, or with a hiding
// flag on it.
func (p *lineParser) runIn(it listItem, g int) (stage, stages int, holds, ok bool) {
	pipes := it.ao.pipes
	for i, pl := range pipes {
		for k, c := range pl.stages {
			if !p.holds(c, g) {
				continue
			}
			s, isSimple := c.(*simple)
			if !isSimple || s.hides || it.bg || pl.neg || i != len(pipes)-1 || holds {
				return 0, 0, false, false
			}
			for _, op := range it.ao.ops {
				if op != "&&" {
					return 0, 0, false, false
				}
			}
			stage, stages, holds = k, len(pl.stages), true
		}
	}
	return stage, stages, holds, true
}

// soleSimple is the item's command where the item is one simple command in
// the foreground, else nil.
func soleSimple(it listItem) *simple {
	if it.bg || len(it.ao.pipes) != 1 || it.ao.pipes[0].neg || len(it.ao.pipes[0].stages) != 1 {
		return nil
	}
	s, _ := it.ao.pipes[0].stages[0].(*simple)
	return s
}

// statusAssignment reports a command that is one assignment of $?.
func (p *lineParser) statusAssignment(c *simple) bool {
	return len(c.assigns) == 1 && c.assigns[0].val.kind == valStatus && len(p.words(c.start, c.end)) == 1
}

// namedOnce reports that name is assigned by one command of the line and is
// no word of it on its own (`for NAME in`, `read NAME`), so that $NAME where
// it is read holds the status the one assignment gave it.
func (p *lineParser) namedOnce(name string) bool {
	n := 0
	for _, t := range p.toks {
		if t.text == name || strings.HasPrefix(t.text, name+"=") || strings.HasPrefix(t.text, name+"+=") {
			n++
		}
	}
	return n == 1
}

// echoTemplate is the output line of c where c is an echo whose words are a
// template PrintedStatus reads (see there): run, its runner's stage and its
// pipe's stages, and vars say which run each status it prints is. ok is false
// for any other command, and for an echo none of whose statuses is a run's.
func (p *lineParser) echoTemplate(c *simple, run, stage, stages int, vars map[string]int) (probeLine, bool) {
	ws := p.words(c.start, c.end)
	if len(ws) < 2 || len(ws) != c.end-c.start {
		return probeLine{}, false
	}
	if t := p.toks[ws[0]]; t.meta || t.opaque || t.quotedAt >= 0 || t.text != "echo" {
		return probeLine{}, false
	}
	var (
		l     = probeLine{lits: []string{""}}
		bound bool
	)
	for n, j := range ws[1:] {
		t := p.toks[j]
		if t.meta || t.ticks > 0 || t.litDollar {
			return probeLine{}, false
		}
		if n > 0 {
			l.lits[len(l.lits)-1] += " "
		}
		w := t.text
		for w != "" {
			i := strings.IndexByte(w, '$')
			if i < 0 {
				i = len(w)
			}
			if strings.ContainsAny(w[:i], "\\*?[]{}~") {
				return probeLine{}, false
			}
			l.lits[len(l.lits)-1] += w[:i]
			if i == len(w) {
				break
			}
			r, size := statusRef(w[i:], run, stage, stages, vars)
			if size == 0 || l.lits[len(l.lits)-1] == "" && len(l.runs) > 0 {
				// Not a status, or two statuses with nothing between them,
				// whose digits no reading can split.
				return probeLine{}, false
			}
			l.runs = append(l.runs, r)
			l.lits = append(l.lits, "")
			bound = bound || r >= 0
			w = w[i+size:]
		}
	}
	return l, bound
}

// statusRef reads the status reference that opens w: its run (-1 for another
// command's status) and its length, 0 where w opens with no reference
// PrintedStatus reads. run is the run the item before the echo was, -1 for
// none, stage its runner's stage and stages its pipe's length.
func statusRef(w string, run, stage, stages int, vars map[string]int) (int, int) {
	switch {
	case strings.HasPrefix(w, "$?"):
		// $? is the pipe's last stage's status.
		if run >= 0 && stage == stages-1 {
			return run, 2
		}
		return -1, 2
	case strings.HasPrefix(w, "${PIPESTATUS[") || strings.HasPrefix(w, "${pipestatus["):
		from := 0
		if w[2] == 'p' {
			from = 1 // zsh counts from 1
		}
		end := strings.Index(w, "]}")
		if end < 0 {
			return 0, 0
		}
		n, err := strconv.Atoi(w[len("${PIPESTATUS["):end])
		if err != nil || n < from {
			return 0, 0
		}
		if run >= 0 && n-from == stage {
			return run, end + 2
		}
		return -1, end + 2
	}
	name, size := "", 0
	if strings.HasPrefix(w, "${") {
		end := strings.IndexByte(w, '}')
		if end < 0 {
			return 0, 0
		}
		name, size = w[2:end], end+1
	} else {
		size = 1
		for size < len(w) && isName(w[1:size+1]) {
			size++
		}
		name = w[1:size]
	}
	if r, ok := vars[name]; ok && isName(name) {
		return r, size
	}
	return 0, 0
}

// readPrinted reads the status of run last from out with the templates of
// lines (PrintedStatus). Each line of out is matched against each template;
// a run read with two different values reads nothing, nor does a last run
// never read.
func readPrinted(lines []probeLine, last int, out []byte) (int, bool) {
	vals := map[int]int{}
	conflict := false
	for len(out) > 0 {
		line := out
		if i := bytes.IndexByte(out, '\n'); i >= 0 {
			line, out = out[:i], out[i+1:]
		} else {
			out = nil
		}
		for _, l := range lines {
			got, ok := l.match(line)
			if !ok {
				continue
			}
			for h, r := range l.runs {
				if r < 0 {
					continue
				}
				if v, seen := vals[r]; seen && v != got[h] {
					conflict = true
				}
				vals[r] = got[h]
			}
		}
	}
	v, seen := vals[last]
	if conflict || !seen {
		return 0, false
	}
	return v, true
}

// match reads line as the template: each literal in place and each status a
// run of one to three digits no greater than 255, the whole line and nothing
// past it.
func (l probeLine) match(line []byte) ([]int, bool) {
	got := make([]int, 0, len(l.runs))
	for k, lit := range l.lits {
		if !bytes.HasPrefix(line, []byte(lit)) {
			return nil, false
		}
		line = line[len(lit):]
		if k == len(l.runs) {
			break
		}
		n := 0
		for n < len(line) && n < 4 && line[n] >= '0' && line[n] <= '9' {
			n++
		}
		if n == 0 || n > 3 {
			return nil, false
		}
		v, _ := strconv.Atoi(string(line[:n]))
		if v > 255 {
			return nil, false
		}
		got = append(got, v)
		line = line[n:]
	}
	return got, len(line) == 0
}
