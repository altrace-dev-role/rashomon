package lang_test

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
)

// TestWire_EveryIndexedReaderStatesItsProducer: each reader this build's
// registry (index.Wire) holds as indexed states a producer string
// (Producer), the one the index it writes names in its header, so
// Unindexed compares the header's string for every indexed language here
// and keeps the key test alone for none. pylite is one, so the list is not
// empty. Break: register an indexed reader with no Producer method, or one
// stating an empty string.
func TestWire_EveryIndexedReaderStatesItsProducer(t *testing.T) {
	rs := lang.IndexedOf(index.Wire())
	if len(rs) == 0 {
		t.Fatal("index.Wire registers no indexed reader; want pylite at least")
	}
	for _, r := range rs {
		v, ok := r.(interface{ Producer() string })
		switch {
		case !ok:
			t.Errorf("the indexed %s reader states no producer string (no Producer method)", r.Lang())
		case v.Producer() == "":
			t.Errorf("the indexed %s reader states an empty producer string", r.Lang())
		}
	}
}
