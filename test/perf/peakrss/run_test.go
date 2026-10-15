package peakrss

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// childEnv selects what the test binary does when the reaper starts it as a
// child.
const childEnv = "PEAKRSS_TEST_CHILD"

// reaper is built once for the package's tests; nil where NewReaper is
// unavailable, and those tests skip by name.
var reaper *Reaper

func TestMain(m *testing.M) {
	os.Exit(buildAndRun(m))
}

func buildAndRun(m *testing.M) int {
	if os.Getenv(childEnv) != "" {
		return m.Run()
	}
	dir, err := os.MkdirTemp("", "peakrss-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	reaper, err = NewReaper(dir)
	if err != nil && !errors.Is(err, ErrUnavailable) {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

// TestPeakRSSChild is the child the tests below measure; run directly, it
// skips.
func TestPeakRSSChild(t *testing.T) {
	switch os.Getenv(childEnv) {
	case "":
		t.Skip("the child of the RES28i tests")
	case "hold48":
		touch(48)
	case "exit3":
		os.Exit(3)
	}
	os.Exit(0)
}

// touch allocates mib MiB on the heap and writes one byte a page, so every
// page is resident.
func touch(mib int) []byte {
	b := make([]byte, mib<<20)
	for i := 0; i < len(b); i += os.Getpagesize() {
		b[i] = 1
	}
	return b
}

func child(mode string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestPeakRSSChild$")
	cmd.Env = append(os.Environ(), childEnv+"="+mode)
	return cmd
}

func runOrSkip(t *testing.T, cmd *exec.Cmd) (float64, error) {
	t.Helper()
	if reaper == nil {
		t.Skipf("%s: %v", runtime.GOOS, ErrUnavailable)
	}
	return reaper.Run(cmd)
}

// TestRES28i_AChildsPeakLeavesOutTheParents: a child that holds nothing
// reads as a child that holds nothing, however much its parent holds. On
// Linux, wait4's ru_maxrss for a child the parent vforked is at least the
// parent's own peak (exec records the shared mm's high-water mark into the
// child's signal struct), so a test parent holding 96 MiB read every child as
// 96 MiB and up. Break: measure the child directly rather than through the
// reaper, and on Linux this reads the parent.
func TestRES28i_AChildsPeakLeavesOutTheParents(t *testing.T) {
	ballast := touch(96)
	got, err := runOrSkip(t, child("none"))
	runtime.KeepAlive(ballast)
	if err != nil {
		t.Fatal(err)
	}
	if got >= 48 {
		t.Errorf("a child that held nothing read %.2f MiB beside a parent holding 96 MiB: the parent's peak is in the reading", got)
	}
	t.Logf("child peak RSS %.2f MiB beside a 96 MiB parent", got)
}

// TestRES28i_RunReportsTheChildsPeak: a child that holds 48 MiB reads as at
// least 48 MiB. Break: return the reaper's own peak, or drop the report, and
// this reads a few MiB or fails.
func TestRES28i_RunReportsTheChildsPeak(t *testing.T) {
	got, err := runOrSkip(t, child("hold48"))
	if err != nil {
		t.Fatal(err)
	}
	if got < 48 || got > 512 {
		t.Errorf("a child holding 48 MiB read %.2f MiB", got)
	}
}

// TestRES28i_RunKeepsTheChildsExitStatus: a child that exits 3 is an error
// whose exit code is 3; a child that cannot start, or a command that would
// shift the report's descriptor, is an error and never a reading. Break: let
// the reaper exit 0 whatever the child did, or report a peak for a child that
// never ran, and this fails.
func TestRES28i_RunKeepsTheChildsExitStatus(t *testing.T) {
	_, err := runOrSkip(t, child("exit3"))
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 3 {
		t.Errorf("a child that exited 3 gave %v", err)
	}
	got, err := runOrSkip(t, exec.Command("/nonexistent/peakrss-child"))
	if err == nil || !strings.Contains(err.Error(), "peakrss reaper:") {
		t.Errorf("a child that could not start read %.2f MiB, error %v", got, err)
	}
	cmd := child("none")
	cmd.ExtraFiles = []*os.File{os.Stdin}
	if _, err := runOrSkip(t, cmd); err == nil {
		t.Error("a command carrying its own extra files ran: the report's descriptor would shift")
	}
}

// TestRES28i_AReadingAtTheReapersFloorIsRefused: on Linux the child's reading
// is the larger of its own peak and the reaper's, so a reading at or under
// the reaper's is not the child's and fails closed; elsewhere the reading is
// the child's alone. A report that is not one well-formed peak line is an
// error, never a zero peak a gate would pass.
func TestRES28i_AReadingAtTheReapersFloorIsRefused(t *testing.T) {
	for _, c := range []struct {
		child, reaper float64
		goos          string
		ok            bool
	}{
		{6.5, 2.1, "linux", true},
		{2.1, 2.1, "linux", false},
		{1.9, 2.1, "linux", false},
		{1.1, 2.1, "darwin", true},
		{0, 2.1, "darwin", false},
	} {
		if err := attributable(c.child, c.reaper, c.goos); (err == nil) != c.ok {
			t.Errorf("attributable(%v, %v, %s) = %v, want ok=%v", c.child, c.reaper, c.goos, err, c.ok)
		}
	}
	for _, c := range []struct {
		report        string
		child, reaper float64
		ok            bool
	}{
		{"peak 6.5 2.25\n", 6.5, 2.25, true},
		{"error: fork/exec /x: no such file\n", 0, 0, false},
		{"", 0, 0, false},
		{"peak 6.5\n", 0, 0, false},
		{"peak x 2\n", 0, 0, false},
		{"peak 6.5 2 3\n", 0, 0, false},
	} {
		child, reaper, err := parseReport([]byte(c.report))
		if (err == nil) != c.ok || child != c.child || reaper != c.reaper {
			t.Errorf("parseReport(%s) = %v, %v, %v", strconv.Quote(c.report), child, reaper, err)
		}
	}
}

// TestRES28i_VmHWMIsReadInMiB: the reaper's own peak on Linux is the VmHWM
// line of /proc/self/status, in kB; a status with no such line, or one in
// another unit, is an error, never a zero floor that would let every reading
// through.
func TestRES28i_VmHWMIsReadInMiB(t *testing.T) {
	status := "Name:\treaper\nVmPeak:\t  712345 kB\nVmHWM:\t    2048 kB\nVmRSS:\t    2048 kB\n"
	if got, err := vmHWM(strings.NewReader(status)); err != nil || got != 2 {
		t.Errorf("vmHWM = %v, %v; want 2 MiB", got, err)
	}
	for _, bad := range []string{"Name:\treaper\nVmRSS:\t 2048 kB\n", "VmHWM:\t 2 MB\n", "VmHWM:\t x kB\n", ""} {
		if got, err := vmHWM(strings.NewReader(bad)); err == nil {
			t.Errorf("vmHWM(%q) = %v with no error", bad, got)
		}
	}
}
