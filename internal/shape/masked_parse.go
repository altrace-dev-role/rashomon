package shape

import "strings"

// parseLine parses the whole line. False where any part of it is not read:
// the line is then null.
func (p *lineParser) parseLine() (list, bool) {
	l, ok := p.parseList()
	return l, ok && p.pos == len(p.toks)
}

// word reports an unquoted word at j that is w: a reserved word, in command
// position.
func (p *lineParser) word(j int, w string) bool {
	if j >= len(p.toks) {
		return false
	}
	t := p.toks[j]
	return !t.meta && t.quotedAt < 0 && !t.opaque && t.text == w
}

func (p *lineParser) op(j int, op string) bool {
	return j < len(p.toks) && p.toks[j].meta && p.toks[j].text == op
}

// atTerminator reports the end of the line, or a token that ends the list
// being read: a subshell's `)`, or a reserved word that closes or continues
// a compound command.
func (p *lineParser) atTerminator() bool {
	if p.pos >= len(p.toks) || p.op(p.pos, ")") {
		return true
	}
	for _, w := range []string{"}", "then", "elif", "else", "fi", "do", "done", "esac"} {
		if p.word(p.pos, w) {
			return true
		}
	}
	return false
}

func (p *lineParser) parseList() (list, bool) {
	var l list
	for !p.atTerminator() {
		ao, ok := p.parseAndOr()
		if !ok {
			return l, false
		}
		it := listItem{ao: ao}
		switch {
		case p.op(p.pos, ";"):
			p.pos++
		case p.op(p.pos, "&"):
			it.bg = true
			p.pos++
		case p.pos < len(p.toks) && p.toks[p.pos].nlBefore, p.atTerminator():
			// A newline, or the end of the list.
		default:
			return l, false
		}
		l.items = append(l.items, it)
	}
	return l, true
}

// parseBody is parseList for the inside of a compound command, which may not
// be empty.
func (p *lineParser) parseBody() (list, bool) {
	l, ok := p.parseList()
	return l, ok && len(l.items) > 0
}

func (p *lineParser) parseAndOr() (andOr, bool) {
	var ao andOr
	for {
		pl, ok := p.parsePipeline()
		if !ok {
			return ao, false
		}
		ao.pipes = append(ao.pipes, pl)
		if p.pos >= len(p.toks) || p.toks[p.pos].nlBefore || !p.op(p.pos, "&&") && !p.op(p.pos, "||") {
			return ao, true
		}
		ao.ops = append(ao.ops, p.toks[p.pos].text)
		p.pos++
	}
}

func (p *lineParser) parsePipeline() (pipeline, bool) {
	var pl pipeline
	if p.word(p.pos, "!") {
		pl.neg = true
		p.pos++
	}
	for {
		c, ok := p.parseCommand()
		if !ok {
			return pl, false
		}
		pl.stages = append(pl.stages, c)
		if !p.op(p.pos, "|") || p.toks[p.pos].nlBefore {
			return pl, true
		}
		p.pos++
		if p.op(p.pos, "&") && p.toks[p.pos].glued {
			// |&, which pipes stderr as well: a pipe.
			p.pos++
		}
	}
}

func (p *lineParser) parseCommand() (command, bool) {
	if p.pos >= len(p.toks) {
		return nil, false
	}
	t := p.toks[p.pos]
	switch {
	case t.meta && t.text == "(":
		if p.op(p.pos+1, "(") && p.toks[p.pos+1].glued {
			// ((, arithmetic: not read.
			return nil, false
		}
		p.pos++
		body, ok := p.parseBody()
		if !ok || !p.op(p.pos, ")") {
			return nil, false
		}
		p.pos++
		return &subshell{body}, p.redirections()
	case t.meta:
		return nil, false
	case p.word(p.pos, "{"):
		p.pos++
		body, ok := p.parseBody()
		if !ok || !p.word(p.pos, "}") {
			return nil, false
		}
		p.pos++
		return &group{body}, p.redirections()
	case p.word(p.pos, "if"):
		return p.parseIf()
	case p.word(p.pos, "while"), p.word(p.pos, "until"):
		from := len(p.simples)
		p.pos++
		if _, ok := p.parseBody(); !ok || !p.word(p.pos, "do") {
			return nil, false
		}
		return p.parseLoopBody(from)
	case p.word(p.pos, "for"):
		from := len(p.simples)
		p.pos++
		if p.pos >= len(p.toks) || p.toks[p.pos].meta || !isName(p.toks[p.pos].text) {
			// for ((...)), or no name.
			return nil, false
		}
		p.pos++
		if p.word(p.pos, "in") && !p.toks[p.pos].nlBefore {
			for p.pos++; p.pos < len(p.toks) && !p.toks[p.pos].nlBefore && !p.op(p.pos, ";"); p.pos++ {
				if p.toks[p.pos].meta {
					return nil, false
				}
			}
		}
		if p.op(p.pos, ";") {
			p.pos++
		}
		if !p.word(p.pos, "do") {
			return nil, false
		}
		return p.parseLoopBody(from)
	case p.word(p.pos, "[["):
		end := p.pos + 1
		for ; end < len(p.toks) && !p.word(end, "]]"); end++ {
		}
		if end >= len(p.toks) {
			return nil, false
		}
		s := &simple{start: p.pos, end: end + 1, op: opOther, runner: -1}
		if p.readsStatus(p.pos+1, end) {
			s.op = opStatusRead
		}
		p.simples = append(p.simples, s)
		p.pos = end + 1
		return s, true
	}
	for _, w := range []string{"case", "select", "function", "coproc", "then", "elif", "else", "fi", "do", "done", "esac", "}", "in", "]]"} {
		if p.word(p.pos, w) {
			return nil, false
		}
	}
	return p.parseSimple()
}

func (p *lineParser) parseIf() (command, bool) {
	c := &ifCmd{}
	p.pos++ // if
	for {
		cond, ok := p.parseBody()
		if !ok || !p.word(p.pos, "then") {
			return nil, false
		}
		p.pos++
		body, ok := p.parseBody()
		if !ok {
			return nil, false
		}
		c.conds, c.bodies = append(c.conds, cond), append(c.bodies, body)
		if p.word(p.pos, "elif") {
			p.pos++
			continue
		}
		break
	}
	if p.word(p.pos, "else") {
		p.pos++
		els, ok := p.parseBody()
		if !ok {
			return nil, false
		}
		c.els = &els
	}
	if !p.word(p.pos, "fi") {
		return nil, false
	}
	p.pos++
	return c, p.redirections()
}

// parseLoopBody reads a loop's `do ... done`, from the `do`; from is the
// first simple command of the loop, for whether it is unsure.
func (p *lineParser) parseLoopBody(from int) (command, bool) {
	p.pos++ // do
	if _, ok := p.parseBody(); !ok || !p.word(p.pos, "done") {
		return nil, false
	}
	p.pos++
	l := &loop{}
	for _, s := range p.simples[from:] {
		switch s.op {
		case opRunner, opExit, opStatusRead, opSet, opAssign, opCd:
			l.unsure = true
		}
	}
	return l, p.redirections()
}

// redirections reads the redirections after a compound command.
func (p *lineParser) redirections() bool {
	for p.pos < len(p.toks) && !p.toks[p.pos].nlBefore {
		j := p.pos
		if t := p.toks[j]; !t.meta && isFDPrefix(t) && j+1 < len(p.toks) && p.toks[j+1].meta && p.toks[j+1].glued && isRedirect(p.toks[j+1].text) {
			j++
		}
		if p.op(j, "&") && j+1 < len(p.toks) && p.toks[j+1].meta && p.toks[j+1].glued && strings.HasPrefix(p.toks[j+1].text, ">") {
			j++
		}
		if !p.toks[j].meta || !isRedirect(p.toks[j].text) {
			return true
		}
		end := operatorEnd(p.toks, j)
		if noTarget(p.toks, end) || p.toks[end+1].meta {
			return false
		}
		p.pos = end + 2
	}
	return true
}

// simpleEnd returns the index of the token after the simple command that
// begins at s: a separator, a pipe, a subshell's `)`, the first token of the
// next line, or the end. A group inside a word -- $( ), <( ), backticks --
// is read past to its close. False where the end cannot be told: a group
// that never closes or whose depth the tokens do not show, an empty `()`
// (a function), a ${ the word does not close, or a redirection with no
// target.
func (p *lineParser) simpleEnd(s int) (int, bool) {
	toks := p.toks
	var open []byte
	for j := s; j < len(toks); j++ {
		t := toks[j]
		if j > s && t.nlBefore && len(open) == 0 {
			return j, true
		}
		if openBrace(t) {
			return 0, false
		}
		if len(open) > 0 {
			if unknownDepth(toks, j, false) {
				return 0, false
			}
			open = inGroup(open, t)
			continue
		}
		switch {
		case t.ticks%2 == 1:
			open = append(open, '`')
		case !t.meta:
		case t.text == "(":
			if j+1 < len(toks) && toks[j+1].meta && toks[j+1].text == ")" {
				return 0, false
			}
			open = append(open, '(')
		case t.text == ")":
			// A subshell's close: the command ends before it.
			return j, true
		case t.text == "&" && j+1 < len(toks) && toks[j+1].meta && toks[j+1].glued && strings.HasPrefix(toks[j+1].text, ">"):
			// &> and &>>: a redirection, not the background separator.
			j++
			fallthrough
		case isRedirect(t.text):
			end := operatorEnd(toks, j)
			if noTarget(toks, end) {
				return 0, false
			}
			j = end
		case t.text == ";" || t.text == "&&" || t.text == "||" || t.text == "|" || t.text == "&":
			return j, true
		}
	}
	if len(open) > 0 {
		return 0, false
	}
	return len(toks), true
}

// words returns the indices of the tokens of the command [s, e) that are
// neither a redirection, its target nor an fd written against it.
func (p *lineParser) words(s, e int) []int {
	toks := p.toks
	var out []int
	for j := s; j < e; j++ {
		t := toks[j]
		switch {
		case !t.meta && j+1 < e && toks[j+1].meta && toks[j+1].glued && isRedirect(toks[j+1].text) && isFDPrefix(t):
			continue
		case t.meta && t.text == "&" && j+1 < e && toks[j+1].meta && toks[j+1].glued && strings.HasPrefix(toks[j+1].text, ">"):
			j++
			fallthrough
		case t.meta && isRedirect(toks[j].text):
			j = operatorEnd(toks, j) + 1
			continue
		}
		out = append(out, j)
	}
	return out
}
