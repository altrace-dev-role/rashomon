package shape

import (
	"path"
	"strconv"
	"strings"
)

// Masked kinds: what shape.status_masked says of a shell line. A closed set,
// like the verb classes, and decided against fixed lists whose words are
// compared and dropped.
const (
	// MaskedNone: the line was read to its end, and every recognised build
	// or test runner on it has its failure reach the line's exit status --
	// or there is none.
	MaskedNone = "none"
	// MaskedTest: a recognised test runner (testCommands) runs on the line,
	// and the line exits 0 when it fails, or with the same status whether it
	// fails or passes.
	MaskedTest = "test"
	// MaskedBuild: the same for a recognised build runner (buildCommands),
	// and no test runner's status is hidden.
	MaskedBuild = "build"
)

// RulesVersion names the lists this build decides a shape with: the test
// runners (testCommands) and the build runners (buildCommands), which set
// verb_class and status_masked, and the pass words (the report's
// passVocabulary), which decide when the masked-runs line fires. Written on
// every declaration and execution as rules_version, so records written before
// and after a list changed can be told apart.
//
// Bump it whenever one of those lists changes, and add the new lists' pin to
// the tests that hold one (rules_version_test.go here and in report): each
// fails when its list changes without a bump.
const RulesVersion = 1

// MaskedKinds is the vocabulary of status_masked, for the schema enum test.
func MaskedKinds() []string {
	return []string{MaskedNone, MaskedTest, MaskedBuild}
}

// buildCommands are the build and check runners status_masked recognises
// besides testCommands, in the same form: the program, as path.Base names it,
// then the plain words that must follow it. make is any target -- `make
// release`, `make dist`, `make check` and `make validate` are a project's own
// gates, and which of them builds and which checks is the Makefile's to say --
// and so are npm, yarn and pnpm's `run`, mvn and gradle. A runner not listed
// is not recognised, and its hidden status is not reported: the under-claim.
// The arguments that make a runner do something other than run (notARun:
// `make -n`, `go build -n`, --help, --version) refuse it as they refuse a
// test run.
var buildCommands = [][]string{
	{"make"}, {"gmake"},
	{"go", "build"}, {"go", "vet"}, {"cargo", "build"}, {"cargo", "check"}, {"cargo", "clippy"},
	{"npm", "run"}, {"yarn", "run"}, {"yarn", "build"}, {"pnpm", "run"}, {"pnpm", "build"},
	{"tsc"}, {"npx", "tsc"}, {"mvn"}, {"mvnw"}, {"gradle"}, {"gradlew"},
	{"dotnet", "build"}, {"cmake", "--build"}, {"ninja"}, {"bazel", "build"}, {"bazel", "test"},
}

// statusMasked reports, for a shell line, whether a recognised build or test
// runner on it has an exit status the line does not return, and the keyed
// digest of that runner's own words -- or, when none does, of a runner whose
// status the line does return (runnerDigest).
//
// The question is the one a failing runner asks of the line: when it fails,
// does the line? The line is parsed (lists, && and ||, pipelines, `!`, ( ),
// { }, if, while, until and for) and then run twice in the abstract, once
// with the runner failing and once passing, every other command succeeding
// and every other recognised runner passing. The same runner written twice is
// one runner, failing or passing at both places. Followed: `set -e` and `set
// -o pipefail` and their `+` forms, which errexit ignores (a command before
// && or ||, a condition, a negated pipeline), `exit` with no argument, a
// number, `$?`, `${PIPESTATUS[N]}` or a variable assigned one of those on the
// line, and a pipeline's stages as subshells, as bash runs them.
//
//   - The line exits 0 when the runner fails, or with the same status either
//     way: the runner's status is hidden, and the kind is reported --
//     `make test 2>&1 | tail -40`, `go test ./...; echo $?`, `npm test ||
//     true`, `if make test; then echo ok; fi`, `! make test`, and `make test
//     &`, whose status is no one's.
//   - The line exits non-zero when the runner fails and 0 when it passes:
//     MaskedNone -- `make test && echo ok`, `make test || exit 1`, `set -o
//     pipefail; make test | tail`, `make test | tail; exit
//     ${PIPESTATUS[0]}`.
//
// Where it is not sure the runner's status reaches the line's -- a word it
// cannot read, a status the abstract run cannot tell (a test of $?, `wait`
// with an argument), a loop that holds a runner, an exit or a status read, a
// runner the abstract run never reaches, `case`, a function, `eval`, `trap`,
// `exec`, or a shell's `-c`, whose line is not looked into -- it says
// nothing: null, never a kind and never "none". So does a line that cannot be
// read to its end (tokenizeList, parseLine).
func statusMasked(cmd string, key []byte) (kind, runner *string) {
	if controlByte(cmd) {
		return nil, nil
	}
	toks, err := tokenizeList(cmd)
	if err != nil {
		return nil, nil
	}
	p := &lineParser{toks: toks, groups: map[string]int{}, assigned: map[string]bool{}}
	body, ok := p.parseLine()
	if !ok {
		return nil, nil
	}
	p.displace(leadingSteps(cmd))

	var (
		hidden []int
		reach  = -1
	)
	for g := range p.runners {
		f, ran := p.run(body, g, true)
		pass, _ := p.run(body, g, false)
		if f < 0 || pass < 0 || !ran {
			// Not sure: null.
			return nil, nil
		}
		if f == 0 || f == pass {
			hidden = append(hidden, g)
			continue
		}
		if reach < 0 || p.runners[g].last > p.runners[reach].last {
			reach = g
		}
	}
	none := MaskedNone
	switch {
	case len(hidden) > 0:
		k := MaskedBuild
		for _, g := range hidden {
			if p.runners[g].kind == MaskedTest {
				k = MaskedTest
			}
		}
		if len(hidden) > 1 {
			// No one later run follows up two runners.
			return &k, nil
		}
		return &k, p.runnerDigest(hidden[0], key)
	case reach >= 0:
		return &none, p.runnerDigest(reach, key)
	}
	return &none, nil
}

// The parsed line. A list is and-or lists separated by `;`, `&` or a
// newline; an and-or list is pipelines joined by && and ||; a pipeline is
// commands joined by | or |&, maybe negated by `!`.
type (
	list     struct{ items []listItem }
	listItem struct {
		ao andOr
		bg bool // ended by &: run in the background
	}
	andOr struct {
		pipes []pipeline
		ops   []string // ops[k] joins pipes[k] and pipes[k+1]
	}
	pipeline struct {
		neg    bool
		stages []command
	}
	command  interface{}
	subshell struct{ body list }
	group    struct{ body list }
	ifCmd    struct {
		conds, bodies []list
		els           *list
	}
	// loop is a while, until or for loop, which is not run: unsure when it
	// holds a runner, an exit, a status read or a set, and otherwise a
	// command that succeeds.
	loop struct{ unsure bool }
)

// simpleOp is what a simple command does to the abstract run.
type simpleOp int

const (
	opOther      simpleOp = iota // succeeds
	opTrue                       // true, :
	opFalse                      // false
	opExit                       // exit
	opSet                        // set, which may change errexit and pipefail
	opAssign                     // assignments only, or export, local and the like
	opStatusRead                 // a test of a status: its own status is not known
	opRunner                     // a recognised build or test runner
	opCd                         // cd, pushd, popd
)

type simple struct {
	start, end int
	op         simpleOp
	runner     int // the runner group, for opRunner
	args       []value
	opts       []setOpt
	assigns    []assignment
}

type setOpt struct {
	errexit bool // else pipefail
	on      bool
}

type assignment struct {
	name string
	val  value
}

// value is a word whose number the abstract run can tell: a literal, $?, an
// element of PIPESTATUS or a variable. Anything else is unknown.
type value struct {
	kind valueKind
	n    int
	name string
}

type valueKind int

const (
	valUnknown valueKind = iota
	valLiteral
	valStatus
	valPipe
	valVar
)

// runnerGroup is one recognised runner of the line: its words, wherever
// they appear, and its kind.
type runnerGroup struct {
	key       string // the words runnerDigest digests
	kind      string
	starts    []int // the first token of each command that runs it
	last      int
	displaced bool // a cd the start directory does not hold ran before it
}

type lineParser struct {
	toks     []token
	pos      int
	simples  []*simple
	runners  []runnerGroup
	groups   map[string]int
	assigned map[string]bool // names an assignment on the line set
}

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
	// A shell's -c anywhere on the command: `bash -c`, `sudo sh -lc`.
	for n, j := range ws {
		if t := toks[j]; j < i || t.meta || t.opaque || !shells[path.Base(t.text)] {
			continue
		}
		for _, a := range ws[n+1:] {
			if w := toks[a].text; len(w) > 1 && w[0] == '-' && w[1] != '-' && strings.ContainsRune(w, 'c') {
				return nil, false
			}
		}
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

	ri, name, ok := runnerPrefix(toks[:e], i, prog)
	if !ok {
		return c, true
	}
	var kind string
	switch {
	case runnerOn(toks, ri, name, e, testCommands):
		kind = MaskedTest
	case runnerOn(toks, ri, name, e, buildCommands):
		kind = MaskedBuild
	default:
		return c, true
	}
	key := p.runnerWords(ri, e)
	g, seen := p.groups[key]
	if !seen {
		g = len(p.runners)
		p.groups[key] = g
		p.runners = append(p.runners, runnerGroup{key: key, kind: kind})
	}
	p.runners[g].starts = append(p.runners[g].starts, s)
	p.runners[g].last = s
	c.op, c.runner = opRunner, g
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

// valueOf reads a word as a value: digits (or a sign and digits), $?,
// ${PIPESTATUS[N]}, $PIPESTATUS, or a variable, $NAME or ${NAME}.
func (p *lineParser) valueOf(t token) value {
	w := t.text
	if t.meta {
		return value{}
	}
	switch {
	case w == "$?":
		return value{kind: valStatus}
	case w == "$PIPESTATUS" || w == "${PIPESTATUS}":
		return value{kind: valPipe}
	case strings.HasPrefix(w, "${PIPESTATUS[") && strings.HasSuffix(w, "]}"):
		n, err := strconv.Atoi(w[len("${PIPESTATUS[") : len(w)-2])
		if err != nil || n < 0 {
			return value{}
		}
		return value{kind: valPipe, n: n}
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
// cannot vouch for.
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
		for _, c := range w[1:] {
			switch c {
			case 'e':
				out = append(out, setOpt{errexit: true, on: on})
			case 'o':
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
	d := digest(key, "\x00runner", []byte(p.runners[g].key))
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
// PIPESTATUS, the variables an assignment set, and the two options.
type evalState struct {
	status            int
	pipe              []int
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
	ran    bool
}

// run runs the line with runner group g failing (fail) or passing, every
// other recognised runner passing, and returns its exit status, -1 when it
// cannot tell, and whether the runner ran.
func (p *lineParser) run(l list, g int, fail bool) (int, bool) {
	ev := &evaluator{p: p, target: g, fail: fail}
	st := &evalState{vars: map[string]int{}, pipe: []int{0}}
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
			st.status, st.pipe = 0, []int{0}
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
		if st.errexit && !ignored && st.status != 0 {
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
		st.pipe = []int{st.status}
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
		st.pipe = statuses
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
	case valPipe:
		if v.n < len(st.pipe) {
			return st.pipe[v.n]
		}
	case valVar:
		if n, ok := st.vars[v.name]; ok {
			return n
		}
	}
	return -1
}
