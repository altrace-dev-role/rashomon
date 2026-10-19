//go:build !unix

package supervise

// Supported reports whether hook entries run supervised on this platform.
//
// They do not on Windows. os/exec cannot hand a child an inherited pipe
// there (ExtraFiles is not supported), so the child could not be recognised
// without an environment variable, which repository settings can set; and
// killing the child with anything it started needs a job object, not a
// process group. An entry runs in-process, as it did before supervision.
const Supported = false

// Run runs the entry unsupervised.
func Run(cfg Config) int { return cfg.Direct() }

// Child is never attached on this platform.
type Child struct{}

// Attach strips Token from args, so `--supervised-child hook` is `hook`
// here; nothing makes a process the supervised child.
func Attach(args []string) (c *Child, rest []string, ok bool) {
	if len(args) > 0 && args[0] == Token {
		return nil, args[1:], false
	}
	return nil, args, false
}

// StandDown is false: an unsupervised entry never stands down.
func (c *Child) StandDown() bool { return false }
