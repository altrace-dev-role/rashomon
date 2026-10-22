package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestTRU28_ConfirmIsYesOnlyForYes: purge's question takes y or yes, in any
// case, as yes, and everything else -- an empty line, end of input, a word
// that starts with y -- as no.
func TestTRU28_ConfirmIsYesOnlyForYes(t *testing.T) {
	for in, want := range map[string]bool{
		"y\n": true, "YES\n": true, " yes \n": true, "y": true,
		"\n": false, "": false, "n\n": false, "yeah\n": false, "no\n": false,
	} {
		var out bytes.Buffer
		if got := confirm(strings.NewReader(in), &out); got != want {
			t.Errorf("confirm(%q) = %v, want %v", in, got, want)
		}
		if out.String() != purgePrompt {
			t.Errorf("confirm(%q) asked %q, want %q", in, out.String(), purgePrompt)
		}
	}
}
