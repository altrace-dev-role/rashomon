// Package store is the on-disk record of declarations.
//
// records.ndjson is append-only and totally ordered by seq. Nothing rewrites a
// line in it except forget, and forget leaves a gap record behind, because a
// record that can be quietly corrected is worth nothing as evidence.
//
// # Concurrency
//
// Every hook invocation is a process of its own, and so is every command, so
// the store is shared between processes that know nothing of each other. What
// keeps them apart is one exclusive file lock per file a writer appends to
// (lockFile: flock on unix, which is advisory, and LockFileEx on Windows,
// which is mandatory and also refuses a read through another handle). A
// Store's own appends take the same locks, which is what orders goroutines
// sharing a Store.
//
// The files, and what each one's lock protects:
//
//   - runs/<dir>/records.ndjson: the ordered stream. Its lock covers the seq
//     counter (runs/<dir>/seq, which has no lock of its own and is only
//     written under this one) and the record that takes the seq, so seqs are
//     unique and contiguous. Writers wait lockBudget for it. forget holds it
//     from its plan to its last rewrite, so no record joins the run between
//     the two.
//   - runs/<dir>/spill.ndjson: what could not wait for the ordered stream.
//     Its writers wait spillBudget, 50 ms, and then append without the lock,
//     relying on a single O_APPEND write. forget takes it to read the file
//     again and remove only the lines its plan counted (rewriteSpill).
//   - runs/<dir>/coverage.ndjson: append-only; its lock orders the appends.
//   - gaps.ndjson: append-only; its lock orders the appends, and Evict holds
//     it for a whole pass, so one evictor runs at a time.
//   - install.key, install.json and the probe marker take no lock: the first
//     two are created whole under their final name or not at all
//     (createExclusive), and the marker is rewritten whole.
//
// The order is records -> gaps -> spill, and it is never reversed: forget
// holds a run's records lock, takes the gaps lock to write its gap, and takes
// the spill lock to rewrite the spill file; Evict holds only the gaps lock;
// nothing that holds the gaps or the spill lock takes a records lock. A hook
// invocation's waits share one deadline (LimitLockWaits), so the waits of
// its three appends cannot add up past the hook timeout.
//
// Reads take no lock, except one. ReadRunDir and ReadRun read a run's three
// files unlocked; their callers read after the fact or accept a line caught
// mid-write as one counted in Run.Skipped: report and spend, which read
// finished sessions; the end probe, which reads its own session as it ends;
// and eviction, which reads only runs nobody has written to for an hour.
// ReadGaps, Runs and NewestRun read unlocked too. ReadRunConsistent, digest's
// read of a session that may still be running, tries each file's lock inside
// one short budget for the whole read and reads anyway when it cannot get it
// (eachLineTryLock); a writer's own read of the records it is about to
// append to (maxSeq) is under the lock that writer holds.
//
// Eviction takes no run's records lock: it removes only runs that nothing
// has written to for evictGrace, an hour, and never the session that ran it.
// A session idle for longer that appends while its run is being removed
// loses that append if its file was already open, and otherwise starts the
// run again from seq 1, in either case after the run's gap record. That is
// an accepted risk, not a guarantee.
package store
