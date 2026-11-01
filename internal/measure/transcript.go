package measure

import (
	"slices"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spend"
)

// Transcript is one session's Claude Code transcript rows: its main
// transcript and every subagent transcript under it, summed. Tokens count
// each response once, with its retried attempts.
type Transcript struct {
	Files             int      `json:"files"`
	Responses         int      `json:"responses"`
	SubagentResponses int      `json:"subagent_responses"`
	Models            []string `json:"models"`
	Tokens            Tokens   `json:"tokens"`
	// UnreadLines counts lines and files spend could not read: the rows
	// undercount by them.
	UnreadLines int `json:"unread_lines"`
}

// Tokens is the transcript's usage by kind.
type Tokens struct {
	Input        int64 `json:"input"`
	Output       int64 `json:"output"`
	CacheRead    int64 `json:"cache_read"`
	CacheWrite5m int64 `json:"cache_write_5m"`
	CacheWrite1h int64 `json:"cache_write_1h"`
}

func (t *Tokens) add(u spend.Tokens) {
	t.Input += u.Input
	t.Output += u.Output
	t.CacheRead += u.CacheRead
	t.CacheWrite5m += u.CacheWrite5m
	t.CacheWrite1h += u.CacheWrite1h
}

// transcriptOfSession reads one session's transcripts under configDir: the
// files whose location names the session, last written at or after sinceMS,
// the session's first record (a file written before it cannot hold it). No
// file is nil.
func transcriptOfSession(configDir, session string, sinceMS int64) (*Transcript, error) {
	found, err := spend.Discover(configDir, time.UnixMilli(sinceMS))
	if err != nil {
		return nil, err
	}
	own := &spend.Found{}
	for _, f := range found.Files {
		if f.Session == session {
			own.Files = append(own.Files, f)
		}
	}
	if len(own.Files) == 0 {
		return nil, nil
	}
	scan, err := spend.Read(own)
	if err != nil {
		return nil, err
	}
	return transcriptOf(scan), nil
}

func transcriptOf(scan *spend.Scan) *Transcript {
	t := &Transcript{Files: len(scan.Files), Models: []string{}, UnreadLines: scan.Unreadable + scan.Unparsed}
	for _, r := range scan.Responses {
		t.Responses++
		if r.Subagent {
			t.SubagentResponses++
		}
		if r.Model != "" && !slices.Contains(t.Models, r.Model) {
			t.Models = append(t.Models, r.Model)
		}
		t.Tokens.add(r.Tokens)
		for _, a := range r.Attempts {
			t.Tokens.add(a)
		}
	}
	slices.Sort(t.Models)
	return t
}
