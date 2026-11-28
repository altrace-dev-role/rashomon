package episode

import (
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/trace"
)

func key(kind Kind, sig ActSig, scope Scope) Key {
	return Key{V: KeyVersion, Kind: kind, Act: sig, Scope: scope}
}

// failures finds ActFailure and CheckRegression episodes. A failed act is one
// episode per run of failures of its kind of act: the next failure of the
// same kind before a pass belongs to the same episode. It is a
// CheckRegression when the same kind of command passed earlier in the
// session and a file was edited since; otherwise an ActFailure, and only when
// the session runs that kind of act again after it (a one-off failure the
// agent moved on from is not a mistake it repeats).
func failures(s *session, b Baseline) []Episode {
	var out []Episode
	open := map[ActSig]bool{}
	for i := range s.acts {
		a := &s.acts[i]
		if a.passed() {
			open[a.sig] = false
		}
		if !a.failed() || a.side || open[a.sig] {
			continue
		}
		open[a.sig] = true
		kind := ActFailure
		if s.regressed(i) {
			kind = CheckRegression
		} else if s.next(i, a.sig, func(*act) bool { return true }) < 0 {
			continue
		}
		e := s.episode(kind, i, b)
		if k := s.next(i, a.sig, (*act).passed); k >= 0 {
			e.Resolution = s.resolution(i, k)
			e.To = s.acts[k].end
		}
		out = append(out, e)
	}
	return out
}

// regressed is whether act i, a failed command, passed earlier in the
// session with a file edited between that pass and this failure.
func (s *session) regressed(i int) bool {
	a := &s.acts[i]
	if a.sig.Class != trace.ActExec {
		return false
	}
	edited := false
	for j := i - 1; j >= 0; j-- {
		p := &s.acts[j]
		if p.edits() && p.passed() {
			edited = true
		}
		if p.sig == a.sig && p.passed() {
			return edited
		}
	}
	return false
}

// next is the index of the first act after i of kind sig that ok accepts, -1
// when none.
func (s *session) next(i int, sig ActSig, ok func(*act) bool) int {
	for k := i + 1; k < len(s.acts); k++ {
		if s.acts[k].sig == sig && !s.acts[k].side && ok(&s.acts[k]) {
			return k
		}
	}
	return -1
}

func (s *session) episode(kind Kind, i int, b Baseline) Episode {
	a := &s.acts[i]
	fp, tmpl := Fingerprint(a.sig, a.result, b)
	k := key(kind, a.sig, scopeOf(a.paths))
	k.FP, k.FPV = fp, FingerprintVersion
	return Episode{Key: k, Session: a.session, At: a.at, From: a.step, To: a.end, Evidence: []int{a.end}, Template: tmpl}
}

// resolution is what came between the failure at act i and the pass at
// act k.
func (s *session) resolution(i, k int) *Resolution {
	r := &Resolution{}
	failed, passed := s.acts[i].call.Command, s.acts[k].call.Command
	if failed != "" || passed != "" {
		r.Added, r.Removed = wordDelta(fields(failed), fields(passed))
	}
	before := map[ActSig]bool{}
	for j := 0; j < i; j++ {
		before[s.acts[j].sig] = true
	}
	seen := map[ActSig]bool{}
	edited := map[string]bool{}
	for j := i + 1; j < k; j++ {
		a := &s.acts[j]
		if a.side {
			continue
		}
		if !before[a.sig] && !seen[a.sig] && a.sig != s.acts[i].sig {
			seen[a.sig] = true
			r.Before = append(r.Before, a.sig)
		}
		if a.edits() && a.passed() {
			for _, p := range a.paths {
				edited[p] = true
			}
		}
	}
	for p := range edited {
		r.Edited = append(r.Edited, p)
	}
	slices.Sort(r.Edited)
	return r
}

// wordDelta is the words b has that a has not, and the reverse, in order.
func wordDelta(a, b []string) (added, removed []string) {
	in := func(w string, ws []string) bool { return slices.Contains(ws, w) }
	for _, w := range b {
		if !in(w, a) {
			added = append(added, w)
		}
	}
	for _, w := range a {
		if !in(w, b) {
			removed = append(removed, w)
		}
	}
	return added, removed
}

// corrections finds Correction episodes: the person refusing a call, a rule
// of theirs refusing one, the person stopping the agent after an act, and,
// with a lexicon, a line the person typed that turns the agent back. A line
// counts only when the agent acted since the person's previous line, and a
// subagent's lines are never the person's.
func corrections(s *session, pushback func(string) bool) []Episode {
	var out []Episode
	for i := range s.acts {
		a := &s.acts[i]
		if a.result == nil || a.side {
			continue
		}
		switch a.result.Denied {
		case trace.DeniedByPerson:
			out = append(out, s.correction(a, "reject", a.end))
		case trace.DeniedByRule:
			out = append(out, s.correction(a, "block", a.end))
		}
	}
	last := -1 // the act the previous line in the person's slot came after
	for _, l := range s.said {
		if l.side {
			continue
		}
		acted := l.after > last
		last = l.after
		if l.after < 0 || !acted {
			continue
		}
		a := &s.acts[l.after]
		switch {
		case l.sig == trace.SignalInterrupt:
			out = append(out, s.correction(a, "interrupt", l.step))
		case l.sig == trace.SignalNone && l.orig == trace.OriginPerson && pushback != nil && pushback(strings.TrimSpace(l.text)):
			out = append(out, s.correction(a, "pushback", l.step))
		}
	}
	return out
}

func (s *session) correction(a *act, detail string, at int) Episode {
	k := key(Correction, a.sig, scopeOf(a.paths))
	k.Detail = detail
	return Episode{Key: k, Session: a.session, At: s.steps[at].At, From: a.step, To: at, Evidence: []int{at}}
}

// reverts finds SelfRevert episodes: an edit whose replacement undoes an
// earlier edit's in the same file (it puts back the old text in place of the
// new).
func reverts(s *session) []Episode {
	var out []Episode
	type done struct {
		act int
		c   trace.Change
	}
	var prior []done
	for i := range s.acts {
		a := &s.acts[i]
		if a.side || !a.passed() || a.sig.Class != trace.ActEdit {
			continue
		}
		for _, c := range a.call.Changes {
			for _, p := range prior {
				if p.c.Path == c.Path && p.c.Old != "" && p.c.Old == c.New && p.c.New == c.Old {
					k := key(SelfRevert, a.sig, scopeOf(a.paths))
					out = append(out, Episode{Key: k, Session: a.session, At: a.at,
						From: s.acts[p.act].step, To: a.end, Evidence: []int{a.step}})
				}
			}
		}
		for _, c := range a.call.Changes {
			prior = append(prior, done{i, c})
		}
	}
	return out
}

// loops finds RetryLoop episodes: the identical call made again after it
// failed the identical way, two or more times in a row.
func loops(s *session, b Baseline) []Episode {
	var out []Episode
	for i := 0; i < len(s.acts); {
		a := &s.acts[i]
		if a.side || !a.failed() {
			i++
			continue
		}
		fp, _ := Fingerprint(a.sig, a.result, b)
		j := i + 1
		for j < len(s.acts) && s.acts[j].failed() && same(&s.acts[j], a) {
			if f, _ := Fingerprint(s.acts[j].sig, s.acts[j].result, b); f != fp {
				break
			}
			j++
		}
		if j-i >= 3 {
			e := s.episode(RetryLoop, i, b)
			e.To = s.acts[j-1].end
			e.Evidence = nil
			for k := i; k < j; k++ {
				e.Evidence = append(e.Evidence, s.acts[k].end)
			}
			out = append(out, e)
		}
		i = j
	}
	return out
}

// same is whether two calls are the identical call: the same tool with the
// same input.
func same(a, b *act) bool {
	return a.call.Tool == b.call.Tool && string(a.call.Input) == string(b.call.Input)
}

func sortEpisodes(es []Episode) {
	slices.SortStableFunc(es, func(a, b Episode) int {
		if a.From != b.From {
			return a.From - b.From
		}
		return strings.Compare(string(a.Key.Kind), string(b.Key.Kind))
	})
}
