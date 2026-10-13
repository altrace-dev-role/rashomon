package shape

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// TestRES9_DeriveSaysALineWasCut: a shell line read only to the token bound
// is reported as cut, so a writer that can carry the mark (schema 5's
// shape.truncated, P-11) never has to guess it. A line of exactly maxTokens
// tokens is read whole and is not cut; one token more is, and so is a
// here-document past the bound, whose program the bound keeps. A short line
// and a tool that is not a shell are not cut.
func TestRES9_DeriveSaysALineWasCut(t *testing.T) {
	boundBuildable(t)
	for _, tc := range []struct {
		name, tool, cmd string
		want            bool
	}{
		{"at the bound", "Bash", words(maxTokens - 1), false},
		{"one past the bound", "Bash", words(maxTokens), true},
		{"a here-document past the bound", "Bash", heredocLine(maxTokens), true},
		{"a short line", "Bash", "ls -la", false},
		{"not a shell", "Read", "", false},
	} {
		in := bashInput(t, tc.cmd)
		if tc.tool == "Read" {
			in = json.RawMessage(`{"file_path":"/x"}`)
		}
		s, cut := DeriveCut(tc.tool, in, benchKey)
		if cut != tc.want {
			t.Errorf("%s: cut %v, want %v", tc.name, cut, tc.want)
		}
		if cut && !s.MayWrite {
			t.Errorf("%s: a cut line with may_write false", tc.name)
		}
		if plain := Derive(tc.tool, in, benchKey); show(plain.Program) != show(s.Program) || plain.MayWrite != s.MayWrite || plain.Digest != s.Digest {
			t.Errorf("%s: Derive and DeriveCut disagree on the shape", tc.name)
		}
	}
}

// TestRES9_HostsSayAnSSHDestinationPastTheBoundWasCut: the ssh destinations
// are read over the token bound for the whole command (RES-9), so one named
// past it is not read, and before this the list came back short with nothing
// to say so. It is now reported as cut: past the bound within one line, and
// past it across lines, where each line costs at least one. The line count
// is tested at N and N+1. cut is about the ssh list: a URL host past the
// bound is still in the wire list, since the URL scan reads the whole text,
// while the same line is cut for the ssh reader, which stopped at the bound.
func TestRES9_HostsSayAnSSHDestinationPastTheBoundWasCut(t *testing.T) {
	boundBuildable(t)
	const h = "h.example.com"
	for _, tc := range []struct {
		name, cmd string
		wantH     bool
		wantCut   bool
	}{
		{"named and read", "ssh " + h, true, false},
		{"past the bound in one line", words(maxTokens) + "; ssh " + h, false, true},
		{"the last line the budget reads", strings.Repeat("true\n", maxTokens-1) + "ssh " + h, true, false},
		{"the first line it does not", strings.Repeat("true\n", maxTokens) + "ssh " + h, false, true},
		{"a long last line cut at the bound", "ssh " + h + "\n" + words(maxTokens), true, true},
	} {
		_, ssh, cut := HostsCut("Bash", bashInput(t, tc.cmd))
		if got := slices.Contains(ssh, h); got != tc.wantH {
			t.Errorf("%s: %s in ssh_hosts %v, want %v", tc.name, h, got, tc.wantH)
		}
		if cut != tc.wantCut {
			t.Errorf("%s: cut %v, want %v", tc.name, cut, tc.wantCut)
		}
	}
	wire, _, cut := HostsCut("Bash", bashInput(t, words(maxTokens)+" https://w.example.com/x"))
	if !slices.Contains(wire, "w.example.com") || !cut {
		t.Errorf("a URL past the bound: wire %v, cut %v; want w.example.com, and cut for the ssh reader", wire, cut)
	}
	if _, _, cut := HostsCut("WebFetch", json.RawMessage(`{"url":"https://w.example.com/"}`)); cut {
		t.Errorf("WebFetch: cut")
	}
}
