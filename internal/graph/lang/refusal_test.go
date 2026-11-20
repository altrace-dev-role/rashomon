package lang

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"
)

// TestRefusal_IsTheStepsErrorWithItsFileAndReason: a Refusal reads as its
// step's error, word for word, and errors.Is finds what that error wraps;
// errors.As finds the Refusal under a further wrap, with its file, and a
// Refusal given no reason is a failed read. A Claim returns a step's
// Refusal as the step returned it. Break: put the file in the text, wrap
// nothing, or start the reasons at another value.
func TestRefusal_IsTheStepsErrorWithItsFileAndReason(t *testing.T) {
	inner := fmt.Errorf("/r/x.cfg: %w", fs.ErrPermission)
	var err error = &Refusal{File: "x.cfg", Err: inner}
	if err.Error() != inner.Error() || !errors.Is(err, fs.ErrPermission) {
		t.Errorf("the refusal reads %q, Is(ErrPermission) %v; want %q and true", err, errors.Is(err, fs.ErrPermission), inner)
	}
	var rf *Refusal
	if !errors.As(fmt.Errorf("claim: %w", err), &rf) || rf.File != "x.cfg" || rf.Reason != ReadFailed {
		t.Errorf("errors.As under a wrap = %+v; want the refusal of x.cfg, a failed read", rf)
	}
	var g Registry
	g.RegisterConfigured(fake{l: "py", ext: ".py"}, func(string) (Resolver, error) {
		return nil, &Refusal{File: "x.cfg", Reason: NotJSON, Err: inner}
	})
	if _, ok, cerr := g.Claim("/r", "m.py"); !ok || !errors.As(cerr, &rf) || rf.Reason != NotJSON {
		t.Errorf("Claim = %v, %v; want the step's refusal, not JSON", ok, cerr)
	}
}
