package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestRP2zl_RedactScrubsATrustedSpecsGateId (census g3 W1, STEP-15): a trusted
// spec's gate id becomes the gate's name in the autosteer block, in
// autosteer.gates[].name and in autosteer.stop.sentence, and its charset
// admits a hostname. Under --redact neither path carries the id; without it
// both keep the owner's own word. Break: skip the autosteer pass in Redact.
func TestRP2zl_RedactScrubsATrustedSpecsGateId(t *testing.T) {
	const id = "a.example.com"
	f := newLW(t)
	f.bind(lwSpec(id))
	f.land("toolu_e", "Edit", lwEdit(f.file("a.go", "package a\n")), nil)
	f.land("toolu_g", "Bash", lwBash("go test ./..."), nil)
	f.stop()

	report := func(args ...string) (string, map[string]any) {
		t.Helper()
		var o, e bytes.Buffer
		code := run(append([]string{"report", "--json", "--session", "s"}, args...), strings.NewReader(""), &o, &e)
		if code != 0 {
			t.Fatalf("report %v: exit %d: %s", args, code, e.String())
		}
		var doc struct {
			Sessions []struct {
				Autosteer map[string]any `json:"autosteer"`
			} `json:"sessions"`
		}
		if err := json.Unmarshal(o.Bytes(), &doc); err != nil || len(doc.Sessions) != 1 || doc.Sessions[0].Autosteer == nil {
			t.Fatalf("premise: one session with an autosteer block (err %v):\n%s", err, o.String())
		}
		return o.String(), doc.Sessions[0].Autosteer
	}
	paths := func(a map[string]any) (names []string, sentence string) {
		for _, g := range a["gates"].([]any) {
			names = append(names, g.(map[string]any)["name"].(string))
		}
		sentence, _ = a["stop"].(map[string]any)["sentence"].(string)
		return names, sentence
	}

	_, plain := report()
	names, sentence := paths(plain)
	if !strings.Contains(strings.Join(names, " "), id) || !strings.Contains(sentence, id) {
		t.Fatalf("premise: the unredacted report names the gate %q in both paths: names %v, sentence %q", id, names, sentence)
	}

	body, red := report("--redact")
	names, sentence = paths(red)
	if strings.Contains(strings.Join(names, " "), id) || strings.Contains(sentence, id) || strings.Contains(body, id) {
		t.Errorf("gate id %q survived --redact: names %v, sentence %q", id, names, sentence)
	}
	if len(names) == 0 || sentence == "" {
		t.Errorf("--redact dropped the gates or the sentence rather than digesting them: %v, %q", names, sentence)
	}
}
