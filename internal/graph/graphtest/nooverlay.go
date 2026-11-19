package graphtest

import (
	"path"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// NoOverlay is a resolver admitted without overlays (state s5 of LANG-REG1),
// the witness a hook test registers: it claims every .fake file, units each
// by its directory, extracts nothing, and its Overlay returns nil, nil, so
// its OverlayCarried is nil and its overlays replace no index edge.
type NoOverlay struct{}

// NoOverlayLang is NoOverlay's language.
const NoOverlayLang = "fake"

// Lang is "fake".
func (NoOverlay) Lang() string { return NoOverlayLang }

// Claims is every .fake file.
func (NoOverlay) Claims(_, rel string) bool { return strings.HasSuffix(rel, ".fake") }

// Unit is unit:fake:<dir>, the file's directory.
func (NoOverlay) Unit(_, rel string) (graph.ID, bool) {
	return graph.UnitID(NoOverlayLang, path.Dir(rel)), true
}

// Extract returns nothing.
func (NoOverlay) Extract(string, []string, graph.Rev) ([]graph.Record, error) { return nil, nil }

// Overlay returns nil, nil: the language has no overlay.
func (NoOverlay) Overlay(string, []byte, graph.Rev, int64) ([]graph.Record, error) { return nil, nil }

// OverlayCarried is nil: an overlay of NoOverlay carries no predicate.
func (NoOverlay) OverlayCarried() graph.Carried { return nil }
