package main

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/selfguard"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// bodyOverCapLine is what probe says when a session's transcript was not
// copied because it is larger than the whole bodies cap (card 4.1b, D111).
// Fixed text, like evictionLine: hook stderr reaches Claude Code's debug
// log, so nothing from the invocation goes into it; `rashomon bodies status`
// names the cap.
const bodyOverCapLine = "rashomon: transcript copy skipped: over the bodies cap"

// cmdBodies is `rashomon bodies on [--max-bytes N] | off | status`, the
// switch for this project's transcript copies (PF-RC14): a command the user
// runs, never a default (D111). on and off edit bodies.json for the project
// the working directory belongs to, keyed as the probe keys it; status
// counts every project's copies, which share the one cap.
func cmdBodies(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("bodies needs on [--max-bytes N], off or status")
	}
	verb, rest := args[0], args[1:]
	var maxBytes int64
	// What it does not know, --help included, is refused as cmdForget's
	// default case refuses it: no command takes --help in this release.
	switch {
	case verb == "on" && len(rest) == 2 && rest[0] == "--max-bytes":
		n, err := strconv.ParseInt(rest[1], 10, 64)
		if err != nil || n <= 0 {
			return fmt.Errorf("--max-bytes takes a positive number of bytes, not %q", rest[1])
		}
		maxBytes = n
	case len(rest) > 0:
		return fmt.Errorf("unknown argument %q", rest[0])
	case verb != "on" && verb != "off" && verb != "status":
		return fmt.Errorf("unknown argument %q", verb)
	}
	if verb == "status" {
		st, err := openStoreForRead()
		if errors.Is(err, store.ErrNoStore) {
			fmt.Fprintln(stdout, "bodies: off (no store here)")
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, bodiesLine(st))
		return nil
	}
	// on and off are the person's switch for what is copied of their
	// transcripts: refused in an agent session either way, before the store
	// is opened or bodies.json written.
	if s := selfguard.AgentSignal(os.Getenv); s != "" {
		return fmt.Errorf("%s is set, so this is an agent session; nothing was written. Run `rashomon bodies %s` in your own terminal.", s, strings.Join(args, " "))
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	key, _, err := bodiesProject()
	if err != nil {
		return err
	}
	set, _, err := store.ReadBodies(st.Root())
	if err != nil {
		return err
	}
	if verb == "on" {
		if !slices.Contains(set.Projects, key) {
			set.Projects = append(set.Projects, key)
		}
		set.MaxBytes = cmp.Or(maxBytes, set.MaxBytes, store.DefaultBodiesMaxBytes)
	} else {
		set.Projects = slices.DeleteFunc(set.Projects, func(p string) bool { return p == key })
	}
	if err := store.WriteBodies(st.Root(), set); err != nil {
		return err
	}
	fmt.Fprintln(stdout, bodiesLine(st))
	return nil
}

// bodiesLine is status's line for the copies, printed beside the store's
// size: every project's copies against the cap, and whether this project
// keeps them.
func bodiesLine(st *store.Store) string {
	set, _, err := store.ReadBodies(st.Root())
	n, total, lerr := st.BodiesTotal()
	key, _, kerr := bodiesProject()
	if err = errors.Join(err, lerr, kerr); err != nil {
		return "bodies: " + statusUnknown
	}
	state := "off"
	if slices.Contains(set.Projects, key) {
		state = "on"
	}
	return fmt.Sprintf("bodies: %d sessions, %s of %s (%s %s)", n, store.FormatBytes(total),
		store.FormatBytes(cmp.Or(set.MaxBytes, store.DefaultBodiesMaxBytes)), key, state)
}

// forgetBodies is `forget --bodies`: this project's copies, and nothing else.
func forgetBodies(stdout io.Writer) error {
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		fmt.Fprintln(stdout, nothingRecordedHere)
		return nil
	}
	if err != nil {
		return err
	}
	key, dir, err := bodiesProject()
	if err != nil {
		return err
	}
	n, err := st.ForgetBodies(dir)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "forgot %d transcript copies of %s\n", n, key)
	return nil
}

// bodiesProject is the working directory's project as the probe names it
// from the payload's cwd: baseline.Key for the switch, and baseline.FileName
// less its extension for the directory (card 4.1).
func bodiesProject() (key, dir string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	key = baseline.Key(cwd)
	return key, strings.TrimSuffix(baseline.FileName(key), baseline.FileExt), nil
}

// forgetKinds counts the kinds of forget named; cmdForget takes one.
func forgetKinds(named ...bool) int {
	n := 0
	for _, b := range named {
		if b {
			n++
		}
	}
	return n
}
