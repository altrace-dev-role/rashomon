package learn

import (
	"errors"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TestSTR11b_ImportRefusesASecretShapedLessonID (Q4): an import file with a
// row whose id passes the id form but is secret-shaped (a `user:token@`
// shape, or a 64-character run) is refused whole with slot.ErrSecretShaped,
// adds nothing, and the error names the row and never the id. A file of
// ordinary ids imports as before.
func TestSTR11b_ImportRefusesASecretShapedLessonID(t *testing.T) {
	for _, id := range []string{"a.b_c:d@e-f", strings.Repeat("a", 64)} {
		l := open(t, t.TempDir(), t0)
		file := `{"lessons":[{"id":"go-test-cwd","text":"run go test from the module root"},` +
			`{"id":"` + id + `","text":"npm ci before npm test"}]}`
		added, skipped, err := l.Import(strings.NewReader(file), t0)
		if !errors.Is(err, slot.ErrSecretShaped) {
			t.Errorf("Import with id %q: err %v, want slot.ErrSecretShaped", id, err)
		}
		if added != 0 || skipped != 0 || len(l.Lessons()) != 0 {
			t.Errorf("Import with id %q added %d, skipped %d, holds %d lessons; want it all refused", id, added, skipped, len(l.Lessons()))
		}
		if err != nil && (strings.Contains(err.Error(), id) || !strings.Contains(err.Error(), "row 2")) {
			t.Errorf("the refusal %q must name row 2 and not the id", err)
		}
	}
	if added, skipped := importText(t, open(t, t.TempDir(), t0), twoLessons); added != 2 || skipped != 0 {
		t.Errorf("ordinary ids: %d added, %d skipped; want 2, 0", added, skipped)
	}
}
