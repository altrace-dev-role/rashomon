package hook

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
)

// backgroundedCall reads two presence bits off tool_response's spans, and the
// value is never a Go string: no copy of a task id, however long, is made.
// Break: decode the response to read the id, and the allocation grows with it.
func TestBackgroundedCall_ReadsTwoBitsAndBuildsNoString(t *testing.T) {
	for _, tc := range []struct {
		response string
		want     bool
	}{
		{`{"backgroundTaskId":"bash_1"}`, true},
		{`{"backgroundTaskId": "x" }`, true},
		{`{"backgroundTaskId":{"id":1}}`, true},
		{`{"backgroundedByUser":true}`, true},
		{`{"backgroundTaskId":0}`, true},
		{`{"backgroundTaskId":null}`, false},
		{`{"backgroundTaskId":false,"backgroundedByUser":null}`, false},
		{`{"stdout":"backgroundTaskId"}`, false},
		{`"backgroundTaskId"`, false},
		{`null`, false},
	} {
		in, err := hookio.Intake(strings.NewReader(`{"tool_name":"Bash","tool_response":` + tc.response + `}`))
		if err != nil {
			t.Fatal(err)
		}
		if got := backgroundedCall(in); got != tc.want {
			t.Errorf("%s: backgrounded %v, want %v", tc.response, got, tc.want)
		}
	}

	allocs := func(idBytes int) float64 {
		in, err := hookio.Intake(strings.NewReader(`{"tool_response":{"backgroundTaskId":"` + strings.Repeat("a", idBytes) + `"}}`))
		if err != nil {
			t.Fatal(err)
		}
		return testing.AllocsPerRun(20, func() {
			if !backgroundedCall(in) {
				t.Fatal("not backgrounded")
			}
		})
	}
	if small, big := allocs(8), allocs(64<<10); big != small {
		t.Errorf("reading a 64 KiB task id allocated %v times, an 8-byte one %v: the value became a string", big, small)
	}
}
