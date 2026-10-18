package consent

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// Questions the consent screen asks, each on the terminal and never on
// stdin (Technical spec 7.2.3 step 5).
const (
	QuestionInstall = "Install? [y/N] "
	QuestionSDK     = "Also record Agent SDK programs on this machine? [y/N] "
	QuestionConsent = "Agree to what this screen describes? [y/N] "
)

// maxAnswerBytes bounds one answer. A longer line is not an answer anyone
// typed, and reading it whole would let a paste grow memory without limit.
const maxAnswerBytes = 1024

// Terminal asks questions on a terminal: the question is written to it and
// one line is read back from it.
type Terminal struct {
	w io.Writer
	r *bufio.Reader
}

// NewTerminal asks on rw, which is the opened /dev/tty in production.
func NewTerminal(rw io.ReadWriter) *Terminal {
	return &Terminal{w: rw, r: bufio.NewReaderSize(rw, maxAnswerBytes)}
}

// Ask writes question and reads one line. eof is true when the terminal
// closed (Ctrl-D at the start of a line) before a line feed arrived; what
// was typed before it is still returned.
func (t *Terminal) Ask(question string) (answer string, eof bool, err error) {
	if _, err := io.WriteString(t.w, question); err != nil {
		return "", false, err
	}
	line, err := t.r.ReadSlice('\n')
	switch {
	case errors.Is(err, io.EOF):
		return string(line), true, nil
	case errors.Is(err, bufio.ErrBufferFull):
		// Drain the rest of the line so the next question does not read it
		// as its answer; the overlong line is no answer at all.
		for errors.Is(err, bufio.ErrBufferFull) {
			_, err = t.r.ReadSlice('\n')
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return "", false, err
		}
		return "", errors.Is(err, io.EOF), nil
	case err != nil:
		return "", false, err
	}
	return string(line), false, nil
}

// IsYes reports whether an answer is y or yes, in any case, with surrounding
// white space ignored. Anything else, an empty line included, is no.
func IsYes(answer string) bool {
	a := strings.TrimSpace(answer)
	return strings.EqualFold(a, "y") || strings.EqualFold(a, "yes")
}
