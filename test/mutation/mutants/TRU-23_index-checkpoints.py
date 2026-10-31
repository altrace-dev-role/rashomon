# TRU-23: the inventory's row for the project index says what the index holds,
# the last and prior checkpoints included (census #171). The break drops that
# clause from the row.
m("TRU-23 the index row omits the checkpoints it holds", "internal/disclose/disclose.go",
  "; and the last and prior checkpoints (last, prior), each the session id, the point seq and when it was recorded, written at every Stop\",",
  "\",", "TestTRU23_IndexRowNamesTheCheckpoints")
