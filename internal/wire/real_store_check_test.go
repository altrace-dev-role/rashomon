package wire

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestRead_AgainstARealProxyStore runs the reader against a causal.db written
// by the actual proxy, when one is present. It is skipped otherwise, so it
// never gates CI -- its job is to catch the case the hand-built fixtures
// cannot: a column set or a timestamp encoding that differs from what this
// package believes.
//
// Point it at a store with RASHOMON_REAL_STORE.
func TestRead_AgainstARealProxyStore(t *testing.T) {
	path := os.Getenv("RASHOMON_REAL_STORE")
	if path == "" {
		t.Skip("RASHOMON_REAL_STORE is unset; the real-store check needs a proxy store")
	}
	obs := Read(path, Window{Start: time.Now().Add(-2 * time.Hour)})
	if !obs.Observed {
		t.Fatalf("a real proxy store was not observed: %s", obs.Reason)
	}
	if !obs.WindowApplied {
		t.Error("window_applied is false against a real store, so the proxy's timestamp " +
			"encoding is not one this reader parses")
	}
	if obs.DistinctHosts == 0 {
		t.Error("no destinations read from a real store")
	}
	b, err := json.MarshalIndent(obs, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("real store observation:\n%s", b)
}
