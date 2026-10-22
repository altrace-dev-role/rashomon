# VER-52: the snapshot is the verify --json golden and the checkpoint is the
# schema-5 verify record. A point kind the reader keys on leaving the list, or
# the checkpoint losing the header key an older reader skips on, is the whole
# regression.
m("VER-52 the query point kind leaves the list", "internal/verify/snapshot.go",
  "var PointKinds = []string{PointStop, PointSubagentStop, PointSubtask, PointCommit, PointPush, PointPR, PointQuery}",
  "var PointKinds = []string{PointStop, PointSubagentStop, PointSubtask, PointCommit, PointPush, PointPR}", "TestVER52d_")
m("VER-52 the checkpoint loses its critical key", "internal/verify/snapshot.go",
  "`json:\"critical\"`", "`json:\"critical_\"`", "TestVER52b_")
