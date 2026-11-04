package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// cmdGraph is the repository graph's person-typed entry:
//
//	rashomon graph index [DIR]              build DIR/.rashomon/graph.jsonl from DIR/.rashomon/graph.json
//	rashomon graph brief --prompt P [DIR]   the brief the hook would deliver for P from DIR, with its explanation
func cmdGraph(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("graph takes index or brief")
	}
	switch args[0] {
	case "index":
		return graphIndex(args[1:], stdout)
	case "brief":
		return graphBriefCmd(args[1:], stdout)
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
	s, files, err := index.Build(root, headCommit(root), cfg)
	if err != nil {
		return err
	}
	if err := graph.SaveFile(root, s, headCommit(root)); err != nil {
		return err
	}
	rev := graph.RevCommit(headCommit(root))
	for _, k := range cfg.Knowledge {
		switch err := slot.Approved(k.Clause).Validate(); {
		case k.Clause == "":
			fmt.Fprintf(stdout, "warning: %s declares no clause; its line carries none\n", k.ID)
		case err != nil:
			fmt.Fprintf(stdout, "warning: %s clause refused, its line carries none: %v\n", k.ID, err)
		}
	}
	_, err = fmt.Fprintf(stdout, "indexed %d file(s): %d node(s), %d edge(s) at %s in %s -> %s\n",
		files, len(s.Nodes(rev)), len(s.Edges(rev)), rev, time.Since(start).Round(time.Millisecond), graph.GraphFile)
	return err
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
	b := engine.Build(s, graph.RevCommit(h.Commit), prompt, opt)
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
// running git; C1 when root is not a repository.
func headCommit(root string) string {
	dir := filepath.Join(root, ".git")
	if b, err := os.ReadFile(dir); err == nil {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: "); ok {
			dir = rest
		}
	}
	head, err := os.ReadFile(filepath.Join(dir, "HEAD"))
	if err != nil {
		return "C1"
	}
	h := strings.TrimSpace(string(head))
	if ref, ok := strings.CutPrefix(h, "ref: "); ok {
		if b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(ref))); err == nil {
			h = strings.TrimSpace(string(b))
		} else {
			h = packedRef(filepath.Join(dir, "packed-refs"), ref)
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
func graphBrief(storeRoot, cwd, sessionID, promptID string, prompt []byte) (engine.Brief, bool) {
	if cwd == "" || len(prompt) == 0 {
		return engine.Brief{}, false
	}
	root := graph.FindIndex(cwd)
	if root == "" {
		return engine.Brief{}, false
	}
	s, h, err := graph.LoadFile(root)
	if err != nil {
		return engine.Brief{}, false
	}
	opt := hookScorer()
	opt.Rel = relTo(cwd, root)
	b := engine.Build(s, graph.RevCommit(h.Commit), string(prompt), opt)
	writeGraphEvidence(storeRoot, sessionID, promptID, b)
	return b, !b.Empty()
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

// graphFinding is the brief as the prompt's task-preflight finding, routed
// in preflightNote's one Route call beside the request's note: a nudge
// with no stop form, its record form (Brief.Record) where the record scope
// keeps digests (digest set), the message itself where it keeps paths.
func graphFinding(b engine.Brief, afterSeq int64, digest func(string) string) (steer.Finding, bool) {
	if b.Empty() {
		return steer.Finding{}, false
	}
	prio, _ := pattern.Priority(pattern.TaskPreflight)
	f := steer.Finding{Pattern: pattern.TaskPreflight, Key: keyGraph, Priority: prio, MaxLevel: steer.LevelNudge,
		StopForm: steer.StopFormNone, Message: b.Message, AfterSeq: afterSeq}
	if digest != nil {
		rec, err := b.Record(digest)
		if err != nil {
			return steer.Finding{}, false
		}
		f.Record = rec
	}
	return f, true
}

func writeGraphEvidence(storeRoot, sessionID, promptID string, b engine.Brief) {
	if storeRoot == "" {
		return
	}
	dir := filepath.Join(storeRoot, "graph")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	name := sessionID + "-" + promptID
	if promptID == "" {
		name = sessionID + "-" + time.Now().UTC().Format("20060102T150405.000")
	}
	text := b.Explain() + "\nbrief:\n" + b.Text() + "\n"
	_ = os.WriteFile(filepath.Join(dir, name+".txt"), []byte(text), 0o600)
}

// hookScorer is the scorer the hook path uses: the inverse-document-
// frequency weights, unless RASHOMON_GRAPH_SCORER=tf asks for the plain
// count. The variable exists for the comparison runs.
func hookScorer() engine.Options {
	return engine.Options{IDF: os.Getenv("RASHOMON_GRAPH_SCORER") != "tf"}
}
