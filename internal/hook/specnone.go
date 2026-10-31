package hook

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The no-spec marker's two ends (card SPEC-RULES1, D210). The prompt's
// preflight, which discovers the spec when the prompt names a path or an
// identifier, calls NoteSpecFound with what it found: none is noted for the
// turn, anything else clears the note. The turn's posts consult it in judge,
// for a rules-only call, and skip a second discovery of the same nothing.
//
// Within a turn the note can be stale only if a spec appeared after the
// preflight looked. A spec that appeared that way is untrusted by
// construction: only a person at a terminal binds a trust (`rashomon spec
// trust`, refused to an agent session), so the worst a stale note costs is
// an untrusted spec's user flag arriving at the Stop instead of at the call.
// Stop never reads the note: it discovers fresh, as does every call that is
// not a rules-only one, and every call that names .claude/rashomon clears the
// note first. A note of another turn or another directory, or one that is
// missing or unreadable, is no note: the call runs the real discovery. A
// found spec is never noted, and neither is a trust.

// specNoneDigest is the keyed digest of the directory a discovery is made
// from: the store's key over the cwd and the two directories the environment
// adds, so the marker holds no path and a call from elsewhere does not match.
func specNoneDigest(key []byte, cwd, projectDir, managedDir string) string {
	h := sha256.New()
	h.Write(key)
	for _, s := range [3]string{cwd, projectDir, managedDir} {
		h.Write([]byte{0})
		h.Write([]byte(s))
	}
	var sum [sha256.Size]byte
	return hex.EncodeToString(h.Sum(sum[:0]))
}

// NoteSpecFound is the prompt preflight's note of its discovery for the
// session's turn promptID, made from cwd: a nil sp is noted as none, any
// other clears the note. It is best effort: a write that fails is a missed
// skip and nothing else.
func NoteSpecFound(st *store.Store, sessionID, promptID, cwd, projectDir, managedDir string, sp *spec.Compiled) {
	if sp != nil {
		st.ClearNoSpec(sessionID)
		return
	}
	_ = st.NoteNoSpec(sessionID, promptID, specNoneDigest(st.Key(), cwd, projectDir, managedDir))
}

// SpecNoted reports that the turn's preflight found no spec from cwd. The
// steer step's rules-only calls are its one reader: nothing else asks.
func SpecNoted(st *store.Store, sessionID, promptID, cwd, projectDir, managedDir string) bool {
	return st.NoSpecNoted(sessionID, promptID, specNoneDigest(st.Key(), cwd, projectDir, managedDir))
}

// noSpecNoted is SpecNoted for this call's turn and directory, so a
// rules-only call has no spec to judge against.
func (p *Post) noSpecNoted(env SteerEnv) bool {
	return SpecNoted(p.st, p.sessionID, p.promptID, p.cwd, env.ProjectDir, env.ManagedDir)
}
