package hookio

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

// TestRubricTEST9_ReadThroughAwkwardAndFailingReaders drives Read, the
// buffered intake every recording hook reads stdin through, with the readers
// a healthy pipe never is. A reader that delivers the whole payload a byte or
// half a buffer at a time, or with EOF on its last data, gives the same bytes
// as a plain one. A reader that fails partway gives an error, never the part
// it read as if it were the payload: a hook that recorded a cut payload would
// record a call that was not the one made.
func TestRubricTEST9_ReadThroughAwkwardAndFailingReaders(t *testing.T) {
	payload := []byte(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"` + strings.Repeat("ls ", 3000) + `"}}`)
	for name, wrap := range map[string]func(io.Reader) io.Reader{
		"one byte":   iotest.OneByteReader,
		"half":       iotest.HalfReader,
		"data + EOF": iotest.DataErrReader,
	} {
		got, err := Read(wrap(bytes.NewReader(payload)))
		if err != nil || !bytes.Equal(got, payload) {
			t.Errorf("%s reader: %d bytes, %v; want the %d-byte payload", name, len(got), err, len(payload))
		}
	}

	failed := errors.New("test9: the read failed")
	for name, r := range map[string]io.Reader{
		"an error at once": iotest.ErrReader(failed),
		"a timeout midway": iotest.TimeoutReader(bytes.NewReader(payload)),
		"an error after 5 KiB": io.MultiReader(bytes.NewReader(payload[:5<<10]),
			iotest.ErrReader(failed)),
	} {
		got, err := Read(r)
		if err == nil {
			t.Errorf("%s: Read returned %d bytes and no error, want the failure", name, len(got))
		}
		if got != nil {
			t.Errorf("%s: Read returned %d bytes beside its error, want none", name, len(got))
		}
	}
}
