package seed

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The kept before count (card SEED1-KEEP): `rashomon audit --keep` writes
// KeptFile under the store root, the corrections of the days before the
// install, so an after has a before to be fewer than. It is read back only
// by the brief, at most KeptMaxBytes of it.
const (
	KeptFile     = "seed.json"
	KeptMaxBytes = 256 << 10
	keptVersion  = 1
	// The record's bounds: every signature is a row of the card's fold, so
	// these keep the file to a few KiB on a machine with tens of projects
	// and far under KeptMaxBytes at the cap (a project's key is a path).
	keptRows        = 50
	keptProjectRows = 10
	keptProjects    = 100
	// historyNextMax is the brief's Width less its second line's indent:
	// the brief prints that line whole, so every row's fits by construction.
	historyNextMax = 76
)

// Kept is the before count: when it was kept, the window it counts (Since to
// Until, Days long, Until the install), the machine's counts, and each
// project's by baseline.Key of its sessions' working directory.
type Kept struct {
	Version  int               `json:"seed_kept"`
	KeptAt   time.Time         `json:"kept_at"`
	Since    time.Time         `json:"since"`
	Until    time.Time         `json:"until"`
	Days     int               `json:"days"`
	All      Counts            `json:"all"`
	Projects map[string]Counts `json:"projects,omitempty"`
}

// Counts is one scope's sessions and tool calls, its corrections by kind id,
// and its signatures in the card's order, by stable ids (kind.id, class.id):
// a later version that rewords a row still reads the record.
type Counts struct {
	Sessions int            `json:"sessions"`
	Calls    int            `json:"calls"`
	Kinds    map[string]int `json:"kinds,omitempty"`
	Rows     []KeptRow      `json:"rows,omitempty"`
}

// KeptRow is one signature's recurrence by stable ids.
type KeptRow struct {
	Kind     string `json:"kind"`
	Class    string `json:"class"`
	Times    int    `json:"times"`
	Sessions int    `json:"sessions"`
}

// Kept is the fold as a record: the machine's counts with its first keptRows
// signatures, and the keptProjects projects with the most sessions (then by
// key) with their first keptProjectRows each.
func (t *Tally) Kept(keptAt, since, until time.Time, days int) Kept {
	k := Kept{Version: keptVersion, KeptAt: keptAt.UTC().Round(0), Since: since.UTC().Round(0),
		Until: until.UTC().Round(0), Days: days, All: t.counts(keptRows)}
	keys := make([]string, 0, len(t.per))
	for key := range t.per {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(t.per[b].sessions), len(t.per[a].sessions)), strings.Compare(a, b))
	})
	for i, key := range keys {
		if i == keptProjects {
			break
		}
		if k.Projects == nil {
			k.Projects = map[string]Counts{}
		}
		k.Projects[key] = t.per[key].counts(keptProjectRows)
	}
	return k
}

func (t *Tally) counts(rows int) Counts {
	c := Counts{Sessions: len(t.sessions), Calls: t.calls}
	for g := kind(0); g < numKinds; g++ {
		if n := t.kinds[g]; n > 0 {
			if c.Kinds == nil {
				c.Kinds = map[string]int{}
			}
			c.Kinds[g.id()] = n
		}
	}
	sigs := make([]sig, 0, len(t.rows))
	for g := range t.rows {
		sigs = append(sigs, g)
	}
	// The card's order (Summary): most sessions, most times, then by text.
	slices.SortFunc(sigs, func(a, b sig) int {
		ra, rb := t.rows[a], t.rows[b]
		return cmp.Or(cmp.Compare(len(rb.sessions), len(ra.sessions)), cmp.Compare(rb.times, ra.times),
			strings.Compare(rowText(a.k, a.c), rowText(b.k, b.c)))
	})
	for i, g := range sigs {
		if i == rows {
			break
		}
		r := t.rows[g]
		c.Rows = append(c.Rows, KeptRow{g.k.id(), g.c.id(), r.times, len(r.sessions)})
	}
	return c
}

// WriteKept replaces KeptFile under root with k (store.ReplaceFile: a
// temporary file, a sync, a rename, mode 0600). A record over KeptMaxBytes,
// which ReadKept would refuse, is an error and nothing is written.
func WriteKept(root string, k Kept) error {
	b, err := json.Marshal(k)
	if err != nil {
		return err
	}
	if len(b)+1 > KeptMaxBytes {
		return fmt.Errorf("seed: the kept count is %d bytes, over its %d-byte cap", len(b)+1, KeptMaxBytes)
	}
	return store.ReplaceFile(filepath.Join(root, KeptFile), append(b, '\n'))
}

// ReadKept reads KeptFile under root: none is no record and no error; one
// over KeptMaxBytes, of another version, or not JSON is an error.
func ReadKept(root string) (Kept, bool, error) {
	path := filepath.Join(root, KeptFile)
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Kept{}, false, nil
	}
	if err != nil {
		return Kept{}, false, err
	}
	defer f.Close() //nolint:errcheck // read-only
	// One byte past the cap, so that a file over it is seen to be.
	b, err := io.ReadAll(io.LimitReader(f, KeptMaxBytes+1))
	if err != nil {
		return Kept{}, false, err
	}
	if len(b) > KeptMaxBytes {
		return Kept{}, false, fmt.Errorf("seed: %s is over its %d-byte cap", path, KeptMaxBytes)
	}
	var k Kept
	if err := json.Unmarshal(b, &k); err != nil {
		return Kept{}, false, fmt.Errorf("seed: %s: %w", path, err)
	}
	if k.Version != keptVersion {
		return Kept{}, false, fmt.Errorf("seed: %s is version %d, not %d", path, k.Version, keptVersion)
	}
	return k, true, nil
}

// The brief's history line: its text, and under it this project's most
// recurring signature or, when the project has none that recurs, the
// machine's.
const (
	historyCount    = "%s in the %d days before AutoSteer"
	historyHere     = ", %d here"
	historyMostHere = "most here: "
	historyMost     = "most: "
)

// History is the brief's two lines for the record k and the project key:
// the corrections kept, the card's "stopped or overruled" (a rule's block is
// the person's settings, not a correction), this project's when it is in the
// record, and the first row seen in cardRecurAt or more sessions.
func History(k Kept, key string) (text, next string) {
	text = fmt.Sprintf(historyCount, count(corrections(k.All), "correction", "corrections"), k.Days)
	p, here := k.Projects[key]
	if here {
		text += fmt.Sprintf(historyHere, corrections(p))
		if r, ok := recurring(p.Rows); ok {
			return text, historyMostHere + r
		}
	}
	if r, ok := recurring(k.All.Rows); ok {
		return text, historyMost + r
	}
	return text, ""
}

func corrections(c Counts) int {
	return c.Kinds[interrupted.id()] + c.Kinds[rejected.id()] + c.Kinds[pushedBack.id()]
}

// recurring is the text of the first row seen in cardRecurAt or more
// sessions whose ids this version knows.
func recurring(rows []KeptRow) (string, bool) {
	for _, r := range rows {
		g, okKind := parseKind(r.Kind)
		c, okClass := parseClass(r.Class)
		if okKind && okClass && r.Sessions >= cardRecurAt {
			return rowText(g, c), true
		}
	}
	return "", false
}

// The stable ids of the kinds and classes: the record's words, never shown.
// A const-indexed array of string constants is static data: no code runs to
// build it at start-up.
var (
	kindIDs = [numKinds]string{interrupted: "interrupted", rejected: "rejected", blocked: "blocked",
		pushedBack: "pushed-back", failedAgain: "failed-again"}
	classIDs = [numClasses]string{cReply: "reply", cShell: "shell", cShellRead: "shell-read",
		cShellWrite: "shell-write", cVCS: "vcs", cPackage: "package", cNetwork: "network", cTestRun: "test-run",
		cBuildRun: "build-run", cEdit: "edit", cEditTest: "edit-test", cEditSecret: "edit-secret", cWrite: "write",
		cWriteTest: "write-test", cWriteSecret: "write-secret", cRead: "read", cSearch: "search", cWeb: "web",
		cAgent: "agent", cMCP: "mcp", cPlan: "plan", cQuestion: "question", cOther: "other",
		eEditNotFound: "edit-not-found", eEditMany: "edit-many", eEditSame: "edit-same", eStale: "stale",
		eUnread: "unread", eTooLarge: "too-large", eNoFile: "no-file", eBadInput: "bad-input"}
)

func (k kind) id() string { return kindIDs[k] }

func parseKind(s string) (kind, bool) {
	for g := kind(0); g < numKinds; g++ {
		if kindIDs[g] == s {
			return g, true
		}
	}
	return 0, false
}

func (c class) id() string { return classIDs[c] }

func parseClass(s string) (class, bool) {
	for c := class(0); c < numClasses; c++ {
		if classIDs[c] == s {
			return c, true
		}
	}
	return 0, false
}
