package store

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
)

// HostDigest is the keyed identifier a gap record carries for a forgotten
// host.
//
// HMAC under the per-install key, for the reason set out on Gap.HostDigest: the
// report has to recognise the host again while the store must not hold its
// name. Two installs produce different digests for the same host, so a store
// cannot be tested against a dictionary of hostnames.
//
// It is "" for a Store without a key of the install's length: the zero Store
// would otherwise digest under an empty key, which anyone can recompute.
func (s *Store) HostDigest(host string) string {
	if host == "" || len(s.key) != keyLen {
		return ""
	}
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte("forget-host\x00"))
	m.Write([]byte(host))
	return hex.EncodeToString(m.Sum(nil))
}

// InstallDigest is the install as a share bundle names it (card 4.3):
// HMAC-SHA256 of the install id under the per-install key, in its own domain,
// hex, the shape HostDigest has. The report carries the id plain; a bundle the
// user hands on does not, and a receiver can tell one install's bundles from
// another's without learning either id. The domain is its own, as every HMAC
// this key signs has one, so the digest is no other digest of the same bytes.
//
// It is "" for a Store without a key of the install's length, as HostDigest is.
func (s *Store) InstallDigest() string {
	if s.installID == "" || len(s.key) != keyLen {
		return ""
	}
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte("share-install\x00"))
	m.Write([]byte(s.installID))
	return hex.EncodeToString(m.Sum(nil))
}

// ForgottenHost reports whether a host has been removed by a host-scoped
// forget, by recomputing its keyed digest and looking for it among the gaps
// and the store-level forget records.
//
// BOTH SOURCES. The store-level records cover every forget since they
// existed; the gaps cover the forgets made before, which wrote only a gap.
//
// Returned as a predicate rather than a set of names because the names are not
// recoverable: the caller asks about a host it already has, which is exactly
// what the report does for each destination it observed.
func (s *Store) ForgottenHost() (func(string) bool, error) {
	times, err := s.forgottenDigestTimes()
	if err != nil {
		return nil, err
	}
	if len(times) == 0 {
		// A predicate that allocates and hashes nothing on the overwhelmingly
		// common path where nothing has been forgotten.
		return func(string) bool { return false }, nil
	}
	return func(host string) bool { _, ok := times[s.HostDigest(host)]; return ok }, nil
}

// ForgottenHostBefore is ForgottenHost's time-scoped sibling.
//
// A host's own name is forgotten forever once any forget --host names it --
// ForgottenHost's predicate has no notion of time, because the sandbox trail
// it exists for has none either: nono's file is not this store's to edit, so
// every one of its past events stays exactly as reachable as the day they
// were written, and the predicate's job is to keep them out of the report
// regardless of when they are read.
//
// A declaration is different: it is this store's own record, forget --host
// removes it on success, and the DECLARATION'S OWN recorded_at_unix_ms says
// whether that removal had a chance to run before it was written. One a
// forget already decided to remove but could not -- a run whose file could
// not be opened, say -- predates the forget and is exactly the stale
// leftover #45 and TRU-58 exist to stop reappearing. One recorded after the
// forget is a fresh declaration of a name that happens to be the same, which
// is real information about THIS session and must not disappear because an
// earlier, unrelated session once asked to forget it -- the whole reason
// forget --host clears the project baseline rather than blocking the name.
func (s *Store) ForgottenHostBefore() (func(host string, declaredAtMS int64) bool, error) {
	times, err := s.forgottenDigestTimes()
	if err != nil {
		return nil, err
	}
	if len(times) == 0 {
		return func(string, int64) bool { return false }, nil
	}
	return func(host string, declaredAtMS int64) bool {
		forgotMS, ok := times[s.HostDigest(host)]
		return ok && declaredAtMS <= forgotMS
	}, nil
}

// forgottenDigestTimes reads both sources ForgottenHost and
// ForgottenHostBefore share -- the gaps a forget wrote on success, and the
// store-level record it writes whatever it removed -- and keeps, per digest,
// the LATEST recorded_at_unix_ms either source carries. Latest, not first,
// because a host forgotten more than once must compare against the most
// recent attempt: a declaration from between two forgets of the same host is
// still stale against the second one.
//
// It reads the gaps itself rather than through ReadGaps, because a gaps file
// it cannot read must fail the caller: ReadGaps counts a read refused by a
// held lock as a skipped line, and here that would read as "nothing was
// forgotten" and show a host the user removed. So it waits for the gaps
// lock first, up to lockBudget, and reads under it: a forget or an eviction
// holding the lock costs the report a wait rather than, where locks are
// mandatory (Windows), the report itself (test-11). A hold longer than that
// is read past where it can be (unix) and fails the report where it cannot.
// A damaged line is discarded with the count: ReadGaps, which every report
// reads first, has already marked every session unverified for it (REC-33).
// The store-level forget records are read the same way, under their own
// file's lock, which forget --host holds while it checks and appends.
func (s *Store) forgottenDigestTimes() (map[string]int64, error) {
	var gaps []Gap
	var skipped int
	err := eachLineTryLock(filepath.Join(s.root, FileGaps), lockBudget, neverRefused, gapLines(&gaps, &skipped))
	if err != nil {
		return nil, err
	}
	times := map[string]int64{}
	bump := func(digest string, candidateMS int64) {
		if digest == "" {
			return
		}
		if cur, ok := times[digest]; !ok || candidateMS > cur {
			times[digest] = candidateMS
		}
	}
	for _, g := range gaps {
		if g.Reason == GapForgetHost {
			bump(g.HostDigest, g.RecordedAtMS)
		}
	}
	err = eachLineTryLock(filepath.Join(s.root, FileForgottenHosts), lockBudget, neverRefused, func(line []byte) {
		var r ForgottenHostRecord
		if json.Unmarshal(line, &r) == nil && r.Type == TypeForgottenHost {
			bump(r.HostDigest, r.RecordedAtMS)
		}
	})
	if err != nil {
		return nil, err
	}
	return times, nil
}
