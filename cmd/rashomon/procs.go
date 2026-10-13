package main

import "runtime"

// oneProc gives the runtime one P for the rest of a hook or post process.
// These entries run one goroutine of their own and, for typical payloads, no
// GC cycle, so a second P has nothing of theirs to run; what the extra Ps did,
// measured, was cost memory that grew with contention.
//
// Measured on an Apple M3 under load about 4 (2026-10-13, RES-30, release
// build, darwin peak footprint): with the default of one P per core, solo
// hook-and-post pairs had a median peak of 3.58 to 3.63 MiB, and in a 64-way
// burst the median rose about 0.1 MiB and the largest of 128 processes
// reached 4.02 to 4.06 MiB, 1.10 to 1.14 times the solo median, which fails
// RES-30's per-process rule of 1.1. With this call, solo pairs measured 3.42
// to 3.53 MiB, the burst median stayed within 0.05 MiB of it, and the largest
// was 1.03 to 1.09 times it in fourteen RES-30 runs of fourteen. Hook and post
// wall time did not move beyond run-to-run noise.
//
// Why the extra Ps cost memory is inferred, not proven: each P keeps its own
// allocation caches, so a goroutine that resumes on another P after a long
// syscall or a preemption allocates from spans, and pages, that P had not yet
// touched; more threads also ran under contention. The runtime has already
// started on every P before main runs, so this call saves less than
// GOMAXPROCS=1 in the environment would (about 0.6 MiB a process), but a hook
// does not choose its environment.
//
// It is unconditional, as the soft memory limit is meant to be (TRU-50): a
// repository's settings env reaches hook processes, and a GOMAXPROCS set
// there must not decide a hook's footprint either.
func oneProc() { runtime.GOMAXPROCS(1) }
