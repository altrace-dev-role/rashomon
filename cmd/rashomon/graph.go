package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/knowledge"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// graphUsage is the graph verb's lines on the usage screen (TRU2c: every
// sub-command and flag a disclosure paragraph names is a word here).
const graphUsage = `  rashomon graph index [DIR]     build DIR/.rashomon/graph.jsonl (default the
                               working directory) from DIR/.rashomon/graph.json:
                               the units' files through each language's
                               producer, the declared knowledge, and the rows
                               under DIR/.rashomon/knowledge/
  rashomon graph brief --prompt TEXT [--tf|--idf|--lessons] [DIR]
                               print the brief the prompt hook would deliver
                               for TEXT from DIR, with its explanation; the
                               flag picks the scorer (the hook reads
                               RASHOMON_GRAPH_SCORER)
  rashomon graph knowledge import FILE...
                               write the decisions, lessons and requirements
                               FILE... hold (a failure ledger, decision lines,
                               a directory of decision records, a truth table)
                               under .rashomon/knowledge/
                               of the index at or above the working directory,
                               each row keyed to this repository, then index
                               again

`

// cmdGraph is the repository graph's person-typed entry (graphUsage).
func cmdGraph(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("graph takes index, brief or knowledge")
	}
	if strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("unknown argument %q", args[0])
	}
	switch args[0] {
	case "index":
		return graphIndex(args[1:], stdout)
	case "brief":
		return graphBriefCmd(args[1:], stdout)
	case "knowledge":
		return graphKnowledge(args[1:], stdout)
	}
	return fmt.Errorf("unknown graph command %q", args[0])
}

func graphIndex(args []string, stdout io.Writer) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	} else if len(args) > 1 {
		return errors.New("graph index takes at most one DIR")
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	cfg, err := index.ReadConfig(root)
	if err != nil {
		return err
	}
	start := time.Now()
	r, err := index.Build(root, headCommit(root), cfg)
	if err != nil {
		return err
	}
	if err := graph.SaveFile(root, r.Store, r.Header); err != nil {
		return err
	}
	rev := graph.RevCommit(r.Header.Rev.Commit)
	for _, k := range cfg.Knowledge {
		switch err := slot.Approved(k.Clause).Validate(); {
		case k.Clause == "":
			fmt.Fprintf(stdout, "warning: %s declares no clause; its line carries none\n", k.ID)
		case err != nil:
			fmt.Fprintf(stdout, "warning: %s clause refused, its line carries none: %v\n", k.ID, err)
		}
	}
	_, err = fmt.Fprintf(stdout, "indexed %d file(s): %d node(s), %d edge(s) at %s in %s -> %s; repo %s\n",
		r.Files, len(r.Store.Nodes(rev)), len(r.Store.Edges(rev)), rev, time.Since(start).Round(time.Millisecond), graph.GraphFile, r.Header.Repo)
	return err
}

// graphKnowledge imports FILE... into the index at or above the working
// directory: the rows are resolved against the saved graph, keyed to the
// repository the graph is built under next (baseline.RepoID, as index.Build
// keys it; the saved header's key is stale once a tree with no origin has
// moved, W-M1.36), written under .rashomon/knowledge/, and the graph is
// built again so they are in it.
func graphKnowledge(args []string, stdout io.Writer) error {
	if len(args) < 2 || args[0] != "import" {
		return errors.New("graph knowledge takes import FILE...")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root := graph.FindIndex(cwd)
	if root == "" {
		return errors.New("graph knowledge import: no " + graph.GraphFile + " at or above the working directory; run rashomon graph index first")
	}
	s, h, err := graph.LoadFile(root)
	if err != nil {
		return err
	}
	rid, _ := baseline.RepoID(root)
	rep, err := knowledge.Import(root, args[1:], s, rid, graph.RevCommit(h.Rev.Commit))
	if err != nil {
		return err
	}
	if _, err := io.WriteString(stdout, rep.String()); err != nil {
		return err
	}
	return graphIndex([]string{root}, stdout)
}

func graphBriefCmd(args []string, stdout io.Writer) error {
	dir, prompt := ".", ""
	opt := engine.Options{IDF: true}
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--prompt" && i+1 < len(args):
			prompt = args[i+1]
			i++
		case args[i] == "--idf":
			opt.IDF = true
		case args[i] == "--tf":
			opt.IDF = false
		case args[i] == "--lessons":
			opt.Lessons = true
		case strings.HasPrefix(args[i], "-"):
			return fmt.Errorf("unknown argument %q", args[i])
		default:
			dir = args[i]
		}
	}
	if prompt == "" {
		return errors.New("graph brief needs --prompt")
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	start := time.Now()
	cwd := root
	if at := graph.FindIndex(root); at != "" {
		root = at
	}
	s, h, err := graph.LoadFile(root)
	if err != nil {
		return err
	}
	loaded := time.Since(start)
	opt.Rel = relTo(cwd, root)
	b := engine.Build(s, graph.RevCommit(h.Rev.Commit), prompt, opt)
	fmt.Fprintf(stdout, "load: %d record(s) in %s; pipeline: %s\n\n", h.Records, loaded, b.Elapsed)
	fmt.Fprintln(stdout, b.Explain())
	fmt.Fprintln(stdout, "brief:")
	if b.Empty() {
		_, err = fmt.Fprintln(stdout, "(empty)")
		return err
	}
	_, err = fmt.Fprintln(stdout, b.Text())
	return err
}

// headCommit is the repository's HEAD, short, read from .git without
// running git: HEAD from the git directory, a branch ref from the common
// directory's refs or packed-refs, so a worktree names its own branch's
// commit (W-M1.8); C1 when root is not a repository.
func headCommit(root string) string {
	gitDir, commonDir, ok := baseline.GitDirs(root)
	if !ok {
		return "C1"
	}
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "C1"
	}
	h := strings.TrimSpace(string(head))
	if ref, ok := strings.CutPrefix(h, "ref: "); ok {
		if b, err := os.ReadFile(filepath.Join(commonDir, filepath.FromSlash(ref))); err == nil {
			h = strings.TrimSpace(string(b))
		} else {
			h = packedRef(filepath.Join(commonDir, "packed-refs"), ref)
		}
	}
	if len(h) < 12 {
		return "C1"
	}
	return h[:12]
}

// packedRef is ref's hash in packed-refs, or "".
func packedRef(path, ref string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if hash, name, ok := strings.Cut(line, " "); ok && name == ref {
			return hash
		}
	}
	return ""
}

// cmdBriefOrGraph is the dispatch for the two person-typed read-side
// entries that share a main.go case.
func cmdBriefOrGraph(name string, args []string, stdout io.Writer) error {
	if name == "graph" {
		return cmdGraph(args, stdout)
	}
	return cmdBrief(args, stdout)
}

// graphBrief is the hook path: the brief for the prompt over the graph
// indexed under cwd, each path relative to cwd, read before the preflight
// decides whether the prompt has anything to say (GRAPH-SLOT1). ok is
// whether a message rendered. Every run that found an index leaves its
// explanation under the store root at graph/<session>-<prompt>.txt.
func graphBrief(storeRoot, cwd, sessionID, promptID string, prompt []byte) (indexBrief, bool) {
	if cwd == "" || len(prompt) == 0 {
		return indexBrief{}, false
	}
	root := graph.FindIndex(cwd)
	if root == "" {
		return indexBrief{}, false
	}
	s, h, err := graph.LoadFile(root)
	if err != nil {
		return indexBrief{}, false
	}
	opt := hookScorer()
	opt.Rel = relTo(cwd, root)
	rev := graph.RevCommit(h.Rev.Commit)
	b := engine.Build(s, rev, string(prompt), opt)
	writeGraphEvidence(storeRoot, sessionID, promptID, b)
	writeUnitBriefs(storeRoot, sessionID, promptID, root, h.Repo, s, rev, b, opt)
	ib := indexBrief{Brief: b, Repo: h.Repo}
	if first := b.FirstKey(); first != "" {
		n, _ := s.Node(first, rev)
		ib.First, ib.Tier = graph.GlobalID(h.Repo, first), n.Tier
		if u := unitOf(s, rev, first); u != "" && b.Lines[0].Role != engine.RoleNotKnown {
			ib.Unit = graph.GlobalID(h.Repo, u)
		}
	}
	return ib, !b.Empty()
}

// indexBrief is the brief with what names it (W-M1.7, AD19): the
// repository id the index's header carries, the global id and tier of the
// node the brief's first keyed line is keyed on (Brief.FirstKey), and the
// global id of the unit that contains that node, "" when none does or
// when the brief opens with the not-known line (W-M1.38), which claims no
// unit.
type indexBrief struct {
	engine.Brief
	Repo  string
	First graph.ID
	Tier  graph.Tier
	Unit  graph.ID
}

// unitOf is the unit that contains id, found by walking up the containing
// and declaring edges, or, for a knowledge node, through the first node it
// applies to; "" when no unit is reached within four steps.
func unitOf(s *graph.Store, rev graph.Rev, id graph.ID) graph.ID {
	for step := 0; step < 4 && id != ""; step++ {
		if strings.HasPrefix(string(id), "unit:") {
			return id
		}
		if _, ok := s.Node(id, rev); !ok {
			return ""
		}
		next := graph.ID("")
		if in := s.In(id, rev, graph.Contains, graph.Declares); len(in) > 0 {
			next = in[0].Src
		} else if out := s.Out(id, rev, graph.AppliesTo); len(out) > 0 {
			next = out[0].Dst
		}
		id = next
	}
	return ""
}

// relTo maps a path as the graph holds it, relative to root, to its form
// relative to cwd, slashed: the path the agent opens from where it runs.
func relTo(cwd, root string) func(string) string {
	return func(p string) string {
		r, err := filepath.Rel(cwd, filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			return p
		}
		return filepath.ToSlash(r)
	}
}

// keyGraph is the graph brief's key among the prompt's task-preflight
// findings, beside the request's note (intent's note and brief keys).
const keyGraph = "graph"

// graphFinding is the prompt's graph brief as a task-preflight finding
// under key (keyGraph), routed in preflightNote's one Route call beside the
// request's note: its message, and its record form (Brief.Record) where the
// record scope keeps digests (digest set).
func graphFinding(b indexBrief, key string, afterSeq int64, digest func(string) string) (steer.Finding, bool) {
	if b.Empty() {
		return steer.Finding{}, false
	}
	var rec render.Message
	if digest != nil {
		r, err := b.Record(digest)
		if err != nil {
			return steer.Finding{}, false
		}
		rec = r
	}
	return briefFinding(b, b.Message, rec, key, afterSeq), true
}

// briefFinding is a brief's message as a task-preflight finding under key:
// the prompt's graph brief (graphFinding) and a unit's brief at the post of
// an edit in it (unitBriefJoin, under the unit's head). A nudge with no stop
// form, rec its record form where the record scope keeps digests (the
// message itself where rec is empty); its source the graph row its first
// keyed line is keyed on and its unit that row's unit (AD19), none when the
// brief says its unit is not known.
func briefFinding(b indexBrief, m, rec render.Message, key string, afterSeq int64) steer.Finding {
	prio, _ := pattern.Priority(pattern.TaskPreflight)
	return steer.Finding{Pattern: pattern.TaskPreflight, Key: key, Priority: prio, MaxLevel: steer.LevelNudge,
		StopForm: steer.StopFormNone, Message: m, Record: rec, AfterSeq: afterSeq, Unit: string(b.Unit),
		Source: steer.Source{Kind: steer.SourceGraph, ID: string(b.First), Tier: string(b.Tier)}}
}

func writeGraphEvidence(storeRoot, sessionID, promptID string, b engine.Brief) {
	if storeRoot == "" {
		return
	}
	dir := filepath.Join(storeRoot, graph.StoreDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	name := graph.EvidenceFileName(sessionID, promptID, time.Now())
	text := b.Evidence() + "\nbrief:\n" + b.Text() + "\n"
	_ = os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600)
}

// hookScorer is the scorer the hook path uses: the inverse-document-
// frequency weights, unless RASHOMON_GRAPH_SCORER=tf asks for the plain
// count. The variable exists for the comparison runs.
func hookScorer() engine.Options {
	scorer := os.Getenv("RASHOMON_GRAPH_SCORER")
	return engine.Options{IDF: scorer != "tf", Lessons: scorer == "lessons"}
}
