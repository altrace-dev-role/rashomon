package disclose

import (
	"regexp"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// depthWords is the word the screen spells a directory depth in.
var depthWords = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8}

// TestB2_EveryDepthOnTheScreenSpellsFindIndexsDepth: the screen's sentence
// for the Stop's read of the graph configuration names, in a word, how far
// above the project's root the index may be, and that word and every other
// "at or up to <n> above" on the screen spell graph.MaxIndexDepth, the depth
// FindIndex climbs, so the words and the code cannot drift apart. Witness:
// the prompt's sentence and the Stop's are both read.
func TestB2_EveryDepthOnTheScreenSpellsFindIndexsDepth(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	const stop = "At the end of a reply that wrote a file"
	depth := regexp.MustCompile(`at or up to ([a-z]+) above`)
	stops, phrases := 0, 0
	for _, p := range inv.Paragraphs {
		for _, m := range depth.FindAllStringSubmatch(p.Text, -1) {
			phrases++
			if n, ok := depthWords[m[1]]; !ok || n != graph.MaxIndexDepth {
				t.Errorf("%q says %q; graph.MaxIndexDepth is %d", m[0], m[1], graph.MaxIndexDepth)
			}
		}
		i := strings.Index(p.Text, stop)
		if i < 0 {
			continue
		}
		stops++
		if sentence, _, _ := strings.Cut(p.Text[i:], ". "); !depth.MatchString(sentence) {
			t.Errorf("the Stop's sentence names no depth as \"at or up to <n> above\": %q", sentence)
		}
	}
	if stops != 1 {
		t.Errorf("%d paragraphs carry the Stop's sentence (%q), want 1", stops, stop)
	}
	if phrases < 2 {
		t.Errorf("witness: %d depth phrases on the screen, want the prompt's and the Stop's", phrases)
	}
}
