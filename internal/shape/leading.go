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
// (`$`, backticks, a glob, quotes or an escape), not `-`, which is the
// previous directory, and not a comment. The one expansion read is the home:
// `~` alone or before a slash, as written, which the hook expands with its
// own home (startDirectory); a named home (`~user`) and the directory stack's
// `~+` and `~-` are refused. `cd DIR;` is left out, since what follows a `;`
// runs whether the cd succeeded or not. `pushd DIR &&` is read as `cd DIR &&`
// is: it changes to DIR and pushes the old directory, which nothing read here
// pops. Its stack forms, a bare pushd and `pushd +N` or `-N`, are refused as
// `~+` and `~-` are. A step in the leading run whose command, past any
// assignments, redirections or an opening ( or { in front of it, is cd, pushd
// or popd and is not such a cd or pushd spoils the run, since the directory
// it leads to is not on the line: popd always. So a `cd -` is refused anywhere
// in the leading run, even when an earlier step names its target: `cd /a &&
// cd b && cd - && go test` loses a pair that way, but it never forms a false
// one. Anything else reports false. Where the program search steps over cd
// steps this does not read, the hook records their location in a domain of
// its own (unreadFold, UnreadCWDDigest); where there are none, the call
// keeps the payload's cwd.
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

// leadingCd reports the DIR of a plain literal `cd DIR &&` or `pushd DIR &&`
// at the start of toks, by LeadingDirectory's rules.
func leadingCd(toks []token) (string, bool) {
	if len(toks) < 3 {
		return "", false
	}
	cd, dir, and := toks[0], toks[1], toks[2]
	if cd.text != "cd" && cd.text != "pushd" || cd.quotedAt >= 0 || cd.opaque {
		return "", false
	}
	if dir.meta || dir.quotedAt >= 0 || dir.opaque || dir.ticks > 0 || dir.nlBefore ||
		dir.text == "" || strings.HasPrefix(dir.text, "-") || cd.text == "pushd" && strings.HasPrefix(dir.text, "+") ||
		strings.HasPrefix(dir.text, "~") && dir.text != "~" && !strings.HasPrefix(dir.text, "~/") ||
		strings.ContainsAny(dir.text, "$`*?[{\\") {
		return "", false
	}
	if !and.meta || and.text != "&&" || and.nlBefore {
		return "", false
	}
	return dir.text, true
}

// unreadFold is the tokenizer's view of the leading cd steps the program
// search steps over in toks, for a line whose leading run leadingRun does not
// read: each token from the first to the last folded separator, its text, a
// NUL, q where the token is quoted and p where it is not, and a NUL. "" when
// the search steps over none. The hook records the call's location from it
// and the payload's cwd (UnreadCWDDigest), a location of its own that no read
// directory's digest equals.
//
// Two folds are one location exactly when the shell reads them alike word for
// word: spacing merges, since the tokenizer drops it, and quoting splits,
// since it changes how an expansion is split and globbed, which a token's
// text, quotes removed, does not show. `${REPO}` and `$REPO` split too, the
// fail-safe side. The steps are counted as the program search counts them,
// without MaxLeadingCd's bound, so a fold of more unread steps than that is
// still unread, and not the payload's cwd, a read location. The walk
// allocates nothing; the view is built only for an unread fold.
func unreadFold(toks []token, uncertain bool) string {
	i, ok := programToken(toks, uncertain)
	if !ok {
		return ""
	}
	after, _, _, _ := directoryChanges(toks, i, uncertain)
	if after == 0 {
		return ""
	}
	n := 0
	for _, t := range toks[:after] {
		n += len(t.text) + 4
	}
	var b strings.Builder
	b.Grow(n)
	for _, t := range toks[:after] {
		q := byte('p')
		if t.quotedAt >= 0 {
			q = 'q'
		}
		b.WriteString(t.text)
		b.WriteByte(0)
		b.WriteByte(q)
		b.WriteByte(0)
	}
	return b.String()
}
