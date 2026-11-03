package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/seed"
	"github.com/altrace-dev-role/rashomon/internal/selfguard"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/spend"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// auditUsage is the usage screen's audit entry (a const of its own, joined in
// verbs.go).
const auditUsage = `  rashomon audit [--days N]      count what you corrected your agent for in
                               the last N days (default 14) of Claude Code
                               sessions on this machine: read here, written
                               nowhere, 0 tokens; a card you can copy
  rashomon audit --keep [--days N]
                               count the N days before rashomon was set up
                               and keep it in the store: your before count

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
// one unreadable file does not cost the person the rest of their card. With
// --keep it is auditKeep.
func cmdAudit(args []string, stdout io.Writer) error {
	days, keep, err := auditWindow(args)
	if err != nil {
		return err
	}
	if keep {
		return auditKeep(days, stdout)
	}
	t := seed.New()
	s, err := auditFold(t, time.Now(), days)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, seed.Card(s, days))
	return err
}

// auditKeep is `rashomon audit --keep` (card SEED1-KEEP): the N days before
// the store's install time counted, every line stamped at or after it left
// out, and kept under the store root as the before count an after is fewer
// than, for the machine and per project (baseline.Key of a session's working
// directory). It only grows: a recount that finds fewer sessions, as when
// Claude Code's cleanup has taken transcripts since, leaves the kept count
// as it was. It refuses in an agent session (the person keeps their own
// count) and with no store, which it never makes.
func auditKeep(days int, stdout io.Writer) error {
	if s := selfguard.AgentSignal(os.Getenv); s != "" {
		return selfguard.Refusal(s, "the before count is what your later sessions are measured against", "audit --keep", "kept")
	}
	root, err := store.DefaultRoot()
	if err != nil {
		return err
	}
	rec, err := store.ReadInstallRecord(root)
	if errors.Is(err, store.ErrNoStore) {
		return errors.New("there is no rashomon store to keep a count in yet; run `rashomon watch` first, then this")
	}
	if err != nil {
		return err
	}
	old, had, err := seed.ReadKept(root)
	if err != nil {
		return fmt.Errorf("%w; nothing was kept", err)
	}
	until := time.UnixMilli(rec.CreatedAtUnixMS)
	t := seed.New()
	t.Until, t.Key = until, baseline.Key
	s, err := auditFold(t, until, days)
	if err != nil {
		return err
	}
	kept := s.Sessions > 0 && (!had || s.Sessions >= old.All.Sessions)
	if kept {
		if err := seed.WriteKept(root, t.Kept(time.Now(), spend.WindowStart(until, days), until, days)); err != nil {
			return err
		}
	}
	_, err = io.WriteString(stdout, seed.KeptCard(s, days, kept))
	return err
}

// auditFold folds the transcripts of the days days before end into t.
func auditFold(t *seed.Tally, end time.Time, days int) (seed.Summary, error) {
	configDir, err := settings.ConfigDir()
	if err != nil {
		return seed.Summary{}, err
	}
	found, err := spend.Discover(configDir, spend.WindowStart(end, days))
	if err != nil {
		return seed.Summary{}, err
	}
	unread := found.UnreadableDirs
	for _, f := range found.Files {
		if auditFile(t, f) != nil {
			unread++
		}
	}
	s := t.Summary()
	s.Stale, s.Unread = found.Stale, s.Unread+unread
	return s, nil
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
// at most spend.MaxDays, and --keep, and nothing else.
func auditWindow(args []string) (days int, keep bool, err error) {
	days = auditDays
	for i := 0; i < len(args); i++ {
		if args[i] == "--keep" {
			keep = true
			continue
		}
		if args[i] != "--days" {
			return 0, false, fmt.Errorf("unknown argument %q", args[i])
		}
		if i+1 >= len(args) {
			return 0, false, errors.New("--days needs a value")
		}
		n, err := strconv.Atoi(args[i+1])
		if err != nil || n < 1 || n > spend.MaxDays {
			return 0, false, fmt.Errorf("--days needs a whole number of days from 1 to %d, got %q", spend.MaxDays, args[i+1])
		}
		days = n
		i++
	}
	return days, keep, nil
}
