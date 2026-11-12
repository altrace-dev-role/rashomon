package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/graph/knowledge"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The conflict base (W-M2.4 hook seam, card m2-seam1 D2): written at the
// prompt, where graphBrief has the index loaded, for the post's conflict
// check (conflictJoin), which never loads the index. One file per session
// and index, under the session's unit-brief directory by
// graph.ConflictBaseFileName, through writeUnitBrief: the unit briefs'
// consent gate, writer, purge rule and disclosure row. Two lines: a head
// (version, commit, repository, the rules' digest) a prompt reads alone to
// see the file is current, then the rules as engine.RulesFor resolved them
// and engine.ConflictBaseOf's slice. It holds no word of the prompt (D160).

// conflictBaseVersion is the base file's format.
const conflictBaseVersion = 1

// maxConflictBaseBytes caps the base file (D2): a base over it is not
// written, any older one is removed, and the post reads none.
const maxConflictBaseBytes = 256 << 10

// maxConflictBaseHead caps the head line a prompt reads to see the base is
// current.
const maxConflictBaseHead = 512

// conflictBaseHead is the base's first line.
type conflictBaseHead struct {
	V           int    `json:"v"`
	Commit      string `json:"commit"`
	Repo        string `json:"repo"`
	RulesDigest string `json:"rules_digest"`
}

// conflictBaseBody is the base's second line.
type conflictBaseBody struct {
	Rules   []engine.Rule       `json:"rules"`
	Records []graph.Record      `json:"records"`
	Digests map[graph.ID]string `json:"digests"`
}

// conflictBase is a base file read back whole.
type conflictBase struct {
	conflictBaseHead
	conflictBaseBody
}

// conflictBasePath is where the session's base for the index at root lives.
func conflictBasePath(storeRoot, sessionID, root string) string {
	return filepath.Join(graph.UnitBriefDir(storeRoot, sessionID), graph.ConflictBaseFileName(root))
}

// writeConflictBase stores the conflict base for the index at root, loaded
// as s at rev, under the rules the spec specOf reads and the index's
// knowledge rows give, while the store keeps the session's content. A base
// whose head is the file's own (same commit, repository and rules) is not
// rebuilt, so a session pays the slice once per commit and rule set. With
// no rule, or a base over the cap, no base is left. A failed write is left:
// the post reads no base and checks nothing.
func writeConflictBase(storeRoot, sessionID, root, repo string, s *graph.Store, rev graph.Rev, specOf func() *spec.Compiled) {
	if storeRoot == "" || specOf == nil || !store.SessionContentMarker(storeRoot) {
		return
	}
	rows, _ := knowledge.ReadDir(root)
	rules := engine.RulesFor(specOf(), rows, s, repo, rev)
	path := conflictBasePath(storeRoot, sessionID, root)
	if len(rules) == 0 {
		_ = os.Remove(path)
		return
	}
	enc, err := json.Marshal(rules)
	if err != nil {
		return
	}
	sum := sha256.Sum256(enc)
	head := conflictBaseHead{V: conflictBaseVersion, Commit: rev.Commit, Repo: repo, RulesDigest: hex.EncodeToString(sum[:16])}
	if old, ok := readConflictBaseHead(path); ok && old == head {
		return
	}
	data, err := encodeConflictBase(head, rules, engine.ConflictBaseOf(s, rev, rules))
	if err != nil {
		return
	}
	if len(data) > maxConflictBaseBytes {
		_ = os.Remove(path)
		return
	}
	_ = writeUnitBrief(path, data)
}

// encodeConflictBase is the base file's bytes: the head line, then the
// body line of rules and their slice cb. writeConflictBase stores them
// under the cap; the K7 measure sizes them over this repository's index.
func encodeConflictBase(head conflictBaseHead, rules []engine.Rule, cb engine.ConflictBase) ([]byte, error) {
	hb, err := json.Marshal(head)
	if err != nil {
		return nil, err
	}
	bb, err := json.Marshal(conflictBaseBody{Rules: rules, Records: cb.Records, Digests: cb.Digests})
	if err != nil {
		return nil, err
	}
	return append(append(append(hb, '\n'), bb...), '\n'), nil
}

// readConflictBaseHead is the head of the base at path, false for none or a
// head that does not read back.
func readConflictBaseHead(path string) (conflictBaseHead, bool) {
	f, err := os.Open(path)
	if err != nil {
		return conflictBaseHead{}, false
	}
	defer f.Close()
	buf := make([]byte, maxConflictBaseHead)
	n, _ := io.ReadFull(f, buf)
	line, _, found := bytes.Cut(buf[:n], []byte{'\n'})
	var h conflictBaseHead
	if !found || json.Unmarshal(line, &h) != nil || h.V != conflictBaseVersion {
		return conflictBaseHead{}, false
	}
	return h, true
}

// readConflictBase is the base at path, false for none, one over the cap, or
// one that does not read back whole at this version (a torn or foreign file
// is a miss, never a guess).
func readConflictBase(path string) (conflictBase, bool) {
	f, err := os.Open(path)
	if err != nil {
		return conflictBase{}, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxConflictBaseBytes+1))
	if err != nil || len(data) > maxConflictBaseBytes {
		return conflictBase{}, false
	}
	head, body, found := bytes.Cut(data, []byte{'\n'})
	var cb conflictBase
	if !found || json.Unmarshal(head, &cb.conflictBaseHead) != nil || cb.V != conflictBaseVersion ||
		json.Unmarshal(bytes.TrimSuffix(body, []byte{'\n'}), &cb.conflictBaseBody) != nil {
		return conflictBase{}, false
	}
	return cb, true
}
