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
		if strings.Contains(p.Text, "below the note") {
			t.Errorf("the screen puts a lesson below the preflight note, which supersedes it: %q", p.Text)
		}
		said = strings.Contains(p.Text, "An approved lesson is sent with a prompt that carries no preflight note, at most one per prompt and each lesson once a session; it waits while a preflight note is sent.") &&
			strings.Contains(p.Text, "If steering is off or you have not agreed to steering notes, it is shown to you and not sent.")
	}
	if !said {
		t.Error("the learn section does not say when an approved lesson is sent: with a prompt that has no preflight note, once a session, and only with steering on")
	}
}

// TestLRN17k_ConsentClaimsOnlyWhatHolds: the lesson files' HMAC is tamper
// evidence, not secrecy: an agent running as the user can read the store
// key. So the learn section and the state.sum row do not say "nothing you
// have not approved is sent", and say the limit spec trust has: a write
// rashomon did not make is detected unless the writer can also read the key.
func TestLRN17k_ConsentClaimsOnlyWhatHolds(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	var said bool
	for _, p := range inv.Paragraphs {
		if p.Owner != OwnerLearn || p.Cite != CiteSection4 {
			continue
		}
		if strings.Contains(p.Text, "nothing you have not approved is sent") {
			t.Errorf("the screen claims what a key-reading writer defeats: %q", p.Text)
		}
		said = strings.Contains(p.Text, "unless the writer can also read the store key") && strings.Contains(p.Text, "rashomon spec trust")
	}
	if !said {
		t.Error("the learn section does not state the store-key limit")
	}
	for _, f := range inv.Files {
		if strings.HasSuffix(f.Pattern, "state.sum") && (!strings.Contains(f.Purpose, "unless the writer can also read the store key") || !strings.Contains(f.Purpose, "not secrecy")) {
			t.Errorf("state.sum's purpose %q does not state the limit", f.Purpose)
		}
	}
}
