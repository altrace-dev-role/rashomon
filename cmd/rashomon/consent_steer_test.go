package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/consent"
)

// TestSC5_ConsentCommandNamesAnUnwrittenSteerMarker: when Append's line is
// on file and the steer-disclosure marker after it is not, the consent
// command says the record is held and notes stay flags; it never says
// nothing was written. Only an Append that wrote nothing says so.
func TestSC5_ConsentCommandNamesAnUnwrittenSteerMarker(t *testing.T) {
	root := t.TempDir()
	steerOnly := fmt.Errorf("%w: %w", consent.ErrSteerMarker, errors.New("disk full"))
	both := errors.Join(fmt.Errorf("%w: %w", consent.ErrScopeMarker, errors.New("disk full")), steerOnly)
	for _, c := range []struct {
		name     string
		err      error
		want     []string
		mustNot  string
		wrapping error
	}{
		{"the steer marker alone", steerOnly, []string{"holds the record, but", "flags"}, "nothing was written", consent.ErrSteerMarker},
		{"both markers", both, []string{"holds the record, but", "v4 shape"}, "nothing was written", consent.ErrSteerMarker},
		{"nothing written", errors.New("consent.json: permission denied"), []string{"nothing was written"}, "holds the record", nil},
	} {
		got := consentAppendError(root, c.err)
		msg := got.Error()
		for _, w := range c.want {
			if !strings.Contains(msg, w) {
				t.Errorf("%s: %q does not say %q", c.name, msg, w)
			}
		}
		if strings.Contains(msg, c.mustNot) {
			t.Errorf("%s: %q says %q", c.name, msg, c.mustNot)
		}
		if c.wrapping != nil && !errors.Is(got, c.wrapping) {
			t.Errorf("%s: the message no longer wraps %v", c.name, c.wrapping)
		}
	}
}
