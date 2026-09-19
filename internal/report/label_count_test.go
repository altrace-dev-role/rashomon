package report

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

func labelled(id string, label *string) store.Declaration {
	return store.Declaration{
		ToolUseID: id,
		ToolName:  "Read",
		SessionID: "sess-1",
		FileLabel: label,
	}
}

func ptr(s string) *string { return &s }

// TestByLabel_CountsOnlyLabelledCalls: a declaration whose tool names no file
// carries no label and is not counted, so the total under by_label is the
// number of calls that named a file, not the number of declarations. Counting
// the nulls would make the row a count of something nobody asked about.
func TestByLabel_CountsOnlyLabelledCalls(t *testing.T) {
	sess := build(&store.Run{Declarations: []store.Declaration{
		labelled("t1", ptr(shape.LabelSSHKey)),
		labelled("t2", ptr(shape.LabelSSHKey)),
		labelled("t3", ptr(shape.LabelEnvFile)),
		labelled("t4", nil),
	}})
	got := sess.Declarations.ByLabel
	if len(got) != 2 || got[shape.LabelSSHKey] != 2 || got[shape.LabelEnvFile] != 1 {
		t.Errorf("by_label = %v, want {ssh-key: 2, env-file: 1}", got)
	}
	if sess.Declarations.Recorded != 4 {
		t.Errorf("recorded = %d, want 4: the unlabelled call is still a declaration", sess.Declarations.Recorded)
	}
}

// TestByLabel_IsAlwaysPresent: an empty map marshals as {} and never as null,
// the rule every other collection on this report follows -- null and [] are
// the same absence to a reader and different values to a consumer.
func TestByLabel_IsAlwaysPresent(t *testing.T) {
	sess := build(&store.Run{Declarations: []store.Declaration{labelled("t1", nil)}})
	if sess.Declarations.ByLabel == nil {
		t.Error("by_label is nil, want an empty map")
	}
}

// TestByLabel_ClampsAValueOffDiskToTheVocabulary is the content guarantee for
// this row, and it is the reason knownLabel exists.
//
// These map keys are rendered verbatim by both the text report and the JSON
// one. The value they come from was read off disk, and a record written by an
// older build, a newer one, or a hand-edited file can carry any string at all
// -- including a path, which is exactly what the rest of this program is
// arranged to keep out of the output. Anything outside shape.Labels() is
// counted as unknown, so the rendered vocabulary stays closed.
func TestByLabel_ClampsAValueOffDiskToTheVocabulary(t *testing.T) {
	const smuggled = "/home/u/.ssh/id_rsa"

	sess := build(&store.Run{Declarations: []store.Declaration{
		labelled("t1", ptr(smuggled)),
		labelled("t2", ptr("a-label-no-build-ever-emitted")),
		labelled("t3", ptr(shape.LabelSSHKey)),
	}})

	vocab := map[string]bool{}
	for _, l := range shape.Labels() {
		vocab[l] = true
	}
	for key := range sess.Declarations.ByLabel {
		if !vocab[key] {
			t.Errorf("by_label carries the key %q, which is not in shape.Labels()", key)
		}
	}
	if got := sess.Declarations.ByLabel[shape.LabelUnknown]; got != 2 {
		t.Errorf("by_label[unknown] = %d, want 2: both unrecognised values are still declarations", got)
	}
	if got := sess.Declarations.ByLabel[shape.LabelSSHKey]; got != 1 {
		t.Errorf("by_label[ssh-key] = %d, want 1", got)
	}
}
