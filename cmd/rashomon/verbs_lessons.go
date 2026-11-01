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
// Quarantined is true when the state on disk does not check, so every lesson
// is read as proposed though nothing has been written to say so yet.
type lessonsView struct {
	Project             *string     `json:"project"`
	Quarantined         bool        `json:"quarantined"`
	QuarantinedAtUnixMS *int64      `json:"quarantined_at_unix_ms"`
	Lessons             []lessonRow `json:"lessons"`
}

// lessonRow is one lesson with its tally's counts. ApprovedUnixMS is when a
// person last approved it at a terminal, 0 for never: a row past proposed
// at 0 is an approval a build before the stamp made, and is not sent
// (LEARN-DX1 F2).
type lessonRow struct {
	ID             string     `json:"id"`
	Tier           learn.Tier `json:"tier"`
	Text           string     `json:"text"`
	SinceUnixMS    int64      `json:"since_unix_ms"`
	ApprovedUnixMS int64      `json:"approved_unix_ms"`
	learn.Tally
}

// cmdLessons is `rashomon lessons [--json]`: this project's lessons under the
// four tiers the learner moves them through, proven first, each tier in the
// ledger's own order. It reads through learn.OpenReadOnly and writes nothing:
// a ledger something else wrote reads as quarantined, every lesson proposed,
// and stays on disk as it is for the next learn command to quarantine; a
// quarantine is said first.
func cmdLessons(args []string, stdout io.Writer) error {
	asJSON, err := jsonFlag(args)
	if err != nil {
		return err
	}
	view := lessonsView{Lessons: []lessonRow{}}
	l, err := openLedgerReadOnly()
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
	view.Project, view.Quarantined = &dir, l.Quarantined
	if at := l.QuarantinedAt(); !at.IsZero() {
		ms := at.UnixMilli()
		view.QuarantinedAtUnixMS = &ms
	}
	for _, tier := range []learn.Tier{learn.TierProven, learn.TierProbation, learn.TierProposed, learn.TierDemoted} {
		for _, ls := range l.Lessons() {
			if ls.Tier == tier {
				view.Lessons = append(view.Lessons, lessonRow{ID: ls.ID, Tier: ls.Tier, Text: ls.Text,
					SinceUnixMS: ls.SinceUnixMS, ApprovedUnixMS: ls.ApprovedUnixMS, Tally: l.Tally(ls.ID)})
			}
		}
	}
	if asJSON {
		return printJSON(stdout, view)
	}
	_, err = io.WriteString(stdout, view.text(l.QuarantinedAt()))
	return err
}

// text is lessons as a person reads it: a tier with none says none, and a
// lesson that is not sent says why under its row, as learn's listing does.
func (v lessonsView) text(quarantined time.Time) string {
	var b strings.Builder
	if !v.Quarantined {
		b.WriteString(quarantineLine(quarantined))
	} else {
		b.WriteString("quarantined: a write this command did not make changed the learning state, " +
			"so every lesson reads as proposed; this command wrote nothing, and the next learn command records it\n")
	}
	fmt.Fprintf(&b, "lessons for this project: %d\n", len(v.Lessons))
	for _, tier := range []learn.Tier{learn.TierProven, learn.TierProbation, learn.TierProposed, learn.TierDemoted} {
		var rows strings.Builder
		for _, ls := range v.Lessons {
			if ls.Tier == tier {
				fmt.Fprintf(&rows, "  %s since %s, %d fires: %s\n", ls.ID,
					time.UnixMilli(ls.SinceUnixMS).UTC().Format(time.DateOnly), ls.Fires, ls.Text)
				rows.WriteString(notSentLine(learn.Lesson{ID: ls.ID, Tier: ls.Tier, Text: ls.Text, ApprovedUnixMS: ls.ApprovedUnixMS}))
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
