package gather

import (
	"encoding/json"

	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// LastStop is a session's last checkpoint record, nil when it has none or
// the last one does not decode as a Stop's: either way there is no Stop-time
// point to read. The error is the store's. The ledger verb's replay reads it
// (card LEDGER-WIRE1); the decode is here, in the report's read step, and
// not in cmd/rashomon, a hook-path package where no decode goes
// (TestRES5_PayloadScannedOnce). The record is rashomon's own, at most
// CheckpointMaxBytes.
func LastStop(st *store.Store, sessionID string) (*verify.Checkpoint, error) {
	line, ok, _, err := st.LatestRecordBytes(sessionID, store.TypeVerify)
	if err != nil || !ok {
		return nil, err
	}
	var cp verify.Checkpoint
	if json.Unmarshal(line, &cp) != nil || cp.Point != verify.PointStop {
		return nil, nil
	}
	return &cp, nil
}
