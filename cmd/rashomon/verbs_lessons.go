package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// lessonsView is lessons --json. Project is the working directory's ledger
// name and QuarantinedAtUnixMS the last quarantine, each null with none.
type lessonsView struct {
	Project             *string     `json:"project"`
	QuarantinedAtUnixMS *int64      `json:"quarantined_at_unix_ms"`
	Lessons             []lessonRow `json:"lessons"`
}

// lessonRow is one lesson with its tally's counts.
type lessonRow struct {
	ID          string     `json:"id"`
	Tier        learn.Tier `json:"tier"`
	Text        string     `json:"text"`
	SinceUnixMS int64      `json:"since_unix_ms"`
	learn.Tally
}

// cmdLessons is `rashomon lessons [--json]`: this project's lessons under the
// four tiers the learner moves them through, proven first, each tier in the
// ledger's own order. It reads through learn.Open, which quarantines a ledger
// something else wrote and writes that back (fail closed); the quarantine is
// said first.
func cmdLessons(args []string, stdout io.Writer) error {
	asJSON, err := jsonFlag(args)
	if err != nil {
		return err
	}
	view := lessonsView{Lessons: []lessonRow{}}
	l, err := openLedger()
	if errors.Is(err, store.ErrNoStore) {
		if asJSON {
			return printJSON(stdout, view)
		}
		_, err = fmt.Fprintln(stdout, "no lessons: nothing has been recorded here")
		return err
	}
	if err != nil {
		return err
	}
	_, dir, err := bodiesProject()
	if err != nil {
		return err
	}
	view.Project = &dir
	if at := l.QuarantinedAt(); !at.IsZero() {
		ms := at.UnixMilli()
		view.QuarantinedAtUnixMS = &ms
	}
	for _, tier := range []learn.Tier{learn.TierProven, learn.TierProbation, learn.TierProposed, learn.TierDemoted} {
		for _, ls := range l.Lessons() {
			if ls.Tier == tier {
				view.Lessons = append(view.Lessons, lessonRow{ID: ls.ID, Tier: ls.Tier, Text: ls.Text,
					SinceUnixMS: ls.SinceUnixMS, Tally: l.Tally(ls.ID)})
			}
		}
	}
	if asJSON {
		return printJSON(stdout, view)
	}
	_, err = io.WriteString(stdout, view.text(l.QuarantinedAt()))
	return err
}

// text is lessons as a person reads it: a tier with none says none.
func (v lessonsView) text(quarantined time.Time) string {
	var b strings.Builder
	b.WriteString(quarantineLine(quarantined))
	fmt.Fprintf(&b, "lessons for this project: %d\n", len(v.Lessons))
	for _, tier := range []learn.Tier{learn.TierProven, learn.TierProbation, learn.TierProposed, learn.TierDemoted} {
		var rows strings.Builder
		for _, ls := range v.Lessons {
			if ls.Tier == tier {
				fmt.Fprintf(&rows, "  %s since %s, %d fires: %s\n", ls.ID,
					time.UnixMilli(ls.SinceUnixMS).UTC().Format(time.DateOnly), ls.Fires, ls.Text)
			}
		}
		if rows.Len() == 0 {
			fmt.Fprintf(&b, "%s: none\n", tier)
			continue
		}
		fmt.Fprintf(&b, "%s:\n%s", tier, rows.String())
	}
	return b.String()
}

// quarantineLine is the line that opens learn's and lessons' listing when the
// ledger was quarantined, "" when it never was.
func quarantineLine(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return fmt.Sprintf("quarantined %s: a write this command did not make changed the learning state, "+
		"so every lesson went back to proposed\n", at.UTC().Format(time.RFC3339))
}
