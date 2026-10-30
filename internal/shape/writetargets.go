package shape

import (
	"encoding/json"
	"path"
	"strings"
)

// WrittenPaths names the files a shell call's line may write, from its words
// alone, and whether the list is complete: every source of a write on the
// line named what it writes. may_write (mayWrite) over-claims on purpose and
// the targets under-claim on purpose, so the two err in opposite safe
// directions: a write this cannot name leaves complete false, and the
// evaluator counts the call as a write it cannot place and fails closed
// (verify's appendWrites). complete with no targets says nothing on the line
// may write.
//
// A tool that is not a shell, or a shell input with no command, is nil and
// complete. A line with a control byte, one the lexer could not read to its
// end, and one whose program cannot be named are nil and incomplete, as
// Derive's may_write is true for each.
//
// A stage ends at a separator or an unquoted newline, and may write by its
// words (stageWrites) and, from the program's stage on, by its program
// (programWrites). Each source names its own targets: a redirection its word
// (outputRedirect); a substitution nothing, since its command is not looked
// into; a program its operands by its rule (ruleOf), where the program is
// the one programToken names and no word after it is read as another. A
// source that names nothing leaves the list incomplete, and so does a word
// that names no one file (name). A relative target past the leading `cd DIR
// &&` run is joined to where the run leads (leadingFoldDir).
//
// The targets are in line order. A line mayWrite says may write that names
// nothing is incomplete even where no stage here may write, since mayWrite
// reads the line as one span (its comment): the record never says "writes
// nothing" of a line may_write says may write.
//
// Cost: one tokenize, as Hosts pays, and one walk over the stages; a line no
// stage of which may write allocates nothing more.
//
// This is one of the four places a substring of a tool input leaves this
// package (see the package comment): each target is a word of the line, or
// one joined to a leading cd's directory.
func WrittenPaths(toolName string, toolInput json.RawMessage) (targets []string, complete bool) {
	if verbForTool(toolName) != VerbExecute {
		return nil, true
	}
	cmd, ok := commandField(toolInput)
	if !ok {
		return nil, true
	}
	if controlByte(cmd) {
		return nil, false
	}
	toks, err := tokenizeProgram(cmd)
	if err != nil {
		return nil, false
	}
	i, ok := programToken(toks, false)
	if ok {
		i, ok = pastDirectoryChange(toks, i, false)
	}
	if !ok {
		return nil, false
	}
	w := targetWalk{toks: toks, prog: i, complete: true}
	for s := 0; s < len(toks); {
		e, next := stageEnd(toks, s)
		w.stage(s, e)
		s = next
	}
	if w.targets == nil && w.complete && mayWrite(toks, i) {
		w.complete = false
	}
	return w.targets, w.complete
}

// stageEnd is the end of the stage that starts at s -- the index of its
// separator, of the first token after an unquoted newline, or len(toks) --
// and where the next stage starts. A redirection's operator is passed whole,
// so the `|` of `>|` ends nothing.
func stageEnd(toks []token, s int) (end, next int) {
	for j := s; j < len(toks); j++ {
		t := toks[j]
		switch {
		case j > s && t.nlBefore:
			return j, j
		case t.meta && isRedirect(t.text):
			j = operatorEnd(toks, j)
		case stageSeparator(toks, j):
			return j, j + 1
		}
	}
	return len(toks), len(toks)
}

// targetWalk is WrittenPaths' state over one line.
type targetWalk struct {
	toks     []token
	prog     int // the program the search names, Derive's
	targets  []string
	complete bool
	// moved: a cd, pushd or popd ran at or after the program's stage, so a
	// relative target after it is relative to a place not on the line.
	moved bool
	// folded, base and based: dir's answer, read at the first relative
	// target.
	folded, based bool
	base          string
}

// stage reads the stage toks[s:e]; an empty one runs nothing.
func (w *targetWalk) stage(s, e int) {
	if s == e {
		return
	}
	toks := w.toks
	ws := writesIn(toks, s, e)
	at := w.prog
	if at < s || at >= e {
		at = -1
		if k, ok := programToken(toks[s:e], false); ok {
			at = s + k
		}
	}
	name := ""
	if at >= 0 {
		name = path.Base(toks[at].text)
	}
	runs := ws.writer || ws.edit || e > w.prog && (at < 0 || programWrites(name))
	if !runs && !ws.redirect && !ws.substitution {
		return
	}
	if ws.substitution {
		w.complete = false
	}
	o := operands{w: w, e: e, name: name, dest: -1}
	if runs && at >= 0 {
		o.start(at, ws.edit)
	}
	for j := s; j < e; j++ {
		t := toks[j]
		switch {
		case t.meta && isRedirect(t.text):
			end := operatorEnd(toks, j)
			writes := outputRedirect(toks, j) || end > j && outputRedirect(toks, j+1)
			if end+1 < e && !toks[end+1].meta {
				if writes {
					w.name(toks[end+1], toks[end+1].text)
				}
				end++
			} else if writes {
				w.complete = false // a process substitution, or no word at all
			}
			j = end
		case t.meta, j <= at, at < 0, fdPrefix(toks, j, e):
		case o.rule != ruleNone:
			o.word(j)
		}
	}
	if runs && !o.named {
		w.complete = false
	}
	if e > w.prog && (name == "cd" || name == "pushd" || name == "popd") {
		w.moved = true
	}
}

// programWrites reports a program whose stage may write by the program alone:
// one programVerb does not class read, since an unknown program is execute
// and may do anything; but not an in-place editor programVerb does not class
// at all, which writes only when its stage arms it, the edit source of
// stageWrites. go and cargo are classed (package) and write either way.
func programWrites(name string) bool {
	if _, editor := inPlaceEditors[name]; editor {
		if _, classed := programVerb[name]; !classed {
			return false
		}
	}
	return verbForProgram(name) != VerbRead
}

// name records text, the word t or the end of it, standing where a written
// path does, and reports whether it named one. A word that names no one file
// -- one holding `$`, a backtick, `*`, `?`, `[` or `{`, one starting with
// `~`, `-`, an empty word, a directory (`.`, `..`, a trailing `/`, Go's
// `...`) -- or a relative one where the line's place is not known (dir)
// leaves the list incomplete; a device is no file (devFile).
func (w *targetWalk) name(t token, text string) bool {
	base := path.Base(text)
	switch {
	case devFile(text):
		return false
	case t.opaque || t.ticks > 0 || text == "" || text == "-" || text[0] == '~' ||
		strings.ContainsAny(text, "$`*?[{") ||
		strings.HasSuffix(text, "/") || strings.HasSuffix(text, "...") || base == "." || base == "..":
		w.complete = false
		return false
	}
	if !path.IsAbs(text) {
		dir, ok := w.dir()
		if !ok {
			w.complete = false
			return false
		}
		if dir != "" {
			text = path.Join(dir, text)
		}
	}
	w.targets = append(w.targets, text)
	return true
}

// dir is the directory a relative target is joined to: "" on a line with no
// leading cd steps, the steps' fold where LeadingDirectory reads them, and
// false where where the line stands is not on it.
func (w *targetWalk) dir() (string, bool) {
	if w.moved {
		return "", false
	}
	if !w.folded {
		w.folded = true
		w.base, w.based = leadingFoldDir(w.toks)
	}
	return w.base, w.based
}

// leadingFoldDir folds the leading cd steps of toks into one directory, the
// hook's fold of the same steps onto the payload's cwd (DeriveWithDirs's
// dirs) but for the cwd itself: an absolute step restarts it, and a relative
// one is joined on. Steps leadingRun does not read, more than MaxLeadingCd
// of them (the hook records an unknown directory there), and a step under
// `~` are false.
func leadingFoldDir(toks []token) (string, bool) {
	first, _ := programToken(toks, false)
	if _, steps, _, _ := directoryChanges(toks, first, false); steps == 0 {
		return "", true
	}
	dirs, _ := leadingRun(toks, nil)
	if len(dirs) == 0 || len(dirs) > MaxLeadingCd {
		return "", false
	}
	dir := ""
	for _, d := range dirs {
		switch {
		case d[0] == '~':
			return "", false
		case path.IsAbs(d):
			dir = d
		default:
			dir = path.Join(dir, d)
		}
	}
	return dir, true
}

// fdPrefix reports a word at j that is an fd number or variable written
// against the redirection after it (the 2 of 2>, the {fd} of {fd}>), and is
// no argument.
func fdPrefix(toks []token, j, e int) bool {
	return j+1 < e && toks[j+1].meta && toks[j+1].glued && isRedirect(toks[j+1].text) && isFDPrefix(toks[j])
}

// nextArg is the first argument word of a stage at or after j and before e:
// not a redirection's operator or its word, and not an fd prefix; e when
// there is none.
func nextArg(toks []token, j, e int) int {
	for ; j < e; j++ {
		t := toks[j]
		switch {
		case t.meta && isRedirect(t.text):
			j = operatorEnd(toks, j)
			if j+1 < e && !toks[j+1].meta {
				j++
			}
		case t.meta, fdPrefix(toks, j, e):
		default:
			return j
		}
	}
	return e
}

// The operand rules of the programs whose targets WrittenPaths names.
const (
	ruleNone  = iota // names nothing
	ruleEvery        // every operand
	ruleLast         // the destination: -t's word, or the last operand
	ruleSed          // the operands after the script
	rulePerl         // the operands after the program file or -e
)

// ruleOf is the operand rule of the program name, edit saying whether its
// stage arms an in-place editor. go fmt and cargo fmt take packages, never
// files, and an editor its stage does not arm writes nothing by its operands.
func ruleOf(name string, edit bool) int {
	switch name {
	case "rm", "mkdir", "touch", "tee", "truncate":
		return ruleEvery
	case "cp", "mv", "install":
		return ruleLast
	}
	if !edit {
		return ruleNone
	}
	switch name {
	case "sed":
		return ruleSed
	case "perl":
		return rulePerl
	case "go", "cargo":
		return ruleNone
	}
	if _, ok := inPlaceEditors[name]; ok {
		return ruleEvery
	}
	return ruleNone
}

// operands reads one stage's argument words by its program's rule.
type operands struct {
	w     *targetWalk
	e     int
	name  string
	rule  int
	ended bool // past `--`, or past perl's first operand
	value bool // the next word is an option's value
	first bool // the first operand is sed's script or perl's program file
	dest  int  // ruleLast: the destination's token, -1 for none
	off   int  // ruleLast: where in that token's text the path starts
	named bool
}

// start sets the rule for the program at at, and what the rule reads ahead.
func (o *operands) start(at int, edit bool) {
	o.rule = ruleOf(o.name, edit)
	switch o.rule {
	case ruleLast:
		o.dest, o.off = destination(o.w.toks, at, o.e)
	case ruleSed:
		o.first = !sedScript(o.w.toks, at, o.e)
	case rulePerl:
		o.first = true
	}
}

// word reads the argument word at j.
func (o *operands) word(j int) {
	t := o.w.toks[j]
	text := t.text
	switch {
	case o.rule == ruleLast:
		if j == o.dest {
			o.target(t, text[o.off:])
		}
		return
	case o.value:
		o.value = false
		return
	case !o.ended && text == "--":
		o.ended = true
		return
	case !o.ended && len(text) > 1 && text[0] == '-':
		o.option(j, text)
		return
	}
	if o.rule == rulePerl {
		o.ended = true // perl reads no option after its first operand
	}
	if o.first {
		o.first = false
		return
	}
	if o.rule == ruleEvery && onList(text, inPlaceEditors[o.name]) {
		return // an arming word that is no option: ruff's format
	}
	o.target(t, text)
}

// option reads the option word text at j for the value it takes.
func (o *operands) option(j int, text string) {
	switch o.rule {
	case ruleEvery:
		o.value = o.name == "truncate" && text == "-s"
	case ruleSed:
		_, o.value = sedOption(text)
		if text == "-i" || text == "-I" {
			// BSD sed's in-place suffix is the next word, and macOS's sed
			// is BSD's: `sed -i '' s/x/y/ a.go`.
			if k := nextArg(o.w.toks, j+1, o.e); k < o.e && o.w.toks[k].text == "" {
				o.value = true
			}
		}
	case rulePerl:
		script, value := perlOption(text)
		if script {
			o.first = false
		}
		o.value = value
	}
}

// target names text, the word t or the end of it, as one of the stage's
// targets.
func (o *operands) target(t token, text string) {
	if o.w.name(t, text) {
		o.named = true
	}
}

// destination is the token that names cp's, mv's or install's destination in
// the stage toks[at+1:e], and where in its text the path starts: the word
// after -t or --target-directory, the rest of -tDIR or
// --target-directory=DIR, else the last operand; -1 where there is none.
func destination(toks []token, at, e int) (int, int) {
	dest, off, last := -1, 0, -1
	ended := false
	for j := nextArg(toks, at+1, e); j < e; j = nextArg(toks, j+1, e) {
		text := toks[j].text
		switch {
		case ended || text == "-" || !strings.HasPrefix(text, "-"):
			last = j
		case text == "--":
			ended = true
		case text == "-t" || text == "--target-directory":
			if k := nextArg(toks, j+1, e); k < e {
				dest, off, j = k, 0, k
			}
		case strings.HasPrefix(text, "--target-directory="):
			dest, off = j, len("--target-directory=")
		case strings.HasPrefix(text, "-t"):
			dest, off = j, 2
		}
	}
	if dest >= 0 {
		return dest, off
	}
	return last, 0
}

// sedScript reports a sed stage, toks[at+1:e], whose script an option gives
// (sedOption), so that its first operand is a file. GNU sed reads options
// after operands, so the whole stage is read, to its `--`.
func sedScript(toks []token, at, e int) bool {
	for j := nextArg(toks, at+1, e); j < e; j = nextArg(toks, j+1, e) {
		text := toks[j].text
		if text == "--" {
			return false
		}
		if script, _ := sedOption(text); script && len(text) > 1 && text[0] == '-' {
			return true
		}
	}
	return false
}

// sedOption reads a sed option word: whether it gives the script -- -e or
// -f, alone or ending a cluster, or --expression and --file -- and whether
// the script is the next word. The walk stops at i or I, whose suffix is the
// rest of the word, so `-ie` is -i with the suffix e.
func sedOption(text string) (script, value bool) {
	switch {
	case text == "--expression", text == "--file":
		return true, true
	case strings.HasPrefix(text, "--expression="), strings.HasPrefix(text, "--file="):
		return true, false
	case strings.HasPrefix(text, "--"):
		return false, false
	}
	for k := 1; k < len(text); k++ {
		switch text[k] {
		case 'i', 'I':
			return false, false
		case 'e', 'f':
			return true, k == len(text)-1
		}
	}
	return false, false
}

// perlOption reads a perl option word (perlrun): whether it gives the script
// (-e or -E), and whether the script is the next word. The walk stops at a
// letter that takes the rest of the word as its value -- i's suffix, M, m,
// I, F, x, and d, D and C -- as clusterHas stops.
func perlOption(text string) (script, value bool) {
	for k := 1; k < len(text); k++ {
		switch c := text[k]; {
		case c == 'e' || c == 'E':
			return true, k == len(text)-1
		case strings.IndexByte("iMmIFxdDC", c) >= 0:
			return false, false
		}
	}
	return false, false
}
