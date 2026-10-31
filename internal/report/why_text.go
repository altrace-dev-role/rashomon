package report

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"
)

// WhyText writes v as `rashomon why` prints it. The words are the line's own
// (its state, delivery and suppressed_by), with what they mean for who saw
// the note, and nothing is re-rendered: a suppressed line prints its slots
// and says its text was not kept.
func WhyText(w io.Writer, v *WhyView) error {
	var b bytes.Buffer
	fmt.Fprintf(&b, "rashomon why %s (session %s, line %s)\n", v.Prefix, v.SessionID, v.LineID)
	fmt.Fprintf(&b, "  what: %s, key %s\n", v.Title, printable(v.Key))
	fmt.Fprintf(&b, "  when: %s at %s, after #%d%s\n", time.UnixMilli(v.AtMS).UTC().Format("15:04:05"), v.Event, v.AfterSeq, promptClause(v.PromptID))
	fmt.Fprintf(&b, "  state: %s\n", stateClause(v))
	if v.TextKept {
		if v.UserText != "" {
			fmt.Fprintf(&b, "  shown to you: %s\n", oneLine(v.UserText))
		}
		if v.ModelText != "" {
			fmt.Fprintf(&b, "  sent to the agent: %s\n", oneLine(v.ModelText))
		}
		if v.Redacted {
			fmt.Fprintln(&b, "  text: dropped by --redact")
		}
	} else {
		fmt.Fprintf(&b, "  text not kept: %s; slots: %s\n", v.State+suppressedBy(v), slotsText(v.Slots))
	}
	if v.TextKept && len(v.Slots) > 0 {
		fmt.Fprintf(&b, "  slots: %s\n", slotsText(v.Slots))
	}
	writeWhyEvidence(&b, v)
	if v.Source.Kind != "none" {
		fmt.Fprintf(&b, "  source: %s\n", sourceText(v.Source))
	} else {
		fmt.Fprintln(&b, "  source: none recorded on the line")
	}
	if r := v.Resolution; r != nil {
		s := fmt.Sprintf("%s %s at the session's last checkpoint (#%d)", gateLabel(r.Gate), strings.ReplaceAll(r.State, "_", " "), r.PointSeq)
		if r.LastGreenSeq != nil {
			s += fmt.Sprintf(", last green #%d", *r.LastGreenSeq)
		} else {
			s += ", no green run recorded"
		}
		fmt.Fprintf(&b, "  resolution: %s\n", s)
	}
	_, err := w.Write(b.Bytes())
	return err
}

func promptClause(id string) string {
	if id == "" {
		return ", no prompt"
	}
	return ", prompt " + printable(id)
}

// stateClause is the state with what it means for who saw the note.
func stateClause(v *WhyView) string {
	s := v.State
	if v.Delivery != "" {
		s += ", delivery " + v.Delivery
	}
	if v.SuppressedBy != "" {
		s += ", suppressed_by " + v.SuppressedBy
	}
	switch {
	case v.Delivery == "flag_only":
		s += ": shown to you, not sent to the agent"
	case v.Delivery == "delivered" && v.State == "delivered":
		s += ": sent to the agent and shown to you"
	case v.Delivery == "holdout":
		s += ": held out, shown to neither"
	case v.State == "suppressed":
		s += ": not shown"
	}
	return s
}

func suppressedBy(v *WhyView) string {
	if v.SuppressedBy == "" {
		return ""
	}
	return " (" + v.SuppressedBy + ")"
}

func slotsText(ss []WhySlot) string {
	if len(ss) == 0 {
		return "none"
	}
	parts := make([]string, len(ss))
	for i, s := range ss {
		if s.Name != "path" && !WhySlotShown(s.Name) {
			n := s.Bytes
			if n == 0 {
				n = len(s.Value)
			}
			parts[i] = fmt.Sprintf("%s=<%d bytes>", s.Name, n)
			continue
		}
		parts[i] = s.Name + "=" + printable(s.Value)
	}
	return strings.Join(parts, ", ")
}

func sourceText(s WhySource) string {
	switch s.Kind {
	case "gate":
		return "gate " + printable(s.Detail) + " (slot gate)"
	case "rules":
		return "rules " + printable(s.Detail) + " (slot rules)"
	case "lesson":
		return "lesson " + printable(s.Detail) + " (the line's key)"
	case "ledger_unchecked":
		return "the lesson ledger did not check"
	}
	return s.Kind
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(printable(s)), " ")
}

func writeWhyEvidence(b *bytes.Buffer, v *WhyView) {
	if len(v.Evidence) == 0 {
		fmt.Fprintln(b, "  evidence: none cited")
		return
	}
	fmt.Fprintln(b, "  evidence:")
	for _, c := range v.Evidence {
		writeTimelineCall(b, c)
	}
}
