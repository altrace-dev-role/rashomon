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
	"strconv"
	"strings"
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

	// SessionToken is the proxy advertising that it accepts a session tag as
	// the password half of a Proxy-Authorization credential on CONNECT, and
	// writes it into its run_id column.
	//
	// ABSENT MEANS NO, and that is the whole reason this field exists rather
	// than the launcher simply always sending one. A proxy that does not know
	// about the credential is entitled to answer 407 to a CONNECT carrying it,
	// which would break every request of every session against any build older
	// than the feature. Capability first, then use -- the alternative is a
	// launcher whose new feature bricks the old server.
	SessionToken bool `json:"session_token"`
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
	// Listen is the listen address parsed whole, set ONLY when Export is true.
	// Callers export Listen.String(), rebuilt from its two parts, and never
	// File.ListenAddr: the file is writable by the user the agent runs as, and
	// its raw string is text a shell's eval would run.
	Listen Listen
}

// Listen is a listen address that parsed completely as HOST:PORT.
type Listen struct {
	// Host is exactly "127.0.0.1", "localhost" or "[::1]" -- one of this
	// package's own literals, never a slice of the file.
	Host string
	// Port is 1-65535.
	Port int
}

// String is the address as a proxy URL carries it. At the stock address it is
// byte-identical to what the proxy writes, "127.0.0.1:18080".
func (l Listen) String() string {
	return l.Host + ":" + strconv.Itoa(l.Port)
}

// parseListen accepts HOST:PORT and nothing else.
//
// HOST is exactly one of three spellings. isLoopback accepts more -- any case
// of localhost, a bare ::1 -- because it answers a narrower question, whether
// traffic would leave the machine. This one answers whether the ADDRESS is
// safe to print into a line a shell evaluates, and the only answer that holds
// for every shell is: only text rebuilt from known parts. PORT is ASCII
// digits, no sign, no leading zero, 1-65535, so nothing after the port
// survives: not a ";cmd", not a " $(x)", not a newline that `eval "$vars"`
// would read as a second command (CWE-78).
//
// The address is not the whole line env prints. NO_PROXY's `*.local` is a
// glob that an unquoted `eval $(rashomon env)` expands against the current
// directory; that residual is the caller's to state, and cmdEnv states it.
//
// Written by hand, for the reason isLoopback gives: H-17 forbids `net` in this
// module. net.SplitHostPort would not be enough anyway -- it accepts a port of
// "1;cmd" and leaves the checking to the caller.
func parseListen(addr string) (Listen, bool) {
	i := strings.LastIndexByte(addr, ':')
	if i < 0 {
		return Listen{}, false
	}
	var host string
	switch addr[:i] {
	case "127.0.0.1":
		host = "127.0.0.1"
	case "localhost":
		host = "localhost"
	case "[::1]":
		host = "[::1]"
	default:
		// Includes a bare "::1", whose last colon is inside the address: the
		// host half is then ":", and a portless IPv6 literal is refused here
		// rather than read as port 1.
		return Listen{}, false
	}
	port, ok := parsePort(addr[i+1:])
	if !ok {
		return Listen{}, false
	}
	return Listen{Host: host, Port: port}, true
}

// parsePort reads a port in plain decimal: one to five ASCII digits, no sign,
// no leading zero, and a value in 1-65535. strconv.Atoi is not used because it
// takes a sign and leading zeros, both of which this refuses.
func parsePort(s string) (int, bool) {
	if len(s) == 0 || len(s) > 5 || s[0] == '0' {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	if n > 65535 {
		return 0, false
	}
	return n, true
}

// isLoopback reports whether an address's HOST half is loopback.
//
// The host half, not the string: "127.0.0.1:18080" and "[::1]:18080" are the
// shapes the proxy writes, and a check that matched a prefix would accept
// "127.0.0.1.evil.example:80".
// It splits the host by hand rather than with net.SplitHostPort, and that is
// not a style choice: H-17 forbids `net` anywhere in the recorder's dependency
// graph, and importing it here for one helper failed that acceptance item
// immediately. The invariant is the point -- code that runs inside an agent's
// tool calls must not be able to reach the network -- and a convenience import
// is exactly how such a graph grows a capability nobody decided to add.
//
// It reads the HOST only, which makes it necessary and not sufficient: it
// accepted "127.0.0.1:1;cmd", whose port is a shell command. Read therefore
// also requires parseListen, which accepts a narrower set and reads the port.
func isLoopback(addr string) bool {
	host := addr
	if strings.HasPrefix(host, "[") {
		// Bracketed IPv6 literal: "[::1]:18080". The host is inside the
		// brackets, and the colons within it are not the port separator.
		if end := strings.Index(host, "]"); end > 0 {
			host = host[1:end]
		}
	} else if i := strings.LastIndex(host, ":"); i >= 0 {
		// A bare "::1" has several colons and no port; splitting on the last
		// one would leave "::" and read as not-loopback. Only split when what
		// follows looks like a port and the host half has no colon of its own.
		if !strings.Contains(host[:i], ":") {
			host = host[:i]
		}
	}
	switch strings.ToLower(host) {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
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
	// Every field of the file that a reason repeats is QUOTED. The file is
	// writable by the user the agent runs as and the reason goes to the
	// operator's terminal, so a raw newline in it would print a second line of
	// the file's choosing -- "observe-mode proxy ... is running" -- under this
	// program's name (CWE-117), and a raw escape sequence would reach the
	// terminal itself (CWE-150).
	if v.File.ConnectMode != ModeObserve {
		v.Reason = "the proxy is in " + strconv.Quote(v.File.ConnectMode) +
			" mode, not observe -- exporting proxy variables at an enforcing proxy" +
			" would refuse the client's own API tunnels and break the session"
		return v
	}
	if v.File.ListenAddr == "" {
		v.Reason = "the proxy status file names no listen address"
		return v
	}
	if !isLoopback(v.File.ListenAddr) {
		// THIS FILE IS WRITABLE BY THE USER THE AGENT RUNS AS, which makes a
		// non-loopback address an escalation across sessions rather than a
		// misconfiguration: an agent in one session writes a status file naming
		// a host it controls, and the next `rashomon run` sends every request
		// -- and, since the session tag rides in the proxy URL as userinfo, the
		// tag as well -- to that host in cleartext, before any report exists to
		// notice. Refusing here rather than in the launcher because a reader
		// that returns Export true for a remote proxy has already made the
		// decision; the launcher only carries it out.
		v.Reason = "the proxy status file names a non-loopback listen address (" +
			strconv.Quote(v.File.ListenAddr) + "); refusing, because exporting proxy variables to a " +
			"remote host would send this session's traffic and its session tag off " +
			"this machine"
		return v
	}
	// AFTER the loopback check, so a routable address keeps that refusal's
	// reason, and narrower than it: loopback is necessary and not sufficient.
	// The address is about to be printed into a line the operator hands to
	// eval, so it must be a host and a port and nothing else. Quoted in the
	// reason, so what the file carried is shown and never interpreted.
	listen, ok := parseListen(v.File.ListenAddr)
	if !ok {
		v.Reason = "the proxy status file names listen address " + strconv.Quote(v.File.ListenAddr) +
			", which is not HOST:PORT with HOST 127.0.0.1, localhost or [::1] and PORT 1-65535" +
			" in plain digits; refusing, because an exported address is text a shell evaluates"
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
	v.Listen = listen
	v.Reason = "observe-mode proxy at " + listen.String() + " is running"
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
