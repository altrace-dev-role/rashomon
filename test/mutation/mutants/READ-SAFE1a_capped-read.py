# READ-SAFE1a, the capped reader's refusal stays (c3). capped.Read
# (internal/regfile/regfile.go) refuses every read once its count has passed
# the cap, with a count of 0. Without the top check, the read after the
# refusal asks the file for no bytes and returns a count of -1 with the
# refusal, which io.Reader forbids (bufio.Reader panics on a negative count).
# Written, not run: for the end sweep.
m("READ-SAFE1a a read after the capped refusal returns a negative count", "internal/regfile/regfile.go",
  "\tif c.n > c.max {\n\t\treturn 0, c.over()\n\t}\n", "",
  "TestRS1a_C3_OpenCappedReadAfterTheRefusalGivesZero")
