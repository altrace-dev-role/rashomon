package report

import (
	"strings"
	"testing"
)

// A host the report lists reads as a host the session reached, and nothing in
// the record observed that. So wherever the text lists one -- under the
// destinations heading, or beside a call under --chain -- one line says the
// hosts were only named. The release notes and the installer's closing lines
// say the report says so; these tests are what keep that true.

// namedLine is the legend as the text prints it, indented under its list.
const namedLine = "    hosts are those each call named; nothing here observed whether it reached them"

// TestText_TheDestinationsSayTheirHostsWereOnlyNamed: the default render
// prints the line right under the host lines when either lists a host, and
// not when neither does.
func TestText_TheDestinationsSayTheirHostsWereOnlyNamed(t *testing.T) {
	for _, tc := range []struct {
		name string
		d    Destinations
		want bool
	}{
		{"hosts named", Destinations{DeclaredHosts: []string{"pypi.org"}}, true},
		{"no host named", Destinations{}, false},
		{"ssh hosts only", Destinations{SSHHosts: []string{"bastion.example"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := renderText(t, &Report{Sessions: []Session{{Destinations: tc.d}}})
			if got := strings.Count(text, namedLine); got != b2i(tc.want) {
				t.Fatalf("the legend appears %d times, want %d:\n%s", got, b2i(tc.want), text)
			}
			if tc.want && lineAfter(text, lastHostLine(tc.d)) != namedLine {
				t.Errorf("the legend is not the line under the host lines:\n%s", text)
			}
		})
	}
}

// TestText_TheChainListingSaysItsHostsWereOnlyNamed: under --chain the line
// heads the listing when a call names a host, ssh or not, and is absent when
// none does. A redacted report lists digests, which are still hosts.
func TestText_TheChainListingSaysItsHostsWereOnlyNamed(t *testing.T) {
	chainWith := func(l Link) *Report {
		l.ToolUseID, l.ToolName, l.VerbClass = "toolu_legend", "Bash", "network"
		return &Report{Sessions: []Session{{Chains: Chains{Prompts: []Chain{{PromptID: "p1", Links: []Link{l}}}}}}}
	}
	for _, tc := range []struct {
		name string
		rep  *Report
		want bool
	}{
		{"a call names a host", chainWith(Link{DeclaredHosts: []string{"pypi.org"}}), true},
		{"no call names one", chainWith(Link{}), false},
		{"an ssh host only", chainWith(Link{SSHHosts: []string{"bastion.example"}}), true},
		{"redacted", Redact(chainWith(Link{DeclaredHosts: []string{"pypi.org"}}), []byte("k")), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := renderText(t, tc.rep, WithChain())
			if !strings.Contains(text, "    prompt p1\n") {
				t.Fatalf("premise: the listing did not render, so the legend's absence proves nothing:\n%s", text)
			}
			if got := strings.Count(text, namedLine); got != b2i(tc.want) {
				t.Fatalf("the legend appears %d times, want %d:\n%s", got, b2i(tc.want), text)
			}
			if tc.want && lineAfter(text, "  chains: 1") != namedLine {
				t.Errorf("the legend does not head the listing:\n%s", text)
			}
		})
	}
}

// lastHostLine is the start of the last destinations line that lists hosts.
func lastHostLine(d Destinations) string {
	if len(d.SSHHosts) > 0 {
		return "  ssh hosts named: "
	}
	return "  hosts named by tool calls: "
}

// lineAfter returns the line after the first one that starts with prefix.
func lineAfter(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, prefix) && i+1 < len(lines) {
			return lines[i+1]
		}
	}
	return ""
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
