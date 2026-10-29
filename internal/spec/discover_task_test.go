package spec

import (
	"io/fs"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

// taskAt is the active task file beside the repository spec of dir.
func taskAt(dir string) string { return filepath.Join(dir, filepath.FromSlash(TaskFile)) }

func (m mapFS) task(dir, doc string) mapFS {
	m.files[taskAt(dir)] = doc
	return m
}

func taskDigestOf(t *testing.T, doc string) string {
	t.Helper()
	_, d, err := ParseTask([]byte(doc))
	if err != nil {
		t.Fatalf("ParseTask(%.60q): %v", doc, err)
	}
	return d
}

// implied is the rules of c that no file declares.
func implied(c *Compiled) []DeclaredRule {
	var out []DeclaredRule
	for _, r := range c.Rules {
		if r.Source == SourceImplied {
			out = append(out, r)
		}
	}
	return out
}

// TestTF7_DiscoverFindsCurrentAndIgnoresSiblings: the task is current.json
// beside the repository spec that applies (the nearest), or under the project
// root with no repository spec; a sibling <id>.json is never read, alone or
// beside current.json.
func TestTF7_DiscoverFindsCurrentAndIgnoresSiblings(t *testing.T) {
	sibling := filepath.Join(filepath.Dir(taskAt("/r")), "abc.json")
	m := newFS().repo("/r", docB).task("/r", taskDoc())
	m.files[sibling] = taskDoc(kv{"id", `"abc"`})
	c := found(t, m, "/r/a", "/r")
	if c.Task == nil || c.Task.ID != "t-1" || c.TaskDigest != taskDigestOf(t, taskDoc()) {
		t.Fatalf("task %+v digest %q; want t-1 and its digest", c.Task, c.TaskDigest)
	}
	alone := newFS().repo("/r", docB)
	alone.files[sibling] = taskDoc(kv{"id", `"abc"`})
	if c := found(t, alone, "/r/a", "/r"); c.Task != nil || c.TaskDigest != "" {
		t.Errorf("a sibling alone: task %+v digest %q; want none", c.Task, c.TaskDigest)
	}
	near := newFS().repo("/r", docA).repo("/r/a", docB).task("/r", taskDoc(kv{"id", `"far"`})).task("/r/a", taskDoc(kv{"id", `"near"`}))
	if c := found(t, near, "/r/a/b", "/r"); c.Task == nil || c.Task.ID != "near" {
		t.Errorf("two specs: task %+v; want the nearest spec's, near", c.Task)
	}
	far := newFS().repo("/r", docA).repo("/r/a", docB).task("/r", taskDoc(kv{"id", `"far"`}))
	if c := found(t, far, "/r/a/b", "/r"); c.Task != nil {
		t.Errorf("a task beside the farther spec only: %+v; want none", c.Task)
	}
	root := newFS().task("/r", taskDoc(kv{"id", `"root"`})).task("/r/a", taskDoc(kv{"id", `"mid"`}))
	c = found(t, root, "/r/a/b", "/r")
	if c.Task == nil || c.Task.ID != "root" || c.Sources != nil || c.Gates != nil {
		t.Errorf("no spec: %+v; want the project root's task and no source", c)
	}
	if c, err := Discover(newFS().task("/r", taskDoc()), nil, ""); c != nil || err != nil {
		t.Errorf("no candidates: %+v, %v; want nil, nil", c, err)
	}
}

// TestTF8_RefusedTaskIsAnErrorLineNotFatal: a task file ParseTask refuses, or
// one that cannot be read, is one Errors line, no Task and no digest, and the
// spec applies as it would alone; with no spec it is a Compiled with the line.
func TestTF8_RefusedTaskIsAnErrorLineNotFatal(t *testing.T) {
	bad := taskDoc(kv{"owner", `"me"`})
	_, _, perr := ParseTask([]byte(bad))
	if perr == nil {
		t.Fatal("ParseTask accepted an unknown key; the rows below read its refusal")
	}
	line := taskAt("/r") + ": " + perr.Error()
	spec := found(t, newFS().repo("/r", docB), "/r/a", "/r")
	c := found(t, newFS().repo("/r", docB).task("/r", bad), "/r/a", "/r")
	if c.Task != nil || c.TaskDigest != "" || !slices.Equal(c.Errors, []string{line}) {
		t.Errorf("refused: task %+v digest %q errors %q; want none and %q", c.Task, c.TaskDigest, c.Errors, line)
	}
	if !slices.Equal(paths(c), paths(spec)) || !reflect.DeepEqual(c.Gates, spec.Gates) || c.Digest != spec.Digest || implied(c) != nil {
		t.Errorf("refused: the spec %+v, want %+v", c, spec)
	}
	m := newFS().repo("/r", docB).task("/r", taskDoc())
	m.errs[taskAt("/r")] = fs.ErrPermission
	c, err := Discover(m, Locations("/r/a", "/r"), managedAt)
	if err != nil || c == nil || c.Task != nil || len(c.Errors) != 1 || !reflect.DeepEqual(c.Gates, spec.Gates) {
		t.Errorf("unreadable: %+v, %v; want the spec and one Errors line", c, err)
	}
	alone := found(t, newFS().task("/r", bad), "/r/a", "/r")
	if alone.Task != nil || alone.Sources != nil || !slices.Equal(alone.Errors, []string{line}) {
		t.Errorf("refused alone: %+v", alone)
	}
}

// TestTF9_ImpliedProtectRuleAppended: with a task, one implied protect_path
// rule over the task directory follows the declared rules, builtin by
// TrustOf(SourceImplied) whatever the spec's own trust; a declared rule of the
// same id keeps its own slot; without a task, or with a refused one, there is
// none.
func TestTF9_ImpliedProtectRuleAppended(t *testing.T) {
	want := DeclaredRule{ID: "builtin:task-file", Source: SourceImplied, Rule: Rule{
		ID: "task-file", Kind: string(ProtectPath), Why: taskRuleWhy, HardControl: taskRuleControl,
		Paths: []string{".claude/rashomon/task/**"}, Modes: []string{"modify", "delete", "create"},
	}}
	doc := withRule(kv{"id", `"task-file"`})
	c := found(t, newFS().repo("/r", doc).task("/r", taskDoc()), "/r/a", "/r")
	if len(c.Rules) != 2 || c.Rules[0].ID != "spec:task-file" || !reflect.DeepEqual(c.Rules[1], want) {
		t.Fatalf("rules %+v; want spec:task-file, then %+v", c.Rules, want)
	}
	if c.TrustOf(c.Rules[1].Source) != Builtin || c.Trust != Untrusted {
		t.Errorf("implied rule trust %q, spec trust %q; want builtin and untrusted", c.TrustOf(c.Rules[1].Source), c.Trust)
	}
	if c := found(t, newFS().task("/r", taskDoc()), "/r/a", "/r"); !reflect.DeepEqual(c.Rules, []DeclaredRule{want}) {
		t.Errorf("no spec: rules %+v; want the implied rule alone", c.Rules)
	}
	for name, m := range map[string]mapFS{
		"no task":      newFS().repo("/r", doc),
		"refused task": newFS().repo("/r", doc).task("/r", taskDoc(kv{"id", "-"})),
	} {
		if c := found(t, m, "/r/a", "/r"); implied(c) != nil {
			t.Errorf("%s: implied rules %+v; want none", name, implied(c))
		}
	}
}
