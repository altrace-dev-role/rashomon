package report

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

// TestFinalAssistantTexts_AReadErrorDropsEveryTurnsWords: a transcript whose
// read fails part way may hold a later reply than any read, so no words read
// before the failure are a turn's final word. A line too long to read is not
// this case: the reader steps over it and reads on (transcript_toolong_test.go).
func TestFinalAssistantTexts_AReadErrorDropsEveryTurnsWords(t *testing.T) {
	transcript := assistantWith(t, `"Done; the build passes."`)
	if whole := FinalAssistantTexts(strings.NewReader(transcript), map[string]bool{"p1": true})["p1"]; !whole.Said {
		t.Fatalf("read whole, p1 = %+v; want its words", whole)
	}
	cut := io.MultiReader(strings.NewReader(transcript), iotest.ErrReader(errors.New("input/output error")))
	if got := FinalAssistantTexts(cut, map[string]bool{"p1": true})["p1"]; got.Said {
		t.Errorf("p1 kept %q from a transcript whose read failed after it", got.Text)
	}
}
