package harness

import "strings"

// A message from another session as the hook receives it, measured on
// Claude Code 2.1.322, idle and queued alike: the bare element, its opening
// tag alone on the first line with the sender's from= key, the body, and its
// closing tag alone on the last line. The lead line and the closing
// paragraph a transcript shows around it are the host's display and never
// reach the hook.
const (
	peerTag   = "cross-session-message"
	peerFrom  = ` from="`
	peerClose = "</" + peerTag + ">"
)

// Cut reads off a prompt the frames the host wraps around text it injects
// (a background job's notification, a message from another session, a
// harness notice), and returns the text left and whether a frame was read.
// A prompt with no frame is returned as it is. cut is whether the prompt is
// only the head the hook's reader took (intent.MaxPromptBytes).
//
// A frame is read by its envelope alone, as the text inside one is free
// text that can quote code, a tag or another frame. A notice is a line that
// opens with a bracketed label ending " notice", through the line that
// closes a «summary» it opens. A notification is an element whose tag ends
// "-notification", from its opening tag through its closing one, text after
// the closing tag on its line kept. A peer message is the whole prompt,
// outer white space trimmed, as one element: its first line the opening tag
// alone with the from= key, its last line the closing tag alone, and no
// other line the closing tag alone; its body is never read for a frame.
// Each reading fails safe: an element never closed, or a peer element with
// text before or after it, is no envelope and is read as typed; a summary
// never closed takes only the label's line. The one exception is a cut
// prompt: its head is held as a peer message when its first line is the
// opening tag alone with the from= key and no line of it is the closing tag
// alone, though the close was never read, since the reader cut it off.
//
// A peer message is held here, nothing typed left of it, and not in Framed
// alone: the graph brief reads Cut and nothing else (engine.Build), and a
// peer message builds no task-preflight finding of any key at its own
// prompt, the graph brief's included. Framed follows Cut.
func Cut(prompt string, cut bool) (typed string, held bool) {
	if peerElement(prompt, cut) {
		return "", true
	}
	if !mayFrame(prompt) {
		return prompt, false
	}
	lines := strings.Split(prompt, "\n")
	var keep []string
	for i := 0; i < len(lines); i++ {
		s := strings.TrimSpace(lines[i])
		if noticeLabel(s) {
			held, i = true, summaryEnd(lines, i)
			continue
		}
		if tag, ok := notificationTag(s); ok {
			if end, after, ok := closing(lines, i, tag); ok {
				held, i, keep = true, end, keepText(keep, after)
				continue
			}
		}
		keep = append(keep, lines[i])
	}
	if !held {
		return prompt, false
	}
	return strings.Join(keep, "\n"), true
}

// Framed is whether a prompt is frames only, nothing typed left once they
// are cut, and the text left. It follows Cut, which holds a message from
// another session whole, so that what reads Cut alone builds nothing from
// it either (Cut says why).
func Framed(prompt string, cut bool) (framed bool, typed string) {
	typed, held := Cut(prompt, cut)
	return held && strings.TrimSpace(typed) == "", typed
}

// mayFrame is whether prompt holds what a notice or a notification needs: a
// notice's opening bracket or a notification's tag end. A prompt without
// either is typed, read without a split.
func mayFrame(prompt string) bool {
	return strings.Contains(prompt, "[") || strings.Contains(prompt, "-notification>")
}

// peerElement is whether the whole prompt, outer white space trimmed, is
// one message from another session: its first line the opening tag alone
// with the from= key; then, read whole, exactly one line the closing tag
// alone, and that line the last; read cut, no line the closing tag alone,
// the close being past the head. A body line is compared trimmed.
func peerElement(prompt string, cut bool) bool {
	first, body, ok := strings.Cut(strings.TrimSpace(prompt), "\n")
	if !ok || !peerOpen(first) || !strings.Contains(first, peerFrom) {
		return false
	}
	closes, last := 0, ""
	for l := range strings.SplitSeq(body, "\n") {
		if last = strings.TrimSpace(l); last == peerClose {
			closes++
		}
	}
	if cut {
		return closes == 0
	}
	return closes == 1 && last == peerClose
}

// peerOpen is whether s is the peer element's opening tag, with or without
// attributes.
func peerOpen(s string) bool {
	rest, ok := strings.CutPrefix(s, "<"+peerTag)
	return ok && strings.HasSuffix(rest, ">") && (rest[0] == ' ' || rest[0] == '>')
}

// noticeLabel is whether line s opens with a bracketed label ending
// " notice" ("[Cross-session idle notice]").
func noticeLabel(s string) bool {
	if !strings.HasPrefix(s, "[") {
		return false
	}
	label, _, ok := strings.Cut(s[1:], "]")
	return ok && strings.HasSuffix(strings.ToLower(label), " notice")
}

// summaryEnd is the last line of the notice that opens at line i: the
// line that closes the «summary» the label's line opens and leaves open,
// else the label's line itself.
func summaryEnd(lines []string, i int) int {
	_, open, ok := strings.Cut(lines[i], "«")
	if !ok || strings.Contains(open, "»") {
		return i
	}
	for j := i + 1; j < len(lines); j++ {
		if strings.Contains(lines[j], "»") {
			return j
		}
	}
	return i
}

// notificationTag is the tag line s opens with when it is one word ending
// "-notification" ("<task-notification>"). A closing tag is none, as no
// line closes "/task-notification".
func notificationTag(s string) (string, bool) {
	if !strings.HasPrefix(s, "<") {
		return "", false
	}
	tag, _, ok := strings.Cut(s[1:], ">")
	return tag, ok && strings.HasSuffix(tag, "-notification") && !strings.ContainsAny(tag, " \t")
}

// closing is the line from i on that closes tag and what follows the
// closing tag there; false when no line does.
func closing(lines []string, i int, tag string) (int, string, bool) {
	for j := i; j < len(lines); j++ {
		if _, after, ok := strings.Cut(lines[j], "</"+tag+">"); ok {
			return j, after, true
		}
	}
	return 0, "", false
}

// keepText is keep with s appended when s holds more than space.
func keepText(keep []string, s string) []string {
	if s = strings.TrimSpace(s); s != "" {
		return append(keep, s)
	}
	return keep
}
