package history

import "encoding/json"

// The text helpers of the reader: what a result's text parts are, the head and
// tail kept of them, and whether an input asks for a background run.

// textParts are the texts of a tool_result's content, which is a plain string
// in some results and an array of blocks in others.
func textParts(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return []string{plain}
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	var out []string
	for _, b := range blocks {
		if b.Type == "text" {
			out = append(out, b.Text)
		}
	}
	return out
}

// headTail returns the first h and last t bytes of the parts joined with
// newlines, without building the join: a result can be as long as a
// transcript line. When the whole text fits in h, the tail is empty.
func headTail(parts []string, h, t int) (head, tail []byte) {
	total := 0
	for i, p := range parts {
		if i > 0 {
			total++
		}
		total += len(p)
	}
	head = make([]byte, 0, min(h, total))
	for i, p := range parts {
		if i > 0 {
			head = appendCapped(head, "\n", h)
		}
		head = appendCapped(head, p, h)
		if len(head) == h {
			break
		}
	}
	if total <= h {
		return head, nil
	}
	want := min(t, total-h)
	tail = make([]byte, want)
	at := want
	for i := len(parts) - 1; i >= 0 && at > 0; i-- {
		p := parts[i]
		n := min(len(p), at)
		at -= n
		copy(tail[at:], p[len(p)-n:])
		if i > 0 && at > 0 {
			at--
			tail[at] = '\n'
		}
	}
	return head, tail
}

func appendCapped(b []byte, s string, max int) []byte {
	return append(b, s[:min(len(s), max-len(b))]...)
}

// runInBackground reports whether a Bash input asks for a background run.
func runInBackground(input json.RawMessage) bool {
	var in struct {
		RunInBackground bool `json:"run_in_background"`
	}
	return json.Unmarshal(input, &in) == nil && in.RunInBackground
}
