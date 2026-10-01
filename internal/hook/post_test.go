package hook

import (
	"bytes"
	"testing"
)

// present decodes a key of tool_response to one bit, and the value is never
// a Go string: no copy of a task id, however long, is made in this process.
// Break: trim a string(b) copy, as the first version did, and the decode
// allocates the value.
func TestPresent_DecodesABitAndBuildsNoString(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{`"bash_1"`, true},
		{` "x" `, true},
		{`{"id":1}`, true},
		{`true`, true},
		{`0`, true},
		{`null`, false},
		{` null `, false},
		{`false`, false},
		{"\tfalse\n", false},
	} {
		var p present
		if err := p.UnmarshalJSON([]byte(tc.raw)); err != nil || bool(p) != tc.want {
			t.Errorf("%q: present %v, %v; want %v", tc.raw, p, err, tc.want)
		}
	}

	id := append(append([]byte(`"`), bytes.Repeat([]byte("a"), 4096)...), '"')
	var p present
	if n := testing.AllocsPerRun(20, func() { _ = p.UnmarshalJSON(id) }); n != 0 {
		t.Errorf("decoding a task id allocated %v times, want 0: the value became a string", n)
	}
}
