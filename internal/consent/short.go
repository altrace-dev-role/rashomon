package consent

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// MaxShortLines is the short screen's ceiling: the rows of a default 80x24
// terminal, so a person sees what a yes records, what it never does and how
// to turn it off without scrolling (CONSENT-SHORT1). TestCSH1 holds it for a
// home directory of up to 40 characters, with the CHANGED SINCE line.
const MaxShortLines = 24

// ShortCommands are the short screen's closing lines: how to turn each part
// off, and where every detail is.
var ShortCommands = []Command{
	{"Stop the notes to your agent", "rashomon steer off --user"},
	{"Stop recording", "rashomon pause"},
	{"Remove the hooks", "rashomon detach"},
	{"Remove everything", "rashomon purge"},
	{"Every detail, section by section", "rashomon consent --full"},
}

// Short renders the screen watch and consent print and ask under
// (CONSENT-SHORT1): the hook entries and where the record is kept, then the
// inventory's CiteShort paragraphs -- what a yes records, what is never
// stored, the notes -- and the off switches, every line at most Width. The
// five sections it sums up are Screen's, which --full prints; the record's
// binary_sha256 names the build whose --full printed them.
func Short(inv disclose.Inventory, in ScreenInput) ([]byte, error) {
	paras := map[string][]disclose.Paragraph{}
	for _, p := range inv.Paragraphs {
		paras[p.Cite] = append(paras[p.Cite], p)
	}
	if len(paras[disclose.CiteShort]) == 0 {
		return nil, fmt.Errorf("consent: the inventory has no %s paragraphs", disclose.CiteShort)
	}
	var w screenWriter

	w.wrap("", "", "rashomon "+in.Version+" changes nothing until you type y.")
	if in.Prior != nil && in.Prior.ScopeVersion < inv.ScopeVersion {
		tmpl, ok := first(paras[disclose.CiteChangedSinceShort])
		if !ok {
			return nil, fmt.Errorf("consent: the inventory has no %s template", disclose.CiteChangedSinceShort)
		}
		w.wrap("", "", fill(tmpl.Text, map[string]string{
			"version": in.Prior.BinaryVersion,
			"scope":   strconv.Itoa(in.Prior.ScopeVersion),
			"current": strconv.Itoa(inv.ScopeVersion),
		}))
	}
	w.blank()
	w.wrap("", "", fmt.Sprintf("With a yes, %s holds %d hook entries of rashomon's and %s (mode 0700) holds the record, on this machine only.",
		in.SettingsPath, len(install.Events()), in.StoreRoot))
	w.blank()
	for _, p := range paras[disclose.CiteShort] {
		w.wrap("", indent, p.Text)
	}
	w.blank()
	for _, c := range ShortCommands {
		w.line(indent + c.Does + strings.Repeat(" ", 34-utf8.RuneCountInString(c.Does)) + c.Run)
	}
	if w.err != nil {
		return nil, w.err
	}
	return w.buf.Bytes(), nil
}

// Covers reports whether recs cover this build: a scope_version at least
// disclose.ScopeVersion and a steer_disclosure at least
// steer.DisclosureVersion, the two facts ScopeCovered and Current read from
// the file, read here from records already in hand.
func Covers(recs []Record) bool {
	scope, ok := maxBy(recs, func(r Record) int { return r.ScopeVersion })
	disclosure, _ := maxBy(recs, func(r Record) int { return r.SteerDisclosure })
	return ok && scope >= disclose.ScopeVersion && disclosure >= steer.DisclosureVersion
}
