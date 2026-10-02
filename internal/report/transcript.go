package report

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// maxLine bounds one transcript line. Transcript lines carry whole messages
// and can be large; the bound is against a corrupted file, not a real one.
const maxLine = 64 << 20

// deniedPrefix is the text Claude Code writes when the USER refuses a tool
// call, and the only thing that separates a denial from an ordinary failure.
//
// There is no structured signal. A denial and a failed command both arrive as a
// tool_result with is_error true; the difference is this sentence, and it is
// matched as a PREFIX because the three variants seen in real transcripts share
// only their opening. Measured across 1,858 transcripts: 14 denials, three
// endings -- "STOP what you are doing...", the same plus a memory note, and
// "To tell you how to proceed, the user said: ...".
//
// Anchored at the start, and never matched on its own. A command whose output
// happens to contain this sentence RAN, and calling that a denial would hide a
// real execution behind a user decision. That is not hypothetical: the search
// that established this prefix printed it, and its own output came back as a
// tool_result in the same session.
const deniedPrefix = "The user doesn't want to proceed with this tool use. The tool use was rejected"

// TranscriptIDs reads the distinct tool_use ids from a session transcript and
// from its subagent transcripts, which live at <session>/subagents/agent-*.jsonl
// where <session> is the transcript path without its extension, the distinct
// ids of the tool_result blocks answering them, and which of those results are
// the user having DENIED the call.
//
// The sets are kept apart because they mean different things: a tool_use block
// is a call the model asked for, a tool_result block is that call having
// finished, and a denial is neither -- the call never ran, and the user is why.
//
// DENIALS ARE NOT RESULTS. A denial does produce a tool_result block, so
// counting it as one put every denied call into executed-but-unrecorded, a list
// that exists to surface the recorder having failed to fire. The user
// exercising the permission prompt then read as the tool being broken.
//
// It reads five fields from each content block -- type, id, tool_use_id,
// is_error and content -- and nothing else. Content is read to classify the
// result and is never retained. A line that does not parse is skipped, not
// fatal: the transcript is Claude Code's file and its shape is not this
// program's to enforce.
func TranscriptIDs(path string) (ids, results, denied map[string]bool, files int, err error) {
	ids, results, denied = map[string]bool{}, map[string]bool{}, map[string]bool{}

	n, err := collectIDs(path, ids, results, denied)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	files += n

	matches, err := subagentTranscripts(filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents"))
	if err != nil {
		return nil, nil, nil, 0, err
	}
	for _, m := range matches {
		n, err := collectIDs(m, ids, results, denied)
		if err != nil {
			return nil, nil, nil, 0, err
		}
		files += n
	}
	return ids, results, denied, files, nil
}

// subagentTranscripts lists every agent-*.jsonl under dir, at any depth.
//
// A plain subagent writes <session>/subagents/agent-*.jsonl, but a workflow's
// subagents write <session>/subagents/workflows/<run>/agent-*.jsonl, two levels
// further down. A one-level glob read 35 of a real session's 407 transcripts,
// and every call the other 372 made then read as recorded-but-not-in-the-
// transcript -- a gap that was this function's, not the recorder's. A missing
// directory is the common case, a session with no subagents, and not an error.
func subagentTranscripts(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == dir {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ok, _ := filepath.Match("agent-*.jsonl", d.Name()); ok {
			out = append(out, p)
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	sort.Strings(out)
	return out, err
}

// resultText renders a tool_result's content for classification only.
//
// Content is a plain string in some messages and an array of blocks in others,
// and both shapes occur in real transcripts, so both are handled rather than
// one being assumed. Only the leading text is needed, and nothing is kept.
func resultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return plain
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			return b.Text
		}
	}
	return ""
}

// isDenial reports whether a tool_result is the user refusing the call.
//
// BOTH halves are required. is_error alone is every failed command; the prefix
// alone is any output that quotes it. Each shortcut is wrong in its own
// direction: the first hides denials among failures, the second hides real
// executions among denials.
func isDenial(isError bool, text string) bool {
	if !isError {
		return false
	}
	for _, p := range deniedPrefixes {
		if strings.HasPrefix(text, p) {
			return true
		}
	}
	// A settings deny rule names the tool and its input between two fixed
	// phrases, so it is anchored on the opening and confirmed by the close.
	if strings.HasPrefix(text, "Permission to use ") && strings.Contains(text, " has been denied") {
		return true
	}
	// Auto mode blocks a call it cannot classify when its classifier model is
	// unreachable. The sentence opens with that model's id, which varies, so it
	// is anchored on the "claude-" id prefix and the fixed clause after it.
	return strings.HasPrefix(text, "claude-") &&
		strings.Contains(text, ", so auto mode cannot determine the safety of ")
}

// deniedPrefixes are the other openings of a call refused before it ran. The
// interactive prompt is deniedPrefix above; these are the ones that never
// reach a human. Matching only the first made every auto-mode or `-p` refusal
// read as executed-but-unrecorded, and so turned a healthy session unverified
// the moment anything was refused. Measured across 703 real transcripts: the
// auto mode classifier's refusal, a hook or policy refusal, and the `-p`
// approval refusal each open exactly as written here.
var deniedPrefixes = []string{
	deniedPrefix,
	"Permission for this action was denied by the Claude Code auto mode classifier.",
	"Permission for this action has been denied.",
	"This command requires approval",
}

func collectIDs(path string, into, results, denied map[string]bool) (int, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, err
	}
	if err != nil {
		return 0, err
	}
	defer f.Close() //nolint:errcheck // read-only

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), maxLine)
	for sc.Scan() {
		var line struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil || len(line.Message.Content) == 0 {
			continue
		}
		var blocks []struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			ToolUseID string          `json:"tool_use_id"`
			IsError   bool            `json:"is_error"`
			Content   json.RawMessage `json:"content"`
		}
		if json.Unmarshal(line.Message.Content, &blocks) != nil {
			continue // a string content, or a shape this reader does not know
		}
		for _, b := range blocks {
			switch {
			case b.Type == "tool_use" && b.ID != "":
				into[b.ID] = true
			case b.Type == "tool_result" && b.ToolUseID != "":
				if isDenial(b.IsError, resultText(b.Content)) {
					denied[b.ToolUseID] = true
					continue
				}
				results[b.ToolUseID] = true
			}
		}
	}
	return 1, sc.Err()
}

// FinalAssistantText returns the text of the LAST assistant message in a
// transcript, and whether one was found.
//
// This is the only place in the product that reads message content, and it is
// deliberately a render-time read: the text is never written to the store,
// never logged, and never leaves the machine. The store holds identifiers and
// hostnames; this is read from Claude Code's own file at the moment a human
// asks for a report, and discarded when the process exits.
//
// It is the agent's own account of what it did. Putting it beside the record is
// the entire point of the report -- not to catch the model out, but because a
// summary and a set of records are two descriptions of one session and only a
// reader can reconcile them.
//
// Only text blocks of the last assistant message are taken. A thinking block is
// not the account the user was given, and tool_use blocks are already recorded
// far more precisely on the declaration side.
func FinalAssistantText(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close() //nolint:errcheck // read-only

	var last string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), maxLine)
	for sc.Scan() {
		var line struct {
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		if line.Message.Role != "assistant" || len(line.Message.Content) == 0 {
			continue
		}
		if text, ok := assistantText(line.Message.Content); ok {
			last = text
		}
	}
	if sc.Err() != nil || last == "" {
		return "", false
	}
	return last, true
}

// assistantText joins the text blocks of one assistant message.
//
// Content is either a plain string or an array of blocks, and both shapes occur
// in real transcripts, so both are handled rather than one being assumed.
func assistantText(content json.RawMessage) (string, bool) {
	var plain string
	if json.Unmarshal(content, &plain) == nil {
		if strings.TrimSpace(plain) == "" {
			return "", false
		}
		return plain, true
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return "", false
	}
	var parts []string
	for _, b := range blocks {
		// Text only. A thinking block is not the account the user was given.
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, b.Text)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "\n"), true
}

// TurnFinal is what one main transcript ties to one prompt: the turn's final
// assistant text with the timestamp of the line that carried it, and the
// message.id of every response the turn made.
type TurnFinal struct {
	// Said is true when the file ties any assistant text to the prompt. A
	// turn whose every tied line was a tool call has Responses and no words.
	Said bool
	Text string
	AtMS int64
	// Responses is the message.id of every API response the file ties to the
	// prompt, text or not, a subagent's sidechain line in this file included:
	// the turn's spend in this file, which spend prices by id. An id is a
	// key, never content.
	Responses map[string]bool
}

// FinalAssistantTexts reads a main transcript ONCE and returns, for each
// prompt id in want, the text of the last assistant message that belongs to
// that prompt, with the line's timestamp, and the message id of every
// response that belongs to it. A wanted prompt the file ties no assistant
// text to is absent, or present with Said false.
//
// It is FinalAssistantText narrowed to turns, for `rashomon spend`'s
// silent-failure line. A turn's silent_failures verdict is the digest's rule
// (BuildSilentFailures) applied to that turn's final message, and the digest
// is handed that message by the Stop hook at the moment the turn ends. spend
// runs long after, with no hook payload, so the only surviving copy is the
// transcript -- read here, under the same render-time rule as
// FinalAssistantText: never written anywhere, never rendered, reduced by the
// caller to the verdict's booleans and counts, and discarded when the process
// exits. It lives in this file because this file is where the product reads
// message content, and a second reader elsewhere would be a second place to
// audit.
//
// A TURN IS ITS PROMPT, NOT A SPAN OF TIME. Assistant lines carry no
// promptId, but every user line Claude Code 2.1.285 writes does -- the typed
// prompt and each tool_result answering the turn's calls -- and it is the
// prompt_id the hooks record. So an assistant line belongs to the prompt of
// the user line before it, in file order. The earlier rule, "the last text
// between this turn's first record and the next recorded turn's", took the
// wrong words whenever a prompt answered without any tool call came between
// the two: such a prompt leaves nothing in the store, so its reply fell
// inside the window and became the previous turn's final message -- firing on
// honest summaries, and hiding silent ones behind a later reply that happened
// to say "error". The same tie keys the turn's SPEND (Responses): a span of
// recorded time left out the response that made the first call and the final
// reply, and could swallow a later turn's responses.
//
// A user line with no promptId that is a prompt, rather than a tool result or
// an injected meta line, ends attribution: its reply belongs to a turn this
// reader cannot key, and crediting it to the previous one would be the same
// wrong-words bug. So does a line that cannot be decoded, unless it is a
// sidechain line. A subagent's sidechain line ties its response to the
// current prompt and never moves the tie. A turn whose words cannot be
// attributed has no Said words, so the caller has no final message and takes
// no verdict -- a floor, never a guess.
//
// One pass, decoding only what can matter: every line's header (type,
// isSidechain, isMeta, promptId, timestamp, message.id); a user line's block
// TYPES only when it carries no promptId (userBlocks); and, when an assistant
// line's prompt is wanted and the line has a text block at all, its blocks'
// types and text (assistantLine) -- no other block's contents. Every such
// line of the turn is decoded, in memory, and only the last one's text is
// kept. The caller hands every wanted prompt of a session in one call, so a
// transcript is read once however many of its turns need a verdict.
func FinalAssistantTexts(path string, want map[string]bool) map[string]TurnFinal {
	out := map[string]TurnFinal{}
	if len(want) == 0 {
		return out
	}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close() //nolint:errcheck // read-only

	var current string
	// tie records a response's id as the current prompt's spend.
	tie := func(id string) {
		tf := out[current]
		if tf.Responses == nil {
			tf.Responses = map[string]bool{}
		}
		tf.Responses[id] = true
		out[current] = tf
	}
	// unsay drops the current turn's words: a line that may be its last text
	// could not be read, so no earlier text is its final word.
	unsay := func() {
		if !want[current] {
			return
		}
		tf := out[current]
		tf.Said, tf.Text, tf.AtMS = false, "", 0
		out[current] = tf
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), maxLine)
	for sc.Scan() {
		raw := sc.Bytes()
		var head struct {
			Type        string `json:"type"`
			IsSidechain bool   `json:"isSidechain"`
			IsMeta      bool   `json:"isMeta"`
			PromptID    string `json:"promptId"`
			Timestamp   string `json:"timestamp"`
			Message     struct {
				ID string `json:"id"`
			} `json:"message"`
		}
		if json.Unmarshal(raw, &head) != nil {
			// A line whose header does not decode (a timestamp written as a
			// number, a line cut short) cannot be placed. It may be the next
			// prompt, and keeping the tie would credit that prompt's spend
			// and words to this turn; it may be this turn's last text, so
			// its words so far may not be its last. So the tie ends and the
			// turn's words are dropped, as the subagent reader ends its tie
			// -- unless the line's bytes mark it a sidechain line, which
			// never moves the tie or speaks for the main agent. Guessing
			// from the bytes "user" and "text" fired on a tool_use input
			// holding the value "user" and on a sidechain user line, and
			// missed a user line truncated before its type.
			if !bytes.Contains(raw, []byte(`"isSidechain":true`)) {
				unsay()
				current = ""
			}
			continue
		}
		if head.IsSidechain {
			// A subagent's line written into the main transcript. Its
			// response is spend of the turn it runs in, so its id is tied
			// to the current prompt; its words are never the main agent's
			// final word, and its user lines (the subagent's task and tool
			// results) never change which prompt is current.
			if head.Type == "assistant" && want[current] && head.Message.ID != "" {
				tie(head.Message.ID)
			}
			continue
		}
		switch head.Type {
		case "user":
			if head.PromptID != "" {
				current = head.PromptID
			} else if !head.IsMeta && !toolResultOnly(raw) {
				current = ""
			}
		case "assistant":
			if !want[current] {
				continue
			}
			if head.Message.ID != "" {
				tie(head.Message.ID)
			}
			if !bytes.Contains(raw, []byte(`"text"`)) {
				continue
			}
			line, err := decodeAssistantLine(raw)
			if err != nil || line.Message.Role != "assistant" {
				unsay()
				continue
			}
			text, ok := line.Message.Content.text()
			if !ok {
				continue
			}
			// A text line with no parseable timestamp cannot be ordered
			// against a second main file of the same session -- and it may
			// be the turn's last word, so the earlier words ("On it.") are
			// not: the turn's words are dropped rather than judged on those.
			at, err := time.Parse(time.RFC3339Nano, head.Timestamp)
			if err != nil {
				unsay()
				continue
			}
			tf := out[current]
			tf.Said, tf.Text, tf.AtMS = true, text, at.UnixMilli()
			out[current] = tf
		}
	}
	if sc.Err() != nil {
		// A file that cannot be read to the end may hold a later reply than
		// any found, so nothing read from it is a turn's final word.
		return map[string]TurnFinal{}
	}
	return out
}

// decodeAssistantLine is the one way FinalAssistantTexts decodes an assistant
// line past its header, into assistantLine and nothing else.
// TestAssistantLine_DecodesOnlyTextBlocks reflects over its result type and
// checks FinalAssistantTexts goes through it, so a decode in place of it
// cannot pass unseen. A variable only so that test can count the calls.
var decodeAssistantLine = func(raw []byte) (assistantLine, error) {
	var line assistantLine
	err := json.Unmarshal(raw, &line)
	return line, err
}

// assistantLine is the whole of what FinalAssistantTexts decodes from an
// assistant line past its header: the role, and each content block's type
// and top-level text. A tool_use block's input, a thinking block's thinking
// and a tool result's output have no field to land in, so encoding/json steps
// over them. Held by TestAssistantLine_DecodesOnlyTextBlocks.
//
// The earlier reader decoded message.content whole into a json.RawMessage,
// which copied every block of the line -- a tool_use input with a "text" key
// included -- into a value, while the README said only the final assistant
// message was read.
type assistantLine struct {
	Message struct {
		Role    string     `json:"role"`
		Content textBlocks `json:"content"`
	} `json:"message"`
}

// textBlocks is an assistant message's content, as blocks. Content is either
// an array of blocks or a plain string, and both shapes occur in real
// transcripts; a plain string is one text block.
type textBlocks []textBlock

type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (b *textBlocks) UnmarshalJSON(data []byte) error {
	var plain string
	if json.Unmarshal(data, &plain) == nil {
		*b = textBlocks{{Type: "text", Text: plain}}
		return nil
	}
	var blocks []textBlock
	if err := json.Unmarshal(data, &blocks); err != nil {
		return err
	}
	*b = blocks
	return nil
}

// text joins the text blocks, as assistantText does: a thinking block is not
// the account the user was given, and a block of any type but "text" is not
// kept.
func (b textBlocks) text() (string, bool) {
	var parts []string
	for _, blk := range b {
		if blk.Type == "text" && strings.TrimSpace(blk.Text) != "" {
			parts = append(parts, blk.Text)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "\n"), true
}

// userBlocks is the whole of what toolResultOnly decodes from a user line:
// the TYPE of each content block, and nothing else. No field is tagged for a
// block's text, a tool result's output or a prompt's words, so encoding/json
// steps over those bytes without ever making a value of them -- the property
// spend's usage read holds for its own shape, held here by
// TestUserBlocks_DecodeOnlyBlockTypes. A prompt typed as a plain string does
// not decode into a slice at all (a type error, reported without the string
// being kept), and is a prompt.
//
// The earlier reader decoded message.content whole into a json.RawMessage --
// a copy of every tool result's output held as a value -- to learn only the
// block types; that was a second content read the README did not disclose.
type userBlocks struct {
	Message struct {
		Content []struct {
			Type string `json:"type"`
		} `json:"content"`
	} `json:"message"`
}

// toolResultOnly reports whether a user line's content is an array of
// tool_result blocks and nothing else: a call finishing inside a turn, not a
// new prompt. Decided from block types alone (userBlocks).
func toolResultOnly(raw []byte) bool {
	var line userBlocks
	if json.Unmarshal(raw, &line) != nil || len(line.Message.Content) == 0 {
		return false
	}
	for _, b := range line.Message.Content {
		if b.Type != "tool_result" {
			return false
		}
	}
	return true
}
