package acceptance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
)

// readmeURL is where the plugin's description says the full list of what is
// kept is: the repository's README, at one section's anchor.
const readmeURL = "https://github.com/altrace-dev-role/rashomon#"

// TestPC1e_TheDescriptionNamesWhereTheFullListIs (the DE's #228 verdict): the
// plugin description's list says "including:", so it is not all that is
// kept, and a person deciding in /plugin must be able to find the rest before
// the keystroke. The description ends with the README section that holds it,
// and that section quotes the full screen's STORED paragraph word for word,
// so the list there is the one a y on the screen agrees to and changes with
// it, as the README's excerpt changes with the render. Break: drop the
// pointer, rename the heading, or edit the STORED paragraph and not the
// README, and this fails.
func TestPC1e_TheDescriptionNamesWhereTheFullListIs(t *testing.T) {
	i := strings.LastIndex(disclose.PluginDescription, readmeURL)
	if i < 0 {
		t.Fatalf("the plugin description names no README section (%s...):\n%s", readmeURL, disclose.PluginDescription)
	}
	anchor := disclose.PluginDescription[i+len(readmeURL):]
	if !regexp.MustCompile(`^[a-z0-9_-]+$`).MatchString(anchor) {
		t.Fatalf("the plugin description does not end with the README section's anchor: %q", anchor)
	}
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	for _, p := range inv.Paragraphs {
		if strings.HasPrefix(p.Text, disclose.StoredHeading+" for each tool call") {
			stored = p.Text
		}
	}
	if stored == "" {
		t.Fatal("premise: the full screen has no STORED paragraph")
	}
	body, err := os.ReadFile(filepath.Join(moduleRoot, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(body), "\n")
	var at []int
	for i, l := range lines {
		if h, ok := strings.CutPrefix(l, "## "); ok && githubAnchor(h) == anchor {
			at = append(at, i)
		}
	}
	if len(at) != 1 {
		t.Fatalf("README.md has %d sections whose anchor is #%s, want 1", len(at), anchor)
	}
	var section []string
	for _, l := range lines[at[0]+1:] {
		if strings.HasPrefix(l, "## ") {
			break
		}
		section = append(section, l)
	}
	for _, q := range quotes(section) {
		if q == stored {
			return
		}
	}
	t.Errorf("README.md's #%s does not quote the full screen's STORED paragraph word for word. "+
		"Update its > quote to the paragraph.\nquotes:\n%q\nwant:\n%s", anchor, quotes(section), stored)
}

// githubAnchor is the fragment GitHub gives a heading: lower case, spaces as
// hyphens, every other character but a letter, a digit, - and _ dropped.
func githubAnchor(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(heading)) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// quotes is each run of "> " lines in lines, joined with one space, the way
// Markdown reads a wrapped quote.
func quotes(lines []string) []string {
	var out, run []string
	flush := func() {
		if len(run) > 0 {
			out = append(out, strings.Join(run, " "))
			run = nil
		}
	}
	for _, l := range lines {
		if q, ok := strings.CutPrefix(l, "> "); ok {
			run = append(run, q)
			continue
		}
		flush()
	}
	flush()
	return out
}
