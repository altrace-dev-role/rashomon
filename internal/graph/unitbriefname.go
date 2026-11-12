package graph

import (
	"encoding/hex"
	"path/filepath"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The unit-brief files (W-M2.3 hook half, D2): the brief for the first
// write in a unit, written at the prompt and read at the post, one file per
// unit under one directory per session:
// <store>/graph/unitbrief/<session>/<name>.json. Split from evidencename.go
// for Law 10; purge reads the names through the predicates here.

// UnitBriefSubdir is the directory under StoreDir that holds the sessions'
// unit-brief directories.
const UnitBriefSubdir = "unitbrief"

const unitBriefExt = ".json"

// UnitBriefTempPattern is the os.CreateTemp pattern the writer makes a unit
// brief under before renaming it over its name, so the names in a session's
// directory are the writer's and purge's one list (D8).
const UnitBriefTempPattern = ".unitbrief-*.tmp"

// UnitBriefDir is the session's unit-brief directory under the store root,
// the session id passed through the evidence files' segment rule so it
// stays one name whatever the hook's id holds.
func UnitBriefDir(storeRoot, sessionID string) string {
	return filepath.Join(storeRoot, StoreDir, UnitBriefSubdir, nameSegment(sessionID))
}

// UnitBriefFileName is the name of unit's brief file for the index at root:
// store.Segment's digest form of the two, NUL-led so it is always the
// digest form, which holds no path.
func UnitBriefFileName(root string, unit ID) string {
	return store.Segment("\x00"+root+"\x00"+string(unit)) + unitBriefExt
}

// IsUnitBriefFileName is whether name is a value UnitBriefFileName can
// return: "x-", 32 lowercase hex and ".json". store.IsSegment alone is not
// the test, since it also admits a plain segment, which "x-" and 31 hex or
// upper-case hex is.
func IsUnitBriefFileName(name string) bool {
	stem, ok := strings.CutSuffix(name, unitBriefExt)
	if !ok {
		return false
	}
	digest, ok := strings.CutPrefix(stem, "x-")
	if !ok || len(digest) != 32 || digest != strings.ToLower(digest) {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

// IsUnitBriefSessionDir is whether name is a value UnitBriefDir can end in.
func IsUnitBriefSessionDir(name string) bool { return isNameSegment(name) }
