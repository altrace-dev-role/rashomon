// Package posture reads the proxy's status file and decides whether it is safe
// to route a session's traffic through it.
//
// It exists as its own package so the DECISION is one auditable thing rather
// than a condition spread across a launcher. Getting it wrong has two failure
// modes and they are not symmetric: exporting proxy variables at something that
// is not an observe-mode Altrace proxy either hands a session's traffic to an
// unidentified process or points it at an enforce-mode proxy that will refuse
// the client's own API tunnels and break the session outright. Declining to
// export when it would have been fine costs one line of coverage in a report.
// So every uncertainty resolves to "do not export".
//
// It reads a FILE and checks a pid. No socket, no subprocess: this package is
// safe for anything in this program to import, including code on a hook path.
package posture

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// FileName is the proxy's status file, written by the observe profile into its
// own data directory.
const FileName = "status.json"

// Product is the value the proxy writes to identify itself. Matched exactly:
// something else may be listening on the port we were told about, and a
// prefix or case-insensitive match is a way for an impostor to qualify.
const Product = "altrace"

// ModeObserve is the only posture it is safe to export against.
//
// An ENFORCE-mode proxy refuses a CONNECT to a host outside its allowlist, and
// the client's own api.anthropic.com tunnel is such a host under a stock
// configuration. Measured: four refused CONNECTs in about three seconds and
// the client never reached the API. So exporting at an enforce proxy does not
// merely fail to record destinations, it stops the session working.
const ModeObserve = "observe"

// File is the proxy's status document.
type File struct {
	Product     string `json:"product"`
	ConnectMode string `json:"connect_mode"`
	ListenAddr  string `json:"listen_addr"`
	HealthAddr  string `json:"health_addr"`
	PID         int    `json:"pid"`
	StartedAt   string `json:"started_at"`
	CausalDB    string `json:"causal_db"`
}

// Verdict is the decision and the reason for it.
//
// The reason is not decoration. When the answer is "do not export", the user is
// about to run a session that records no destinations, and the only thing that
// makes that acceptable rather than mysterious is being told why in one line.
type Verdict struct {
	// Export is true only when every check passed.
	Export bool
	// Reason is always set, including on success, so a caller can print one
	// line either way rather than having two shapes of output.
	Reason string
	// File is the document that was read, when it was readable.
	File File
	// Path is where it was looked for, so a reason can name it.
	Path string
}

// DefaultPath is ~/.altrace/observe/status.json.
//
// Hard-coded rather than read from the proxy's configuration: this tool must
// not have to parse the closed product's config to do its job, and an operator
// who moved it passes --proxy-status.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".altrace", "observe", FileName)
}

// Read reads the status file at path and decides.
//
// Every failure is a refusal with a reason, never an error return: the caller's
// job is to launch the user's command either way, and a launcher that aborted
// because a status file was missing would be worse than one that runs without
// recording.
func Read(path string) Verdict {
	v := Verdict{Path: path}
	if path == "" {
		v.Reason = "no proxy status file path could be resolved (no home directory?)"
		return v
	}

	body, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		v.Reason = "no proxy status file at " + path +
			" -- the observe proxy does not appear to be running"
		return v
	case err != nil:
		v.Reason = "the proxy status file at " + path + " could not be read"
		return v
	}

	if err := json.Unmarshal(body, &v.File); err != nil {
		// Could be a torn read, could be something else's file entirely. Both
		// resolve the same way, and neither is worth guessing about.
		v.Reason = "the proxy status file at " + path + " is not valid JSON"
		return v
	}

	if v.File.Product != Product {
		// The most dangerous case, and the reason product is in the file at
		// all: something else is listening where we were told the proxy is.
		v.Reason = "the status file at " + path + " does not identify an altrace proxy"
		return v
	}
	if v.File.ConnectMode != ModeObserve {
		v.Reason = "the proxy is in " + v.File.ConnectMode +
			" mode, not observe -- exporting proxy variables at an enforcing proxy" +
			" would refuse the client's own API tunnels and break the session"
		return v
	}
	if v.File.ListenAddr == "" {
		v.Reason = "the proxy status file names no listen address"
		return v
	}
	if !alive(v.File.PID) {
		// The file survives a crash by design, so this is the check that makes
		// it trustworthy rather than merely present.
		v.Reason = "the proxy that wrote " + path + " is no longer running" +
			" (process is gone; the file is left behind by a crash)"
		return v
	}

	v.Export = true
	v.Reason = "observe-mode proxy at " + v.File.ListenAddr + " is running"
	return v
}

// alive reports whether a process exists and is signalable by this user.
//
// Signal 0 is the portable existence check: it performs permission and
// existence checks and delivers nothing. A pid that exists but belongs to
// another user answers EPERM, which is still proof that something holds the
// pid -- and the conservative reading is that the proxy may well be running
// under that pid, so it counts as alive rather than as an invitation to
// export blind.
//
// A recycled pid is the residual risk and cannot be closed from here: a stale
// file whose pid was reused by an unrelated process reads as alive. The
// product check above is what bounds the damage, and the proxy removes the
// file on every clean shutdown so the window needs a crash to open at all.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return errors.Is(err, os.ErrPermission)
}
