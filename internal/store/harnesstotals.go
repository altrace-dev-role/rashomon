package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// HarnessTotal is one harness_counts.json key's sums over the daily slots the
// file still holds: the sessions counted as seen under it, and of those the
// sessions that stood down.
type HarnessTotal struct {
	Key       string
	Seen      int
	StoodDown int
}

// HarnessTotals is every key's HarnessTotal, sorted by key. A file that was
// never written is none; one that exists and cannot be read or parsed is an
// error, never none, as HarnessSeenCount has it.
func HarnessTotals(root string) ([]HarnessTotal, error) {
	b, err := os.ReadFile(filepath.Join(root, FileHarnessCounts))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var hc harnessCounts
	if err := json.Unmarshal(b, &hc); err != nil {
		return nil, err
	}
	totals := make([]HarnessTotal, 0, len(hc.Counts))
	for key, entry := range hc.Counts {
		t := HarnessTotal{Key: key}
		if entry != nil {
			for _, d := range entry.Days {
				t.Seen += d.Seen
				t.StoodDown += d.StoodDown
			}
		}
		totals = append(totals, t)
	}
	slices.SortFunc(totals, func(a, b HarnessTotal) int { return strings.Compare(a.Key, b.Key) })
	return totals, nil
}
