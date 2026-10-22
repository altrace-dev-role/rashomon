package shape

import "strings"

// runnerWords is what runnerDigest digests: the words of the runner whose
// program is the token at i, to the end of its command, with its
// redirections left out, so that `make test 2>&1 | tail` and a later plain
// `make test` are the same runner. A word glued to the one before it, an
// operator inside a word and a word this tokenizer split where the shell may
// not are marked, so that only the same tokens are the same runner.
func (p *lineParser) runnerWords(i, end int) string {
	var words []string
	for _, j := range p.words(i, end) {
		t := p.toks[j]
		w := t.text
		if t.meta {
			w = "\x01" + w
		}
		if t.opaque {
			w = "\x02" + w
		}
		if t.glued && j > i {
			w = "\x03" + w
		}
		words = append(words, w)
	}
	return strings.Join(words, "\x00")
}

// runnerDigest is the keyed digest of runner group g's words. Like the shape
// digest, compared and never printed; the domain is a name no tool has. Nil
// when a cd the start directory does not hold ran before it: where it ran is
// not known, so it neither is nor gets a follow-up.
func (p *lineParser) runnerDigest(g int, key []byte) *string {
	if p.runners[g].displaced {
		return nil
	}
	d := digest(key, "\x00runner", p.runners[g].key)
	return &d
}

// displace marks each runner that a cd, pushd or popd ran before on the
// line, other than the leading plain `cd DIR &&` steps the call's start
// directory folds in (folded of them, three tokens each).
func (p *lineParser) displace(folded int) {
	for _, s := range p.simples {
		if s.op != opCd || s.start < 3*folded && s.start%3 == 0 {
			continue
		}
		for g := range p.runners {
			for _, at := range p.runners[g].starts {
				if s.start < at {
					p.runners[g].displaced = true
				}
			}
		}
	}
}

// evalState is the abstract run's state: the last status (-1 not known),
// the variables an assignment set, and the two options.
type evalState struct {
	status            int
	vars              map[string]int
	errexit, pipefail bool
	// exited: an exit, or errexit, ended the line (or its subshell). lost:
	// the run cannot go on, a status it branches on not being known.
	exited, lost bool
}

func (st *evalState) copy() *evalState {
	c := *st
	c.vars = make(map[string]int, len(st.vars))
	for k, v := range st.vars {
		c.vars[k] = v
	}
	c.exited = false
	return &c
}

type evaluator struct {
	p      *lineParser
	target int
	fail   bool
	zsh    bool // errexit never fires, as in zsh under Claude Code's wrapper
	ran    bool
}

// runShells runs the line with runner group g failing and then passing, as
// bash runs it and, where a set on the line turns errexit on, again as zsh
// runs it inside Claude Code's `eval '<line>' && ...` wrapper, where errexit
// never fires. It returns the two exit statuses and whether the runner ran;
// -1 for both where either run cannot tell, or where the two shells disagree
// on whether the runner's status reaches the line's: the record does not say
// which shell ran it.
func (p *lineParser) runShells(l list, g int) (f, pass int, ran bool) {
	f, ran = p.run(l, g, true, false)
	// Whether the runner is reached depends only on what runs before it, the
	// same in both runs, so ran comes from the first.
	pass, _ = p.run(l, g, false, false)
	if !p.errexit {
		// Without errexit the two shells run it alike.
		return f, pass, ran
	}
	zf, zran := p.run(l, g, true, true)
	// As above: zran comes from the first of the two zsh runs.
	zpass, _ := p.run(l, g, false, true)
	if zf < 0 || zpass < 0 || (f == 0 || f == pass) != (zf == 0 || zf == zpass) {
		return -1, -1, ran && zran
	}
	return f, pass, ran && zran
}

// run runs the line with runner group g failing (fail) or passing, every
// other recognised runner passing, errexit never firing where zsh, and
// returns its exit status, -1 when it cannot tell, and whether the runner
// ran.
func (p *lineParser) run(l list, g int, fail, zsh bool) (int, bool) {
	ev := &evaluator{p: p, target: g, fail: fail, zsh: zsh}
	st := &evalState{vars: map[string]int{}}
	ev.list(st, l, false)
	if st.lost {
		return -1, ev.ran
	}
	return st.status, ev.ran
}

// list runs l. noErr: errexit is ignored here.
func (ev *evaluator) list(st *evalState, l list, noErr bool) {
	for _, it := range l.items {
		if st.exited || st.lost {
			return
		}
		if it.bg {
			// Its status is no one's: the line goes on with 0.
			ev.andOr(st.copy(), it.ao, true)
			st.status = 0
			continue
		}
		ev.andOr(st, it.ao, noErr)
	}
}

func (ev *evaluator) andOr(st *evalState, ao andOr, noErr bool) {
	for k, pl := range ao.pipes {
		if k > 0 {
			if st.status < 0 {
				st.lost = true
				return
			}
			if ao.ops[k-1] == "&&" && st.status != 0 || ao.ops[k-1] == "||" && st.status == 0 {
				continue
			}
		}
		// errexit ignores every pipeline of the list but the last, and a
		// negated one.
		ignored := noErr || k < len(ao.pipes)-1 || pl.neg
		ev.pipeline(st, pl, ignored)
		if st.exited || st.lost {
			return
		}
		if st.errexit && !ignored && !ev.zsh && st.status != 0 {
			if st.status < 0 {
				st.lost = true
				return
			}
			st.exited = true
			return
		}
	}
}

func (ev *evaluator) pipeline(st *evalState, pl pipeline, noErr bool) {
	if len(pl.stages) == 1 {
		ev.command(st, pl.stages[0], noErr)
		if st.exited || st.lost {
			return
		}
	} else {
		// Each stage is a subshell; the status is the last stage's, or under
		// pipefail the last that failed.
		statuses := make([]int, len(pl.stages))
		for n, c := range pl.stages {
			sub := st.copy()
			ev.command(sub, c, noErr)
			if sub.lost {
				st.lost = true
				return
			}
			statuses[n] = sub.status
		}
		s := statuses[len(statuses)-1]
		if st.pipefail {
			s = 0
			for _, x := range statuses {
				if x != 0 {
					s = x
				}
			}
		}
		for _, x := range statuses {
			if x < 0 && (st.pipefail || s < 0) {
				s = -1
			}
		}
		st.status = s
	}
	if pl.neg && st.status >= 0 {
		if st.status == 0 {
			st.status = 1
		} else {
			st.status = 0
		}
	}
}

func (ev *evaluator) command(st *evalState, c command, noErr bool) {
	switch c := c.(type) {
	case *simple:
		ev.simple(st, c)
	case *subshell:
		sub := st.copy()
		ev.list(sub, c.body, noErr)
		if sub.lost {
			st.lost = true
			return
		}
		st.status = sub.status
	case *group:
		ev.list(st, c.body, noErr)
	case *ifCmd:
		for k, cond := range c.conds {
			ev.list(st, cond, true)
			if st.exited || st.lost {
				return
			}
			if st.status < 0 {
				st.lost = true
				return
			}
			if st.status == 0 {
				ev.list(st, c.bodies[k], noErr)
				return
			}
		}
		if c.els != nil {
			ev.list(st, *c.els, noErr)
			return
		}
		st.status = 0
	case *loop:
		if c.unsure {
			st.lost = true
			return
		}
		st.status = 0
	}
}

func (ev *evaluator) simple(st *evalState, c *simple) {
	switch c.op {
	case opFalse:
		st.status = 1
	case opStatusRead:
		st.status = -1
	case opAssign:
		for _, a := range c.assigns {
			st.vars[a.name] = ev.value(st, a.val)
		}
		st.status = 0
	case opSet:
		for _, o := range c.opts {
			if o.errexit {
				st.errexit = o.on
			} else {
				st.pipefail = o.on
			}
		}
		st.status = 0
	case opExit:
		switch len(c.args) {
		case 0:
		case 1:
			st.status = ev.value(st, c.args[0])
		default:
			// Too many arguments: bash does not exit.
			st.lost = true
			return
		}
		st.exited = true
	case opRunner:
		st.status = 0
		if c.runner == ev.target {
			ev.ran = true
			if ev.fail {
				st.status = 1
			}
		}
	default:
		st.status = 0
	}
}

// value is v's number in st, -1 when it is not known.
func (ev *evaluator) value(st *evalState, v value) int {
	switch v.kind {
	case valLiteral:
		return v.n
	case valStatus:
		return st.status
	case valVar:
		if n, ok := st.vars[v.name]; ok {
			return n
		}
	}
	return -1
}
