package shape

import (
	"path"
	"strings"
)

// The launch rule (card LAUNCH-ANY1, the manager's ruling of 11-03): which
// shell lines may start a gate's run that no runner list names, read over
// every command of the line, not its first program alone. The record keeps a
// line's first program only, so before this `mkdir -p out && ./run-tests.sh`
// read as mkdir, no wrapper, and its run as no run, while `goslot.sh status`
// and `bash -n x.sh` read as wrappers a line then named as the call a gate
// may have run in (R67 at main e4bb97ee: 28 false "not run after the last
// edit" lines, and 4 lines naming a call that ran nothing).

// maxLaunchDepth bounds how deep a shell's -c line is read for a launch: `sh
// -c 'bash -c "./t.sh"'` is two; a line nested deeper launches nothing that
// is read, since no run is guessed.
const maxLaunchDepth = 2

// reportWords are a script's first arguments, and a shell's long options,
// that ask it to report, never to run: `goslot.sh status`, `./t.sh --help`.
var reportWords = map[string]bool{
	"status": true, "list": true, "show": true, "help": true, "-h": true, "--help": true, "version": true, "--version": true,
}

// launches reports a shell line one of whose commands, in line order as
// Commands reads them, is a launch (launch): a script, or a shell running a
// script or a line. Where the reading stops (an expanded program word, a
// reading it cannot vouch for) the commands after the stop are unknown, and
// no launch is guessed in them. Cost: a byte scan of the line (launchWord),
// and for a line that passes it one more tokenize, as Commands does.
func launches(line string) bool {
	return launchWord(line) && launchesIn(line, 0)
}

// launchesIn is launches without the byte scan, at a -c line's depth. It
// reads past a stage of assignments alone, as Commands does not.
func launchesIn(line string, depth int) bool {
	cmds, _ := commands(line, true)
	for _, c := range cmds {
		if launch(c, depth) {
			return true
		}
	}
	return false
}

// launch reports a command that may start a gate's run: a script named by
// its extension (wrapperProgram's rule) whose first argument is no report
// word, or a shell (shellLaunch). Any other program is no launch, a shell
// behind timeout, env or sudo included, as wrapperProgram reads them: no run
// is guessed from a program no rule names.
func launch(c Command, depth int) bool {
	if len(c.Words) == 0 {
		return false
	}
	prog, args := path.Base(c.Words[0]), c.Words[1:]
	switch {
	case shells[prog]:
		return shellLaunch(args, depth)
	case strings.HasSuffix(prog, ".sh") || strings.HasSuffix(prog, ".bash"):
		return len(args) == 0 || !reportWords[args[0]]
	}
	return false
}

// shellLaunch reports a shell, given its arguments, that runs something: a
// script whose first argument is no report word, its standard input (no
// script, no -c: a here-document or a pipe), or a -c line that launches
// (launchesIn) or names a runner (namesRunner). A shell asked to read
// without running (-n, -o noexec) or for its help or version launches
// nothing. Options are read as the shells read them: single-dash letters in
// one word, -o taking the next word, -- ending them.
func shellLaunch(args []string, depth int) bool {
	cline := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			i++
		case strings.HasPrefix(a, "--"):
			if reportWords[a] {
				return false
			}
			continue
		case len(a) > 1 && (a[0] == '-' || a[0] == '+'):
			letters := a[1:]
			if a[0] == '-' && strings.ContainsRune(letters, 'n') {
				return false
			}
			if strings.ContainsRune(letters, 'c') {
				cline = true
			}
			if strings.ContainsRune(letters, 'o') && i+1 < len(args) {
				i++
				if a[0] == '-' && args[i] == "noexec" {
					return false
				}
			}
			continue
		}
		if i >= len(args) {
			break
		}
		if cline {
			return depth < maxLaunchDepth && (launchesIn(args[i], depth+1) || namesRunner(args[i]))
		}
		return i+1 >= len(args) || !reportWords[args[i+1]]
	}
	return !cline
}

// namesRunner reports a line one of whose commands is a test or build
// runner on the runner lists, as statusMasked's parser marks them: `sh -c
// 'go test ./...'` runs one, which the outer line's parser does not look
// into (runsShellLine).
func namesRunner(line string) bool {
	if controlByte(line) {
		return false
	}
	toks, err := tokenizeList(line)
	if err != nil {
		return false
	}
	p := &lineParser{toks: toks, groups: map[string]int{}, assigned: map[string]bool{}}
	if _, ok := p.parseLine(); !ok {
		return false
	}
	return len(p.runners) > 0
}

// launchWord is launches' first step, a byte scan: a line holding no
// script's extension and no shell's name as a word launches nothing, and
// costs no tokenize. A word here is the name with no letter, digit, _, - or
// . on either side, so bash and /bin/sh are words and shasum, push and
// sh.rc are not.
func launchWord(line string) bool {
	if strings.Contains(line, ".sh") || strings.Contains(line, ".bash") {
		return true
	}
	for name := range shells {
		for from := 0; ; {
			i := strings.Index(line[from:], name)
			if i < 0 {
				break
			}
			i += from
			if j := i + len(name); (i == 0 || !wordByte(line[i-1])) && (j == len(line) || !wordByte(line[j])) {
				return true
			}
			from = i + 1
		}
	}
	return false
}

// wordByte is a byte a program name may hold beside a shell's name.
func wordByte(b byte) bool {
	return b == '_' || b == '-' || b == '.' || '0' <= b && b <= '9' || 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z'
}

// RecordedLaunch is how a record reads a line's launch: its launch bit where
// the record carries one (recorded: a declaration of call schema 10 or
// later), else its first program alone (wrapperProgram), the reading every
// record before the bit had, which stands for the line's.
func RecordedLaunch(program *string, launch, recorded bool) bool {
	if recorded {
		return launch
	}
	return wrapperProgram(program)
}
