package slot

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
)

// TestGI2s_GateAdmitsARunnersRowAndNothingElseItsWordsRefuse: every row of
// the runner lists is a gate name, the binary's own words however many and
// whatever their shape (`python3 -m pytest`, `uv run pytest`, `tsc
// --noEmit`), as the Stop names an inferred gate by its row (card
// GATE-INFER2). A name the words refuse that is no row stays refused: a row
// with an argument after it, a row cut short, a row's first word with a
// flag. Break: refuse a row, admit whatever the words refuse, or look a name
// up by its first word.
func TestGI2s_GateAdmitsARunnersRowAndNothingElseItsWordsRefuse(t *testing.T) {
	var rows []Gate
	for _, n := range shape.RunnerNames() {
		rows = append(rows, Gate(n))
	}
	if len(rows) == 0 {
		t.Fatal("no runner rows")
	}
	checkSlot(t, rows, []Gate{
		"python3 -m pytest -q", "python3 -m", "go test -v", "pytest -x", "make -n", "npm run test --watch",
		"uv run pytest tests", "bundle exec rspec spec", "tsc --noEmit --watch", " python3 -m pytest", "python3  -m pytest",
	})
}
