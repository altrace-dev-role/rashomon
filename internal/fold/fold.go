// Package fold is the contract of runs/<dir>/fold.json: what a hook may
// remember about a run between processes so that it never re-reads the whole
// record to answer a per-call question.
//
// A hook process is new every time; without a fold every turn-end read is
// O(session), which the measurements in Archive 85.1.2 put at 3.6 KiB of RSS
// and 23 us per recorded call. The fold holds offsets into the record files
// plus the few derived counts the readers need, so a reader starts from the
// offset and reads only what arrived since. Its identity fields exist because
// an offset is only meaningful into the file it was taken from: a records
// file restored from a copy has a new inode, and a reader that trusted the
// old offset would read from the middle of a line. A forget or eviction
// rewrite does not change the inode -- it compacts the file in place
// (internal/store/gaps.go's doc on rewrite) -- which is why Cursor also
// carries SizeSeen: Validate catches that case by size, not by identity.
// Technical spec 8.2.2; Archive 85.3.3 to 85.3.5.
//
// Contract only (P-07): the file's shape and its cap. recap writes fold.json
// and post advances fold.mid.json from P-50 on.
package fold

import "encoding/json"

// The names rashomon writes in a run directory (runs/<dir>), exported so
// purge lists the same spellings the writers use. FileName is the fold,
// MidFileName the one post advances from P-50 on, and TempPrefix the start of
// the temporary name Write leaves between writing and renaming (its
// os.CreateTemp pattern is TempPrefix + "*").
const (
	FileName    = "fold.json"
	MidFileName = "fold.mid.json"
	TempPrefix  = "." + FileName + ".rashomon-"
)

// MaxBytes is the size past which a fold is not written: a fold that grew
// without bound would be the O(session) read it exists to replace.
const MaxBytes = 256 << 10

// Version is the fold's own schema version, carried as "v".
const Version = 1

// File is runs/<dir>/fold.json, 0600. The JSON keys are the lower-case field
// names, as the spec states them.
type File struct {
	V      int    `json:"v"`
	Writer string `json:"writer"`
	// Dev and Inode identify records.ndjson across a restore from a copy
	// or an eviction that replaces the file by rename: either changes the
	// inode, voiding every offset below. They are an extra signal, not
	// the only one a reader can rely on: store's own forget rewrites
	// records.ndjson in place on purpose (same file, same inode), so a
	// cursor also needs its own Generation (see Cursor) to catch that.
	Dev   uint64 `json:"dev"`
	Inode uint64 `json:"inode"`
	// Records, Spill and Coverage are how far each file has been folded.
	Records  Cursor `json:"records"`
	Spill    Cursor `json:"spill"`
	Coverage Cursor `json:"coverage"`
	// Gaps is how far the root gaps.ndjson has been folded for this run
	// (RES-49). digest.Build calls ReadGaps for every digest, which decodes
	// the WHOLE append-only root file (internal/store/read.go's ReadGaps);
	// without a cursor here, folding the three run files from an offset
	// would still leave Stop reading every gap ever recorded on every call.
	// Offset and Size are into gaps.ndjson exactly as Records' are into
	// records.ndjson; Seq is unused (a gap carries no seq of its own) and
	// stays zero.
	Gaps Cursor `json:"gaps"`
	// Prompts are the last 64 prompt marks, oldest first.
	Prompts []Mark `json:"prompts"`
	// Digest is what the digest command needs without a full read.
	Digest DigestSt `json:"digest"`
	// Verify is VERIFY's section, written by recap, at most 64 KiB (RES-20).
	// Raw, because its shape is VERIFY's contract and this package reads none
	// of it.
	Verify json.RawMessage `json:"verify"`
}

// Cursor is a position in one record file: the byte offset folded to, the
// last seq folded, the file size seen at that point (so a file that became
// shorter is detected before an offset past its end is trusted), and the
// rewrite generation seen at that point.
//
// Generation exists because a file's size and a File's Dev/Inode are not
// enough on their own: store.rewrite replaces a file's content in place
// rather than by rename, so neither changes when a rewrite voids every
// offset folded before it, including on a file a same-size rewrite leaves
// at the same length. A writer bumps the generation on every in-place
// rewrite; a cursor whose Generation is behind the file's current one is
// stale regardless of what its Offset and SizeSeen say. Records, Spill,
// Coverage and Gaps read different files, each with its own generation --
// this is also the only identity check Spill, Coverage and Gaps get, since
// File's Dev/Inode names records.ndjson alone. Wiring the counter itself is the
// store PR's; this package only names where a cursor keeps its copy.
type Cursor struct {
	Offset     int64 `json:"offset"`
	Seq        int64 `json:"seq"`
	SizeSeen   int64 `json:"size_seen"`
	Generation int64 `json:"generation"`
}

// Mark is one prompt's extent in the record.
type Mark struct {
	PromptID    string `json:"prompt_id"`
	FirstOffset int64  `json:"first_offset"`
	FirstSeq    int64  `json:"first_seq"`
	LastSeq     int64  `json:"last_seq"`
	// Stopped is set when a Stop has claimed the prompt, so the next Stop of
	// the same prompt is known to be a repeat.
	Stopped bool `json:"stopped"`
}

// DigestSt is the digest command's view of the run, folded.
type DigestSt struct {
	StartRecorded    bool   `json:"start_recorded"`
	HookEntryAtStart string `json:"hook_entry_at_start"`
	Skipped          int    `json:"skipped"`
	Calls            int    `json:"calls"`
}

// MaxPrompts is how many prompt marks a fold keeps.
const MaxPrompts = 64

// VerifyMaxBytes caps the verify section (RES-20).
const VerifyMaxBytes = 64 << 10
