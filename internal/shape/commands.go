package shape

import (
	"encoding/json"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
)

// Command is one simple command of a line as Commands reads it: its words
// from the program on, quotes removed, without assignments before the
// program, redirections with their words, or fd prefixes. Expands marks,
// word by word, a word the shell expands before running it -- a parameter, a
// command substitution, a backtick, an unquoted glob or brace -- whose text
// is not the word the program receives. Heredoc is a here-document or
// here-string feeding the command, whose text is the command's input, not
// its words.
type Command struct {
	Words   []string
	Expands []bool
	Heredoc bool
}

// Commands is every simple command of line in line order, split where the
// shell splits a list -- ;, &&, ||, |, & and an unquoted newline -- with the
// shell's reserved words before a program passed over and a stage holding
// nothing else giving no command. It reads the line once, with
// tokenizeList's reading, here-document bodies skipped, and names each
// program as the program search does (programToken). ok is false, with the
// commands read before it, where that reading stops: a control byte, a
// reading the tokenizer cannot vouch for, a program it cannot name or a
// program word the shell expands.
func Commands(line string) ([]Command, bool) {
	if controlByte(line) {
		return nil, false
	}
	toks, err := tokenizeList(line)
	if err != nil {
		return nil, false
	}
	var out []Command
	for s := 0; s < len(toks); {
		e, next := stageEnd(toks, s)
		b := s
		for b < e && !toks[b].meta && toks[b].quotedAt < 0 && reservedWord(toks[b].text) {
			b++
		}
		if b < e && hasWord(toks[b:e]) {
			p, ok := programToken(toks[b:e], false)
			if !ok || expands(toks[b+p]) {
				return out, false
			}
			out = append(out, commandOf(toks, b+p, e))
		}
		s = next
	}
	return out, true
}

// commandOf is the command whose program is toks[p], its stage ending at e.
func commandOf(toks []token, p, e int) Command {
	var c Command
	for j := p; j < e; j++ {
		t := toks[j]
		switch {
		case t.meta && isRedirect(t.text):
			if t.text == "<<" {
				c.Heredoc = true
			}
			j = operatorEnd(toks, j)
			if j+1 < e && !toks[j+1].meta {
				j++
			}
		case t.meta, fdPrefix(toks, j, e):
		default:
			c.Words = append(c.Words, t.text)
			c.Expands = append(c.Expands, expands(t))
		}
	}
	return c
}

// hasWord reports a stage holding a word, not only operators.
func hasWord(toks []token) bool {
	for _, t := range toks {
		if !t.meta {
			return true
		}
	}
	return false
}

// reservedWord reports a reserved word that may stand before a program or
// alone in a stage: it opens or closes a compound, or negates a pipeline.
func reservedWord(s string) bool {
	switch s {
	case "!", "if", "then", "else", "elif", "fi", "while", "until", "do", "done", "}", "esac":
		return true
	}
	return false
}

// expands reports a word the shell rewrites before the program sees it: an
// expansion the tokenizer does not parse, a backtick, a `$` anywhere (a
// single-quoted one too, which only costs a decision), or a glob or brace
// byte before the word's first quoted byte.
func expands(t token) bool {
	if t.opaque || t.ticks > 0 || strings.ContainsAny(t.text, "$`") {
		return true
	}
	bare := t.text
	if t.quotedAt >= 0 && t.quotedAt <= len(bare) {
		bare = bare[:t.quotedAt]
	}
	return strings.ContainsAny(bare, "*?[{")
}

// CommandLine is the command line a shell tool's input carries, the one
// Derive reads, decoded for a caller that matches it against a rule in
// memory: false for any other tool, for an input with no string command,
// and for a command whose encoded form is over max bytes, refused before it
// is decoded so a large line costs no copy. It is one of the four places a
// substring of a tool input leaves this package (see the package comment),
// and no record stores what it returns.
func CommandLine(toolName string, toolInput json.RawMessage, max int) (string, bool) {
	if verbForTool(toolName) != VerbExecute {
		return "", false
	}
	s, ok := inputField(toolInput, "command")
	if !ok || s.Kind != jsonscan.String || int(s.Len)-2 > max {
		return "", false
	}
	// The encoded length bounds the read only roughly: decoding can grow the
	// text, an invalid byte becoming U+FFFD's three, so the cap is held again
	// on what was decoded.
	line, ok := stringValue(toolInput, s)
	if !ok || len(line) > max {
		return "", false
	}
	return line, true
}
