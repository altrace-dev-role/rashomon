# RES-49: Stop reads gaps from a cursor. fold.File's new Gaps field
# (internal/fold/fold.go) is where that cursor is meant to live, and
# fold.Validate checks it the same way it checks Records, Spill and
# Coverage: a cursor Validate silently skipped would let an incremental
# reader resume past gaps.ndjson's actual end once a later PR wires a
# reader to it. Nothing reads or writes the Gaps cursor in production
# yet -- this PR adds the field and the check, not the reader -- so this
# mutant pins only that the check is not skipped, never RES-49's own
# production claim (TestRES49_StopDoesNotReadAllGaps is still unwritten).

m("FOLD Validate never checks the gaps cursor",
  "internal/fold/validate.go",
  "\t\t{gapsPath, f.Gaps},\n",
  "",
  "TestValidate_GapsCursorIsChecked")
