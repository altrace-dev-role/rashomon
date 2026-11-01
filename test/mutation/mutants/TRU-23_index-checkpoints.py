# TRU-23: the project's last and prior checkpoints have a row of their own,
# which says what the file holds, and the index's row no longer claims them
# (census #171; INDEX-POINTER-FILE1). One break drops the checkpoints from
# their row; the other puts them back in the index's.
C = "internal/disclose/checkpoints.go"
D = "internal/disclose/disclose.go"
m("TRU-23 the checkpoint file's row omits what it holds", C,
  "\"the last and prior checkpoints (last, prior), each the session id, the point seq and when it was recorded, written at every Stop that cuts one\",",
  "\"two records\",", "TestTRU23_TheCheckpointsHaveARowOfTheirOwn")
m("TRU-23 the index row claims the checkpoints again", D,
  "the last of them by session id, and when it was first and last seen\",",
  "the last of them by session id, and when it was first and last seen; and the last and prior checkpoints\",",
  "TestTRU23_TheCheckpointsHaveARowOfTheirOwn")
