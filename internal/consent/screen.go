package consent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/install"
)

// Width is the consent screen's line limit, counted in characters.
const Width = 80

// indent is the left margin of every section body.
const indent = "   "

// NewInstallPlaceholder stands for the install id on the screen of a machine
// with no store yet. The id is made when the store is opened, after y, and
// the screen says so rather than show an id that is then not the one used;
// it also keeps `--show` on such a machine printing the same bytes each time.
const NewInstallPlaceholder = "<made when you answer y>"

// Command is one line of the screen's closing table: what it does and the
// command that does it.
type Command struct {
	Does, Run string
}

// Commands are the eight commands the screen ends with, the short version of
// everything above them (Technical spec 7.2.3; FUN-12).
var Commands = []Command{
	{"What is installed and recorded", "rashomon status"},
	{"What each session recorded", "rashomon report"},
	{"Read this screen again", "rashomon watch --show"},
	{"Stop recording", "rashomon pause"},
	{"Start recording again", "rashomon resume"},
	{"Stop the notes to your agent", "rashomon steer off --user"},
	{"Remove the hooks", "rashomon detach"},
	{"Remove everything", "rashomon purge"},
}

// ScreenInput is everything the screen computes rather than reads from the
// inventory.
type ScreenInput struct {
	// Version is the binary's version.
	Version string
	// SettingsPath is the settings file the entries go into.
	SettingsPath string
	// Executable is the absolute path every entry runs.
	Executable string
	// InstallID is the install's id, or "" on a machine with no store yet.
	InstallID string
	// StoreRoot is where the record is kept, and StoreExists whether it is
	// already there.
	StoreRoot   string
	StoreExists bool
	// Prior is the latest accepted consent on file, or nil for none. When
	// its scope is below this build's, the screen opens with the CHANGED
	// SINCE block.
	Prior *Record
}

// Screen renders the consent screen: five sections, the eight commands, every
// line at most Width characters. The layer paragraphs are printed verbatim
// from the inventory's Paragraphs and the variants rendered from its
// templates; nothing else in it is free text about what rashomon does.
func Screen(inv disclose.Inventory, in ScreenInput) ([]byte, error) {
	paras := map[string][]disclose.Paragraph{}
	for _, p := range inv.Paragraphs {
		paras[p.Cite] = append(paras[p.Cite], p)
	}
	var w screenWriter

	w.wrap("", "", "rashomon "+in.Version+" asks before it changes anything. Nothing below is written until you type y.")
	w.blank()

	if in.Prior != nil && in.Prior.ScopeVersion < inv.ScopeVersion {
		tmpl, ok := first(paras[disclose.CiteChangedSince])
		if !ok {
			return nil, fmt.Errorf("consent: the inventory has no %s template", disclose.CiteChangedSince)
		}
		w.wrap("", "", fill(tmpl.Text, map[string]string{
			"version": in.Prior.BinaryVersion,
			"scope":   strconv.Itoa(in.Prior.ScopeVersion),
			"current": strconv.Itoa(inv.ScopeVersion),
		}))
		w.blank()
	}

	w.line("1. HOOK ENTRIES")
	id := in.InstallID
	if id == "" {
		id = NewInstallPlaceholder
	}
	w.wrap(indent, indent, fmt.Sprintf("%s (Claude Code reads it for every project here) gets %d entries of rashomon's own under \"hooks\". Other keys and other tools' entries stay byte for byte as they are. Each entry runs",
		in.SettingsPath, len(install.Events())))
	w.wrap(indent+"  ", indent+"  ", in.Executable+" <step>")
	w.wrap(indent+"    ", indent+"    ", install.Marker+" "+id)
	w.wrap(indent, indent+"    ", "on: "+strings.Join(install.Events(), " "))
	w.wrap(indent, indent, limits()+" Each can delay Claude by at most its limit; after that Claude Code carries on.")
	if err := w.paragraphs(paras[disclose.CiteSection1]); err != nil {
		return nil, err
	}
	w.blank()

	state := "new"
	if in.StoreExists {
		state = "already there"
	}
	w.wrap("", indent, fmt.Sprintf("2. WHAT IS RECORDED  in %s (%s; mode 0700: readable by you and by any program you run, including the agent)", in.StoreRoot, state))
	if err := w.paragraphs(paras[disclose.CiteSection2]); err != nil {
		return nil, err
	}
	w.blank()

	w.line("3. WHAT RASHOMON ADDS TO YOUR AGENT'S CONTEXT  (on)")
	if err := w.paragraphs(paras[disclose.CiteSection3]); err != nil {
		return nil, err
	}
	w.blank()

	w.line("4. LESSONS  (on this machine only)")
	if err := w.paragraphs(paras[disclose.CiteSection4]); err != nil {
		return nil, err
	}
	w.blank()

	w.line("5. WHAT YOU WILL SEE")
	if err := w.paragraphs(paras[disclose.CiteSection5]); err != nil {
		return nil, err
	}
	w.blank()
	for _, c := range Commands {
		w.line(indent + c.Does + strings.Repeat(" ", 34-utf8.RuneCountInString(c.Does)) + c.Run)
	}
	if w.err != nil {
		return nil, w.err
	}
	return w.buf.Bytes(), nil
}

// TextSHA256 is the text_sha256 of the bytes shown.
func TextSHA256(shown []byte) string {
	sum := sha256.Sum256(shown)
	return hex.EncodeToString(sum[:])
}

// limits is section 1's time-limit sentence, rendered from the entries'
// own timeouts: the common limit, and every event with another one.
func limits() string {
	byLimit := map[int][]string{}
	for _, e := range install.Events() {
		t := install.TimeoutFor(e)
		byLimit[t] = append(byLimit[t], e)
	}
	common, n := 0, -1
	for t, es := range byLimit {
		if len(es) > n || len(es) == n && t < common {
			common, n = t, len(es)
		}
	}
	others := make([]int, 0, len(byLimit))
	for t := range byLimit {
		if t != common {
			others = append(others, t)
		}
	}
	sort.Ints(others)
	s := fmt.Sprintf("Limit %d s each", common)
	if len(others) > 0 {
		var parts []string
		for _, t := range others {
			parts = append(parts, fmt.Sprintf("%d s for %s", t, strings.Join(byLimit[t], ", ")))
		}
		s += " (" + strings.Join(parts, "; ") + ")"
	}
	return s + "."
}

func first(ps []disclose.Paragraph) (disclose.Paragraph, bool) {
	if len(ps) == 0 {
		return disclose.Paragraph{}, false
	}
	return ps[0], true
}

// fill replaces each {slot} in a variant template.
func fill(tmpl string, slots map[string]string) string {
	for k, v := range slots {
		tmpl = strings.ReplaceAll(tmpl, "{"+k+"}", v)
	}
	return tmpl
}

// screenWriter builds the screen and records the first line that would pass
// Width, so a paragraph that cannot fit is an error rather than a wide line.
type screenWriter struct {
	buf bytes.Buffer
	err error
}

func (w *screenWriter) blank() { w.buf.WriteByte('\n') }

func (w *screenWriter) line(s string) {
	if n := utf8.RuneCountInString(s); n > Width && w.err == nil {
		w.err = fmt.Errorf("consent: screen line of %d characters passes %d: %q", n, Width, s)
	}
	w.buf.WriteString(s)
	w.buf.WriteByte('\n')
}

// paragraphs prints each paragraph's text verbatim: wrapped to the width at
// the section indent, or, when the text carries its own line feeds, line for
// line at that indent.
func (w *screenWriter) paragraphs(ps []disclose.Paragraph) error {
	for _, p := range ps {
		if strings.Contains(p.Text, "\n") {
			for _, l := range strings.Split(p.Text, "\n") {
				w.line(indent + l)
			}
			continue
		}
		w.wrap(indent, indent, p.Text)
	}
	return w.err
}

// wrap prints text word-wrapped to Width, its first line at firstPrefix and
// the rest at restPrefix. A word longer than the room left -- a long path --
// is cut at the width, never let past it.
func (w *screenWriter) wrap(firstPrefix, restPrefix, text string) {
	prefix := firstPrefix
	var cur []rune
	room := func() int { return Width - utf8.RuneCountInString(prefix) }
	flush := func() {
		w.line(prefix + string(cur))
		cur = cur[:0]
		prefix = restPrefix
	}
	for _, word := range strings.Fields(text) {
		r := []rune(word)
		if len(cur) > 0 && len(cur)+1+len(r) > room() {
			flush()
		}
		for len(cur) == 0 && len(r) > room() {
			n := room()
			cur = append(cur, r[:n]...)
			flush()
			r = r[n:]
		}
		if len(cur) > 0 {
			cur = append(cur, ' ')
		}
		cur = append(cur, r...)
	}
	if len(cur) > 0 {
		flush()
	}
}
