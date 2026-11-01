package seed

import "time"

// The kept before count (card SEED1-KEEP): `rashomon audit --keep` writes
// KeptFile under the store root.
const (
	KeptFile     = "seed.json"
	KeptMaxBytes = 256 << 10
	// historyNextMax is the brief's Width less its second-line indent.
	historyNextMax = 76
)

// Kept is the before count of a before and after.
type Kept struct {
	Version  int               `json:"seed_kept"`
	KeptAt   time.Time         `json:"kept_at"`
	Since    time.Time         `json:"since"`
	Until    time.Time         `json:"until"`
	Days     int               `json:"days"`
	All      Counts            `json:"all"`
	Projects map[string]Counts `json:"projects,omitempty"`
}

// Counts is one scope's counts.
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

// Kept is the fold as a record.
func (t *Tally) Kept(keptAt, since, until time.Time, days int) Kept { return Kept{} }

// WriteKept writes k under root.
func WriteKept(root string, k Kept) error { return nil }

// ReadKept reads the record under root.
func ReadKept(root string) (Kept, bool, error) { return Kept{}, false, nil }

// History is the brief's two lines for k.
func History(k Kept, ok bool, key string) (text, next string) { return "", "" }

func (k kind) id() string               { return "" }
func parseKind(s string) (kind, bool)   { return 0, false }
func (c class) id() string              { return "" }
func parseClass(s string) (class, bool) { return 0, false }
