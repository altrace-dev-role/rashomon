package shape

import (
	"path"
	"strconv"
	"strings"
)

// shells are the programs whose -c runs a line of its own, which is not
// looked into.
var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "mksh": true}

func (p *lineParser) parseSimple() (command, bool) {
	s := p.pos
	e, ok := p.simpleEnd(s)
	if !ok || e == s {
		return nil, false
	}
	p.pos = e
	c := &simple{start: s, end: e, runner: -1}
	p.simples = append(p.simples, c)
	toks := p.toks
	ws := p.words(s, e)
	if len(ws) == 0 {
		// Redirections only.
		return c, true
	}

	if assigns, ok := p.assignments(ws); ok {
		c.op, c.assigns = opAssign, assigns
		return c, true
	}

	k, ok := programToken(toks[s:e], false)
	if !ok {
		return nil, false
	}
	i := s + k
	prog := path.Base(toks[i].text)
	var args []int
	for _, j := range ws {
		if j > i {
			args = append(args, j)
		}
	}
	if runsShellLine(toks, ws, i) {
		return nil, false
	}
	if toks[i].quotedAt < 0 {
		switch prog {
		case "eval", "trap", "return", "exec", "function", "coproc", "select", "case", "alias":
			// What they run, or what they make the line do, is not on it.
			return nil, false
		case "set":
			opts, ok := setOptions(toks, args)
			if !ok {
				return nil, false
			}
			c.op, c.opts = opSet, opts
			for _, o := range opts {
				if o.errexit && o.on {
					p.errexit = true
				}
			}
			return c, true
		case "exit":
			c.op = opExit
			for _, j := range args {
				c.args = append(c.args, p.valueOf(toks[j]))
			}
			return c, true
		case "true", ":":
			c.op = opTrue
			return c, true
		case "false":
			c.op = opFalse
			return c, true
		case "[", "test", "let", "expr":
			if p.readsStatus(i+1, e) {
				c.op = opStatusRead
			}
			return c, true
		case "wait":
			if len(args) > 0 {
				c.op = opStatusRead
			}
			return c, true
		case "cd", "pushd", "popd":
			c.op = opCd
			return c, true
		case "export", "local", "declare", "typeset", "readonly":
			c.op = opAssign
			for _, j := range args {
				if t := toks[j]; !t.meta && isShellAssignment(t) {
					name, v := splitAssignment(t.text)
					p.assigned[name] = true
					c.assigns = append(c.assigns, assignment{name, p.valueOf(token{text: v, quotedAt: -1})})
				}
			}
			return c, true
		}
	}

	c.stage = stageOf(toks, i, prog, args)
	return p.runner(c, s, i, e, prog)
}

// runsShellLine reports a shell's -c anywhere on the command whose words are
// ws and whose program is toks[i] -- `bash -c`, `sudo sh -lc` -- which runs a
// line of its own that is not looked into.
func runsShellLine(toks []token, ws []int, i int) bool {
	for n, j := range ws {
		if t := toks[j]; j < i || t.meta || t.opaque || !shells[path.Base(t.text)] {
			continue
		}
		for _, a := range ws[n+1:] {
			if w := toks[a].text; len(w) > 1 && w[0] == '-' && w[1] != '-' && strings.ContainsRune(w, 'c') {
				return true
			}
		}
	}
	return false
}

// runner marks the simple command c, the tokens s to e whose program prog is
// at i, as a run of a test or build runner when it is one, in the group of
// the runs with the same words, and as hiding its result when a hiding flag
// stands on it (runnerOn); any other command is returned as it is.
func (p *lineParser) runner(c *simple, s, i, e int, prog string) (command, bool) {
	toks := p.toks
	ri, name, ok := runnerPrefix(toks[:e], i, prog)
	if !ok {
		return c, true
	}
	kind, names := MaskedTest, testCommandNames
	row, hides := runnerRow(toks, ri, name, e, testCommands)
	if row < 0 {
		kind, names = MaskedBuild, buildCommandNames
		if row, hides = runnerRow(toks, ri, name, e, buildCommands); row < 0 {
			return c, true
		}
	}
	key := p.runnerWords(ri, e)
	g, seen := p.groups[key]
	if !seen {
		g = len(p.runners)
		p.groups[key] = g
		p.runners = append(p.runners, runnerGroup{key: key, kind: kind, name: &names[row]})
	}
	p.runners[g].starts = append(p.runners[g].starts, s)
	p.runners[g].last = s
	c.op, c.runner, c.hides = opRunner, g, hides
	return c, true
}

// assignments reads a command of assignments only, `rc=$?` and the like.
// False when a word is not an assignment, or is one whose value holds an
// expansion this tokenizer does not parse and is not one value reads.
func (p *lineParser) assignments(ws []int) ([]assignment, bool) {
	var out []assignment
	for _, j := range ws {
		t := p.toks[j]
		if t.meta || !isShellAssignment(t) {
			return nil, false
		}
		name, v := splitAssignment(t.text)
		val := p.valueOf(token{text: v, quotedAt: -1})
		if t.opaque && val.kind == valUnknown {
			return nil, false
		}
		out = append(out, assignment{name, val})
	}
	for _, a := range out {
		p.assigned[a.name] = true
	}
	return out, true
}

// splitAssignment splits NAME=value; a subscript or += leaves a name no
// reader looks up.
func splitAssignment(w string) (name, val string) {
	eq := strings.IndexByte(w, '=')
	return w[:eq], w[eq+1:]
}

// isName reports a shell variable name.
func isName(s string) bool {
	if s == "" {
		return false
	}
	for j := 0; j < len(s); j++ {
		c := s[j]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c >= '0' && c <= '9' && j > 0) {
			return false
		}
	}
	return true
}

// valueOf reads a word as a value: digits (or a sign and digits), $?, or a
// variable, $NAME or ${NAME}. PIPESTATUS is not read: zsh has none, so its
// value is unknown.
func (p *lineParser) valueOf(t token) value {
	w := t.text
	if t.meta {
		return value{}
	}
	switch {
	case w == "$?":
		return value{kind: valStatus}
	case strings.HasPrefix(w, "${") && strings.HasSuffix(w, "}") && isName(w[2:len(w)-1]):
		return value{kind: valVar, name: w[2 : len(w)-1]}
	case strings.HasPrefix(w, "$") && isName(w[1:]):
		return value{kind: valVar, name: w[1:]}
	}
	digits := strings.TrimPrefix(w, "-")
	if digits == "" || strings.Trim(digits, "0123456789") != "" || len(digits) > 18 {
		return value{}
	}
	n, err := strconv.ParseInt(w, 10, 64)
	if err != nil {
		return value{}
	}
	return value{kind: valLiteral, n: int(((n % 256) + 256) % 256)}
}

// readsStatus reports a word of [s, e) that reads a status: $?, PIPESTATUS,
// or a variable an assignment on the line set.
func (p *lineParser) readsStatus(s, e int) bool {
	for j := s; j < e; j++ {
		w := p.toks[j].text
		if strings.Contains(w, "$?") || strings.Contains(w, "PIPESTATUS") {
			return true
		}
		for name := range p.assigned {
			if strings.Contains(w, "$"+name) || strings.Contains(w, "${"+name) {
				return true
			}
		}
	}
	return false
}

// setOptions reads a `set` command's arguments for the two options that
// change whether a status reaches the line's: errexit (`-e`, in any cluster
// of short options such as -euo, or `-o errexit`) and pipefail (`-o
// pipefail`), and their `+` forms, which turn them off. A cluster holding o
// takes the next word as the option's name. False on a word this tokenizer
// cannot vouch for, and on a cluster where o is not last, which bash and zsh
// read differently.
func setOptions(toks []token, args []int) ([]setOpt, bool) {
	var out []setOpt
	for n := 0; n < len(args); n++ {
		t := toks[args[n]]
		if t.meta || t.opaque {
			return nil, false
		}
		w := t.text
		if w == "--" || w == "-" {
			break
		}
		if len(w) < 2 || w[0] != '-' && w[0] != '+' {
			continue
		}
		on := w[0] == '-'
		for k, c := range w[1:] {
			switch c {
			case 'e':
				out = append(out, setOpt{errexit: true, on: on})
			case 'o':
				if k < len(w)-2 {
					// o before the cluster's end, `-opipefail`: bash rejects
					// the word and zsh reads the rest as the name.
					return nil, false
				}
				if n+1 >= len(args) {
					continue
				}
				n++
				switch toks[args[n]].text {
				case "pipefail":
					out = append(out, setOpt{errexit: false, on: on})
				case "errexit":
					out = append(out, setOpt{errexit: true, on: on})
				}
			}
		}
	}
	return out, true
}
