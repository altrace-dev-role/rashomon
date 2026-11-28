package episode

import (
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/trace"
)

// act is a call joined to its result.
type act struct {
	step    int // index of the call's step
	end     int // index of the result's step, or step when none came
	call    *trace.CallStep
	result  *trace.ResultStep
	sig     ActSig
	paths   []string // repository-relative
	at      time.Time
	session string
	side    bool
}

// failed is a failure of the act itself: an error or a nonzero exit, not a
// refusal by the person or a rule.
func (a *act) failed() bool {
	return a.result != nil && a.result.Failed && a.result.Denied == trace.DeniedNone
}

func (a *act) passed() bool { return a.result != nil && !a.result.Failed }

func (a *act) edits() bool {
	return a.sig.Class == trace.ActEdit || a.sig.Class == trace.ActWrite
}

// said is a line in the person's slot after at least one act.
type said struct {
	step  int
	after int // index into acts of the last act before it, -1 when none
	text  string
	orig  trace.Origin
	sig   trace.Signal
	side  bool
	at    time.Time
	sess  string
}

// session is the steps of one session joined into acts.
type session struct {
	steps []trace.Step
	acts  []act
	said  []said
	root  string
}

func view(steps []trace.Step, root string) *session {
	s := &session{steps: steps, root: root}
	byID := map[string]int{}
	for i := range steps {
		st := &steps[i]
		if s.root == "" && st.Cwd != "" {
			s.root = st.Cwd
		}
		switch st.Kind {
		case trace.Call:
			byID[st.Call.ID] = len(s.acts)
			s.acts = append(s.acts, act{step: i, end: i, call: st.Call, sig: sigOf(st.Call),
				at: st.At, session: st.Session, side: st.Sidechain})
		case trace.Result:
			if j, ok := byID[st.Result.CallID]; ok && s.acts[j].result == nil {
				s.acts[j].result, s.acts[j].end = st.Result, i
			}
		case trace.Said:
			s.said = append(s.said, said{step: i, after: len(s.acts) - 1, text: st.Text, orig: st.Origin,
				sig: st.Signal, side: st.Sidechain, at: st.At, sess: st.Session})
		}
	}
	for i := range s.acts {
		s.acts[i].paths = s.relPaths(&s.acts[i])
	}
	return s
}

func sigOf(c *trace.CallStep) ActSig {
	sig := ActSig{Class: c.Class}
	if c.Class == trace.ActExec {
		sig.Program, sig.Verb = commandSig(c.Command)
	}
	return sig
}

// relPaths are the paths an act names or changes, and for a command the
// path-shaped words in it, relative to the root; a path outside the root is
// left out.
func (s *session) relPaths(a *act) []string {
	raw := append([]string(nil), a.call.Paths...)
	for _, c := range a.call.Changes {
		raw = append(raw, c.Path)
	}
	if a.call.Command != "" {
		raw = append(raw, pathWords(a.call.Command)...)
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range raw {
		r, ok := rel(s.root, p)
		if ok && r != "." && !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}

func rel(root, p string) (string, bool) {
	if p == "" {
		return "", false
	}
	p = filepath.ToSlash(p)
	if !path.IsAbs(p) {
		return path.Clean(p), !strings.HasPrefix(path.Clean(p), "..")
	}
	if root == "" {
		return "", false
	}
	r, err := filepath.Rel(filepath.FromSlash(root), filepath.FromSlash(p))
	if err != nil || strings.HasPrefix(filepath.ToSlash(r), "..") {
		return "", false
	}
	return filepath.ToSlash(r), true
}

// maxScopeDepth is how many directory levels a scope keeps: deep enough to
// tell a package or module apart, shallow enough that two files of one
// component share it.
const maxScopeDepth = 3

// scopeOf is the scope of a set of repository-relative paths.
func scopeOf(paths []string) Scope {
	if len(paths) == 0 {
		return Scope{}
	}
	dirs := make([][]string, 0, len(paths))
	exts := map[string]int{}
	for _, p := range paths {
		d := path.Dir(p)
		if path.Ext(p) == "" && !strings.Contains(path.Base(p), ".") {
			d = p // a directory, or a file with no extension: its own scope
		}
		if d == "." {
			d = ""
		}
		var parts []string
		if d != "" {
			parts = strings.Split(d, "/")
		}
		dirs = append(dirs, parts)
		if e := path.Ext(p); e != "" {
			exts[e]++
		}
	}
	common := dirs[0]
	for _, d := range dirs[1:] {
		n := 0
		for n < len(common) && n < len(d) && common[n] == d[n] {
			n++
		}
		common = common[:n]
	}
	if len(common) > maxScopeDepth {
		common = common[:maxScopeDepth]
	}
	best, bestN := "", 0
	for e, n := range exts {
		if n > bestN || n == bestN && e < best {
			best, bestN = e, n
		}
	}
	return Scope{Dir: strings.Join(common, "/"), Ext: best}
}
