// Package intent is the request's preflight: the identifiers a prompt
// names, resolved against what the repository already holds, and the
// task-preflight note that says so at UserPromptSubmit. The prompt is read in
// memory and never kept: only paths the project holds and the note survive.
package intent

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

const (
	// MaxPromptBytes is how much of a request is read.
	MaxPromptBytes = 16 << 10
	// MaxPaths is how many named paths are kept.
	MaxPaths = 8
	// MaxLstat is how many path lookups one request may cost.
	MaxLstat = 32
)

// Verb is a request's class.
type Verb string

const (
	VerbAdd      Verb = "add"
	VerbFix      Verb = "fix"
	VerbRefactor Verb = "refactor"
	VerbTest     Verb = "test"
	VerbMigrate  Verb = "migrate"
	VerbDeploy   Verb = "deploy"
	VerbDocument Verb = "document"
	VerbOther    Verb = "other"
)

// Verbs is the closed class list.
func Verbs() []Verb {
	return []Verb{VerbAdd, VerbFix, VerbRefactor, VerbTest, VerbMigrate, VerbDeploy, VerbDocument, VerbOther}
}

// Brief is the project facts the first prompt's note carries: the fields of
// brief.Brief it reads, copied by the caller.
type Brief struct {
	Instructions []string
	Manifests    []string
	Module       string
	GoDirs       int
	WalkCapped   bool
}

// has reports a brief with something to say.
func (b *Brief) has() bool {
	return b != nil && (len(b.Instructions) > 0 || len(b.Manifests) > 0 || b.Module != "" || b.GoDirs > 0)
}

// Project is what a request is resolved against.
type Project struct {
	Root  string
	Spec  *spec.Compiled
	Index *verify.ProjectIndex
	NowMS int64
	// Lstat looks up a root-relative slash path; nil confines lookups to Root.
	Lstat func(rel string) (fs.FileInfo, error)
}

// Rule is a spec rule that applies to the request.
type Rule struct {
	ID    string
	Trust spec.Trust
	Why   string
}

// Gate is a gate the request's class runs before done.
type Gate struct {
	Words string
	Trust spec.Trust
}

// Intent is a request resolved against the project. Nothing in it is the
// request's text: Paths are paths the project holds, and every other field
// comes from the spec, the index or a closed table.
type Intent struct {
	Verb  Verb
	Paths []string
	Rules []Rule
	// Withheld is the rules that apply from an untrusted source, counted
	// and never named (INV-2).
	Withheld int
	Gates    []Gate
	// Learned is the test families earlier sessions made required; -1 with
	// no index.
	Learned  int
	Evidence []string
	// FromTask says Evidence is the task file's, not the class table's.
	FromTask bool
	// Unknown is what could not be read or matched, in EnumPreflightUnknown's
	// order.
	Unknown   []string
	Truncated bool
	// Named says the request named a path or an identifier, found or not.
	Named bool
}

// The unknown values Resolve and Finding say, from EnumPreflightUnknown.
const (
	unknownNoSpec    = "no spec found"
	unknownUntrusted = "spec present, untrusted"
	unknownSpecError = "spec unreadable"
	unknownNoHistory = "no gate history for this project"
	unknownSymbols   = "symbols not matched, no platform graph log"
	unknownCut       = "request over 16 KiB, the rest not read"
	unknownNoPath    = "no named path exists in the project"
	unknownNoModule  = "no go.mod, packages not counted"
	unknownCapped    = "package walk capped, count is a lower bound"
)

// classEvidence is the class table (card appendix, P12): what a done claim
// of each class carries beyond its gates.
func classEvidence(v Verb) []string {
	switch v {
	case VerbFix:
		return []string{"judgment_disclosed"}
	case VerbDeploy:
		return []string{"mechanism_stated"}
	}
	return nil
}

// Resolve reads at most MaxPromptBytes of prompt; cut says the reader
// already cut it.
func Resolve(prompt []byte, cut bool, p Project) Intent {
	in := Intent{Truncated: cut || len(prompt) > MaxPromptBytes}
	if len(prompt) > MaxPromptBytes {
		prompt = prompt[:MaxPromptBytes]
	}
	var named []string
	symbols := false
	scan(prompt, in.Truncated, func(tok []byte) {
		switch {
		case pathLike(tok):
			named = append(named, string(tok))
		case identifierLike(tok):
			symbols = true
		case in.Verb == "":
			in.Verb = verbOf(tok)
		}
	})
	if in.Verb == "" {
		in.Verb = VerbOther
	}
	in.Named = len(named) > 0 || symbols
	in.Paths = resolvePaths(named, p)

	sp := p.Spec
	if sp != nil {
		for _, r := range sp.Rules {
			if r.Rule.Kind != string(spec.RequireEvidence) && !slices.ContainsFunc(in.Paths, r.Rule.Covers) {
				continue
			}
			t := trustOf(sp, r.Source)
			if !trusted(t) {
				in.Withheld++
				continue
			}
			in.Rules = append(in.Rules, Rule{ID: r.Rule.ID, Trust: t, Why: r.Rule.Why})
		}
	}
	if in.Verb != VerbDocument && sp != nil {
		for _, d := range sp.Gates {
			g, t := d.Gate, trustOf(sp, d.Source)
			test := g.Kind == "test" && (g.Required == nil || *g.Required)
			supplies := in.Verb == VerbDeploy && slices.Contains(g.Supplies, "real_data_run")
			w := gateWords(g.Argv)
			if !trusted(t) || !test && !supplies || w == "" || slices.ContainsFunc(in.Gates, func(x Gate) bool { return x.Words == w }) {
				continue
			}
			in.Gates = append(in.Gates, Gate{Words: w, Trust: t})
		}
	}
	in.Learned = -1
	if p.Index != nil {
		in.Learned = 0
		for _, e := range p.Index.Entries {
			if in.Verb != VerbDocument && e.Kind == "test" && p.Index.Required(e.Family, p.NowMS) {
				in.Learned++
			}
		}
	}
	if sp != nil && sp.Task != nil && len(sp.Task.ExpectedEvidence) > 0 {
		in.FromTask = true
		for _, k := range sp.Task.ExpectedEvidence {
			if slices.Contains(spec.EvidenceKinds, k) && !slices.Contains(in.Evidence, k) {
				in.Evidence = append(in.Evidence, k)
			}
		}
	} else {
		in.Evidence = classEvidence(in.Verb)
	}

	say := func(cond bool, u string) {
		if cond {
			in.Unknown = append(in.Unknown, u)
		}
	}
	say(sp == nil || len(sp.Sources) == 0 && len(sp.Errors) == 0, unknownNoSpec)
	say(sp != nil && slices.ContainsFunc(sp.Sources, func(s spec.Source) bool { return !trusted(s.Trust) }), unknownUntrusted)
	say(sp != nil && len(sp.Errors) > 0, unknownSpecError)
	say(p.Index == nil, unknownNoHistory)
	say(symbols, unknownSymbols)
	say(in.Truncated, unknownCut)
	say(len(named) > 0 && len(in.Paths) == 0, unknownNoPath)
	return in
}

// Names reports, from one scan and no lookup, whether a request names a
// path or an identifier, found or not: Resolve's Named. The catch-up asks it
// first, so a request that names nothing costs no spec, index or steer-log
// read.
func Names(prompt []byte, cut bool) bool {
	cut = cut || len(prompt) > MaxPromptBytes
	if len(prompt) > MaxPromptBytes {
		prompt = prompt[:MaxPromptBytes]
	}
	named := false
	scan(prompt, cut, func(tok []byte) { named = named || pathLike(tok) || identifierLike(tok) })
	return named
}

// resolvePaths keeps, in the order named, at most MaxPaths of the named
// paths that the project holds as a file or a directory, at most MaxLstat
// lookups in all. A path is cleaned first; one that leaves the root, an
// absolute path outside it, a link, and a path the note could not print are
// never kept, and only a path that may be kept costs a lookup.
func resolvePaths(named []string, p Project) []string {
	if len(named) == 0 {
		return nil
	}
	lstat := p.Lstat
	if lstat == nil {
		r, err := os.OpenRoot(p.Root)
		if err != nil {
			return nil
		}
		defer r.Close()
		lstat = func(rel string) (fs.FileInfo, error) { return r.Lstat(filepath.FromSlash(rel)) }
	}
	var kept []string
	looked := 0
	for _, n := range named {
		if len(kept) == MaxPaths || looked == MaxLstat {
			break
		}
		rel, ok := relative(n, p.Root)
		if !ok || slices.Contains(kept, rel) {
			continue
		}
		if err := slot.Path(rel).Validate(); err != nil && !errors.Is(err, slot.ErrSecretShaped) {
			continue
		}
		looked++
		if info, err := lstat(rel); err == nil && (info.Mode().IsRegular() || info.IsDir()) {
			kept = append(kept, rel)
		}
	}
	return kept
}

// relative is a named path as a cleaned root-relative slash path, or false
// when it is the root itself or outside it.
func relative(name, root string) (string, bool) {
	if strings.HasPrefix(name, "/") {
		r := filepath.ToSlash(filepath.Clean(root))
		rest, ok := strings.CutPrefix(path.Clean(name), r+"/")
		if root == "" || !ok {
			return "", false
		}
		name = rest
	}
	c := path.Clean(name)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") || strings.HasPrefix(c, "/") {
		return "", false
	}
	return c, true
}

// trustOf is a declaration's source trust; a source outside the spec's is
// untrusted rather than a panic.
func trustOf(sp *spec.Compiled, source int) spec.Trust {
	if source != spec.SourceImplied && (source < 0 || source >= len(sp.Sources)) {
		return spec.Untrusted
	}
	return sp.TrustOf(source)
}

// trusted reports a trust whose text a note may carry.
func trusted(t spec.Trust) bool {
	return t == spec.Trusted || t == spec.Managed || t == spec.Builtin
}

// gateWords is a gate's first two argv words, the command a note names it
// by ("go test"), or "" with no argv.
func gateWords(argv []string) string {
	if len(argv) > 2 {
		argv = argv[:2]
	}
	return strings.Join(argv, " ")
}
