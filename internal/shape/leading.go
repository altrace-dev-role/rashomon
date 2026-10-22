package shape

import (
	"encoding/json"
	"strings"
)

// LeadingDirectory reports the directories a shell call's line moves through
// before anything else runs, when the line begins with plain literal
// `cd DIR &&` steps: each DIR as written, in order, which the caller folds
// onto the call's cwd. The command after each `&&` runs only if its cd
// succeeded, so the runner starts where the last one leads.
//
// Only a word the shell takes as it stands: nothing it expands or unquotes
// (`$`, backticks, a glob, `~`, quotes or an escape), not `-`, which is the
// previous directory, and not a comment. `cd DIR;` is left out, since what
// follows a `;` runs whether the cd succeeded or not. A step in the leading
// run whose command, past any assignments, redirections or an opening ( or {
// in front of it, is cd, pushd or popd and is not such a cd spoils the run,
// since the directory it leads to is not on the line. So a `cd -` is refused
// anywhere in the leading run, even when an earlier step names its target:
// `cd /a && cd b && cd - && go test` loses a pair that way, but it never
// forms a false one. Anything else reports false, and the call keeps the
// payload's cwd.
func LeadingDirectory(toolName string, toolInput json.RawMessage) ([]string, bool) {
	if verbForTool(toolName) != VerbExecute {
		return nil, false
	}
	cmd, ok := commandField(toolInput)
	if !ok || controlByte(cmd) {
		return nil, false
	}
	return leadingDirectories(cmd)
}

// MaxLeadingCd is how many leading `cd DIR &&` steps a call's start
// directory is folded from (RES-10). The hook folds no more and records the
// directory as unknown past them: each step joins and cleans the whole path
// so far, so the fold is quadratic in the steps, and a 480 KB payload of
// them took 10.9 s against the hook's 5 s timeout. 64 is far past any line
// an agent writes -- one or two steps is the norm -- and folds in well under
// a millisecond.
const MaxLeadingCd = 64

// The longest leading run leadingDirectories reads, MaxLeadingCd+1 steps of
// three tokens, lies inside the token bound: this fails to compile otherwise,
// since a run the bound cut short is not where the line ran.
const _ = uint(maxTokens - 3*(MaxLeadingCd+1))

// leadingSteps is how many leading `cd DIR &&` steps LeadingDirectory folds
// onto a call's cwd for a line whose leading directories are dirs: the cds
// on it that the call's cwd digest already holds. None past MaxLeadingCd,
// where the hook records the directory as unknown and the digest holds none
// of them.
func leadingSteps(dirs []string) int {
	if len(dirs) > MaxLeadingCd {
		return 0
	}
	return len(dirs)
}

// leadingDirectories is LeadingDirectory over the line cmd.
//
// A line cut at the token bound (errTokenBound) is read to the bound, since
// its leading steps come first: the most this reads is MaxLeadingCd+1 steps
// of three tokens, far inside the bound (see the constant below), so the cut
// never falls in the run. Giving up there instead kept the payload's cwd for
// `cd sub && python3 - <<EOF` and a long script, a directory the line moved
// away from. Any other error is an uncertain reading, which may begin inside
// the run, and reports false.
func leadingDirectories(cmd string) ([]string, bool) {
	return leadingRun(tokenizeProgram(cmd))
}

// leadingRun is leadingDirectories over the program search's reading of the
// line, toks and err, which Derive has already made.
func leadingRun(toks []token, err error) ([]string, bool) {
	if err != nil && err != errTokenBound {
		return nil, false
	}
	var dirs []string
	for i := 0; i < len(toks); i += 3 {
		// The step's command as pastDirectoryChange finds it (programToken),
		// past any assignments, redirections or an opening ( or {: `X=1 cd
		// b`, `>/dev/null cd b` and `(cd b` change directory too, and
		// leadingCd refuses them.
		k, ok := programToken(toks[i:], false)
		if !ok {
			return dirs, len(dirs) > 0
		}
		switch toks[i+k].text {
		case "cd", "pushd", "popd":
		default:
			return dirs, len(dirs) > 0
		}
		dir, ok := leadingCd(toks[i:])
		if !ok {
			return nil, false
		}
		dirs = append(dirs, dir)
		if len(dirs) > MaxLeadingCd {
			// One past the bound says the line is past it; the
			// rest would be read for nothing.
			break
		}
	}
	return dirs, len(dirs) > 0
}

// leadingCd reports the DIR of a plain literal `cd DIR &&` at the start of
// toks, by LeadingDirectory's rules.
func leadingCd(toks []token) (string, bool) {
	if len(toks) < 3 {
		return "", false
	}
	cd, dir, and := toks[0], toks[1], toks[2]
	if cd.text != "cd" || cd.quotedAt >= 0 || cd.opaque {
		return "", false
	}
	if dir.meta || dir.quotedAt >= 0 || dir.opaque || dir.ticks > 0 || dir.nlBefore || isComment(dir) ||
		dir.text == "" || strings.HasPrefix(dir.text, "-") || strings.HasPrefix(dir.text, "~") ||
		strings.ContainsAny(dir.text, "$`*?[{\\") {
		return "", false
	}
	if !and.meta || and.text != "&&" || and.nlBefore {
		return "", false
	}
	return dir.text, true
}
