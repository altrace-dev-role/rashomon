package settings

import (
	"strings"
	"testing"
)

// TestTRU44_ValuesFreeDiffKeepsKeysAndLayoutAndNoValue: the detach
// self-check's diff shows what changed in a line's keys, punctuation and
// whitespace, and nothing of any string, number or literal the file held.
func TestTRU44_ValuesFreeDiffKeepsKeysAndLayoutAndNoValue(t *testing.T) {
	before := "{\"env\":{\"TOKEN\":\"s3cr3t\"},\"n\":42,\"on\":true}"
	after := "{\n  \"env\": {\"TOKEN\": \"s3cr3t\"},\n\t\"n\": -4.2e1,\n  \"on\": null  \r\n}\n"
	lines := ValuesFreeDiff([]byte(before), []byte(after))
	out := strings.Join(lines, "\n")
	for _, value := range []string{"s3cr3t", "42", "4.2", "true", "null"} {
		if strings.Contains(out, value) {
			t.Errorf("the diff carries the value %q:\n%s", value, out)
		}
	}
	for _, want := range []string{
		`- {"env":{"TOKEN":` + Elided + `},"n":` + Elided + `,"on":` + Elided + `}`,
		`+   "env": {"TOKEN": ` + Elided + `},`,
		`+ \t"n": ` + Elided + `,`,
		`+   "on": ` + Elided + `\x20\x20\r`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the diff does not hold %q:\n%s", want, out)
		}
	}
	if same := ValuesFreeDiff([]byte(before), []byte(before)); len(same) != 0 {
		t.Errorf("identical files diff as %q", same)
	}
}

// TestTRU44_ValuesFreeDiffEscapesWhatAKeyCouldSmuggle: a key is printed,
// so a control byte or a non-ASCII byte in one is escaped rather than sent
// to the terminal.
func TestTRU44_ValuesFreeDiffEscapesWhatAKeyCouldSmuggle(t *testing.T) {
	out := strings.Join(ValuesFreeDiff([]byte("{}"), []byte("{\"a\x1b[2J\u202e\": 1}")), "\n")
	if strings.ContainsAny(out, "\x1b\u202e") {
		t.Errorf("the diff passed a control or bidi byte through: %q", out)
	}
	if !strings.Contains(out, `\x1b`) || !strings.Contains(out, `\xe2`) {
		t.Errorf("the diff does not show the escaped bytes: %q", out)
	}
}
