//go:build unix

package peakrss

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
)

// Reaper is a built reaper binary; Run measures a child through it. Its
// path is unix-only: the !unix build's Reaper (peakrss_other.go) never
// builds or runs one, since NewReaper there returns ErrUnavailable first.
type Reaper struct{ path string }

// reaperPkg is the reaper's import path, built by NewReaper.
const reaperPkg = "github.com/altrace-dev-role/rashomon/test/perf/peakrss/reaper"

// NewReaper builds the reaper into dir with the release flags, so its own
// peak is the smallest a Go binary that starts a child has.
func NewReaper(dir string) (*Reaper, error) {
	path := filepath.Join(dir, "peakrss-reaper")
	build := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", path, reaperPkg)
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=")
	if out, err := build.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("peakrss: building the reaper: %v\n%s", err, out)
	}
	return &Reaper{path: path}, nil
}

// Run runs cmd, an unstarted command with no extra files, under the reaper
// and returns cmd's peak RSS in MiB. cmd's stdin, stdout, stderr, environment
// and directory reach the child unchanged; a child that exits non-zero is an
// *exec.ExitError with its exit code, as from cmd.Run.
func (r *Reaper) Run(cmd *exec.Cmd) (float64, error) {
	if cmd.Err != nil {
		return 0, cmd.Err
	}
	if cmd.Process != nil || len(cmd.ExtraFiles) > 0 {
		return 0, errors.New("peakrss: Run takes an unstarted command with no extra files")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return 0, err
	}
	defer pr.Close()
	cmd.Args = append([]string{r.path, cmd.Path}, cmd.Args...)
	cmd.Path = r.path
	cmd.ExtraFiles = []*os.File{pw} // descriptor 3 in the reaper
	startErr := cmd.Start()
	closeErr := pw.Close() // the parent's copy: the reaper's exit is the report's EOF
	if startErr != nil {
		return 0, startErr
	}
	waitErr := cmd.Wait()
	report, readErr := io.ReadAll(io.LimitReader(pr, 4096))
	if err := errors.Join(closeErr, readErr); err != nil {
		return 0, err
	}
	child, reaper, err := parseReport(report)
	if err != nil {
		return 0, errors.Join(err, waitErr)
	}
	if waitErr != nil {
		return child, waitErr
	}
	if err := attributable(child, reaper, runtime.GOOS); err != nil {
		return 0, err
	}
	return child, nil
}

// Reap is the reaper: it runs args[0] with argv args[1:] on this process's
// stdin, stdout and stderr, writes "peak <child MiB> <reaper MiB>" to report,
// and returns the child's exit status for the reaper to exit with (128 plus
// the signal for a child a signal ended, as a shell reports it).
func Reap(args []string, report *os.File) int {
	if report == nil {
		return 126
	}
	// The report is not closed: the reaper exits on return, which closes it.
	syscall.CloseOnExec(int(report.Fd())) // the child never holds the report open
	if len(args) < 2 {
		return fail(report, errors.New("no command"))
	}
	cmd := &exec.Cmd{Path: args[0], Args: args[1:], Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
	runErr := cmd.Run()
	if cmd.ProcessState == nil {
		return fail(report, runErr)
	}
	child, err := fromState(cmd.ProcessState)
	if err != nil {
		return fail(report, err)
	}
	self, err := selfPeakMiB()
	if err != nil {
		return fail(report, err)
	}
	if _, err := fmt.Fprintf(report, "peak %g %g\n", child, self); err != nil {
		return 126
	}
	code := cmd.ProcessState.ExitCode()
	if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		code = 128 + int(ws.Signal())
	}
	return code
}

// fail writes why the child has no reading to report and returns the
// reaper's own status: 125 when the reason was written, 126 when it could not
// be (the parent then finds no report, which is an error too).
func fail(report io.Writer, err error) int {
	if _, werr := fmt.Fprintf(report, "error: %v\n", err); werr != nil {
		return 126
	}
	return 125
}

// fromState returns the peak RSS, in MiB, of the process ps describes. ps
// must be a finished process's state.
func fromState(ps *os.ProcessState) (float64, error) {
	if ps == nil {
		return 0, errors.New("no process state: the child did not run")
	}
	ru, ok := ps.SysUsage().(*syscall.Rusage)
	if !ok || ru == nil {
		return 0, errors.New("no rusage for the child")
	}
	return fromMaxrss(int64(ru.Maxrss), runtime.GOOS), nil
}

// selfPeakMiB is the reaper's own peak: on Linux the address space's
// high-water mark (VmHWM), which leaves out the parent's peak that
// getrusage(RUSAGE_SELF) carries there; elsewhere ru_maxrss.
func selfPeakMiB() (float64, error) {
	if runtime.GOOS == "linux" {
		f, err := os.Open("/proc/self/status")
		if err != nil {
			return 0, err
		}
		defer f.Close()
		return vmHWM(f)
	}
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0, err
	}
	return fromMaxrss(int64(ru.Maxrss), runtime.GOOS), nil
}
