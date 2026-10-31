package hook

import (
	"bytes"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/gateout"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// gateOutputBudget is how long after its capture starts a gate run's output
// may still be read. The hook holds no time bound of its own (handle.go), so
// this is the tech spec's figure (2.2.3): a capture past it records unread,
// budget, and the execution lands as it would have.
const gateOutputBudget = 50 * time.Millisecond

// streamKeys are the keys of a Bash tool_response that hold its output, in the
// order they are read: stderr only when stdout gives no summary.
var streamKeys = jsonscan.NewKeySet("stdout", "stderr")

// The reasons a gate run's output is unread (store.GateOutput.Reason).
const (
	unreadBudget   = "budget"
	unreadTooLarge = "too_large"
	unreadDecode   = "decode"
)

// gateKind is the kind gateout.Classify takes for a gate run's shape, whose
// words are the runner kinds': the runner's list, else the kind whose status
// is hidden, else test for a line of class test.
func gateKind(s *shape.Shape) string {
	switch {
	case s.RunnerKind != nil:
		return *s.RunnerKind
	case s.StatusMasked != nil && *s.StatusMasked != shape.MaskedNone:
		return *s.StatusMasked
	case s.VerbClass == shape.VerbTest:
		return shape.RunnerTest
	}
	return ""
}

// programOf is the program gateout's tables are keyed by: the table's own word
// for the runner the line launched (gateout.ProgramFor: pytest for `python3 -m
// pytest`, go for `go test`), else path.Base of the line's program word, or
// nothing where the shape could not name one. The first word of a line names
// the interpreter and not the runner, so keyed by it a piped `python3 -m
// pytest` read as unattributable before a byte of its output was looked at
// (D229, MASKED-OUTCOME1).
func programOf(s *shape.Shape) string {
	program, runner := "", ""
	if s.Program != nil {
		program = *s.Program
	}
	if s.RunnerName != nil {
		runner = *s.RunnerName
	}
	return gateout.ProgramFor(program, runner)
}

// gateOutput classifies a gate run's output (2.2.3, 2.2.10) and returns only
// the three closed words that survive it. A build kind, or a test kind whose
// program no table knows, is decided without reading anything. Otherwise the
// text is a failure event's error after its first line, which is the exit
// code's (exitCode), or else the response's stdout, then its stderr where
// stdout gives no summary (cargo prints its summary there). Both decode into
// one buffer local to this call, sized by the spans and capped at
// gateout.MaxScanBytes, so a response of any size costs at most that much:
// past the cap the output is unread, too_large. A capture already past
// deadline reads nothing, and an output that does not decode is unread too.
func gateOutput(in *hookio.Payload, failure bool, kind, program string, deadline time.Time, now func() time.Time) store.GateOutput {
	if out := gateout.Classify(kind, program, nil); kind != shape.RunnerTest || out.Kind == "unattributable" {
		return out
	}
	if now().After(deadline) {
		return unread(unreadBudget)
	}
	var src [2]jsonscan.Span
	if failure {
		src[0] = in.Spans[hookio.Error]
	} else if jsonscan.Field(in.Raw, in.Spans[hookio.ToolResponse], streamKeys, src[:]) != nil {
		return unread(unreadDecode)
	}
	n := 0
	for _, s := range src {
		if s.Kind == jsonscan.String {
			n += int(s.Len)
		}
	}
	return readOutput(in.Raw, src, failure, program, make([]byte, 0, min(n, gateout.MaxScanBytes)))
}

// readOutput decodes each string span of src into buf, the second only where
// the first gives no summary, and classifies what it decoded. Every byte of
// buf is cleared before it returns, so neither a panic value nor a later read
// of the heap finds the output; buf is never held past this call.
//
// buf's capacity is the bound: a span that decodes past it was longer than
// the cap, too_large, unless the cap is below gateout.MaxScanBytes, where
// buf was sized by the spans' encoded lengths and only invalid UTF-8, each
// byte of which decodes to three, outgrows them -- an output that does not
// decode as text.
func readOutput(raw []byte, src [2]jsonscan.Span, failure bool, program string, buf []byte) store.GateOutput {
	defer clear(buf[:cap(buf)])
	out := gateout.Classify(shape.RunnerTest, program, nil)
	for i, s := range src {
		if i > 0 && out.Kind != "no_summary" {
			break
		}
		if s.Kind != jsonscan.String {
			continue
		}
		text, truncated, err := jsonscan.Head(raw, s, cap(buf)-len(buf), buf[len(buf):])
		switch {
		case err != nil, truncated && cap(buf) < gateout.MaxScanBytes:
			return unread(unreadDecode)
		case truncated:
			return unread(unreadTooLarge)
		}
		buf = buf[:len(buf)+len(text)]
		if failure {
			_, text, _ = bytes.Cut(text, []byte{'\n'})
		}
		out = gateout.Classify(shape.RunnerTest, program, text)
	}
	return out
}

func unread(reason string) store.GateOutput {
	return store.GateOutput{Kind: "unread", Reason: &reason}
}
