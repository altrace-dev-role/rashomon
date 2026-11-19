# READ-SAFE1a, the pgraph module's sites. A sync hashes each source file
# through openRegular (pgraph/ingest.go): an open that does not wait, then a
# refusal of anything the descriptor's fstat does not show as a regular file,
# so a named pipe at go.sum neither holds the sync nor enters it as an empty
# file (R18). writeAtomic (pgraph/cmd/pgraph/main.go) sets a written file's
# 0644 on the open handle, never by name (R20 reads the mode, R22 pins that
# no mode is changed by path). Each row drops one of those pieces. All three
# tests are the pgraph module's own, which `go test ./...` from the root
# never enters, so the rows run there. Written, not run: DEFERRED TO THE END
# SWEEP (A61).
MODULE = "pgraph"
I = "pgraph/ingest.go"
W = "pgraph/cmd/pgraph/main.go"
R18 = "TestRS1a_R18_LoadGoSumNamedPipeReturnsAtOnce"
m("READ-SAFE1a a source file's open waits for a writer", I,
  "os.O_RDONLY|syscall.O_NONBLOCK, 0)", "os.O_RDONLY|0*syscall.O_NONBLOCK, 0)", R18)
m("READ-SAFE1a a source file that is not regular is hashed", I,
  "if err == nil && !fi.Mode().IsRegular() {", "if false && err == nil && !fi.Mode().IsRegular() {", R18)
m("READ-SAFE1a a written sync file keeps createTemp's 0600", W,
  "\t\terr = f.Chmod(0o644)\n", "\t\terr = nil\n", "TestRS1a_R20_SyncFilesReadMode0644")
m("READ-SAFE1a a written sync file's mode is set by its name", W,
  "\t\terr = f.Chmod(0o644)\n", "\t\terr = os.Chmod(tmp, 0o644)\n", "TestRS1a_R22_NoModeChangeByPath")
