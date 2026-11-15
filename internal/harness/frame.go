package harness

import "strings"

// The peer frame, as the host lays out a message from another session: the
// lead line, the element's opening tag on the next line, the body, the
// closing tag alone on its line, an empty line, then the closing paragraph
// on one line.
const (
	peerLead    = "Another Claude session sent a message:"
	peerTag     = "cross-session-message"
	peerClose   = "</" + peerTag + ">"
	peerTrailer = "This came from another Claude session"
)

// Cut reads off a prompt the frames the host wraps around text it injects
// (a background job's notification, a message from another session, a
// harness notice), and returns the text left and whether a frame was read.
// A prompt with no frame is returned as it is.
//
// A frame is read by its envelope alone, as the text inside one is free
// text that can quote code, a tag or another frame. A notice is a line that
// opens with a bracketed label ending " notice", through the line that
// closes a «summary» it opens. A notification is an element whose tag ends
// "-notification", from its opening tag through its closing one, text after
// the closing tag on its line kept. A peer message is the lead line, the
// opening tag on the next line, and the first closing tag alone on a line
// that is followed by an empty line and the closing paragraph; its body is
// never read for a frame. Each reading fails safe: an element never closed,
// a peer element missing its lead line or its closing paragraph, is no
// envelope and is read as typed; a summary never closed takes only the
// label's line.
func Cut(prompt string) (typed string, held bool) {
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
		if end, ok := peerFrame(lines, i); ok {
			held, i = true, end
			continue
		}
		keep = append(keep, lines[i])
	}
	if !held {
		return prompt, false
	}
	return strings.Join(keep, "\n"), true
}

// Framed is whether a prompt is frames only, nothing typed left once they
// are cut, and the text left.
func Framed(prompt string) (framed bool, typed string) {
	typed, held := Cut(prompt)
	return held && strings.TrimSpace(typed) == "", typed
}

// mayFrame is whether prompt holds what every frame needs: a notice's
// opening bracket, a notification's tag end or the peer frame's lead line.
// A prompt without any of them is typed, read without a split.
func mayFrame(prompt string) bool {
	return strings.Contains(prompt, "[") || strings.Contains(prompt, "-notification>") || strings.Contains(prompt, peerLead)
}

// peerFrame is the last line of the peer frame whose lead line is line i:
// the closing paragraph's line. False when line i is not the lead line, the
// next line is not the opening tag, or no closing tag alone on its line is
// followed by an empty line and the closing paragraph.
func peerFrame(lines []string, i int) (int, bool) {
	if strings.TrimSpace(lines[i]) != peerLead || i+1 >= len(lines) || !peerOpen(strings.TrimSpace(lines[i+1])) {
		return 0, false
	}
	for j := i + 2; j+2 < len(lines); j++ {
		if lines[j] == peerClose && lines[j+1] == "" && strings.HasPrefix(lines[j+2], peerTrailer) {
			return j + 2, true
		}
	}
	return 0, false
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
