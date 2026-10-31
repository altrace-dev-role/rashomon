package disclose

import (
	"strings"
	"testing"
)

// TestLRN17k_ConsentSaysApprovedLessonsReachTheAgent: this build gives the
// agent an approved lesson with a prompt (LEARN-DELIVER1), so the learn
// section of the consent screen says so, and no longer says that no lesson
// reaches the agent. The screen's scope was bumped for it (TestTRU8b_), so
// an earlier yes does not stand for model-visible lesson text.
func TestLRN17k_ConsentSaysApprovedLessonsReachTheAgent(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	var said bool
	for _, p := range inv.Paragraphs {
		if p.Owner != OwnerLearn || p.Cite != CiteSection4 {
			continue
		}
		if strings.Contains(p.Text, "No lesson reaches your agent") {
			t.Errorf("the screen still says no lesson reaches the agent: %q", p.Text)
		}
		if strings.Contains(p.Text, "An approved lesson reaches your agent with your prompt, once a session") {
			said = true
		}
	}
	if !said {
		t.Error("the learn section does not say an approved lesson reaches the agent with a prompt, once a session")
	}
}
