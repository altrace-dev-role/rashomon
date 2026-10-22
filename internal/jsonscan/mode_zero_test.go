package jsonscan

import (
	"errors"
	"strings"
	"testing"
)

// TestRubricNAMING12_AnUnsetCaptureModeIsRefused holds CaptureMode's zero
// value neutral: a Path whose Mode was never set names no capture, and Stream
// refuses it with ErrPaths rather than quietly copying the value's head.
func TestRubricNAMING12_AnUnsetCaptureModeIsRefused(t *testing.T) {
	var zero CaptureMode
	for _, m := range []CaptureMode{CaptureHead, CaptureTail, CaptureHeadTail, CapturePresence, CaptureLines} {
		if zero == m {
			t.Errorf("the zero CaptureMode is mode %d; it must be none", m)
		}
	}
	_, err := Stream(strings.NewReader(`{"a":"value"}`), []Path{{Elems: []string{"a"}, Max: 4}},
		func(int, []byte, bool) bool { return true })
	if !errors.Is(err, ErrPaths) {
		t.Errorf("Stream with an unset Mode: err %v, want ErrPaths", err)
	}
	// ErrPaths's own message lists three other causes (too many paths, a
	// path too long, a capture of no bytes) and names none of them here, so
	// the error must say more on its own for a caller to tell this case
	// apart from those without re-deriving it from the inputs.
	if err == nil || err.Error() == ErrPaths.Error() {
		t.Errorf("Stream with an unset Mode: err %v, want a message naming the unset mode, not ErrPaths' own", err)
	}
}

// TestRubricNAMING12_ACaptureLinesBelowOneRuneIsRefused holds CaptureLines'
// own minimum to the same rule: a Max too small to hold one UTF-8 rune is
// refused with a message naming that cause, not ErrPaths' own list of three
// unrelated ones.
func TestRubricNAMING12_ACaptureLinesBelowOneRuneIsRefused(t *testing.T) {
	_, err := Stream(strings.NewReader(`{"a":"value"}`), []Path{{Elems: []string{"a"}, Max: 1, Mode: CaptureLines}},
		func(int, []byte, bool) bool { return true })
	if !errors.Is(err, ErrPaths) {
		t.Errorf("Stream with CaptureLines Max=1: err %v, want ErrPaths", err)
	}
	if err == nil || err.Error() == ErrPaths.Error() {
		t.Errorf("Stream with CaptureLines Max=1: err %v, want a message naming the Max too small, not ErrPaths' own", err)
	}
}
