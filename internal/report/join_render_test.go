package report

import (
	"bytes"
	"strings"
	"testing"
)

func TestJoinRender_NamesBothHalvesAndTheOverlap(t *testing.T) {
	var b bytes.Buffer
	writeJoin(&b, Destinations{TokenMatched: 4, WindowMatched: 2, OtherToken: 7})
	out := b.String()
	for _, want := range []string{
		"join: token (4 requests) + window (2 requests)",
		"git (measured",
		"other sessions in this window: 7 requests",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestJoinRender_UntokenedSaysSoRatherThanClaimingAJoin(t *testing.T) {
	var b bytes.Buffer
	writeJoin(&b, Destinations{WindowMatched: 3})
	out := b.String()
	if !strings.Contains(out, "no session token was in use") {
		t.Errorf("an untokened run should say so plainly:\n%s", out)
	}
	if strings.Contains(out, "join: token") {
		t.Errorf("an untokened run claimed a token join:\n%s", out)
	}
	if strings.Contains(out, "git (measured") {
		t.Errorf("the client caveat is noise when no token was in use:\n%s", out)
	}
}

func TestJoinRender_NoCaveatWhenEveryRowCarriedTheToken(t *testing.T) {
	var b bytes.Buffer
	writeJoin(&b, Destinations{TokenMatched: 5})
	if out := b.String(); strings.Contains(out, "git (measured") {
		t.Errorf("every row was tokened; naming a client that could not be is noise:\n%s", out)
	}
}
