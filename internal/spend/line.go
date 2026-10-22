package spend

// This file is the transcript line as spend decodes it -- the whole usage
// read path's shape -- and the rules that read a decoded line's usage. The
// scan over a file is scan.go's.

import "strings"

// The decoded shape of one transcript line. THIS IS THE WHOLE USAGE READ
// PATH -- every total, breakdown and heuristic in the document comes from it
// -- and it is stated as narrowly as the recorder's own rules are:
// message.id, message.model, message.stop_reason, message.stop_details'
// category, message.usage's token
// counts and speed, each usage.iterations entry's counts, type and model, and
// the line's timestamp, sessionId, isSidechain and requestId (a closed shape,
// requestIDShaped) -- and, for a
// subagent transcript's user lines, type, isMeta and promptId, the key that
// ties the responses after them to a turn. There is no
// field for message.content -- or for anything else -- so encoding/json skips
// those bytes without materialising them: on this path the text of a
// conversation is never a value in this process, not even briefly, which is a
// stronger property than "read and then discarded".
// TestContentHasNoFieldToLandIn holds this shape.
//
// The one other read is the silent-failure line's, and it is not this one:
// for a recorded turn with a failed call, Join has report.FinalAssistantTexts
// decode, in memory, the text blocks of every assistant line tied to that
// turn -- no other block's contents -- and keep only the last, to take the
// digest's verdict. That is message content, read under report's render-time
// rule -- reduced to the verdict, never written or output -- and it departs
// from the design's "never message.content", which did not account for this
// line needing the summary it is judged against. The same pass reads the
// message ids of the turn's responses (its spend) and, on a user line with no
// promptId, the content block TYPES only, never their text.
//
// cwd, which the design lists for a project name, is deliberately not read:
// nothing here renders a project, and a directory name is a path, which the
// privacy rule keeps out of every output this program has.
type line struct {
	Type        string  `json:"type"`
	Timestamp   string  `json:"timestamp"`
	SessionID   string  `json:"sessionId"`
	IsSidechain bool    `json:"isSidechain"`
	IsMeta      bool    `json:"isMeta"`
	PromptID    string  `json:"promptId"`
	RequestID   string  `json:"requestId"`
	Message     message `json:"message"`
}

// requestIDShaped reports whether a line's requestId has the closed shape an
// API request id has: "req_" and then letters and digits, bounded. Anything
// else is no request id, and matches nothing (foldRefusalMessages).
func requestIDShaped(s string) bool {
	rest, ok := strings.CutPrefix(s, "req_")
	if !ok || rest == "" || len(rest) > 128 {
		return false
	}
	for _, c := range rest {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

type message struct {
	ID          string       `json:"id"`
	Model       string       `json:"model"`
	StopReason  *string      `json:"stop_reason"`
	StopDetails *stopDetails `json:"stop_details"`
	Usage       *usage       `json:"usage"`
}

// stopDetails is a refusal's stop_details: its category alone, read as a
// closed word (refusalCategory). The explanation beside it is prose and has
// no field to land in.
type stopDetails struct {
	Category *string `json:"category"`
}

// Refusal categories, as refusalCategory reads them: the five the
// refusals-and-fallback page names, "uncategorized" for a null (or absent)
// category -- which the page calls "a normal, permanent value" -- and
// "other" for anything else.
const (
	CategoryCyber               = "cyber"
	CategoryBio                 = "bio"
	CategoryFrontierLLM         = "frontier_llm"
	CategoryReasoningExtraction = "reasoning_extraction"
	CategoryGeneralHarms        = "general_harms"
	CategoryUncategorized       = "uncategorized"
	CategoryOther               = "other"
)

// refusalCategory is a refusal's category as a closed word.
func refusalCategory(d *stopDetails) string {
	if d == nil || d.Category == nil {
		return CategoryUncategorized
	}
	switch c := *d.Category; c {
	case CategoryCyber, CategoryBio, CategoryFrontierLLM, CategoryReasoningExtraction, CategoryGeneralHarms:
		return c
	}
	return CategoryOther
}

// tokens is the counting part of a usage object, shared by the top level and
// by each entry of iterations[].
type tokens struct {
	Input         int64          `json:"input_tokens"`
	Output        int64          `json:"output_tokens"`
	CacheRead     int64          `json:"cache_read_input_tokens"`
	CacheCreation int64          `json:"cache_creation_input_tokens"`
	CacheSplit    *cacheCreation `json:"cache_creation"`
}

type cacheCreation struct {
	Ephemeral5m int64 `json:"ephemeral_5m_input_tokens"`
	Ephemeral1h int64 `json:"ephemeral_1h_input_tokens"`
}

type usage struct {
	tokens
	// Speed is "fast" on a fast-mode response, which bills at a premium the
	// table does not hold (FastMode): a closed word, counted, never printed.
	Speed string `json:"speed"`
	// Iterations is one entry per attempt. The API documents it as the per-
	// attempt source of truth, with the top-level counts covering only the
	// attempt that produced the returned message.
	Iterations []iteration `json:"iterations"`
}

// iteration is one usage.iterations entry: an attempt's counts, its kind and
// the model that ran it. The refusals-and-fallback page documents both
// fields on every entry.
type iteration struct {
	tokens
	// Type is "message" for an attempt by the model asked, or by a hop that
	// declined, and "fallback_message" for the fallback model that served.
	// Compared with iterFallback, never printed.
	Type string `json:"type"`
	// Model is the model that ran the attempt. Of the last entry, it prices
	// the response (route), and it is printed only through displayModel's
	// closed-shape rule, as message.model is.
	Model string `json:"model"`
}

// iterFallback is the type of the entry a fallback model served.
const iterFallback = "fallback_message"

// maxTokens bounds one count of one response. The largest context window a
// model in the table has is a few million tokens, so a count past a hundred
// million is not a response's; and the bound keeps every price this package
// computes (tokens x nanodollars per token, summed) far inside an int64.
const maxTokens = 100_000_000

// plausible reports whether every count in a usage -- top level, TTL split
// and each iteration -- is at least zero and at most maxTokens.
func (u *usage) plausible() bool {
	ok := func(t tokens) bool {
		n := []int64{t.Input, t.Output, t.CacheRead, t.CacheCreation}
		if t.CacheSplit != nil {
			n = append(n, t.CacheSplit.Ephemeral5m, t.CacheSplit.Ephemeral1h)
		}
		for _, v := range n {
			if v < 0 || v > maxTokens {
				return false
			}
		}
		return true
	}
	if !ok(u.tokens) {
		return false
	}
	for _, it := range u.Iterations {
		if !ok(it.tokens) {
			return false
		}
	}
	return true
}

// carriesTokens reports whether any count in the usage, top level or an
// iteration's, is non-zero -- or is not a count at all (plausible).
func (u *usage) carriesTokens() bool {
	if !u.plausible() {
		return true
	}
	if u.split().Total() > 0 {
		return true
	}
	for _, it := range u.Iterations {
		if it.split().Total() > 0 {
			return true
		}
	}
	return false
}

// Tokens is one response's token counts by kind, with the cache writes
// split by TTL because the two TTLs are priced differently.
type Tokens struct {
	Input        int64 `json:"input"`
	Output       int64 `json:"output"`
	CacheRead    int64 `json:"cache_read"`
	CacheWrite5m int64 `json:"cache_write_5m"`
	CacheWrite1h int64 `json:"cache_write_1h"`
}

// Total is every token of every kind.
func (t Tokens) Total() int64 {
	return t.Input + t.Output + t.CacheRead + t.CacheWrite5m + t.CacheWrite1h
}

func (t *Tokens) add(o Tokens) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheRead += o.CacheRead
	t.CacheWrite5m += o.CacheWrite5m
	t.CacheWrite1h += o.CacheWrite1h
}

// split turns a usage object's counts into Tokens.
//
// cache_creation_input_tokens is the total write; cache_creation carries its
// TTL split. Whatever part of the total the split does not account for --
// all of it, on a response that carries no split -- is priced at the 5-minute
// rate, because 5 minutes is the API's TTL when a request names none. That is
// the API's own default, not a guess between two numbers, and it is also the
// lower of the two rates, so an unsplit write can only be under-, never over-
// stated.
func (t tokens) split() Tokens {
	out := Tokens{Input: t.Input, Output: t.Output, CacheRead: t.CacheRead}
	if t.CacheSplit != nil {
		out.CacheWrite5m = t.CacheSplit.Ephemeral5m
		out.CacheWrite1h = t.CacheSplit.Ephemeral1h
	}
	if rest := t.CacheCreation - out.CacheWrite5m - out.CacheWrite1h; rest > 0 {
		out.CacheWrite5m += rest
	}
	return out
}
