package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/brief"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/receipt"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// briefUsage is the usage screen's brief entry, joined as receiptUsage is, so
// usage keeps its size under Law 10.
const briefUsage = `  rashomon brief [DIR]           print the brief a session starting in DIR
                               (default the working directory) is shown:
                               instruction files, manifests, the test
                               command, the spec and its trust, the task and
                               the Go package directories, each line naming
                               the file it was read from

`

// cmdBrief prints, as plain text, the brief probe start prints at a session's
// start in dir (the working directory by default), read now.
func cmdBrief(args []string, stdout io.Writer) error {
	dir := "."
	switch {
	case len(args) > 0 && strings.HasPrefix(args[0], "-"):
		return fmt.Errorf("unknown argument %q", args[0])
	case len(args) > 1:
		return errors.New("brief takes at most one DIR")
	case len(args) == 1:
		dir = args[0]
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	root, ok := briefRoot(abs)
	if !ok {
		return fmt.Errorf("%s is not a directory this process can read", dir)
	}
	// The index is read from the store's place; nothing is opened or made.
	storeRoot, err := store.DefaultRoot()
	if err != nil {
		storeRoot = ""
	}
	_, err = fmt.Fprintln(stdout, brief.Render(buildBrief(abs, root, storeRoot, brief.Pointer{})))
	return err
}

// printBrief is probe start's one write to stdout (card BRIEF1), args the
// probe's (the phase first); probe end writes nothing there. At startup,
// clear or a payload with no source, it is the brief of the project holding
// the payload's cwd, its verify line naming the report command of the entry
// that ran it (briefFromPlugin); at resume, one line pointing at the verb; at
// compact and any other source, nothing. lead, the plugin route's consent
// line (pluginConsent), opens it whenever it is not "", the brief or none. It
// is one JSON object whose only key is systemMessage, the channel the user
// reads and the model does not, as recap_turn.go prints its line. A cwd that
// is not a directory this process can read prints no brief (B7).
func printBrief(args []string, payload *hookio.Payload, st *store.Store, lead string, stdout io.Writer) {
	if len(args) == 0 || args[0] != store.PhaseStart || payload == nil {
		return
	}
	// Arm off withholds every hook print (card MEASURE1, armOff), the
	// brief's user-channel line included: the paired run compares what the
	// person saw as well as what the model did.
	if armOff(st.Root()) {
		return
	}
	msg := briefText(args, payload, st)
	if lead != "" {
		msg = strings.TrimSuffix(lead+"\n"+msg, "\n")
	}
	if msg == "" {
		return
	}
	if b, err := json.Marshal(map[string]string{"systemMessage": msg}); err == nil {
		_, _ = stdout.Write(b)
	}
}

// briefText is printBrief's brief: the project's at startup, clear or no
// source, the resume line at resume, and "" at any other source, for a
// payload with no cwd, or a cwd this process cannot read.
func briefText(args []string, payload *hookio.Payload, st *store.Store) string {
	var source, cwd, sessionID string
	err := payload.Strs(hookio.Into(hookio.Source, &source), hookio.Into(hookio.CWD, &cwd), hookio.Into(hookio.SessionID, &sessionID))
	if err != nil || cwd == "" {
		return ""
	}
	root, ok := briefRoot(cwd)
	switch {
	case !ok:
		return ""
	case source == "" || source == "startup" || source == "clear":
		plugin := briefFromPlugin(args)
		b := buildBrief(cwd, root, st.Root(), brief.Pointer{Plugin: plugin, SessionID: sessionID})
		return brief.Render(withLastSession(b, st, root, sessionID, plugin))
	case source == "resume":
		return brief.Resumed(root)
	}
	return ""
}

// withLastSession replaces the brief's verify line with the line for the
// project's previous session (card REPORT1b): its time, one reason and the
// report command its origin has. Two small reads and no walk, whatever the
// store holds: the project's checkpoint file, into which the Stop that cut
// each checkpoint wrote the last two (verify.Checkpoints), names the one
// checkpoint read. No file, one with no pointer for an earlier session (a
// first session), a pointer to a checkpoint that is not the one named, or any
// error leaves the verify line, and a pointer to a run that is gone says so.
func withLastSession(b brief.Brief, st *store.Store, root, sessionID string, plugin bool) brief.Brief {
	c, _ := verify.ReadCheckpoints(verify.CheckpointsPath(st.Root(), root))
	ptr := c.PreviousOf(sessionID)
	if ptr == nil {
		return b
	}
	last, scan, err := receipt.Pointed(st, root, ptr.SessionID, ptr.PointSeq)
	if err != nil || !scan.Found && scan.Bound == "" {
		return b
	}
	pointer := "rashomon report --session last"
	if plugin {
		pointer = brief.PluginReport
	}
	source := "the project's checkpoint file, whose last checkpoint is of an earlier session"
	if !scan.Found {
		source = "the project's checkpoint file, whose last checkpoint is gone"
		if scan.Bound == receipt.BoundTail {
			source = "the project's checkpoint file, whose last checkpoint was not in the end of its run"
		}
	}
	text := strings.TrimPrefix(receipt.PreviousLine(last, scan, pointer), brief.LastSessionLead)
	return b.WithVerify(brief.Fact{Kind: brief.KindLastSession, Text: text, Source: source})
}

// briefRoot is the project root of dir, baseline.Key as recap_stop.go reads
// it, when dir is a directory this process can open.
func briefRoot(dir string) (string, bool) {
	f, err := os.Open(dir)
	if err != nil {
		return "", false
	}
	fi, err := f.Stat()
	_ = f.Close()
	if err != nil || !fi.IsDir() {
		return "", false
	}
	return baseline.Key(dir), true
}

// buildBrief is brief.Build with the spec discovered from cwd as the steer
// step discovers it, trust read under storeRoot as hook.NoRun: the probe
// holds no run here (it records the start and reads nothing back). That is
// sound while the full brief prints only at a run's first record (startup,
// clear or no source), before any store write TrustRoot would see; a clear
// that continued a run would break it. The type makes the reading a named
// one (SPEC-TRUST2); the durable trust drop and its counter are not built.
func buildBrief(cwd, root, storeRoot string, report brief.Pointer) brief.Brief {
	projectDir, managedDir := specDirs()
	return brief.Build(root, storeRoot, report, hook.DiscoverSpec(cwd, projectDir, managedDir, hook.NoRun(storeRoot)))
}

// briefFromPlugin is recapFromPlugin's rule for probe start's entry (D175):
// the plugin's slash command exists only while the plugin is loaded, and a
// settings entry, even one made from the plugin's own binary, carries the
// --install marker, so only the plugin's own entry points at it.
func briefFromPlugin(args []string) bool {
	if installArg(args) != "" {
		return false
	}
	present, err := install.PluginFires(install.EventSessionStart)
	return err == nil && present
}
