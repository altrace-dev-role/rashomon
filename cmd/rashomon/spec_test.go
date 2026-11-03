package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/selfguard"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

const (
	specUnit = `{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]}]}`
	specVet  = `{"rashomon_spec": 1, "gates": [{"id": "vet", "kind": "lint", "argv": ["go", "vet", "./..."]}]}`
)

// specProject is a project in the working directory, with a .git and doc
// as its repository spec, beside a store under RASHOMON_HOME: the store's
// root, the project as discovery names it (symlinks resolved) and the spec's
// path under it. No agent signal and no CLAUDE_PROJECT_DIR is set.
type specProject struct {
	t               *testing.T
	root, dir, path string
}

func newSpecProject(t *testing.T, doc string) specProject {
	t.Helper()
	st, _, _ := bodiesHere(t)
	t.Setenv(selfguard.EnvClaudeCode, "")
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := specProject{t: t, root: st.Root(), dir: dir, path: filepath.Join(dir, ".claude", "rashomon", "spec.json")}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		t.Fatal(err)
	}
	p.edit(doc)
	return p
}

// edit replaces the spec's bytes with doc.
func (p specProject) edit(doc string) {
	p.t.Helper()
	if err := os.WriteFile(p.path, []byte(doc), 0o644); err != nil {
		p.t.Fatal(err)
	}
}

// trust is what discovery from the project reads the spec's trust as.
func (p specProject) trust() spec.Trust {
	p.t.Helper()
	sp := hook.DiscoverSpec(p.dir, "", "", hook.NoRun(p.root))
	if sp == nil || len(sp.Sources) != 1 || sp.Sources[0].Path != p.path {
		p.t.Fatalf("discovery from %s: %+v; want the one repository spec %s", p.dir, sp, p.path)
	}
	return sp.Trust
}

// bindings is the store's bindings file, decoded; none is an empty list.
func (p specProject) bindings() []spec.Binding {
	p.t.Helper()
	b, err := spec.ReadBindings(spec.OSFS{}, p.root)
	if err != nil {
		p.t.Fatal(err)
	}
	return b.Bindings
}

// typedPrefix is the first 8 hex characters of doc's digest, what a person types.
func typedPrefix(t *testing.T, doc string) string {
	t.Helper()
	_, d, err := spec.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimPrefix(d, spec.DigestPrefix)[:8]
}

// digestOf is doc's digest as a source carries it.
func digestOf(t *testing.T, doc string) string {
	t.Helper()
	_, d, err := spec.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// atSpecTerminal makes p the terminal spec trust asks on, or no terminal
// when err is set.
func atSpecTerminal(t *testing.T, p *person, err error) {
	t.Helper()
	old := specTerminal
	specTerminal = func() (io.Reader, io.Writer, func(), error) {
		if err != nil {
			return nil, nil, nil, err
		}
		return p, p, func() {}, nil
	}
	t.Cleanup(func() { specTerminal = old })
}

// specRun runs `rashomon spec args...` with stdin carrying in, which the verb
// must never read as the answer.
func specRun(t *testing.T, in string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(append([]string{"spec"}, args...), strings.NewReader(in+"\n"), &out, &errOut)
	return code, out.String(), errOut.String()
}

// TestST1_TrustBindsTheDigest: `spec trust` shows the project's repository
// spec, its digest and what it declares (quoted) on the terminal, and binds
// (project, path, digest) under the store root only when the digest's first
// 8 characters are typed back there; discovery then reads the spec as
// trusted. No terminal, an agent signal, a wrong answer (even with the right
// one on stdin) and a path discovery never reads each bind nothing.
func TestST1_TrustBindsTheDigest(t *testing.T) {
	p := newSpecProject(t, specUnit)
	right := typedPrefix(t, specUnit)

	atSpecTerminal(t, nil, errors.New("no tty"))
	if code, out, errOut := specRun(t, right, "trust"); code != 1 || out != "" || !strings.Contains(errOut, "no terminal to ask on") {
		t.Errorf("trust with no terminal: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	agent := &person{answer: right}
	atSpecTerminal(t, agent, nil)
	t.Setenv(selfguard.EnvClaudeCode, "1")
	if code, out, errOut := specRun(t, right, "trust"); code != 1 || out != "" || agent.read {
		t.Errorf("trust under an agent signal: exit %d, stdout %q, stderr %q, terminal read %v", code, out, errOut, agent.read)
	} else {
		wantRefusal(t, "spec trust", errOut, "spec trust "+shellWord(p.path), "written")
	}
	t.Setenv(selfguard.EnvClaudeCode, "")
	atSpecTerminal(t, &person{answer: "00000000"}, nil)
	if code, out, errOut := specRun(t, right, "trust"); code != 1 || out != "" || !strings.Contains(errOut, "did not match") {
		t.Errorf("trust with a wrong answer: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	atSpecTerminal(t, &person{answer: right}, nil)
	if code, out, errOut := specRun(t, right, "trust", filepath.Join(p.dir, "spec.json")); code != 1 || out != "" || !strings.Contains(errOut, "not a repository spec") {
		t.Errorf("trust of a path discovery never reads: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got := p.bindings(); len(got) != 0 || p.trust() != spec.Untrusted {
		t.Fatalf("after four refusals: bindings %+v, trust %q; want none, untrusted", got, p.trust())
	}

	shown := &person{answer: right}
	atSpecTerminal(t, shown, nil)
	code, out, errOut := specRun(t, "", "trust")
	if code != 0 || !strings.HasPrefix(out, "trusted "+p.path) || errOut != "" {
		t.Fatalf("trust: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	screen := shown.shown.String()
	for _, want := range []string{p.path, digestOf(t, specUnit), `"unit"`, `["go" "test" "./..."]`, "first 8 characters"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the terminal screen %q does not show %q", screen, want)
		}
	}
	got := p.bindings()
	if len(got) != 1 || got[0].ProjectRoot != p.dir || got[0].SpecPath != p.path || got[0].SpecDigest != digestOf(t, specUnit) || got[0].BoundAtMS <= 0 {
		t.Errorf("bindings %+v; want one of (%s, %s, %s) with a time", got, p.dir, p.path, digestOf(t, specUnit))
	}
	if fi, err := os.Stat(filepath.Join(p.root, spec.TrustFile)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("the bindings file: %v, %v; want mode 0600", fi, err)
	}
	if tr := p.trust(); tr != spec.Trusted {
		t.Errorf("discovery after trust: %q, want trusted", tr)
	}
}

// TestST1b_EditDropsTrust: an edit to a trusted spec is a new digest, which
// no binding names, so discovery reads it as untrusted with the old binding
// still on file; trusting the edited file replaces that binding rather than
// adding a second for the same path.
func TestST1b_EditDropsTrust(t *testing.T) {
	p := newSpecProject(t, specUnit)
	atSpecTerminal(t, &person{answer: typedPrefix(t, specUnit)}, nil)
	if code, out, errOut := specRun(t, "", "trust"); code != 0 {
		t.Fatalf("trust: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	p.edit(specVet)
	if tr := p.trust(); tr != spec.Untrusted {
		t.Errorf("discovery after an edit: %q, want untrusted", tr)
	}
	if got := p.bindings(); len(got) != 1 || got[0].SpecDigest != digestOf(t, specUnit) {
		t.Errorf("bindings after an edit %+v; want the old one only", got)
	}
	atSpecTerminal(t, &person{answer: typedPrefix(t, specVet)}, nil)
	if code, out, errOut := specRun(t, "", "trust"); code != 0 {
		t.Fatalf("trust again: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got := p.bindings(); len(got) != 1 || got[0].SpecDigest != digestOf(t, specVet) {
		t.Errorf("bindings after trusting the edit %+v; want one, at the new digest", got)
	}
	if tr := p.trust(); tr != spec.Trusted {
		t.Errorf("discovery after trusting the edit: %q, want trusted", tr)
	}
}

// TestST1c_UntrustRemovesTheBinding: `spec untrust` removes the binding for
// the project's spec, named or found, and discovery reads the spec as untrusted
// again; a second untrust says there was nothing to remove and succeeds.
func TestST1c_UntrustRemovesTheBinding(t *testing.T) {
	p := newSpecProject(t, specUnit)
	atSpecTerminal(t, &person{answer: typedPrefix(t, specUnit)}, nil)
	if code, out, errOut := specRun(t, "", "trust"); code != 0 {
		t.Fatalf("trust: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if code, out, errOut := specRun(t, "", "untrust", p.path); code != 0 || !strings.HasPrefix(out, "untrusted "+p.path) {
		t.Fatalf("untrust: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got := p.bindings(); len(got) != 0 {
		t.Errorf("bindings after untrust %+v; want none", got)
	}
	if tr := p.trust(); tr != spec.Untrusted {
		t.Errorf("discovery after untrust: %q, want untrusted", tr)
	}
	if code, out, errOut := specRun(t, "", "untrust"); code != 0 || !strings.Contains(out, "was not trusted") {
		t.Errorf("untrust again: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

// An agent session cannot take a spec's trust away: untrust stops the spec's
// rules being nudges to the agent, so it is refused as trust is, naming the
// command to run in a terminal, and writes nothing (the bindings file's bytes
// and the key stay as they were).
func TestAGSU1_AnAgentSessionCannotUntrustASpec(t *testing.T) {
	p := newSpecProject(t, specUnit)
	atSpecTerminal(t, &person{answer: typedPrefix(t, specUnit)}, nil)
	if code, out, errOut := specRun(t, "", "trust"); code != 0 {
		t.Fatalf("trust: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	bindingsFile := filepath.Join(p.root, spec.TrustFile)
	before, err := os.ReadFile(bindingsFile)
	if err != nil {
		t.Fatal(err)
	}
	key := p.readKey()
	t.Setenv(selfguard.EnvClaudeCode, "1")
	for _, args := range [][]string{{"untrust"}, {"untrust", p.path}} {
		code, out, errOut := specRun(t, "", args...)
		if code != 1 || out != "" {
			t.Errorf("%v in an agent session: exit %d, stdout %q, stderr %q", args, code, out, errOut)
		}
		wantRefusal(t, strings.Join(args, " "), errOut, "spec untrust "+shellWord(p.path), "written")
	}
	if after, _ := os.ReadFile(bindingsFile); !bytes.Equal(after, before) {
		t.Errorf("an agent session changed %s: %q, was %q", spec.TrustFile, after, before)
	}
	if again := p.readKey(); !bytes.Equal(again, key) {
		t.Error("an agent session changed the key")
	}
	if tr := p.trust(); tr == spec.Untrusted {
		t.Error("an agent session untrusted the spec")
	}
}
