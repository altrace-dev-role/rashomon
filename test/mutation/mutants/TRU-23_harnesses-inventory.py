# TRU-23: harnesses.json, which watch now writes, and its replacement
# target are in the inventory.
m("TRU-23 harnesses.json drops out of the inventory", "internal/disclose/disclose.go",
  "\t\t\tPattern:    store.FileHarnesses,\n", "\t\t\tPattern:    \"harness.json\",\n",
  "TestTRU23_ConsentWritesAreInTheInventory")
m("TRU-23 the harnesses.json replacement drops out of the inventory", "internal/disclose/disclose.go",
  "\t\t\tPattern:    \".\" + store.FileHarnesses + \".tmp-*\",\n", "\t\t\tPattern:    store.FileHarnesses + \".tmp\",\n",
  "TestTRU23_ConsentWritesAreInTheInventory")
