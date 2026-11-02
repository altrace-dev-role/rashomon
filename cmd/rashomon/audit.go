package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/seed"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/spend"
)

// auditUsage is the usage screen's audit entry (a const of its own, joined in
// verbs.go).
const auditUsage = `  rashomon audit [--days N]      count what you corrected your agent for in
                               the last N days (default 14) of Claude Code
                               sessions on this machine: read here, written
                               nowhere, 0 tokens; a card you can copy

`

// auditDays is the window when none is asked for: two weeks, the first
// corpus the card was measured on.
const auditDays = 14

// cmdAudit is `rashomon audit` (card SEED1; PF-AU1, PF-AU2, PF-HD17's read
// path): the corrections the person made to their agent in the last N days,
// counted from Claude Code's own transcripts under settings.ConfigDir, the
// files spend.Discover finds. It opens no store, writes nothing, starts no
// model and makes no connection: one explicit command, never run on its own.
// A transcript that cannot be opened is counted as unread, not an error, so
// one unreadable file does not cost the person the rest of their card.
func cmdAudit(args []string, stdout io.Writer) error {
	days, err := auditWindow(args)
	if err != nil {
		return err
	}
	configDir, err := settings.ConfigDir()
	if err != nil {
		return err
	}
	found, err := spend.Discover(configDir, spend.WindowStart(time.Now(), days))
	if err != nil {
		return err
	}
	t := seed.New()
	unread := found.UnreadableDirs
	for _, f := range found.Files {
		if auditFile(t, f) != nil {
			unread++
		}
	}
	s := t.Summary()
	s.Stale, s.Unread = found.Stale, s.Unread+unread
	_, err = io.WriteString(stdout, seed.Card(s, days))
	return err
}

// auditFile folds one transcript; its project is the projects/ folder its
// main transcript sits in, the subagent's parent's for a subagent.
func auditFile(t *seed.Tally, f spend.TranscriptFile) error {
	r, err := os.Open(f.Path)
	if err != nil {
		return err
	}
	defer r.Close()
	return t.Read(r, filepath.Base(filepath.Dir(f.Main)), f.Subagent)
}

// auditWindow reads --days N, spend's rule: a positive whole number of days,
// at most spend.MaxDays, and nothing else.
func auditWindow(args []string) (int, error) {
	days := auditDays
	for i := 0; i < len(args); i++ {
		if args[i] != "--days" {
			return 0, fmt.Errorf("unknown argument %q", args[i])
		}
		if i+1 >= len(args) {
			return 0, errors.New("--days needs a value")
		}
		n, err := strconv.Atoi(args[i+1])
		if err != nil || n < 1 || n > spend.MaxDays {
			return 0, fmt.Errorf("--days needs a whole number of days from 1 to %d, got %q", spend.MaxDays, args[i+1])
		}
		days = n
		i++
	}
	return days, nil
}
