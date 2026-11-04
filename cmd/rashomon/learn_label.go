package main

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// labelSession is `rashomon learn label SESSION` (card LABEL-WRITER1, the
// graph lines): for each knowledge record the session's graph brief
// delivered (learn.GivenGraph), one label row judged on the session's
// writes (learn.Judge): the declarations' file paths, each taken relative
// to its call's cwd, the form the brief names a path in, as the path and
// as its keyed digest (intent.PathDigest under the store key), whichever
// form the line holds. A session already labeled for a record is left as
// it is, so the verb is run as often as one likes. It runs at a terminal
// after the session, as approve does; no hook path opens the ledger
// (RES-5, learn's package comment).
func labelSession(l *learn.Ledger, st *store.Store, sessionID string, now time.Time, stdout io.Writer) error {
	lines, _, err := st.ReadSteer(sessionID)
	if err != nil {
		return err
	}
	run, err := st.ReadRun(sessionID)
	if err != nil {
		return err
	}
	written := writtenPaths(st.Key(), run.Declarations)
	given := learn.GivenGraph(lines)
	if len(given) == 0 {
		_, err = fmt.Fprintf(stdout, "session %s: no graph line delivered a lesson or decision; nothing to label\n", sessionID)
		return err
	}
	for _, g := range given {
		id := learn.FoldID(g.ID)
		if l.Labeled(id, sessionID) {
			fmt.Fprintf(stdout, "%s: labeled by this session already\n", id)
			continue
		}
		label, basis := learn.Judge(g, written)
		row := learn.Label{LessonID: id, SessionID: sessionID, AtUnixMS: now.UnixMilli(), Label: label, Basis: basis}
		if err := l.AppendLabel(row); err != nil {
			fmt.Fprintf(stdout, "%s: not labeled: %v\n", id, err)
			continue
		}
		fmt.Fprintf(stdout, "%s: %s (%s); %d path(s) written, %d named by its lines\n", id, label, basis, written.Paths, len(g.Paths))
	}
	return nil
}

// writtenPaths is the paths the session's file-tool writes name (verify's
// hashed rule: a non-Bash write that names its path), each relative to its
// call's cwd with forward slashes, as the brief names a path from the
// hook's cwd, keyed as the path and as its keyed digest. A relative path as
// written is taken from that cwd first. At record scope v4 the declaration
// withholds the path and nothing is written: no label, not a wrong one.
func writtenPaths(key []byte, decls []store.Declaration) learn.Written {
	w := learn.Written{Keys: map[string]bool{}}
	seen := map[string]bool{}
	for i := range decls {
		d := &decls[i]
		if d.ToolName == "Bash" || d.Shape.VerbClass != shape.VerbWrite || d.FilePath == nil || d.CWD == nil {
			continue
		}
		p := *d.FilePath
		if !filepath.IsAbs(p) {
			p = filepath.Join(*d.CWD, p)
		}
		rel, err := filepath.Rel(*d.CWD, p)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if !seen[rel] {
			seen[rel] = true
			w.Paths++
		}
		w.Keys[rel] = true
		w.Keys[intent.PathDigest(key, rel)] = true
	}
	return w
}
