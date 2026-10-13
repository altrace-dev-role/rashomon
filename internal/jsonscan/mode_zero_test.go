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
}
