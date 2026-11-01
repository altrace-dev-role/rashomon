package disclose

import (
	"strings"
	"testing"
)

// TestGI1d_TheBriefParagraphSaysWhereTheTestCommandIsRead (card
// GATE-INFER1, W6): the brief and the Stop now read the content of the
// project's CI workflows, Makefile, package.json and test manifests for its
// test command, when a session starts and at the end of each turn, so the
// paragraph that says what the brief reads says so, and that none of it is
// stored.
// Break: the screen keeps saying the brief reads names and sizes only.
func TestGI1d_TheBriefParagraphSaysWhereTheTestCommandIsRead(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	var brief string
	for _, p := range inv.Paragraphs {
		if strings.HasPrefix(p.Text, "When a session starts or is cleared") {
			brief = p.Text
		}
	}
	for _, want := range []string{"CI workflows", "Makefile", "package.json", "test manifests",
		"when a session starts and at the end of each turn", "none of their text is stored"} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief paragraph does not say %q:\n%s", want, brief)
		}
	}
}
