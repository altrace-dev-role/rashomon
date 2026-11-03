package main

import (
	"bytes"
	"strconv"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// taskEnd is one background command's end as a task notification states it
// (card BG-OUTCOME phase 1): the launching call's id, the outcome its status
// word names (store.ExecOK for completed, store.ExecFailed for failed) and
// the exit code the summary ends with.
type taskEnd struct {
	id      string
	outcome string
	exit    int
}

const (
	taskOpen  = "<task-notification>"
	taskClose = "</task-notification>"
)

// taskEnds reads the ends a prompt states, when the prompt is nothing but
// task notifications (Claude Code's text for an ended background task,
// handed to UserPromptSubmit in an idle turn or as a queued command
// mid-turn). Text that quotes a notification is a person's, and is not
// read. A block whose status or summary is not one of the closed forms is
// passed over: "completed (exit code N)", "completed (exit code N: ...)" or
// "failed with exit code N" at the summary's end, after the agent's
// description, which is never read for a code. A subagent's or a
// workflow's end ("Agent ... finished") names no exit code, and is passed
// over. Nothing of the prompt is kept but the three fields.
func taskEnds(prompt []byte) []taskEnd {
	var ends []taskEnd
	for rest := bytes.TrimSpace(prompt); len(rest) > 0; {
		if !bytes.HasPrefix(rest, []byte(taskOpen)) {
			return nil
		}
		block, after, ok := bytes.Cut(rest[len(taskOpen):], []byte(taskClose))
		if !ok {
			return nil
		}
		if end, ok := readTaskEnd(block); ok {
			ends = append(ends, end)
		}
		rest = bytes.TrimSpace(after)
	}
	return ends
}

// readTaskEnd is one block's end, false where a field is missing or not in
// its closed form.
func readTaskEnd(block []byte) (taskEnd, bool) {
	id, status, summary := taskTag(block, "tool-use-id"), taskTag(block, "status"), taskTag(block, "summary")
	if len(id) == 0 || bytes.ContainsAny(id, "<> \t\n") {
		return taskEnd{}, false
	}
	var code []byte
	outcome := store.ExecOK
	switch string(status) {
	case "completed":
		i := bytes.LastIndex(summary, []byte(" completed (exit code "))
		if i < 0 || !bytes.HasSuffix(summary, []byte(")")) {
			return taskEnd{}, false
		}
		code = summary[i+len(" completed (exit code ") : len(summary)-1]
		if c, _, found := bytes.Cut(code, []byte(": ")); found {
			code = c
		}
	case "failed":
		outcome = store.ExecFailed
		i := bytes.LastIndex(summary, []byte(" failed with exit code "))
		if i < 0 {
			return taskEnd{}, false
		}
		code = summary[i+len(" failed with exit code "):]
	default:
		return taskEnd{}, false
	}
	n, err := strconv.Atoi(string(code))
	if err != nil || n < 0 || n > 255 {
		return taskEnd{}, false
	}
	return taskEnd{id: string(id), outcome: outcome, exit: n}, true
}

// taskTag is the text of the block's one <name>...</name>, nil with none.
func taskTag(block []byte, name string) []byte {
	_, v, ok := bytes.Cut(block, []byte("<"+name+">"))
	if !ok {
		return nil
	}
	v, _, ok = bytes.Cut(v, []byte("</"+name+">"))
	if !ok {
		return nil
	}
	return v
}

// keepTaskEnds appends an outcome_join for each end the prompt states. A
// failed append is dropped: the run reads as launched, as before the card.
func keepTaskEnds(st *store.Store, sessionID string, prompt []byte, now time.Time) {
	if sessionID == "" {
		return
	}
	for _, end := range taskEnds(prompt) {
		_ = st.AppendRecord(sessionID, store.NewOutcomeJoin(sessionID, end.id, end.outcome, end.exit,
			store.SourceTaskNotification, store.ParserTaskNotification, now.UnixMilli()))
	}
}
