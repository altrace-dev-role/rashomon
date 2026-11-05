package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// An architecture decision record directory holds one Markdown file per
// record named NNNN-title.md: a "# N. Title" heading, then "## Status",
// "## Context", "## Decision" and "## Consequences" sections. Any other
// file in the directory is not a record.

// ParseADR is the records of dir as decisions keyed ADR-NNNN: the heading
// as the title, the Decision section (the first section without one) as
// the text and its first sentence as the clause; an accepted record is
// approved and DECLARED, a deprecated one retired, a superseded one
// superseded, any other a candidate. The count of unparsed files is zero.
func ParseADR(dir string) ([]Row, int, error) {
	es, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, err
	}
	var rows []Row
	for _, e := range es {
		num, ok := adrNumber(e.Name())
		if !ok || e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, 0, err
		}
		r := adrRow(filepath.Base(dir)+"/"+e.Name(), "ADR-"+num, string(b))
		sum := sha256.Sum256(b)
		r.Source.Digest = hex.EncodeToString(sum[:])
		rows = append(rows, r)
	}
	return rows, 0, nil
}

// adrNumber is the digits of NNNN-title.md.
func adrNumber(name string) (string, bool) {
	rest, ok := strings.CutSuffix(name, ".md")
	if !ok {
		return "", false
	}
	n := 0
	for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
		n++
	}
	if n < 3 || n >= len(rest) || rest[n] != '-' {
		return "", false
	}
	return rest[:n], true
}

func adrRow(file, id, text string) Row {
	title, sections := adrSections(text)
	body := sections["decision"]
	if body == "" {
		body = sections[""]
	}
	r := Row{ID: id, Kind: KindDecision, Title: title, Clause: firstSentence(body), Text: body, Source: Source{File: file, Row: id}, Status: StatusCandidate, Tier: TierInferred}
	switch strings.ToLower(strings.Join(strings.Fields(sections["status"])[:min(1, len(strings.Fields(sections["status"])))], "")) {
	case "accepted":
		r.Status, r.Tier = StatusApproved, TierDeclared
	case "deprecated":
		r.Status, r.Tier = StatusRetired, TierDeclared
	case "superseded":
		r.Status, r.Tier = StatusSuperseded, TierDeclared
	}
	return r
}

// adrSections is the record's title (its first heading, the leading
// number dropped) and each "## " section's lines joined by a space, keyed
// by the lower-cased heading; lines before any section key "".
func adrSections(text string) (string, map[string]string) {
	title, cur := "", ""
	sections := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "":
		case strings.HasPrefix(t, "## "):
			cur = strings.ToLower(strings.TrimSpace(t[3:]))
		case strings.HasPrefix(t, "# ") && title == "":
			title = strings.TrimLeft(strings.TrimSpace(t[2:]), "0123456789.")
			title = strings.TrimSpace(title)
		default:
			if sections[cur] != "" {
				sections[cur] += " "
			}
			sections[cur] += t
		}
	}
	return title, sections
}
