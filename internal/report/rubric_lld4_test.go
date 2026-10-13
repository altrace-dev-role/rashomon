package report

import (
	"reflect"
	"testing"
)

// TestRubricLLD4_LoopbackSpellingsAreNotNovel: a local server is not a new
// place a project's traffic goes, in whatever spelling the call named it.
// Loopback was a list of five spellings, so [0:0:0:0:0:0:0:1], 127.0.0.2,
// 2130706433 and api.localhost were offered to the baseline as candidates
// for "new destination" while [::1] and 127.0.0.1 were not.
func TestRubricLLD4_LoopbackSpellingsAreNotNovel(t *testing.T) {
	got := novelHostCandidates([]string{
		"localhost", "127.0.0.1", "[::1]", "0.0.0.0",
		"[0:0:0:0:0:0:0:1]", "127.0.0.2", "2130706433", "api.localhost", "[::]",
		"api.anthropic.com", "example.com", "10.1.2.3",
	}, false)
	if want := []string{"example.com", "10.1.2.3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("novelty candidates %q, want %q", got, want)
	}
}
