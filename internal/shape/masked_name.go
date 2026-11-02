package shape

import (
	"slices"
	"strings"
	"sync"
)

// The runner's name and the stage that hid its status (D229). Both are words
// the binary ships and never a word of the line: RunnerName is a row of
// testCommands or buildCommands, read back by the row's index; MaskedBy is
// one of stageNames, chosen by comparing the stage's program and flags with
// fixed words, which are then dropped. A line whose runner no row names, and a
// stage no list names, give "other" or nothing -- never the line's own text.

// testCommandNames and buildCommandNames are the rows of the runner lists
// written out as one string each, in the lists' order, built once from them so
// that a row added to a list is named with it (TestMO1_RowNamesAreTheRows). A
// runnerGroup holds a pointer to its entry: two lines that run the same row
// carry the same string. They are built on first use, not at package init:
// every hook call would allocate them there, and RES-2 bounds init at 64 KiB.
var (
	testCommandNames  = sync.OnceValue(func() []string { return rowNames(testCommands) })
	buildCommandNames = sync.OnceValue(func() []string { return rowNames(buildCommands) })
)

func rowNames(rows [][]string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = strings.Join(r, " ")
	}
	return out
}

// The stages of a pipe, by index into stageNames. stageOther is zero, so a
// command no stage read sets is "other".
const (
	stageOther = iota
	stageTail
	stageTee
	stageCat
	stageHead
	stageGrep
	stageWC
	stageSort
	stageCut
)

var stageNames = [...]string{"other", "tail", "tee", "cat", "head", "grep", "wc", "sort", "cut"}

// OutputOutcomeRulesVersion is the first RulesVersion at which masked_by is
// decided, so that a masked run's outcome may be read from its output's summary
// (D229). A record written under an earlier version holds no such word, and a
// reader that takes an outcome from the output asks this of it first.
const OutputOutcomeRulesVersion = 11

// RunnerNames is the vocabulary of runner_name: the words of every row of the
// two runner lists, for the store check that holds a record's runner_name to
// the binary's own words.
func RunnerNames() []string {
	return append(append([]string(nil), testCommandNames()...), buildCommandNames()...)
}

// The ids below are for a reader that holds a run in memory for every call of
// a turn (verify's gateRun): a word it would hold as a string costs sixteen
// bytes in every run, masked or not, where its place in the list is one byte
// beside the flags the run already has (D209, D173d). Id 0 is no word.
var runnerIDs struct {
	once sync.Once
	m    map[string]uint8
}

// RunnerNameID is the place of a runner_name among RunnerNames, counted from
// 1: 0 for a word that is none, such as an empty one or one a record was
// tampered to hold. RunnerNameOf reads it back.
func RunnerNameID(name string) uint8 {
	runnerIDs.once.Do(func() {
		names := RunnerNames()
		runnerIDs.m = make(map[string]uint8, len(names))
		for i := len(names) - 1; i >= 0 && i < 255; i-- {
			runnerIDs.m[names[i]] = uint8(i + 1)
		}
	})
	return runnerIDs.m[name]
}

// RunnerNameOf is the runner_name an id from RunnerNameID stands for, "" for
// 0 or an id past the lists. The string is the binary's own.
func RunnerNameOf(id uint8) string {
	switch i := int(id) - 1; {
	case i < 0:
		return ""
	case i < len(testCommandNames()):
		return testCommandNames()[i]
	case i-len(testCommandNames()) < len(buildCommandNames()):
		return buildCommandNames()[i-len(testCommandNames())]
	}
	return ""
}

// RunnerKey is the key a gate matched at its runner (match:default) and a run
// meet at, the one function both sides call (card GATE-BIND1, R47-R49): the
// runner's row name where the row names the work, so no argument enters it
// (`go test -count=1 ./...` is "go test"), else digest, the keyed digest of
// the runner's exact words, for a launcher row (launcherRows) and for a run
// recorded before call schema 8, which names no runner. name is RunnerName,
// "" for none; digest is RunnerDigest; ok is false where there is no runner.
// The two forms never collide: a digest is hex, a name key starts with a NUL.
// RunnerDigest itself keeps the words: it is the learned families' key, and
// `go test ./a` red then `go test ./b` green must stay two families (D-e).
func RunnerKey(name string, digest *string) (key string, ok bool) {
	switch {
	case digest == nil:
		return "", false
	case name != "" && !slices.Contains(launcherRows, name):
		return "\x00" + name, true
	}
	return *digest, true
}

// MaskedByID is the place of a masked_by word among MaskedByWords, counted
// from 1: 0 for a word that is none. MaskedByOf reads it back.
func MaskedByID(word string) uint8 {
	for i, w := range stageNames {
		if w == word {
			return uint8(i + 1)
		}
	}
	return 0
}

// MaskedByOf is the masked_by word an id from MaskedByID stands for, "" for 0
// or an id past the words. The string is the binary's own.
func MaskedByOf(id uint8) string {
	if i := int(id) - 1; i >= 0 && i < len(stageNames) {
		return stageNames[i]
	}
	return ""
}

// MaskedByWords is the vocabulary of masked_by, for the schema enum test.
func MaskedByWords() []string {
	return append([]string(nil), stageNames[:]...)
}

// MaskedByKeeps reports whether the stage a masked_by word names keeps the
// runner's summary line in what the hook reads: `tail` without -c or --bytes,
// `tee` and a bare `cat` pass the whole of the output on, or the end of it,
// where a summary is. Every other word, and a word that is not one, drops it.
// The reader that takes an outcome from the output asks this and nothing else
// of the word.
func MaskedByKeeps(word string) bool {
	return word == stageNames[stageTail] || word == stageNames[stageTee] || word == stageNames[stageCat]
}

// keepsSummary reports a stage index that passes the summary on.
func keepsSummary(stage int) bool {
	return stage == stageTail || stage == stageTee || stage == stageCat
}

// stageOf says which stage the command is, whose program is the token at i,
// named prog, and whose argument words are the tokens args. The words are
// compared with fixed lists and dropped.
//
//   - tail keeps the end of the output, where a summary is, unless it counts
//     bytes (-c, --bytes, or a short cluster holding c: it may cut a line in
//     two) or reads a file named on its line, which is not the pipe; its -n
//     count is passed over.
//   - tee writes its input to its output whole, whatever files it names.
//   - cat keeps the output only with no operand: a flag may number or squeeze
//     it, and a file named is read in its place. A lone - is its input.
//   - head, grep, wc, sort and cut drop it, each by its own name.
//   - a program no list names, a quoted program, and a word the shell expands
//     are "other", which drops: an under-claim, never a read of the line.
func stageOf(toks []token, i int, prog string, args []int) int {
	if toks[i].quotedAt >= 0 || toks[i].opaque {
		return stageOther
	}
	switch prog {
	case "tail":
		return tailStage(toks, args)
	case "tee":
		return stageTee
	case "cat":
		for _, j := range args {
			if !literalWord(toks, j) || toks[j].text != "-" {
				return stageOther
			}
		}
		return stageCat
	case "head":
		return stageHead
	case "grep":
		return stageGrep
	case "wc":
		return stageWC
	case "sort":
		return stageSort
	case "cut":
		return stageCut
	}
	return stageOther
}

// tailStage is stageTail for a tail that reads the pipe's lines and stageOther
// for any other. Its arguments are read as tail reads them: `-n N`, `-nN`,
// `-N`, `+N`, `-q` and the like are counts and flags; `-c`, `--bytes` and
// their clusters count bytes; a word that is not a flag and not the count
// after a bare -n is a file, which tail reads in place of its input.
func tailStage(toks []token, args []int) int {
	countNext := false
	for _, j := range args {
		if countNext {
			// The count: a number, or a word the shell expands to one; it is
			// not a file either way.
			countNext = false
			continue
		}
		if !literalWord(toks, j) {
			return stageOther
		}
		w := toks[j].text
		switch {
		case w == "--lines" || w == "-n":
			countNext = true
		case strings.HasPrefix(w, "--"):
			// --lines=N and --quiet are not byte counts; --bytes and a long
			// option this does not list are not read.
			if !strings.HasPrefix(w, "--lines=") && w != "--quiet" && w != "--silent" {
				return stageOther
			}
		case len(w) > 1 && w[0] == '-':
			// A short cluster: any c counts bytes. A cluster that ends in n
			// takes the next word as its count (-qn 5).
			if strings.ContainsRune(w, 'c') {
				return stageOther
			}
			countNext = strings.HasSuffix(w, "n") && !allDigitsAfterDash(w)
		case len(w) > 1 && w[0] == '+' && allDigits(w[1:]):
			// The old `tail +N`, from line N.
		default:
			return stageOther
		}
	}
	if countNext {
		return stageOther
	}
	return stageTail
}

// allDigitsAfterDash reports w as - and then digits only, `-20`.
func allDigitsAfterDash(w string) bool {
	return len(w) > 1 && w[0] == '-' && allDigits(w[1:])
}

// maskedBy is the word of the stage that hid runner group g's status, which
// is one of stageNames, or nil where g is hidden by no pipe. It is read from
// the stages after the runner's in every pipeline of the line that holds the
// runner before its last stage: the first stage that drops the summary, else
// the first that keeps it. The pointer is to the binary's own word.
func (p *lineParser) maskedBy(l list, g int) *string {
	var drop, keep *string
	p.stagesAfter(l, g, &drop, &keep)
	if drop != nil {
		return drop
	}
	return keep
}

func (p *lineParser) stagesAfter(l list, g int, drop, keep **string) {
	for _, it := range l.items {
		for _, pl := range it.ao.pipes {
			for n, c := range pl.stages {
				if n < len(pl.stages)-1 && p.holds(c, g) {
					for _, after := range pl.stages[n+1:] {
						w := &stageNames[p.stageWord(after)]
						switch {
						case !keepsSummary(p.stageWord(after)):
							if *drop == nil {
								*drop = w
							}
						case *keep == nil:
							*keep = w
						}
					}
				}
				p.stagesIn(c, g, drop, keep)
			}
		}
	}
}

// stagesIn looks inside a compound command for pipelines of its own.
func (p *lineParser) stagesIn(c command, g int, drop, keep **string) {
	switch c := c.(type) {
	case *subshell:
		p.stagesAfter(c.body, g, drop, keep)
	case *group:
		p.stagesAfter(c.body, g, drop, keep)
	case *ifCmd:
		for k := range c.conds {
			p.stagesAfter(c.conds[k], g, drop, keep)
			p.stagesAfter(c.bodies[k], g, drop, keep)
		}
		if c.els != nil {
			p.stagesAfter(*c.els, g, drop, keep)
		}
	}
}

// stageWord is the stage a pipeline's command is: its own for a simple
// command, "other" for any compound one.
func (p *lineParser) stageWord(c command) int {
	if s, ok := c.(*simple); ok {
		return s.stage
	}
	return stageOther
}

// holds reports whether command c runs runner group g anywhere inside it.
func (p *lineParser) holds(c command, g int) bool {
	switch c := c.(type) {
	case *simple:
		return c.op == opRunner && c.runner == g
	case *subshell:
		return p.listHolds(c.body, g)
	case *group:
		return p.listHolds(c.body, g)
	case *ifCmd:
		for k := range c.conds {
			if p.listHolds(c.conds[k], g) || p.listHolds(c.bodies[k], g) {
				return true
			}
		}
		return c.els != nil && p.listHolds(*c.els, g)
	}
	return false
}

func (p *lineParser) listHolds(l list, g int) bool {
	for _, it := range l.items {
		for _, pl := range it.ao.pipes {
			for _, c := range pl.stages {
				if p.holds(c, g) {
					return true
				}
			}
		}
	}
	return false
}
