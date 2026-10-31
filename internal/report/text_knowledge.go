package report

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// knowledgeWidth is the column a knowledge line wraps at. The report has no
// 80-column rule (listWidth is 200); these lines are read, not pasted, and
// wrap at about this many columns with their continuation indented.
const knowledgeWidth = 100

// writeKnowledge is the knowledge block, after the autosteer block: nothing
// when the report did not read it. Every item names its store, the
// checkpoint seq it rests on, or the baseline.
func writeKnowledge(b *bytes.Buffer, k *Knowledge) {
	if k == nil {
		return
	}
	fmt.Fprintf(b, "  knowledge (project %s)\n", k.Project)
	switch {
	case k.Baseline == nil && k.BaselineWalkCap > 0:
		wrapKnowledge(b, fmt.Sprintf("baseline: not found in the first %d session directories walked", k.BaselineWalkCap))
	case k.Baseline == nil:
		wrapKnowledge(b, "baseline: no earlier checkpoint of this project")
	case k.BaselineWalkCap > 0:
		wrapKnowledge(b, fmt.Sprintf("baseline: session %s, last checkpoint %s at #%d (the newest of the first %d session directories walked; a newer one may exist)",
			k.Baseline.SessionID, knowledgeTime(k.Baseline.RecordedAtUnixMS), k.Baseline.PointSeq, k.BaselineWalkCap))
	default:
		wrapKnowledge(b, fmt.Sprintf("baseline: session %s, last checkpoint %s at #%d (the project's newest before this session)",
			k.Baseline.SessionID, knowledgeTime(k.Baseline.RecordedAtUnixMS), k.Baseline.PointSeq))
	}
	wrapKnowledge(b, "retained: "+retainedText(k.Retained))
	wrapKnowledge(b, "discovered this session: "+changesText(k.Discovered))
	wrapKnowledge(b, "invalidated this session: "+changesText(k.Invalidated))
	wrapKnowledge(b, "gate states at the Stops: "+gateStatesText(k))
	wrapKnowledge(b, "project facts: "+k.Retained.Facts)
}

// wrapKnowledge writes s as one knowledge item: indented four columns, a
// continuation six, broken at spaces before knowledgeWidth.
func wrapKnowledge(b *bytes.Buffer, s string) {
	line, indent := "    ", "    "
	for _, w := range strings.Fields(s) {
		if len(line) > len(indent) && len(line)+1+len(w) > knowledgeWidth {
			b.WriteString(line + "\n")
			line, indent = "      ", "      "
		}
		if len(line) > len(indent) {
			line += " "
		}
		line += w
	}
	b.WriteString(line + "\n")
}

// knowledgeTime is a time as the report prints one in a line: UTC, month,
// day and minute.
func knowledgeTime(ms int64) string {
	return time.UnixMilli(ms).UTC().Format("01-02 15:04")
}

// shortDigest is a "sha256:<hex>" digest cut to twelve hex characters, the
// form the card prints; anything else is kept whole.
func shortDigest(d string) string {
	if h, ok := strings.CutPrefix(d, "sha256:"); ok && len(h) > 12 {
		return "sha256:" + h[:12]
	}
	return d
}

// retainedText is what the project holds now, one clause per store.
func retainedText(r Retained) string {
	var parts []string
	if r.Prior {
		parts = append(parts, "no family established; every gate required (index)")
	} else {
		parts = append(parts, fmt.Sprintf("%d gate famil%s required (index)", r.RequiredFamilies, map[bool]string{true: "y", false: "ies"}[r.RequiredFamilies == 1]))
	}
	switch {
	case r.BindingsUnsigned:
		parts = append(parts, "spec-trust.json does not carry this store's MAC, so no binding is read")
	case len(r.Specs) == 0:
		parts = append(parts, "no spec bound in spec-trust.json")
	}
	for _, s := range r.Specs {
		parts = append(parts, fmt.Sprintf("spec %s bound in spec-trust.json (file %s, bound %s)", s.Path, shortDigest(s.FileDigest), knowledgeTime(s.BoundAtMS)))
	}
	if st := r.SpecAtLastStop; st != nil {
		s := "read " + st.Trust + " at the last Stop"
		if st.Compiled != "" {
			s += " (compiled " + shortDigest(st.Compiled) + ")"
		}
		parts = append(parts, s)
	}
	switch {
	case r.LedgerUnchecked:
		parts = append(parts, "the lesson ledger did not check, so every lesson reads proposed (learn)")
	case len(r.Lessons) == 0:
		parts = append(parts, "no lessons approved (learn)")
	default:
		ls := make([]string, len(r.Lessons))
		for i, l := range r.Lessons {
			ls[i] = l.ID + " " + l.Tier
		}
		parts = append(parts, fmt.Sprintf("%d lesson%s approved: %s (learn)", len(r.Lessons), plural(len(r.Lessons)), strings.Join(ls, ", ")))
	}
	return strings.Join(parts, "; ")
}

// changesText is each change with its source, or none.
func changesText(cs []Change) string {
	if len(cs) == 0 {
		return "none"
	}
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = c.What + " (" + c.Source + ")"
	}
	return strings.Join(parts, "; ")
}

// gateStatesText is each gate's changes and its last transition, or the one
// count line when the fold cut gates.
func gateStatesText(k *Knowledge) string {
	if k.GatesCut > 0 {
		return fmt.Sprintf("not compared; a checkpoint cut up to %d gate%s (gates_omitted)", k.GatesCut, plural(k.GatesCut))
	}
	if len(k.GateStates) == 0 {
		return "none changed"
	}
	parts := make([]string, len(k.GateStates))
	for i, g := range k.GateStates {
		parts[i] = fmt.Sprintf("%s %d change%s, last %s to %s at #%s", gateLabel(g.Gate), g.Changes, plural(g.Changes),
			strings.ReplaceAll(g.LastFrom, "_", " "), strings.ReplaceAll(g.LastTo, "_", " "), strconv.FormatInt(g.LastSeq, 10))
	}
	return strings.Join(parts, "; ")
}

// gateLabel is a gate's name as the text prints it: a learned gate's key is
// its family, "learned:<kind>:<hash>@<hash>", and the hashes depend on where
// the project sits on this machine, so the text names the kind (the JSON
// keeps the key whole).
func gateLabel(gate string) string {
	if rest, ok := strings.CutPrefix(gate, "learned:"); ok {
		kind, _, _ := strings.Cut(rest, ":")
		return "learned:" + kind
	}
	return gate
}
