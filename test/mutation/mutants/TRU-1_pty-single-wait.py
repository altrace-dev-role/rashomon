# TRU-1: the pseudo-terminal harness reaps its child with one cmd.Wait, made
# by the goroutine reap; wait and the cleanup learn the exit from a channel
# that goroutine closes. os/exec sends the result of the copies of stdout and
# stderr once, on a channel that holds one value, and every Wait receives from
# it, so a second Wait leaves one of the two blocked for good: a test that
# hangs to the go test alarm, which the sweep reads as no verdict. The row
# puts the second Wait back in the cleanup; the pin of the kill path judges it
# by its own deadline.
m("TRU-1 the PTY harness reaps a killed child with a second cmd.Wait", "test/acceptance/pty_unix_test.go",
  "\ts.cmd.Process.Kill() //nolint:errcheck // the test has already failed\n",
  "\ts.cmd.Process.Kill() //nolint:errcheck // the test has already failed\n\ts.cmd.Wait() //nolint:errcheck // reaping only\n",
  "TestPTYHarness_TheKillPathReapsTheChildOnce")
