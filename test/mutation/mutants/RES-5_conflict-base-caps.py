# RES-5 (ruling 2026-11-12 16:37): the conflict base's two readers are
# allowed by name in res5Allowed on the ground that each reads a capped file
# rashomon wrote; these rows pin the read-side caps that ground rests on.
C = "cmd/rashomon/conflictbase.go"
m("RES-5 the post reads a conflict base over its cap", C,
  "\tif err != nil || len(data) > maxConflictBaseBytes {\n",
  "\tif err != nil {\n",
  "TestConflictBase_ABaseOverItsCapIsRefused")
m("RES-5 the prompt reads a head line that does not end inside its cap", C,
  "\tif !found || json.Unmarshal(line, &h) != nil || h.V != conflictBaseVersion {\n",
  "\tif false && !found || json.Unmarshal(line, &h) != nil || h.V != conflictBaseVersion {\n",
  "TestConflictBase_AHeadWithNoNewlineInItsCapIsRefused")
