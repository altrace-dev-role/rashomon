package store

import (
	"encoding/json"
	"path/filepath"
)

// LatestRecordBytes is the last line of a run's records.ndjson whose head
// (runHead) names kind at a version AcceptsKind accepts, exactly as it was
// written and without its newline. A receipt digests a record's stored bytes
// (5.5), and a re-marshal of the decoded record could differ from them, so
// only each line's head is decoded and the line itself is handed back, as
// ReadSteer leaves its lines' decoding to their reader. A line that does not
// decode, a torn tail among them, counts in skipped. A run with no records
// file, or no record of the kind, is ok false.
//
// The file is read unlocked, as ReadRunDir and ReadSteer read: a line caught
// mid-write counts in skipped, and where locks are mandatory (Windows) a read
// the writer's lock refused counts as one.
func (s *Store) LatestRecordBytes(sessionID, kind string) (line []byte, ok bool, skipped int, err error) {
	fn := func(l []byte) {
		var head runHead
		if len(l) == 0 || json.Unmarshal(l, &head) != nil {
			skipped++
			return
		}
		if head.Type == kind && AcceptsKind(kind, head.SchemaVersion) {
			line, ok = l, true
		}
	}
	err = refusedAsSkipped(eachLine(filepath.Join(s.RunDir(sessionID), FileRecords), fn), s.refusal(), fn)
	return line, ok, skipped, err
}
