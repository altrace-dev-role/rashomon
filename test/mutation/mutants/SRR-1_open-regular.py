# SRR-1 (card STORE-READ-REGULAR1, BEFORE-TAG row 8a; #157 F3, #171 F4): a
# store file that is a named pipe must not hold the read. regfile.Open opens
# without blocking and refuses what is not a regular file on the descriptor it
# opened; ReadConfig, LatestRecordTail, EachRecordOfKind and recap's readState
# read through it, and readState reads at most maxStateBytes. The two
# latestrecord.go call sites carry no site mutant: that file imports no os, so
# a break back to os.Open does not build; the helper's two rows judge the pipe
# behaviour both sites read through.
RR = "TestSRR1_OpenRefusesWhatIsNotARegularFile|TestSRR1_OpenRefusesANamedPipeAtOnce|TestSRR1_AFIFOConfigReadsUnreadableAtOnce|TestSRR1_AFIFORecordsFileEndsTheTailReadAtOnce|TestSRR1_AFIFORecordsFileEndsTheKindWalkAtOnce"
m("SRR-1 a named pipe opens as a regular file", "internal/regfile/regfile.go",
  "\tif err == nil && !fi.Mode().IsRegular() {\n", "\tif err == nil && fi == nil {\n", RR)
m("SRR-1 the open waits on a named pipe for a writer", "internal/regfile/regfile.go",
  "os.O_RDONLY|syscall.O_NONBLOCK, 0)", "os.O_RDONLY|syscall.O_NONBLOCK&0, 0)", RR)
m("SRR-1 ReadConfig opens a named pipe and waits", "internal/store/config.go",
  "\tf, err := regfile.Open(filepath.Join(root, FileConfig))\n",
  # The import stays used, so the break builds (QA-1: BUILD BROKEN at 811656e0).
  "\tf, err := os.Open(filepath.Join(root, FileConfig))\n\t_ = regfile.Open\n",
  "TestSRR1_AFIFOConfigReadsUnreadableAtOnce")
m("SRR-1 recap.json's cap lets one byte over through", "internal/recap/state.go",
  "len(b) > maxStateBytes {", "len(b) > maxStateBytes+1 {", "TestSRR1_AnOverCapStateFileIsAnError")
# #179 F2: TRU-23 pins regfile at its one read, so a write added there is
# counted (it survives at 811656e0, where regfile was not pinned).
m("SRR-1 regfile gains a write call site", "internal/regfile/regfile.go",
  'var ErrNotRegular = errors.New("not a regular file")\n',
  'var ErrNotRegular = errors.New("not a regular file")\n\nvar _ = func() error { return os.WriteFile(os.DevNull, nil, 0o600) }\n',
  "TestTRU23_WriteCallSitesArePinned")
