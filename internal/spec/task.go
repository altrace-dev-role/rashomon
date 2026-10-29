package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The task file (Luis's package, section 3.2; SD1 section 17): the goal
// lives outside the agent's context, written by the person, read by the tool,
// checked by neither. Three lines and an optional fourth: the goal, the
// decision the output serves, the evidence kinds the person expects, and the
// mechanism. 3.3 grades a done claim against it.

// MaxTaskBytes caps the task file.
const MaxTaskBytes = 4 << 10

// TaskFile is the active task, relative to the directory that holds the
// repository spec's .claude (or the project root): one file, its id inside
// it, so discovery needs no id from the session. Its <id>.json siblings are
// the person's archive and are not read.
const TaskFile = ".claude/rashomon/task/current.json"

// EvidenceKinds is the closed list of what a task may expect, in the order
// rows are emitted (3.3). The first three are runs a declared gate supplies
// (Gate.Supplies, supplyKinds); the last two are read from the final message
// and from the task itself.
var EvidenceKinds = []string{"real_data_run", "hostile_input_run", "cross_check", "judgment_disclosed", "mechanism_stated"}

// Task is one parsed task file.
type Task struct {
	ID               string
	Goal             string
	Decision         string
	ExpectedEvidence []string
	Mechanism        string
}

// ParseTask reads one task file with Parse's strictness and returns the Task
// and its digest, DigestPrefix and the hex SHA-256 of the RFC 8785 form. The
// first error is deterministic, in this order: the byte cap (MaxTaskBytes),
// the lexical rules of readTree, unknown keys in file order, then id, goal,
// decision, expected_evidence and mechanism. The digest is returned whenever
// the bytes are one JSON document, even when the schema refused it.
func ParseTask(data []byte) (*Task, string, error) {
	if len(data) > MaxTaskBytes {
		return nil, "", &Error{Reason: ReasonTooLarge, Limit: MaxTaskBytes}
	}
	root, terr := readTree(data)
	if terr != nil {
		return nil, "", terr
	}
	sum := sha256.Sum256(appendJCS(nil, root))
	digest := DigestPrefix + hex.EncodeToString(sum[:])
	r := &reader{}
	t := r.task(&root)
	if r.err != nil {
		return nil, digest, r.err
	}
	return t, digest, nil
}

func (r *reader) task(root *node) *Task {
	f := r.object(root, "id", "goal", "decision", "expected_evidence", "mechanism")
	if f == nil {
		return nil
	}
	t := &Task{}
	t.ID = field(r, f, "id", true, r.id(nil))
	t.Goal = field(r, f, "goal", true, r.why)
	t.Decision = field(r, f, "decision", true, r.why)
	t.ExpectedEvidence = field(r, f, "expected_evidence", true, r.expectedKinds)
	t.Mechanism = field(r, f, "mechanism", false, r.mechanism)
	return t
}

// expectedKinds reads the expected kinds: a non-empty list from EvidenceKinds
// with no kind twice, as supplies reads a gate's.
func (r *reader) expectedKinds(n *node) []string {
	out := list(r, n, r.oneOf(EvidenceKinds))
	for i := range out {
		if slices.Contains(out[:i], out[i]) {
			r.failAt(strconv.Itoa(i), ReasonDuplicateID, 0)
			return nil
		}
	}
	return out
}

// mechanism is optional text under MaxWhyRunes; unlike every other text it
// may be empty, because an empty mechanism is what opens 3.3's
// mechanism_stated obligation.
func (r *reader) mechanism(n *node) string {
	if n.kind == kString && n.str == "" {
		return ""
	}
	return r.why(n)
}

// IsTaskFile reports whether the slash-separated path p names the active
// task file: TaskFile itself, or TaskFile under a directory.
func IsTaskFile(p string) bool {
	p = path.Clean(p)
	return p == TaskFile || strings.HasSuffix(p, "/"+TaskFile)
}

// The implied rule's text, under MaxWhyRunes as a declared rule's must be.
const (
	taskRuleWhy     = "the task file is the person's statement of the goal; an agent that edits it is graded against its own words"
	taskRuleControl = "rashomon implies this rule whenever a task file applies; the person edits the file, the agent does not"
)

// taskRule is the protect_path rule a task file implies (README 3.2): the
// whole task directory, every mode, under the id task-file. Its slot is
// "builtin:task-file", never "spec:task-file", so a declared rule that
// happens to use the id keeps its own; its Source is SourceImplied, so its
// trust is TrustOf's builtin and not the spec's.
func taskRule() DeclaredRule {
	return DeclaredRule{ID: "builtin:task-file", Source: SourceImplied, Rule: Rule{
		ID: "task-file", Kind: string(ProtectPath), Why: taskRuleWhy, HardControl: taskRuleControl,
		Paths: []string{".claude/rashomon/task/**"}, Modes: slices.Clone(pathModes),
	}}
}

// taskPath is the task file beside the repository spec candidate spec, a
// Locations path <dir>/.claude/rashomon/spec.json: <dir>/TaskFile.
func taskPath(spec string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(spec))), filepath.FromSlash(TaskFile))
}

// task reads the task file at p into the Compiled (README 3.2). A missing
// file is no task. One that cannot be read, or that ParseTask refuses, is an
// Errors line and no task, and the spec applies as it would alone: unlike a
// repository spec's read error, which Discover returns, a task file's costs
// nothing else, since the task is the lesser source and a FIFO or a mode
// planted at its path must not take the gates with it.
func (d *discovery) task(fsys FS, p string) {
	data, err := fsys.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	var t *Task
	digest := ""
	if err == nil {
		t, digest, err = ParseTask(data)
	}
	if err != nil {
		d.c.Errors = append(d.c.Errors, p+": "+err.Error())
		return
	}
	d.c.Task, d.c.TaskDigest = t, digest
}
