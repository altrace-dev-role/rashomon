# RES-28: the peak every RSS gate compares is ru_maxrss read in its
# platform's unit; a unit slip moves every gate by a factor of 1024.
m("RES-28 ru_maxrss is read as bytes on Linux", "test/perf/peakrss/peakrss.go",
  "\t\trss *= 1024\n", "", "TestRES28i_")
m("RES-28 ru_maxrss is read as KiB on darwin", "test/perf/peakrss/peakrss.go",
  '\tif goos != "darwin" && goos != "ios" {\n', '\tif goos != "ios" {\n', "TestRES28i_")
# On Linux a child's ru_maxrss is at least its vforking parent's peak, so
# every RSS gate reads the child through a reaper that holds nothing;
# measuring directly, or trusting a reading the reaper could own, puts the
# test binary's peak in every gate. The first and fourth turn red only on
# Linux, where the defect lives.
m("RES-28 the child is measured directly, not through the reaper", "test/perf/peakrss/peakrss_unix.go",
  "\tpr, pw, err := os.Pipe()\n",
  "\tif err := cmd.Run(); err != nil {\n\t\treturn 0, err\n\t}\n\treturn fromState(cmd.ProcessState)\n\tpr, pw, err := os.Pipe()\n",
  "TestRES28i_")
m("RES-28 the reaper reports its own peak as the child's", "test/perf/peakrss/peakrss_unix.go",
  '\tif _, err := fmt.Fprintf(report, "peak %g %g\\n", child, self); err != nil {\n',
  '\tif _, err := fmt.Fprintf(report, "peak %g %g\\n", self+0*child, self); err != nil {\n', "TestRES28i_")
m("RES-28 the reaper exits 0 whatever the child did", "test/perf/peakrss/peakrss_unix.go",
  "\treturn code\n}\n", "\treturn code * 0\n}\n", "TestRES28i_")
m("RES-28 the reaper's own peak on Linux is read from rusage, which carries the parent's", "test/perf/peakrss/peakrss_unix.go",
  '\tif runtime.GOOS == "linux" {\n\t\tf, err := os.Open("/proc/self/status")\n',
  '\tif false {\n\t\tf, err := os.Open("/proc/self/status")\n', "TestRES28i_")
m("RES-28 a reading at the reaper's floor is taken as the child's", "test/perf/peakrss/peakrss.go",
  '\tif goos == "linux" && child <= reaper {\n', '\tif goos == "linux" && child < reaper {\n', "TestRES28i_")
m("RES-28 a below-floor reading does not wrap ErrBelowFloor", "test/perf/peakrss/peakrss.go",
  ", so it may be the reaper's: %w\", child, reaper, ErrBelowFloor)",
  ", so it may be the reaper's: %v\", child, reaper, ErrBelowFloor)", "TestRES28i_")
m("RES-28 a report that is not one peak line reads as a peak", "test/perf/peakrss/peakrss.go",
  '\tif len(f) != 3 || f[0] != "peak" ||', '\tif len(f) < 3 || f[0] != "peak" ||', "TestRES28i_")
m("RES-28 VmHWM's kB are read as thousands", "test/perf/peakrss/peakrss.go",
  "\t\treturn float64(kib) / 1024, nil\n", "\t\treturn float64(kib) / 1000, nil\n", "TestRES28i_")
# The harness reads the child's rusage itself again, as it did before the
# reaper (rss_test.go is perf-tagged, so the sweep sees this through the
# source pin; the perf run's live check reads it on Linux). The call moved
# from maxRSSMiB into runOnce (RES-28s: medianRSS needs the error, not just
# a Fatal), so the anchor and the detector's function name moved with it.
m("RES-28 the harness reads the child's rusage in its own process", "test/perf/rss_test.go",
  "\trss, err = reaper.Run(cmd)\n",
  "\terr = cmd.Run()\n\tif err == nil {\n\t\tru, _ := cmd.ProcessState.SysUsage().(*syscall.Rusage)\n\t\trss = float64(ru.Maxrss) / 1024\n\t\tif runtime.GOOS == \"darwin\" {\n\t\t\trss /= 1024\n\t\t}\n\t}\n",
  "TestRES28p_")
IMPORTS["RES-28 the harness reads the child's rusage in its own process"] = ("test/perf/rss_test.go", '\t"strings"\n\t"testing"\n', '\t"strings"\n\t"syscall"\n\t"testing"\n')
