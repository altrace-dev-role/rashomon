package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/host"
)

// LockName is the lock file beside the baselines, taken around every
// load-modify-save of one (store.LockPath). One lock for the directory, not
// one per file: renders and forgets are rare, and a forget walks every file.
const LockName = ".lock"

// maxBaselineHosts caps one project's baseline (RES-11; spec archive 85.3.7).
// A project names tens to a few hundred distinct hosts over its life, so the
// cap is reached only by a project that names hosts without end -- generated
// names, or a crawl -- and there the file would otherwise grow with every
// render, which reads and rewrites it whole. Past the cap the least recently
// seen hosts go (evict): a host not named for the longest time is the one
// whose return is least misleading to call new.
const maxBaselineHosts = 4096

// maxSessionIDBytes bounds a session id as foldOrigin stores it, in either
// of the two fields that hold one (entry.FirstSeenSession,
// File.EstablishedBySession). Nothing upstream caps a session id's length
// the way a hostname is capped (host.Shaped) -- it is free text from the
// hook payload, bounded only by hookio.MaxPayloadBytes (8 MiB) -- so
// maxEntryBytes's own math below used to ASSUME a 256-byte id without this
// package enforcing it anywhere. clampSessionID makes that assumption true
// by construction: foldOrigin bounds sessionID once, before either field is
// set from it, rather than each of the two call sites re-deriving the same
// bound.
const maxSessionIDBytes = 256

// clampSessionID is a session id as this package stores it: as it is when
// it is at most maxSessionIDBytes and JSON writes every byte of it as itself
// (jsonPlain), and otherwise a 34-byte digest of it. A cap on its length
// alone did not bound what save writes: JSON writes `<`, a control byte or a
// byte of invalid UTF-8 as six, so a 256-byte id took 1,536. A session id is
// an opaque comparison key here, never displayed, and the same input gives
// the same digest every time, so a later render of the same session still
// compares equal to what an earlier one stored.
func clampSessionID(id string) string {
	if len(id) <= maxSessionIDBytes && jsonPlain(id) {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return "x-" + hex.EncodeToString(sum[:16])
}

// jsonPlain reports whether encoding/json writes every byte of s as itself:
// printable ASCII other than the quote and backslash, and the <, > and & it
// escapes for HTML.
func jsonPlain(s string) bool {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c < 0x20 || c > 0x7e, c == '"', c == '\\', c == '<', c == '>', c == '&':
			return false
		}
	}
	return true
}

// maxEntryBytes bounds one host's entry as save writes it: a 253-byte name,
// a session id of up to maxSessionIDBytes that JSON writes byte for byte
// (clampSessionID), two 13-digit stamps, and the keys and indentation, about
// 140 bytes -- about 680 in all, under 1,024. A host is hostname-shaped
// (host.Shaped), so it is written byte for byte too. maxBaselineBytes is the file at the cap of such entries, plus
// 64 KiB for the header and its project key, a path. A file larger than that
// is not one this package wrote, and is read as corrupt rather than read
// whole; a save that would write one is refused rather than leaving a file
// the next render cannot read.
//
// The spec's 256 KiB per file (archive 85.3.7) does not hold beside its
// 4,096 hosts: an entry takes about 150 bytes as written, so 4,096 of them
// take about 600 KiB, and at 256 KiB the effective cap would be about 1,700.
const (
	maxEntryBytes    = 1 << 10
	maxBaselineBytes = maxBaselineHosts*maxEntryBytes + 64<<10
)

// ErrTooManyHosts is Update's answer for a session that alone names more
// hosts than the cap: it cannot be folded without dropping hosts it named,
// so its novelty is unknown rather than wrong, and nothing is written.
var ErrTooManyHosts = errors.New("baseline: the session names more hosts than a baseline holds")

// errTooLarge refuses a baseline file over maxBaselineBytes, read or written.
var errTooLarge = errors.New("baseline: file larger than a baseline can be")

// Fold folds one session's hosts into a project's baseline f and reports
// which were new. It is pure: f is not modified, and the file it returns is
// the one Update saves when changed is true. existed is false for a project
// with no baseline yet, which this session then establishes.
//
// Ownership is earliest-session-wins (see the package note): a host goes to
// the earliest session that named it, whichever order sessions are folded
// in. Each host also records the latest session that named it, which decides
// eviction at the cap.
func Fold(f File, existed bool, sessionID string, sessionStart time.Time, hosts []string) (File, Result, bool) {
	return foldOrigin(f, existed, sessionID, sessionStart, hosts, nil)
}

// FoldOrigin is Fold, telling it which of hosts were named only through an
// ssh-family destination; see UpdateOrigin and entry.SSH.
func FoldOrigin(f File, existed bool, sessionID string, sessionStart time.Time, hosts []string, sshHosts map[string]bool) (File, Result, bool) {
	return foldOrigin(f, existed, sessionID, sessionStart, hosts, sshHosts)
}

func foldOrigin(f File, existed bool, sessionID string, sessionStart time.Time, hosts []string, sshHosts map[string]bool) (File, Result, bool) {
	// Bounded once, here, before establish and the per-host loop below each
	// store it: see clampSessionID.
	sessionID = clampSessionID(sessionID)
	stamp := sessionStart.UTC().UnixMilli()
	out := f
	out.Hosts = maps.Clone(f.Hosts)
	if out.Hosts == nil {
		out.Hosts = map[string]entry{}
	}
	changed := establish(&out, existed, sessionID, stamp)
	res := Result{Established: out.EstablishedBySession != "" && out.EstablishedBySession == sessionID}

	named := map[string]bool{}
	for _, h := range hosts {
		if h != "" {
			named[h] = true
		}
	}
	if len(named) > maxBaselineHosts {
		res.Err = ErrTooManyHosts
		return f, res, false
	}
	for h := range named {
		prev, seen := out.Hosts[h]
		next := prev
		switch {
		case !seen:
			next = entry{FirstSeenSession: sessionID, FirstSeenAtUnixMS: stamp}
		case stamp > 0 && (prev.FirstSeenAtUnixMS == 0 || stamp < prev.FirstSeenAtUnixMS):
			// An earlier session is being rendered after a later one. The
			// earlier one owns the host; correcting toward it is what keeps a
			// session's novel set a property of the session rather than of the
			// order somebody read the reports in.
			next.FirstSeenSession, next.FirstSeenAtUnixMS = sessionID, stamp
		}
		next.LastSeenAtUnixMS = max(next.LastSeenAtUnixMS, stamp)
		next.SSH = sshHosts[h]
		if !seen || next != prev {
			out.Hosts[h] = next
			changed = true
		}
	}
	res.Evicted = evict(out.Hosts, named)
	changed = changed || res.Evicted > 0
	res.Known = len(out.Hosts)

	// Novel is computed from the file AFTER folding, so it is exactly "the
	// hosts this session owns" -- stable across renders by construction.
	for h := range named {
		if e := out.Hosts[h]; !slices.Contains(ubiquitous, h) && e.FirstSeenSession == sessionID && !res.Established {
			res.Novel = append(res.Novel, h)
		}
	}
	sort.Strings(res.Novel)
	return out, res, changed
}

// establish records the session that established f, when it is the first or
// an earlier one than the recorded establisher, and reports whether f
// changed.
func establish(f *File, existed bool, sessionID string, stamp int64) bool {
	switch {
	case !existed:
		f.EstablishedBySession, f.EstablishedAtUnixMS = sessionID, stamp
		return true
	case f.EstablishedBySession != "" && stamp > 0 &&
		(f.EstablishedAtUnixMS == 0 || stamp < f.EstablishedAtUnixMS):
		// An earlier session than the recorded establisher. It was the project's
		// first, so it takes the slot -- the same earliest-wins correction the
		// per-host entries make, for the same reason: the answer must be a
		// property of the sessions and not of the order someone read them in.
		// Monotone, so it converges rather than oscillating.
		//
		// Guarded on a recorded establisher EXISTING. A file written before this
		// field names nobody, and "nobody established it" must not become "the
		// next session to render claims it" -- that would hand the slot, and the
		// suppression of every novelty finding that comes with it, to whoever
		// happened to run report first on an upgraded install.
		f.EstablishedBySession, f.EstablishedAtUnixMS = sessionID, stamp
		return true
	}
	return false
}

// evict removes the least recently seen hosts past maxBaselineHosts, never
// one in keep -- the hosts the folding session named, which are what it
// reports on -- and returns how many it removed. Ties go by name, so two
// folds of the same file evict the same hosts.
func evict(hosts map[string]entry, keep map[string]bool) int {
	over := len(hosts) - maxBaselineHosts
	if over <= 0 {
		return 0
	}
	cands := make([]string, 0, len(hosts))
	for h := range hosts {
		if !keep[h] {
			cands = append(cands, h)
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		a, b := lastSeen(hosts[cands[i]]), lastSeen(hosts[cands[j]])
		if a != b {
			return a < b
		}
		return cands[i] < cands[j]
	})
	// keep never holds more than the cap (Fold refuses such a session), so
	// there are always enough candidates; min keeps that a fact here too.
	over = min(over, len(cands))
	for _, h := range cands[:over] {
		delete(hosts, h)
	}
	return over
}

// lastSeen is when a host was last named: its first sighting on an entry
// written before the field existed.
func lastSeen(e entry) int64 {
	if e.LastSeenAtUnixMS > 0 {
		return e.LastSeenAtUnixMS
	}
	return e.FirstSeenAtUnixMS
}

// canonicalizeHosts re-keys a loaded baseline through host.Canonical and
// host.Shaped, merging what collapses together and dropping what is
// refused.
//
// A file written before host.Canonical folded an IP literal to its one
// spelling holds the uncanonicalised string -- "167838211" where a session
// now names "10.1.2.3" -- and compared as stored, a host the baseline
// already knew read as novel again. A key host.Canonical cannot read, or
// that host.Shaped refuses once canonicalised, is dropped: it was never a
// host, only something authority() stopped scanning at before this
// package's own shape check existed downstream of it.
//
// Collision keeps the earliest FirstSeenSession and the later of the two
// LastSeenAtUnixMS (lastSeen), the same ownership rule Fold applies when two
// sessions name one host. Keys are walked in sorted order so that a tie --
// both entries predate the LastSeenAtUnixMS field, or name the same instant
// -- resolves the same way on every render rather than on map iteration
// order.
func canonicalizeHosts(hosts map[string]entry) map[string]entry {
	keys := make([]string, 0, len(hosts))
	for h := range hosts {
		keys = append(keys, h)
	}
	sort.Strings(keys)
	out := make(map[string]entry, len(hosts))
	for _, h := range keys {
		e := hosts[h]
		// An entry marked SSH (entry.SSH) was named only through an
		// ssh-family destination, never through a URL a client resolves, so
		// it is re-keyed without the inet_aton fold (host.CanonicalName):
		// folding "1234" to 0.0.4.210 here would do on every load exactly
		// what fixing the ssh extraction path alone could not undo. A
		// legacy entry with no field at all is SSH == false and keeps the
		// fold, which is what TestRubricLLD11_UpdateLegacyIPLiteralIsNotNovelAgain and
		// TestRubricLLD11_LoadMergesCollidingLegacyKeys pin for a file old enough to
		// predate the distinction.
		canonicalize := host.Canonical
		if e.SSH {
			canonicalize = host.CanonicalName
		}
		canon, ok := canonicalize(h)
		if !ok || !host.Shaped(canon) {
			continue
		}
		prev, seen := out[canon]
		if !seen {
			out[canon] = e
			continue
		}
		next := prev
		if e.FirstSeenAtUnixMS > 0 && (prev.FirstSeenAtUnixMS == 0 || e.FirstSeenAtUnixMS < prev.FirstSeenAtUnixMS) {
			next.FirstSeenSession, next.FirstSeenAtUnixMS = e.FirstSeenSession, e.FirstSeenAtUnixMS
		}
		next.LastSeenAtUnixMS = max(lastSeen(prev), lastSeen(e))
		// Conservative once either contributor is SSH: a merged entry stays
		// un-foldable rather than reverting to a fold neither spelling may
		// have earned.
		next.SSH = prev.SSH || e.SSH
		out[canon] = next
	}
	return out
}

// readBounded reads a baseline file, refusing one over maxBaselineBytes.
func readBounded(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBaselineBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBaselineBytes {
		return nil, errTooLarge
	}
	return body, nil
}
